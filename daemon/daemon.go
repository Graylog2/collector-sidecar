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

package daemon

import (
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
)

var Daemon *DaemonConfig
var log = common.Log()

type DaemonConfig struct {
	Name        string
	DisplayName string
	Description string

	Dir string
	Env []string

	Runner map[string]Runner
}


func init() {
	Daemon = NewConfig()
}

func NewConfig() *DaemonConfig {
	rootDir, err := common.GetRootPath()
	if err != nil {
		log.Error("Can not access root directory")
	}

	dc := &DaemonConfig{
		Name:        "collector-sidecar",
		DisplayName: "Graylog collector sidecar",
		Description: "Wrapper service for Graylog controlled collector",
		Dir:         rootDir,
		Env:         []string{},
		Runner:      map[string]Runner{},
	}

	return dc
}

func (dc *DaemonConfig) AddBackend(backend backends.Backend, context *context.Ctx) {
	var runner Runner
	switch backend.Driver() {
	case "exec":
		runner = NewExecRunner(backend, context)
	case "svc":
		runner = NewSvcRunner(backend, context)
	}
	runner.SetDaemon(dc)
	dc.Runner[backend.Name()] = runner
}
