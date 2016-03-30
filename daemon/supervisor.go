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

type Supervisor struct {
	Running        bool
	service        service.Service
	exit           chan struct{}
}

func (dc *DaemonConfig) NewSupervisor() *Supervisor {
	sv := &Supervisor{
		Running: false,
		exit:    make(chan struct{}),
	}

	return sv
}

func (sv *Supervisor) BindToService(s service.Service) {
	sv.service = s
}

func (sv *Supervisor) Start(s service.Service) error {
	log.Info("Starting supervisor process")
	go sv.run()
	return nil
}

func (sv *Supervisor) Stop(s service.Service) error {
	for name, runner := range Daemon.Runner {
		log.Infof("Stopping '%s'", name)
		runner.Stop(sv.service)
	}
	close(sv.exit)
	sv.Running = false
	return nil
}

func (sv *Supervisor) Restart(s service.Service) error {
	sv.Stop(s)
	sv.exit = make(chan struct{})
	sv.Start(s)
	return nil
}

func (sv *Supervisor) run() {
	sv.Running = true
	for name, runner := range Daemon.Runner {
		log.Infof("Sending start signal to '%s'", name)
		runner.Start(sv.service)
	}
	return
}
