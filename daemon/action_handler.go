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
	"github.com/Graylog2/collector-sidecar/api/graylog"
)

func HandleCollectorActions(actions []graylog.ResponseCollectorAction) {
	for _, action := range actions {
		switch {
		case action.Properties["restart"] == true:
			restartAction(action)
		}
	}
}

func restartAction(action graylog.ResponseCollectorAction) {
	for name, runner := range Daemon.Runner {
		if name == action.Backend {
			log.Infof("[%s] Executing requested collector restart", name)
			runner.Restart()
		}
	}
}