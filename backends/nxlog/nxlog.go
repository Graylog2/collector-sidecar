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
	"path/filepath"
	"runtime"

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/system"
	"github.com/Graylog2/collector-sidecar/logger"
)

const name = "nxlog"

var (
	log           = logger.Log()
	backendStatus = system.Status{}
)

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

func (nxc *NxConfig) Driver() string {
	if runtime.GOOS == "windows" {
		return "svc"
	}
	return "exec"
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
		err := common.CreatePathToFile(configurationPath)
		if err != nil {
			log.Fatalf("[%s] Configured path to collector configuration does not exist: %s", nxc.Name(), configurationPath)
		}
	}

	return configurationPath
}

func (nxc *NxConfig) ExecArgs() []string {
	// nxlog runs as system service on Windows, no foreground mode. Configuration path needs to be double quoted for service configuration
	if runtime.GOOS == "windows" {
		return []string{"-c", "\"" + nxc.ConfigurationPath() + "\""}
	}
	return []string{"-f", "-c", nxc.ConfigurationPath()}
}

func (nxc *NxConfig) ValidatePreconditions() bool {
	if runtime.GOOS == "linux" {
		runDir := "/var/run/graylog/collector-sidecar" // set in default-snippet
		if nxc.UserConfig.RunPath != "" {
			runDir = nxc.UserConfig.RunPath
		}
		if !common.IsDir(runDir) {
			log.Errorf("[%s] Path to PidFile doesn't exist. Trying to create it before starting NXLog.", nxc.Name())
			err := common.CreatePathToFile(filepath.Join(runDir, "nxlog.run"))
			if err != nil {
				log.Errorf("[%s] Unable to create directory %s: %v", nxc.Name(), runDir, err)
				return false
			}
		}
	}
	return true
}

func (nxc *NxConfig) SetStatus(state int, message string) {
	backendStatus.Set(state, message)
}

func (nxc *NxConfig) Status() system.Status {
	return backendStatus
}
