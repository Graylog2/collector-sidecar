package daemon

import (
	"fmt"
	"time"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/kardianos/service"

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/context"
)

type SvcRunner struct {
	RunnerCommon
	exec           string
	args           []string
	startTime      time.Time
	service        service.Service
	serviceName    string
}

func NewSvcRunner(backend backends.Backend, context *context.Ctx) Runner {
	r := &SvcRunner{
		RunnerCommon: RunnerCommon{
			name: backend.Name(),
			isRunning: false,
			context: context,
			backend:      backend,
		},
		exec:         backend.ExecPath(),
		args:         backend.ExecArgs(),
		serviceName:  "graylog-collector-" + backend.Name(),
	}

	return r
}

func (r *SvcRunner) Name() string {
	return r.name
}

func (r *SvcRunner) Running() bool {
	return r.isRunning
}

func (r *SvcRunner) SetDaemon(d *DaemonConfig) {
	r.daemon = d
}

func (r *SvcRunner) BindToService(s service.Service) {
	r.service = s
}

func (r *SvcRunner) GetService() service.Service {
	return r.service
}

func (r *SvcRunner) ValidateBeforeStart() error {
	execPath, err := exec.LookPath(r.exec)
	if err != nil {
		msg := "Failed to find collector executable"
		r.backend.SetStatus(backends.StatusError, msg)
		return fmt.Errorf("[%s] %s %q: %v", r.name, msg, r.exec, err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("[%s] Failed to connect to service manager: %v", r.name, err)
	}
	defer m.Disconnect()

	serviceConfig := mgr.Config{
		DisplayName: "Graylog collector sidecar - " + r.name + " backend",
		Description: "Wrapper service for the NXLog backend",
		BinaryPathName: r.exec + " " + strings.Join(r.args, " ")}

	s, err := m.OpenService(r.serviceName)
	// service exist so we only update the properties
	if err == nil {
		log.Debugf("[%s] service %s already exists, updating properties", r.name)
		currentConfig, err := s.Config()
		if err == nil {
			currentConfig.DisplayName = serviceConfig.DisplayName
			currentConfig.Description = serviceConfig.Description
			currentConfig.BinaryPathName = serviceConfig.BinaryPathName
		}
		err = s.UpdateConfig(currentConfig)
		if err != nil {
			log.Errorf("[%s] Failed to update service: %v", r.name, err)
		}
	// service needs to be created
	} else {
		s, err = m.CreateService(r.serviceName,
			execPath,
			serviceConfig)
		if err != nil {
			log.Errorf("[%s] Failed to install service: %v", r.name, err)
		}
		defer s.Close()
		err = eventlog.InstallAsEventCreate(r.serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)
		if err != nil {
			s.Delete()
			log.Errorf("[%s] SetupEventLogSource() failed: %v", r.name, err)
		}
	}

	return nil
}

func (r *SvcRunner) Start(s service.Service) error {
	if err := r.ValidateBeforeStart(); err != nil {
		log.Error(err.Error())
		return err
	}

	r.startTime = time.Now()
	log.Infof("[%s] Starting with %s driver", r.name, r.backend.Driver())

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("[%s] Failed to connect to service manager: %v", r.name, err)
	}
	defer m.Disconnect()

	ws, err := m.OpenService(r.serviceName)
	if err != nil {
		return fmt.Errorf("[%s] Could not access service: %v", r.name, err)
	}
	defer ws.Close()

	err = ws.Start("is", "manual-started")
	if err != nil {
		return fmt.Errorf("[%s] Could not start service: %v", r.name, err)
	}

	r.isRunning = true
	r.backend.SetStatus(backends.StatusRunning, "Running")

	return err
}

func (r *SvcRunner) Stop(s service.Service) error {
	log.Infof("[%s] Stopping", r.name)

	r.isRunning = false

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("[%s] Failed to connect to service manager: %v", r.name, err)
	}
	defer m.Disconnect()

	ws, err := m.OpenService(r.serviceName)
	if err != nil {
		return fmt.Errorf("[%s] Could not access service: %v", r.name, err)
	}
	defer ws.Close()

	status, err := ws.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("[%s] Could not send stop control: %v", r.name, err)
	}

	timeout := time.Now().Add(10 * time.Second)
	for status.State != svc.Stopped {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("[%s] Timeout waiting for service to go to stopped state: %v", r.name, err)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = ws.Query()
		if err != nil {
			return fmt.Errorf("[%s] Could not retrieve service status: %v", r.name, err)
		}
	}

	return nil
}

func (r *SvcRunner) Restart(s service.Service) error {
	r.Stop(s)
	time.Sleep(2 * time.Second)
	r.Start(s)

	return nil
}
