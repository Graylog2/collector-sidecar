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
	"time"

	"github.com/Graylog2/collector-sidecar/api"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/daemon"
	"github.com/Graylog2/collector-sidecar/backends"
)

var log = common.Log()

func StartPeriodicals(context *context.Ctx) {
	go func() {for{updateCollectorRegistration(context)}}()
	go func() {for{checkForUpdateAndRestart(context)}}()
}

// report collector status to Graylog server
func updateCollectorRegistration(context *context.Ctx) {
	time.Sleep(10 * time.Second)
	api.UpdateRegistration(context)
}

// fetch configuration periodically
func checkForUpdateAndRestart(context *context.Ctx) {
	time.Sleep(10 * time.Second)
	jsonConfig, err := api.RequestConfiguration(context)
	if err != nil {
		log.Error("Can't fetch configuration from Graylog API: ", err)
		return
	}
	for name, runner := range daemon.Daemon.Runner {
		backend := backends.Store.GetBackend(name)
		if backend.RenderOnChange(jsonConfig) {
			if !backend.ValidateConfigurationFile() {
				log.Infof("[%s] Collector configuration file is not valid, waiting for update...", name)
				continue
			}

			if runner.Running {
				// collector was already started so a Restart will not fail
				err = runner.Restart(runner.GetService())
				if err != nil {
					log.Error("Failed to restart collector %v", err)
				}
			}

		}
	}
}
