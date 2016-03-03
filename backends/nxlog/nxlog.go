package nxlog

import (
	"runtime"

	"github.com/Graylog2/sidecar/backends"
	"github.com/Graylog2/sidecar/context"
	"github.com/Graylog2/sidecar/util"
)

const name = "nxlog"

var log = util.Log()

func init() {
	if err := backends.RegisterBackend(name, New); err != nil {
		log.Fatal(err)
	}
}

func New(context *context.Ctx) backends.Backend {
	return NewCollectorConfig(context)
}

func (nxc *NxConfig) Name() string {
	return name
}

func (nxc *NxConfig) ExecPath() string {
	var err error
	execPath := nxc.Context.CollectorPath
	if runtime.GOOS == "windows" {
		execPath, err = util.AppendIfDir(nxc.Context.CollectorPath, "nxlog.exe")
	} else {
		execPath, err = util.AppendIfDir(nxc.Context.CollectorPath, "nxlog")
	}
	if err != nil {
		log.Error("Failed to auto-complete nxlog path. Please provide full path to binary")
	}

	return execPath
}

func (nxc *NxConfig) ExecArgs(configurationPath string) []string {
	err := util.FileExists(configurationPath)
	if err != nil {
		log.Error("Collector configuration file is not accessable: ", configurationPath)
	}
	return []string{"-f", "-c", configurationPath}
}

func (nxc *NxConfig) ValidatePreconditions() bool {
	if runtime.GOOS == "linux" {
		if !util.IsDir("/var/run/nxlog") {
			err := util.CreatePathToFile("/var/run/nxlog/nxlog.run")
			if err != nil {
				return false
			}
		}
	}
	return true
}
