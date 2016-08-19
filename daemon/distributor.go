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
	"github.com/kardianos/service"
)

type Distributor struct {
	Running bool
	service service.Service
	exit    chan struct{}
}

func (dc *DaemonConfig) NewDistributor() *Distributor {
	sv := &Distributor{
		Running: false,
		exit:    make(chan struct{}),
	}

	return sv
}

func (dist *Distributor) BindToService(s service.Service) {
	dist.service = s
}

func (dist *Distributor) Start(s service.Service) error {
	log.Info("Starting signal distributor")
	go dist.run()
	return nil
}

func (dist *Distributor) Stop(s service.Service) error {
	for _, runner := range Daemon.Runner {
		runner.Stop(dist.service)
	}
	close(dist.exit)
	dist.Running = false
	return nil
}

func (dist *Distributor) Restart(s service.Service) error {
	dist.Stop(s)
	dist.exit = make(chan struct{})
	dist.Start(s)
	return nil
}

func (dist *Distributor) run() {
	dist.Running = true
	for _, runner := range Daemon.Runner {
		runner.Start(dist.service)
	}
	return
}
