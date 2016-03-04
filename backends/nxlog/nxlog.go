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

package nxlog

import (
	"runtime"

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/util"
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
		if !util.IsDir("/var/run/graylog/collector-sidecar") {
			err := util.CreatePathToFile("/var/run/graylog/collector-sidecar/nxlog.run")
			if err != nil {
				return false
			}
		}
	}
	return true
}
