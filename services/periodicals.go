package services

import (
	"path/filepath"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"

	"github.com/Graylog2/nxlog-sidecar/util"
	"github.com/Graylog2/nxlog-sidecar/backends/nxlog"
	"github.com/Graylog2/nxlog-sidecar/context"
	"github.com/Graylog2/nxlog-sidecar/api"
)

func StartPeriodicals(context *context.Ctx) {
	fetchConfiguration(context)
	updateCollectorRegistration(context)
}

// fetch configuration periodically
func fetchConfiguration (context *context.Ctx) {
	nxc := context.NxConfig
	gxlogPath, _ := util.GetGxlogPath()

	go func() {
		for {
			time.Sleep(10 * time.Second)
			tmpConfig, err := fetchConfigurationFromServer(context)
			if err != nil {
				// can't access Graylog's API
				continue
			}

			if !nxc.Equals(tmpConfig) {
				logrus.Info("Configuration change detected, reloading nxlog.")
				nxc = tmpConfig
				nxc.RenderToFile(filepath.Join(gxlogPath, "nxlog", "nxlog.conf"))
				err = context.Program.Restart(context.Service)
				if err != nil {
					logrus.Error("Failed to restart nxlog %v", err)
				}
			}
		}
	}()
}

func updateCollectorRegistration (context *context.Ctx) {
	go func() {
		for {
			time.Sleep(10 * time.Second)
			api.UpdateRegistration(context)
		}
	}()
}

func fetchConfigurationFromServer(context *context.Ctx) (*nxlog.NxConfig, error) {
	nxc := context.NxConfig

	jsonConfig, err := api.RequestConfiguration(context)
	if err != nil {
		logrus.Error("Can't fetch configuration from Graylog API: ", err)
		return nil, err
	}

	nxConfig := nxlog.NewNxConfig(nxc.Nxpath)
	for _, output := range jsonConfig.Outputs {
		if output.Type == "nxlog" {
			nxConfig.AddOutput(output.Name, output.Properties)
		}
	}
	for i, input := range jsonConfig.Inputs {
		if input.Type == "nxlog" {
			nxConfig.AddInput(input.Name, input.Properties)
			nxConfig.AddRoute("route-" + strconv.Itoa(i), map[string]string{"Path": input.Name + " => " + input.ForwardTo})
		}
	}
	return nxConfig, err
}