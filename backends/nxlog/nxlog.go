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
	"github.com/Graylog2/collector-sidecar/common"
	"path/filepath"
)

const name = "nxlog"
var log = common.Log()

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
	execPath := nxc.UserConfig.BinaryPath
	if common.FileExists(execPath) != nil {
		log.Fatalf("[%s] Configured path to collector binary does not exist: %s", nxc.Name(), execPath)
	}

	return execPath
}

func (nxc *NxConfig) ConfigurationPath() string {
	configurationPath := nxc.UserConfig.ConfigurationPath
	if !common.IsDir(filepath.Dir(configurationPath)) {
		log.Fatalf("[%s] Configured path to collector configuration does not exist: %s", nxc.Name(), configurationPath)
	}

	return configurationPath
}


func (nxc *NxConfig) ExecArgs() []string {
	return []string{"-f", "-c", nxc.ConfigurationPath()}
}

func (nxc *NxConfig) ValidatePreconditions() bool {
	if runtime.GOOS == "linux" {
		if !common.IsDir("/var/run/graylog/collector-sidecar") {
			err := common.CreatePathToFile("/var/run/graylog/collector-sidecar/nxlog.run")
			if err != nil {
				return false
			}
		}
	}
	return true
}
