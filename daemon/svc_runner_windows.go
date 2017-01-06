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
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/context"
)

type SvcRunner struct {
	RunnerCommon
	exec        string
	args        []string
	startTime   time.Time
	serviceName string
	isRunning   bool
}

func init() {
	if err := RegisterBackendRunner("svc", NewSvcRunner); err != nil {
		log.Fatal(err)
	}
}

func NewSvcRunner(backend backends.Backend, context *context.Ctx) Runner {
	r := &SvcRunner{
		RunnerCommon: RunnerCommon{
			name:    backend.Name(),
			context: context,
			backend: backend,
		},
		exec:        backend.ExecPath(),
		args:        backend.ExecArgs(),
		serviceName: "graylog-collector-" + backend.Name(),
		isRunning:   false,
	}

	return r
}

func (r *SvcRunner) Name() string {
	return r.name
}

func (r *SvcRunner) Running() bool {
	m, err := mgr.Connect()
	if err != nil {
		backends.SetStatusLogErrorf(r.name, "Failed to connect to service manager: %v", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(r.serviceName)
	// service exist so we only update the properties
	if err != nil {
		backends.SetStatusLogErrorf(r.name, "Can't get status of service %s cause it doesn't exist: %v", r.serviceName, err)
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		backends.SetStatusLogErrorf(r.name, "Can't query status of service %s: %v", r.serviceName, err)
	}

	return status.State == svc.Running
}

func (r *SvcRunner) SetDaemon(d *DaemonConfig) {
	r.daemon = d
}

func (r *SvcRunner) ValidateBeforeStart() error {
	execPath, err := exec.LookPath(r.exec)
	if err != nil {
		return backends.SetStatusLogErrorf(r.name, "Failed to find collector executable %s", r.exec)
	}

	m, err := mgr.Connect()
	if err != nil {
		return backends.SetStatusLogErrorf(r.name, "Failed to connect to service manager: %v", err)
	}
	defer m.Disconnect()

	serviceConfig := mgr.Config{
		DisplayName:    "Graylog collector sidecar - " + r.name + " backend",
		Description:    "Wrapper service for the NXLog backend",
		BinaryPathName: "\"" + r.exec + "\" " + strings.Join(r.args, " ")}

	s, err := m.OpenService(r.serviceName)
	// service exist so we only update the properties
	if err == nil {
		defer s.Close()
		log.Debugf("[%s] service %s already exists, updating properties", r.name)
		currentConfig, err := s.Config()
		if err == nil {
			currentConfig.DisplayName = serviceConfig.DisplayName
			currentConfig.Description = serviceConfig.Description
			currentConfig.BinaryPathName = serviceConfig.BinaryPathName
		}
		err = s.UpdateConfig(currentConfig)
		if err != nil {
			backends.SetStatusLogErrorf(r.name, "Failed to update service: %v", err)
		}
		// service needs to be created
	} else {
		s, err = m.CreateService(r.serviceName,
			execPath,
			serviceConfig)
		if err != nil {
			backends.SetStatusLogErrorf(r.name, "Failed to install service: %v", err)
		}
		defer s.Close()
		err = eventlog.InstallAsEventCreate(r.serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)
		if err != nil {
			s.Delete()
			backends.SetStatusLogErrorf(r.name, "SetupEventLogSource() failed: %v", err)
		}
	}

	return nil
}

func (r *SvcRunner) start() error {
	if err := r.ValidateBeforeStart(); err != nil {
		log.Error(err.Error())
		return err
	}

	r.startTime = time.Now()
	log.Infof("[%s] Starting (%s driver)", r.name, r.backend.Driver())

	m, err := mgr.Connect()
	if err != nil {
		return backends.SetStatusLogErrorf(r.name, "Failed to connect to service manager: %v", err)
	}
	defer m.Disconnect()

	ws, err := m.OpenService(r.serviceName)
	if err != nil {
		return backends.SetStatusLogErrorf(r.name, "Could not access service: %v", err)
	}
	defer ws.Close()

	err = ws.Start("is", "manual-started")
	if err != nil {
		return backends.SetStatusLogErrorf(r.name, "Could not start service: %v", err)
	}

	r.isRunning = true
	go func() {
		for {
			time.Sleep(10 * time.Second)
			if r.isRunning && !r.Running() {
				backends.SetStatusLogErrorf(r.name, "Backend crashed, sending restart signal")
				r.start()
				break
			}

			if !r.isRunning {
				break
			}
		}
	}()

	r.backend.SetStatus(backends.StatusRunning, "Running")

	return err
}

func (r *SvcRunner) Shutdown() error {
	log.Infof("[%s] Stopping", r.name)

	// deactivate supervisor
	r.isRunning = false

	m, err := mgr.Connect()
	if err != nil {
		return backends.SetStatusLogErrorf(r.name, "Failed to connect to service manager: %v", err)
	}
	defer m.Disconnect()

	ws, err := m.OpenService(r.serviceName)
	if err != nil {
		return backends.SetStatusLogErrorf(r.name, "Could not access service: %v", err)
	}
	defer ws.Close()

	status, err := ws.Control(svc.Stop)
	if err != nil {
		return backends.SetStatusLogErrorf(r.name, "Could not send stop control: %v", err)
	}

	timeout := time.Now().Add(10 * time.Second)
	for status.State != svc.Stopped {
		if timeout.Before(time.Now()) {
			return backends.SetStatusLogErrorf(r.name, "Timeout waiting for service to go to stopped state: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = ws.Query()
		if err != nil {
			return backends.SetStatusLogErrorf(r.name, "Could not retrieve service status: %v", err)
		}
	}

	return nil
}

func (r *SvcRunner) Restart() error {
	r.Shutdown()
	time.Sleep(2 * time.Second)
	r.start()

	return nil
}
