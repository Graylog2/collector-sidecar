package services

import (
	"time"

	"github.com/Graylog2/sidecar/api"
	"github.com/Graylog2/sidecar/context"
	"github.com/Graylog2/sidecar/util"
)

var log = util.Log()

func StartPeriodicals(context *context.Ctx) {
	updateCollectorRegistration(context)
	//fetchConfiguration(context)
	checkForUpdateAndRestart(context)
}

// report collector status to Graylog server
func updateCollectorRegistration(context *context.Ctx) {
	go func() {
		for {
			time.Sleep(10 * time.Second)
			api.UpdateRegistration(context)
		}
	}()
}

// fetch configuration periodically
func checkForUpdateAndRestart(context *context.Ctx) {
	backend := context.Backend

	go func() {
		for {
			time.Sleep(10 * time.Second)
			jsonConfig, err := api.RequestConfiguration(context)
			if err != nil {
				log.Error("Can't fetch configuration from Graylog API: ", err)
				continue
			}
			if backend.RenderOnChange(jsonConfig, context.CollectorConfPath) {
				if !backend.ValidateConfigurationFile(context.CollectorConfPath) {
					log.Info("Collector configuration file is not valid, waiting for update...")
					continue
				}

				err = context.Program.Restart(context.Service)
				if err != nil {
					log.Error("Failed to restart collector %v", err)
				}

			}
		}
	}()
}
