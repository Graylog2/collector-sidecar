// This file is part of Graylog.
//
// Graylog is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Graylog is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Graylog.  If not, see <http://www.gnu.org/licenses/>.

package services

import (
	"net/http"
	"time"

	"github.com/Graylog2/collector-sidecar/api"
	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/api/rest"
	"github.com/Graylog2/collector-sidecar/assignments"
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/daemon"
	"github.com/Graylog2/collector-sidecar/logger"
)

var log = logger.Log()
var httpClient *http.Client

func StartPeriodicals(context *context.Ctx) {
	if httpClient == nil {
		httpClient = rest.NewHTTPClient(api.GetTlsConfig(context))
	}

	go func() {
		configChecksums := make(map[string]string)
		backendChecksum := ""
		assignmentChecksum := ""
		logOnce := true
		for {
			time.Sleep(time.Duration(context.UserConfig.UpdateInterval) * time.Second)

			// registration response contains configuration assignments
			response, err := updateCollectorRegistration(httpClient, assignmentChecksum, context)
			if err != nil {
				continue
			}
			assignmentChecksum = response.Checksum
			// backend list is needed before configuration assignments are updated
			backendResponse, err := fetchBackendList(httpClient, backendChecksum, context)
			if err != nil {
				continue
			}
			backendChecksum = backendResponse.Checksum

			if !response.NotModified || !backendResponse.NotModified {
				assignments.Store.Update(response.Assignments)
				// create process instances
				daemon.Daemon.SyncWithAssignments(configChecksums, context)
				// test for new or updated configurations and start the corresponding collector
				if assignments.Store.Len() == 0 {
					if logOnce {
						log.Info("No configurations assigned to this instance. Skipping configuration request.")
						logOnce = false
					}
					continue
				} else {
					logOnce = true
				}
			}
			checkForUpdateAndRestart(httpClient, configChecksums, context)
		}
	}()
}

// report collector status to Graylog server and receive assignments
func updateCollectorRegistration(httpClient *http.Client, checksum string, context *context.Ctx) (graylog.ResponseCollectorRegistration, error) {
	statusRequest := api.NewStatusRequest()
	return api.UpdateRegistration(httpClient, checksum, context, &statusRequest)
}

func fetchBackendList(httpClient *http.Client, checksum string, ctx *context.Ctx) (graylog.ResponseBackendList, error) {
	response, err := api.RequestBackendList(httpClient, checksum, ctx)
	if err != nil {
		log.Error("Can't fetch collector list from Graylog API: ", err)
		return response, err
	}
	if response.NotModified {
		// etag match, skipping all other actions
		return response, nil
	}

	backendList := []backends.Backend{}
	for _, backendEntry := range response.Backends {
		backendList = append(backendList, *backends.BackendFromResponse(backendEntry, ctx))
	}
	backends.Store.Update(backendList)

	return response, nil
}

// fetch configuration periodically
func checkForUpdateAndRestart(httpClient *http.Client, checksums map[string]string, context *context.Ctx) {
	for backendId, configurationId := range assignments.Store.GetAll() {
		runner := daemon.Daemon.GetRunnerByBackendId(backendId)
		if runner == nil {
			log.Errorf("Got collector ID with no existing instance, skipping configuration check: %s", backendId)
			continue
		}
		backend := runner.GetBackend()
		response, err := api.RequestConfiguration(httpClient, configurationId, checksums[backendId], context)
		if err != nil {
			log.Error("Can't fetch configuration from Graylog API: ", err)
			return
		}

		if response.NotModified {
			// etag match, skip file render
			continue
		}

		if backend.RenderOnChange(backends.Backend{Template: response.Template}, context) {
			checksums[backendId] = response.Checksum
			if err, output := backend.ValidateConfigurationFile(context); err != nil {
				backend.SetStatusLogErrorf(err.Error())
				if output != "" {
					log.Errorf("[%s] Validation command output: %s", backend.Name, output)
					backend.SetVerboseStatus(output)
				}
				continue
			}

			if err := daemon.Daemon.Runner[backend.Id].Restart(); err != nil {
				msg := "Failed to restart collector"
				backend.SetStatus(backends.StatusError, msg, "")
				log.Errorf("[%s] %s: %v", backend.Name, msg, err)
			}

		}
	}
}
