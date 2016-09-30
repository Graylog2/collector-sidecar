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

// (at your option) any later version.
//
// Graylog is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Graylog.  If not, see <http://www.gnu.org/licenses/>.

package winlogbeat

import (
	"path/filepath"
	"runtime"

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/system"
)

const (
	name   = "winlogbeat"
	driver = "exec"
)

var (
	log           = common.Log()
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

func (wlbc *WinLogBeatConfig) Name() string {
	return name
}

func (wlbc *WinLogBeatConfig) Driver() string {
	return driver
}

func (wlbc *WinLogBeatConfig) ExecPath() string {
	execPath := wlbc.Beats.UserConfig.BinaryPath
	if common.FileExists(execPath) != nil {
		log.Fatal("Configured path to collector binary does not exist: " + execPath)
	}

	return execPath
}

func (wlbc *WinLogBeatConfig) ConfigurationPath() string {
	configurationPath := wlbc.Beats.UserConfig.ConfigurationPath
	if !common.IsDir(filepath.Dir(configurationPath)) {
		err := common.CreatePathToFile(configurationPath)
		if err != nil {
			log.Fatal("Configured path to collector configuration does not exist: " + configurationPath)
		}
	}

	return configurationPath
}

func (wlbc *WinLogBeatConfig) ExecArgs() []string {
	if runtime.GOOS == "windows" {
		return []string{"-c", "\"" + wlbc.ConfigurationPath() + "\""}
	}
	return []string{"-c", wlbc.ConfigurationPath()}
}

func (wlbc *WinLogBeatConfig) ValidatePreconditions() bool {
	return true
}

func (wlbc *WinLogBeatConfig) SetStatus(state int, message string) {
	// if error state is already set don't overwrite the message to get the root cause
	if state > backends.StatusRunning &&
		backendStatus.Status > backends.StatusRunning &&
		len(backendStatus.Message) != 0 {
		backendStatus.Set(state, backendStatus.Message)
	} else {
		backendStatus.Set(state, message)
	}
}

func (wlbc *WinLogBeatConfig) Status() system.Status {
	return backendStatus
}
