package services

import (
	"time"

	"github.com/Sirupsen/logrus"

	"github.com/Graylog2/nxlog-sidecar/api"
	"github.com/Graylog2/nxlog-sidecar/context"
	"github.com/Graylog2/nxlog-sidecar/util"
)

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
	sidecarPath, _ := util.GetSidecarPath()

	go func() {
		for {
			time.Sleep(10 * time.Second)
			jsonConfig, err := api.RequestConfiguration(context)
			if err != nil {
				logrus.Error("Can't fetch configuration from Graylog API: ", err)
				continue
			}
			if backend.RenderOnChange(jsonConfig) {
				if !backend.ValidateConfigurationFile(sidecarPath) {
					logrus.Info("Collector configuration file is not valid, waiting for update...")
					continue
				}


				err = context.Program.Restart(context.Service)
				if err != nil {
					logrus.Error("Failed to restart collector %v", err)
				}

			}
		}
	}()
}
