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
		for {
			updateCollectorRegistration(httpClient, context)
		}
	}()
	go func() {
		checksum := ""
		for {
			checksum = fetchBackendList(httpClient, checksum, context)
		}
	}()
	go func() {
		checksum := ""
		for {
			checksum = checkForUpdateAndRestart(httpClient, checksum, context)
		}
	}()
}

// report collector status to Graylog server
func updateCollectorRegistration(httpClient *http.Client, context *context.Ctx) {
	time.Sleep(time.Duration(context.UserConfig.UpdateInterval) * time.Second)
	statusRequest := api.NewStatusRequest()
	response := api.UpdateRegistration(httpClient, context, &statusRequest)
	assignments.Store.Update(response.Assignments)
}

func fetchBackendList(httpClient *http.Client, checksum string, ctx *context.Ctx) string {
	time.Sleep(time.Duration(ctx.UserConfig.UpdateInterval) * time.Second)
	backendList, err := api.RequestBackendList(httpClient, checksum, ctx)
	if err != nil {
		log.Error("Can't fetch configuration from Graylog API: ", err)
		return ""
	}
	if backendList.IsEmpty() {
		// etag match, skipping all other actions
		return backendList.Checksum
	}

	for _, backendEntry := range backendList.Backends {
		backend := backends.BackendFromResponse(backendEntry)
		if backends.Store.GetBackendById(backend.Id) == nil {
			backends.Store.AddBackend(backend)
		}
	}

	return backendList.Checksum
}

// fetch configuration periodically
func checkForUpdateAndRestart(httpClient *http.Client, checksum string, context *context.Ctx) string {
	time.Sleep(time.Duration(context.UserConfig.UpdateInterval) * time.Second)

	// cleanup backends that should not run anymore
	for name := range daemon.Daemon.Runner {
		backend := backends.Store.GetBackend(name)
		if assignments.Store.GetAll()[backend.Id] == "" {
			log.Info("Removing backend from registry: " + backend.Name)
			daemon.Daemon.DeleteBackend(*backend)
		}
	}

	// add new backends to daemon registry
	for backendId := range assignments.Store.GetAll() {
		backend := backends.Store.GetBackendById(backendId)
		if daemon.Daemon.Runner[backend.Name] == nil {
			log.Info("Adding backend to registry: " + backend.Name)
			daemon.Daemon.AddBackend(*backend, context)
		}
	}

	// skip anything else if no assignments exist
	if assignments.Store.Len() == 0 {
		log.Info("No configurations assigned to this instance. Skipping configuration request.")
		return ""
	}

	for backendId, configurationId := range assignments.Store.GetAll() {
		response, err := api.RequestConfiguration(httpClient, configurationId, checksum, context)
		if err != nil {
			log.Error("Can't fetch configuration from Graylog API: ", err)
			return ""
		}

		if response.IsEmpty() {
			// etag match, skip file render
			continue
		}

		backend := backends.Store.GetBackendById(backendId)
		if backend.RenderOnChange(backends.Backend{Template: response.Template}) {
			if valid, output := backend.ValidateConfigurationFile(); !valid {
				backends.SetStatusLogErrorf(backend.Name,
					"Collector configuration file is not valid, waiting for the next update. "+output)
				continue
			}

			if err := daemon.Daemon.Runner[backend.Name].Restart(); err != nil {
				msg := "Failed to restart collector"
				backend.SetStatus(backends.StatusError, msg)
				log.Errorf("[%s] %s: %v", backend.Name, msg, err)
			}

		}
	}

	return ""
}
