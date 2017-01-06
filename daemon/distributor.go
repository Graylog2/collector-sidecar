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
	"time"

	"github.com/kardianos/service"
)

type Distributor struct {
	Running bool
	service service.Service
}

func (dc *DaemonConfig) NewDistributor() *Distributor {
	dist := &Distributor{
		Running: false,
	}

	return dist
}

func (dist *Distributor) BindToService(s service.Service) {
	dist.service = s
}

// start all backend runner, don't block
func (dist *Distributor) Start(s service.Service) error {
	log.Info("Starting signal distributor")
	dist.Running = true
	for _, runner := range Daemon.Runner {
		runner.Restart()
	}

	return nil
}

// stop all backend runner parallel and wait till they are finished
func (dist *Distributor) Stop(s service.Service) error {
	log.Info("Stopping signal distributor")
	for _, runner := range Daemon.Runner {
		runner.Shutdown()
	}
	for _, runner := range Daemon.Runner {
		for runner.Running() {time.Sleep(300 * time.Millisecond)}
	}
	dist.Running = false

	return nil
}
