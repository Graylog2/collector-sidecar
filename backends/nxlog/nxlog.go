package nxlog

import (
	"github.com/Sirupsen/logrus"

	"github.com/Graylog2/nxlog-sidecar/backends"
	"runtime"
	"github.com/Graylog2/nxlog-sidecar/util"
	"path/filepath"
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
	return []string{"-f", "-c", filepath.Join(configurationPath, "nxlog", "nxlog.conf")}
}