package daemon

import (
	"time"
	"fmt"
	"os/exec"

	"golang.org/x/sys/windows/svc/mgr"
	"github.com/kardianos/service"

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/context"
	"golang.org/x/sys/windows/svc"
	"strings"
)

type SvcRunner struct {
	RunnerCommon
	exec           string
	args           []string
	startTime      time.Time
	service        service.Service
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

	// check if service is installed
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("[%s] Failed to connect to service manager: %v", r.name, err)
	}
	defer m.Disconnect()

	// if yes delete it
	s, err := m.OpenService("graylog-collector-" + r.name)
	if err == nil {
		log.Debugf("[%s] service %s already exists", r.name)
		err = s.Delete()
		if err != nil {
			return fmt.Errorf("[%s] Failed to delete service: %v", r.name, err)
		}
//		err = eventlog.Remove("graylog-collector-" + r.name)
//		if err != nil {
//			return fmt.Errorf("[%s] RemoveEventLogSource() failed: %v", r.name, err)
//		}
	}

	// and create a new service
	s, err = m.CreateService("graylog-collector-" + r.name,
		execPath,
		mgr.Config{
			DisplayName: "Graylog collector sidecar - " + r.name + " backend",
			BinaryPathName: r.exec + " " + strings.Join(r.args, " ")},
		"is",
		"auto-started")
	if err != nil {
		return fmt.Errorf("[%s] Failed to install service: %v", r.name, err)
	}
	defer s.Close()

//	err = eventlog.InstallAsEventCreate(r.name, eventlog.Error|eventlog.Warning|eventlog.Info)
//	if err != nil {
//		s.Delete()
//		return fmt.Errorf("[%s] SetupEventLogSource() failed: %v", r.name, err)
//	}

	return err
}

func (r *SvcRunner) Start(s service.Service) error {
	if err := r.ValidateBeforeStart(); err != nil {
		log.Error(err.Error())
		return err
	}

	r.startTime = time.Now()
	log.Infof("[%s] Starting", r.name)

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("[%s] Failed to connect to service manager: %v", r.name, err)
	}
	defer m.Disconnect()

	ws, err := m.OpenService("graylog-collector-" + r.name)
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

	ws, err := m.OpenService("graylog-collector-" + r.name)
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
