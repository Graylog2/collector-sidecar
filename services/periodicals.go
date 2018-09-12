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
		logOnce := true
		for {
			time.Sleep(time.Duration(context.UserConfig.UpdateInterval) * time.Second)

			// registration response contains configuration assignments
			response, err := updateCollectorRegistration(httpClient, context)
			if err != nil {
				continue
			}
			// backend list is needed before configuration assignments are updated
			backendChecksum, err = fetchBackendList(httpClient, backendChecksum, context)
			if err != nil {
				continue
			}
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
			checkForUpdateAndRestart(httpClient, configChecksums, context)
		}
	}()
}

// report collector status to Graylog server
func updateCollectorRegistration(httpClient *http.Client, context *context.Ctx) (graylog.ResponseCollectorRegistration, error) {
	statusRequest := api.NewStatusRequest()
	return api.UpdateRegistration(httpClient, context, &statusRequest)
}

func fetchBackendList(httpClient *http.Client, checksum string, ctx *context.Ctx) (string, error) {
	response, err := api.RequestBackendList(httpClient, checksum, ctx)
	if err != nil {
		log.Error("Can't fetch collector list from Graylog API: ", err)
		return "", err
	}
	if response.IsEmpty() {
		// etag match, skipping all other actions
		return response.Checksum, nil
	}

	backendList := []backends.Backend{}
	for _, backendEntry := range response.Backends {
		backendList = append(backendList, *backends.BackendFromResponse(backendEntry))
	}
	backends.Store.Update(backendList)

	return response.Checksum, nil
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

		if response.IsEmpty() {
			// etag match, skip file render
			continue
		}

		if backend.RenderOnChange(backends.Backend{Template: response.Template}, context) {
			checksums[backendId] = response.Checksum
			if valid, output := backend.ValidateConfigurationFile(context); !valid {
				backend.SetStatusLogErrorf("Collector configuration file is not valid, waiting for the next update. " + output)
				continue
			}

			if err := daemon.Daemon.Runner[backend.Id].Restart(); err != nil {
				msg := "Failed to restart collector"
				backend.SetStatus(backends.StatusError, msg)
				log.Errorf("[%s] %s: %v", backend.Name, msg, err)
			}

		}
	}
}
