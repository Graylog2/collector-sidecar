package services

import (
	"time"

	"github.com/Sirupsen/logrus"

	"github.com/Graylog2/nxlog-sidecar/api"
	"github.com/Graylog2/nxlog-sidecar/context"
)

func StartPeriodicals(context *context.Ctx) {
	updateCollectorRegistration(context)
	//fetchConfiguration(context)
	checkForUpdateAndRestart(context)
}

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
				logrus.Error("Can't fetch configuration from Graylog API: ", err)
				continue
			}
			if backend.RenderOnChange(jsonConfig) {
				err = context.Program.Restart(context.Service)
				if err != nil {
					logrus.Error("Failed to restart collector %v", err)
				}

			}
		}
	}()
}

//func fetchConfiguration(context *context.Ctx) {
//	nxc := context.NxConfig
//	sidecarPath, _ := util.GetSidecarPath()
//
//	go func() {
//		for {
//			time.Sleep(10 * time.Second)
//			tmpConfig, err := fetchConfigurationFromServer(context)
//			if err != nil {
//				// can't access Graylog's API
//				continue
//			}
//
//			if !nxc.Equals(tmpConfig) {
//				logrus.Info("Configuration change detected, reloading nxlog.")
//				nxc = tmpConfig
//				nxc.RenderToFile(filepath.Join(sidecarPath, "nxlog", "nxlog.conf"))
//				err = context.Program.Restart(context.Service)
//
//				if err != nil {
//					logrus.Error("Failed to restart nxlog %v", err)
//				}
//			}
//		}
//	}()
//}
//
//
//func fetchConfigurationFromServer(context *context.Ctx) (*nxlog.NxConfig, error) {
//	//nxc := context.NxConfig
//	backend := context.Backend
//
//	jsonConfig, err := api.RequestConfiguration(context)
//	if err != nil {
//		logrus.Error("Can't fetch configuration from Graylog API: ", err)
//		return nil, err
//	}
//
//	//nxConfig := nxlog.NewCollectorConfig(nxc.CollectorPath)
//	nxConfig := nxlog.NewCollectorConfig(backend.Name())
//	for _, output := range jsonConfig.Outputs {
//		if output.Type == "nxlog" {
//			nxConfig.Add("output", output.Name, output.Properties)
//		}
//	}
//	for i, input := range jsonConfig.Inputs {
//		if input.Type == "nxlog" {
//			nxConfig.Add("input", input.Name, input.Properties)
//			nxConfig.Add("route", "route-"+strconv.Itoa(i), map[string]string{"Path": input.Name + " => " + input.ForwardTo})
//		}
//	}
//	for _, snippet := range jsonConfig.Snippets {
//		if snippet.Type == "nxlog" {
//			nxConfig.Add("snippet", snippet.Name, snippet.Value)
//		}
//	}
//	return nxConfig, err
//}
