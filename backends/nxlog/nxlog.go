package nxlog

import (
	"runtime"

	"github.com/Sirupsen/logrus"

	"github.com/Graylog2/sidecar/backends"
	"github.com/Graylog2/sidecar/util"
)

const name = "nxlog"

func init() {
	if err := backends.RegisterBackend(name, New); err != nil {
		logrus.Fatal(err)
	}
}

func New(collectorPath string) backends.Backend {
	return NewCollectorConfig(collectorPath)
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
		logrus.Error("Failed to auto-complete nxlog path. Please provide full path to binary")
	}

	return execPath
}

func (nxc *NxConfig) ExecArgs(configurationPath string) []string {
	err := util.FileExists(configurationPath)
	if err != nil {
		logrus.Error("Collector configuration file is not accessable: ", configurationPath)
	}
	return []string{"-f", "-c", configurationPath}
}
