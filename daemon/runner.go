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
	"github.com/Graylog2/collector-sidecar/context"
)

type Runner interface {
	Name() string
	Running() bool
	ValidateBeforeStart() error
	Restart() error
	Shutdown() error
	SetDaemon(*DaemonConfig)
}

type RunnerCommon struct {
	name    string
	context *context.Ctx
	backend backends.Backend
	daemon  *DaemonConfig
}

type RunnerCreator func(backends.Backend, *context.Ctx) Runner
