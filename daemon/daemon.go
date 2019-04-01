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
	"github.com/Graylog2/collector-sidecar/assignments"
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/logger"
)

var (
	Daemon         *DaemonConfig
	runnerRegistry = make(map[string]RunnerCreator)
	log            = logger.Log()
)

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
		Name:        "graylog-sidecar",
		DisplayName: "Graylog Sidecar",
		Description: "Wrapper service for Graylog controlled collector",
		Dir:         rootDir,
		Env:         []string{},
		Runner:      map[string]Runner{},
	}

	return dc
}

func RegisterBackendRunner(name string, c RunnerCreator) error {
	if _, ok := runnerRegistry[name]; ok {
		log.Error("Execution driver named " + name + " is already registered")
		return nil
	}
	runnerRegistry[name] = c
	return nil
}

func (dc *DaemonConfig) AddRunner(backend backends.Backend, context *context.Ctx) {
	var runner Runner
	if runnerRegistry[backend.ServiceType] == nil {
		backend.SetStatusLogErrorf("Execution driver %s is not supported on this platform", backend.ServiceType)
		return
	}
	switch backend.ServiceType {
	case "exec":
		runner = runnerRegistry["exec"](backend, context)
	case "svc":
		runner = runnerRegistry["svc"](backend, context)
	default:
		log.Fatalf("Execution driver %s is not supported on this platform", backend.ServiceType)
	}
	runner.SetDaemon(dc)
	dc.Runner[backend.Id] = runner
}

func (dc *DaemonConfig) DeleteRunner(backendId string) {
	if dc.Runner[backendId] == nil {
		return
	}

	if dc.Runner[backendId].Running() {
		if err := dc.Runner[backendId].Shutdown(); err != nil {
			log.Errorf("[%s] Failed to stop backend during deletion: %v", backendId, err)
		}
	}
	delete(dc.Runner, backendId)
}

func (dc *DaemonConfig) GetRunnerByBackendId(id string) Runner {
	for _, runner := range dc.Runner {
		if runner.GetBackend().Id == id {
			return runner
		}
	}
	return nil
}

func (dc *DaemonConfig) SyncWithAssignments(context *context.Ctx) {
	if dc.Runner == nil {
		return
	}

	for id, runner := range dc.Runner {
		backend := backends.Store.GetBackend(id)

		// update outdated runner backend
		runnerBackend := runner.GetBackend()
		if backend != nil && !runnerBackend.EqualSettings(backend) {
			log.Infof("[%s] Updating process configuration", runner.Name())
			runnerServiceType := runnerBackend.ServiceType
			runner.SetBackend(*backend)
			if backend.ServiceType != runnerServiceType {
				log.Infof("Changing process runner (%s -> %s) for: %s",
					runnerServiceType, backend.ServiceType, backend.Name)
				dc.DeleteRunner(id)
				dc.AddRunner(*backend, context)
			}
			// XXX
			// We should, but cannot trigger a restart here.
			//
			// If a backend gets renamed, it expects the configuration under a new path.
			// Therefore, we don't copy the configuration from the old backend to the new backend,
			// but keep it empty.
			// This will trigger `services.checkForUpdateAndRestart()` to write a new
			// configuration and then restart the runner.
		}

		// cleanup backends that should not run anymore
		if backend == nil || assignments.Store.GetAll()[backend.Id] == "" {
			log.Info("Removing process runner: " + backend.Name)
			dc.DeleteRunner(id)
		}
	}
	assignedBackends := []*backends.Backend{}
	for backendId := range assignments.Store.GetAll() {
		backend := backends.Store.GetBackendById(backendId)
		if backend != nil {
			assignedBackends = append(assignedBackends, backend)
		}
	}
	CleanOldServices(assignedBackends)

	// add new backends to registry
	for _, backend := range assignedBackends {
		if dc.Runner[backend.Id] == nil {
			log.Info("Adding process runner for: " + backend.Name)
			dc.AddRunner(*backend, context)
		}
	}
}
