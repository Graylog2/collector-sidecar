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
			checksum = checkForUpdateAndRestart(httpClient, checksum, context)
		}
	}()
}

// report collector status to Graylog server
func updateCollectorRegistration(httpClient *http.Client, context *context.Ctx) {
	time.Sleep(time.Duration(context.UserConfig.UpdateInterval) * time.Second)
	statusRequest := api.NewStatusRequest()
	api.UpdateRegistration(httpClient, context, &statusRequest)
}

// fetch configuration periodically
func checkForUpdateAndRestart(httpClient *http.Client, checksum string, context *context.Ctx) string {
	time.Sleep(time.Duration(context.UserConfig.UpdateInterval) * time.Second)
	jsonConfig, err := api.RequestConfiguration(httpClient, checksum, context)
	if err != nil {
		log.Error("Can't fetch configuration from Graylog API: ", err)
		return ""
	}
	if jsonConfig.IsEmpty() {
		// etag match, skipping all other actions
		return jsonConfig.Checksum
	}

	for name, runner := range daemon.Daemon.Runner {
		backend := backends.Store.GetBackend(name)
		if backend.RenderOnChange(jsonConfig) {
			if !backend.ValidateConfigurationFile() {
				backends.SetStatusLogErrorf(name, "Collector configuration file is not valid, waiting for the next update.")
				continue
			}

			if err := runner.Restart(); err != nil {
				msg := "Failed to restart collector"
				backend.SetStatus(backends.StatusError, msg)
				log.Errorf("[%s] %s: %v", name, msg, err)
			}

		}
	}

	return jsonConfig.Checksum
}
