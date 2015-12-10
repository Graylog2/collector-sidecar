package nxlog

import (
	"path/filepath"
	"time"

	"github.com/Sirupsen/logrus"

	"github.com/Graylog2/nxlog-sidecar/daemon"
	"github.com/Graylog2/nxlog-sidecar/util"
	"github.com/Graylog2/nxlog-sidecar/api"
)

func StartPeriodicals(context *daemon.Ctx) {
	fetchConfiguration(context)
	updateCollectorRegistration(context)
}

// fetch configuration periodically
func fetchConfiguration (context *daemon.Ctx) {
	nxc := context.NxConfig
	gxlogPath, _ := util.GetGxlogPath()

	go func() {
		for {
			time.Sleep(10 * time.Second)
			tmpConfig, err := nxc.FetchFromServer(context.ServerUrl.String())
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

func updateCollectorRegistration (context *daemon.Ctx) {
	go func() {
		for {
			time.Sleep(10 * time.Second)
			api.UpdateRegistration(context.ServerUrl.String())
		}
	}()
}