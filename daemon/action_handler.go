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
	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/helpers"
)

func HandleCollectorActions(actions []graylog.ResponseCollectorAction) {
	for _, action := range actions {
		backends := backends.Store.GetBackendsForCollectorId(action.BackendId)
		if backends == nil {
			log.Errorf("Got action for non-existing collector: %s", action.BackendId)
			continue
		}
		for _, backend := range backends {
			switch {
			case action.Properties["start"] == true:
				startAction(backend)
			case action.Properties["restart"] == true:
				restartAction(backend)
			case action.Properties["stop"] == true:
				stopAction(backend)
			default:
				log.Infof("Got unsupported collector command: %s", helpers.Inspect(action.Properties))
			}
		}
	}
}

func startAction(backend *backends.Backend) {
	for id, runner := range Daemon.Runner {
		if id == backend.Id {
			if !runner.Running() {
				log.Infof("[%s] Got remote start command", backend.Name)
				runner.Restart()
			} else {
				log.Infof("Collector [%s] is already running, skipping start action.", backend.Name)
			}
		}
	}
}

func restartAction(backend *backends.Backend) {
	for id, runner := range Daemon.Runner {
		if id == backend.Id {
			log.Infof("[%s] Got remote restart command", backend.Name)
			runner.Restart()
		}
	}
}

func stopAction(backend *backends.Backend) {
	for id, runner := range Daemon.Runner {
		if id == backend.Id {
			log.Infof("[%s] Got remote stop command", backend.Name)
			runner.Shutdown()
		}
	}
}
