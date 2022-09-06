// Copyright (C) 2020 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.

package services

import (
	"github.com/hashicorp/go-version"
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
		var lastBackendResponse graylog.ResponseBackendList
		var lastRegResponse graylog.ResponseCollectorRegistration
		logOnce := true
		for {
			time.Sleep(time.Duration(context.UserConfig.UpdateInterval) * time.Second)

			serverVersion, _ := api.GetServerVersion(httpClient, context)

			// registration regResponse contains configuration assignments
			regResponse, err := updateCollectorRegistration(httpClient, lastRegResponse.Checksum, context, serverVersion)
			if err != nil {
				continue
			}
			if !regResponse.NotModified {
				lastRegResponse = regResponse
			}

			// backend list is needed before configuration assignments are updated
			backendResponse, err := fetchBackendList(httpClient, lastBackendResponse.Checksum, context)
			if err != nil {
				continue
			}
			if !backendResponse.NotModified {
				lastBackendResponse = backendResponse
			}

			if !regResponse.NotModified || !backendResponse.NotModified {
				modified := assignments.Store.Update(lastRegResponse.Assignments)

				backendList := []backends.Backend{}
				// TODO this is inefficient
				for _, assignment := range lastRegResponse.Assignments {
					configId := assignment.ConfigurationId
					for _, backend := range lastBackendResponse.Backends {
						if backend.Id == assignment.BackendId {
							backendList = append(backendList, *backends.BackendFromResponse(backend, configId, context))
						}
					}
				}
				backends.Store.Update(backendList)

				// regResponse.NotModified is always false, because graylog does not implement caching yet.
				// Thus, we need to double-check.
				if modified || !backendResponse.NotModified {
					configChecksums = make(map[string]string)
				}
				// create process instances
				daemon.Daemon.SyncWithAssignments(context)
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
			log.Debugf("backend store %v", *backends.Store)
			log.Debugf("assignments store %v", assignments.Store.GetAll())
			log.Debugf("runner store %v", daemon.Daemon.Runner)
			checkForUpdateAndRestart(httpClient, configChecksums, context)
		}
	}()
}

// report collector status to Graylog server and receive assignments
func updateCollectorRegistration(httpClient *http.Client, checksum string, context *context.Ctx, serverVersion *version.Version) (graylog.ResponseCollectorRegistration, error) {
	statusRequest := api.NewStatusRequest()
	return api.UpdateRegistration(httpClient, checksum, context, serverVersion, &statusRequest)
}

func fetchBackendList(httpClient *http.Client, checksum string, ctx *context.Ctx) (graylog.ResponseBackendList, error) {
	response, err := api.RequestBackendList(httpClient, checksum, ctx)
	if err != nil {
		log.Error("Can't fetch collector list from Graylog API: ", err)
		return response, err
	}
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
		checksums[backendId] = response.Checksum

		if backend.RenderOnChange(backends.Backend{Template: response.Template}, context) {
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
