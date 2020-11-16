// Copyright (C) 2020 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.


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
		for runner.Running() {
			time.Sleep(300 * time.Millisecond)
		}
	}
	dist.Running = false

	return nil
}
