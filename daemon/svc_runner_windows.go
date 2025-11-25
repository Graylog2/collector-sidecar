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
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Graylog2/collector-sidecar/common"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/context"
)

func ServiceNamePrefix() string {
	return fmt.Sprintf("%s-collector-", strings.ToLower(common.VendorName))
}

type SvcRunner struct {
	RunnerCommon
	exec         string
	args         string
	startTime    time.Time
	serviceName  string
	isSupervised atomic.Value
	signals      chan string
}

func init() {
	if err := RegisterBackendRunner("svc", NewSvcRunner); err != nil {
		log.Fatal(err)
	}
}

func NewSvcRunner(backend backends.Backend, context *context.Ctx) Runner {
	r := &SvcRunner{
		RunnerCommon: RunnerCommon{
			name:    backend.Name,
			context: context,
			backend: backend,
		},
		exec:        backend.ExecutablePath,
		args:        backend.ExecuteParameters,
		signals:     make(chan string),
		serviceName: ServiceNamePrefix() + backend.Name,
	}

	// set default state
	r.setSupervised(false)

	r.startSupervisor()
	r.signalProcessor()

	return r
}

func (r *SvcRunner) Name() string {
	return r.name
}

func (r *SvcRunner) Running() bool {
	m, err := mgr.Connect()
	if err != nil {
		r.backend.SetStatusLogErrorf("Failed to connect to service manager: %v", err)
		return false
	}
	defer m.Disconnect()

	s, err := m.OpenService(r.serviceName)
	if err != nil {
		return false
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		r.backend.SetStatusLogErrorf("Can't query status of service %s: %v", r.serviceName, err)
		return false
	}

	return status.State == svc.Running
}

func (r *SvcRunner) Supervised() bool {
	return r.isSupervised.Load().(bool)
}

func (r *SvcRunner) setSupervised(state bool) {
	r.isSupervised.Store(state)
}

func (r *SvcRunner) SetDaemon(d *DaemonConfig) {
	r.daemon = d
}

func (r *SvcRunner) GetBackend() *backends.Backend {
	return &r.backend
}

func (r *SvcRunner) SetBackend(b backends.Backend) {
	r.backend = b
	r.name = b.Name
	r.serviceName = ServiceNamePrefix() + b.Name
	r.exec = b.ExecutablePath
	r.args = b.ExecuteParameters
}

func (r *SvcRunner) ValidateBeforeStart() error {
	err := r.backend.CheckExecutableAgainstAccesslist(r.context)
	if err != nil {
		r.backend.SetStatusLogErrorf(err.Error())
		return err
	}

	if _, err := exec.LookPath(r.exec); err != nil {
		return r.backend.SetStatusLogErrorf("Failed to find collector executable %s", r.exec)
	}

	m, err := mgr.Connect()
	if err != nil {
		return r.backend.SetStatusLogErrorf("Failed to connect to service manager: %v", err)
	}
	defer m.Disconnect()

	serviceConfig := mgr.Config{
		DisplayName:    fmt.Sprintf("%s collector sidecar - %s backend", common.VendorName, r.name),
		Description:    fmt.Sprintf("Wrapper service for the %s backend", r.name),
		BinaryPathName: "\"" + r.exec + "\" " + r.args}

	s, err := m.OpenService(r.serviceName)
	// service exist so we only update the properties
	if err == nil {
		defer s.Close()
		log.Debugf("Service %s already exists, updating properties", r.name)
		currentConfig, err := s.Config()
		if err == nil {
			currentConfig.DisplayName = serviceConfig.DisplayName
			currentConfig.Description = serviceConfig.Description
			currentConfig.BinaryPathName = serviceConfig.BinaryPathName
		}
		err = s.UpdateConfig(currentConfig)
		if err != nil {
			r.backend.SetStatusLogErrorf("Failed to update service: %v", err)
		}
		// service needs to be created
	} else {
		s, err = m.CreateService(r.serviceName,
			r.exec,
			serviceConfig)
		if err != nil {
			return r.backend.SetStatusLogErrorf("Failed to install service: %v", err)
		}
		// It seems impossible to create a service with properly quoted arguments :-(
		// Updating the BinaryPathName afterwards does the trick
		currentConfig, err := s.Config()
		if err == nil {
			currentConfig.BinaryPathName = serviceConfig.BinaryPathName
		}
		err = s.UpdateConfig(currentConfig)
		if err != nil {
			r.backend.SetStatusLogErrorf("Failed to update the created service: %v", err)
		}
		defer s.Close()
		err = eventlog.InstallAsEventCreate(r.serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)
		if err != nil {
			s.Delete()
			return r.backend.SetStatusLogErrorf("SetupEventLogSource() failed: %v", err)
		}
	}

	return nil
}

func (r *SvcRunner) startSupervisor() {
	go func() {
		for {
			// prevent cpu lock
			time.Sleep(10 * time.Second)

			// ignore regular shutdown
			if !r.Supervised() {
				continue
			}

			// check if process exited
			if r.Running() {
				continue
			}

			r.backend.SetStatusLogErrorf("Backend finished unexpectedly, sending restart signal")
			r.Restart()
		}
	}()
}

func (r *SvcRunner) start() error {
	if err := r.ValidateBeforeStart(); err != nil {
		log.Errorf("[%s] %s", r.Name(), err)
		return err
	}

	r.startTime = time.Now()
	log.Infof("[%s] Starting (%s driver)", r.name, r.backend.ServiceType)

	m, err := mgr.Connect()
	if err != nil {
		return r.backend.SetStatusLogErrorf("Failed to connect to service manager: %v", err)
	}
	defer m.Disconnect()

	ws, err := m.OpenService(r.serviceName)
	if err != nil {
		return r.backend.SetStatusLogErrorf("Could not access service %s: %v", r.serviceName, err)
	}
	defer ws.Close()

	err = ws.Start("is", "manual-started")
	if err != nil {
		return r.backend.SetStatusLogErrorf("Could not start service: %v", err)
	}

	r.setSupervised(true)
	r.backend.SetStatus(backends.StatusRunning, "Running", "")

	return err
}

func (r *SvcRunner) Shutdown() error {
	r.signals <- "shutdown"
	return nil
}

func (r *SvcRunner) stop() error {
	log.Infof("[%s] Stopping", r.name)

	// deactivate supervisor
	r.setSupervised(false)

	err := stopService(r.serviceName)
	if err != nil {
		return r.backend.SetStatusLogErrorf("%s", err)
	}
	r.backend.SetStatus(backends.StatusStopped, "Stopped", "")

	return nil
}

func (r *SvcRunner) Restart() error {
	r.signals <- "restart"
	return nil
}

func (r *SvcRunner) restart() error {
	if r.Running() {
		r.stop()
		for timeout := 0; r.Running() || timeout >= 5; timeout++ {
			log.Debugf("[%s] waiting for process to finish...", r.Name())
			time.Sleep(1 * time.Second)
		}
	}
	r.start()

	return nil
}

// process signals sequentially to prevent race conditions with the supervisor
func (r *SvcRunner) signalProcessor() {
	go func() {
		seq := 0
		for {
			cmd := <-r.signals
			seq++
			log.Debugf("[signal-processor] (seq=%d) handling cmd: %v", seq, cmd)
			switch cmd {
			case "restart":
				r.restart()
			case "shutdown":
				r.stop()
			}
			log.Debugf("[signal-processor] (seq=%d) cmd done: %v", seq, cmd)
		}
	}()
}
