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

package filebeat

import (
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/context"
	"path/filepath"
)

const name = "filebeat"

var log = common.Log()

func init() {
	if err := backends.RegisterBackend(name, New); err != nil {
		log.Fatal(err)
	}
}

func New(context *context.Ctx) backends.Backend {
	return NewCollectorConfig(context)
}

func (fbc *FileBeatConfig) Name() string {
	return name
}

func (fbc *FileBeatConfig) ExecPath() string {
	execPath := fbc.Beats.UserConfig.BinaryPath
	if common.FileExists(execPath) != nil {
		log.Fatal("Configured path to collector binary does not exist: " + execPath)
	}

	return execPath
}

func (fbc *FileBeatConfig) ConfigurationPath() string {
	configurationPath := fbc.Beats.UserConfig.ConfigurationPath
	if !common.IsDir(filepath.Dir(configurationPath)) {
		log.Fatal("Configured path to collector configuration does not exist: " + configurationPath)
	}

	return configurationPath
}

func (fbc *FileBeatConfig) ExecArgs() []string {
	return []string{"-c", fbc.ConfigurationPath()}
}

func (fbc *FileBeatConfig) ValidatePreconditions() bool {
	return true
}