package nxlog

import (
	"runtime"

	"github.com/Graylog2/sidecar/backends"
	"github.com/Graylog2/sidecar/util"
)

const name = "nxlog"
var log = util.Log()

func init() {
	if err := backends.RegisterBackend(name, New); err != nil {
		log.Fatal(err)
	}
}

func New(collectorPath string, collectorId string) backends.Backend {
	return NewCollectorConfig(collectorPath, collectorId)
}

func (nxc *NxConfig) Name() string {
	return name
}

func (nxc *NxConfig) ExecPath() string {
	var err error
	execPath := nxc.CollectorPath
	if runtime.GOOS == "windows" {
		execPath, err = util.AppendIfDir(nxc.CollectorPath, "nxlog.exe")
	} else {
		execPath, err = util.AppendIfDir(nxc.CollectorPath, "nxlog")
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
