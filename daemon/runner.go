package daemon

import (
	"github.com/kardianos/service"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/backends"
)

type Runner interface {
	Name() string
	Running() bool
	BindToService(service.Service)
	GetService() service.Service
	ValidateBeforeStart() error
	Start(service.Service) error
	Stop(service.Service) error
	Restart(service.Service) error
	SetDaemon(*DaemonConfig)
}

type RunnerCommon struct {
	name           string
	context        *context.Ctx
	backend        backends.Backend
	daemon         *DaemonConfig
	isRunning      bool
}