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

package topbeat

import (
	"runtime"

	"github.com/Graylog2/collector-sidecar/util"
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/context"
)

const name = "topbeat"

var log = util.Log()

func init() {
	if err := backends.RegisterBackend(name, New); err != nil {
		log.Fatal(err)
	}
}

func New(context *context.Ctx) backends.Backend {
	return NewCollectorConfig(context)
}

func (tbc *TopBeatConfig) Name() string {
	return name
}

func (tbc *TopBeatConfig) ExecPath() string {
	var err error
	execPath := tbc.Beats.Context.CollectorPath
	if runtime.GOOS == "windows" {
		execPath, err = util.AppendIfDir(tbc.Beats.Context.CollectorPath, "topbeat.exe")
	} else {
		execPath, err = util.AppendIfDir(tbc.Beats.Context.CollectorPath, "topbeat")
	}
	if err != nil {
		log.Error("Failed to auto-complete topbeat path. Please provide full path to binary")
	}

	return execPath
}

func (tbc *TopBeatConfig) ExecArgs(configurationPath string) []string {
	err := util.FileExists(configurationPath)
	if err != nil {
		log.Error("Collector configuration file is not accessable: ", configurationPath)
	}
	return []string{"-c", configurationPath}
}

func (tbc *TopBeatConfig) ValidatePreconditions() bool {
	return true
}
