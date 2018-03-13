package generic

import (
	"path/filepath"

	"github.com/Graylog2/collector-sidecar/logger"
	"github.com/Graylog2/collector-sidecar/system"
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
)

const name = "generic"

var (
	log           = logger.Log()
	backendStatus = system.Status{}
)

func New(context *context.Ctx) backends.Backend {
	return NewCollectorConfig(context)
}

func (g *GenericConfig) Name() string {
	return name
}

func (g *GenericConfig) Driver() string {
	if validDriver(g.serviceType) {
		return g.serviceType
	}
	log.Errorf("[%s] Configured service type is invalid: %s", g.Name(), g.serviceType)
	return "exec"
}

func (g *GenericConfig) ExecPath() string {
	execPath := g.executablePath
	if common.FileExists(execPath) != nil {
		log.Fatalf("[%s] Configured path to collector binary does not exist: %s", g.Name(), execPath)
	}

	return execPath
}

func (g *GenericConfig) ConfigurationPath() string {
	configurationPath := g.configurationPath
	if !common.IsDir(filepath.Dir(configurationPath)) {
		err := common.CreatePathToFile(configurationPath)
		if err != nil {
			log.Fatalf("[%s] Configured path to collector configuration does not exist: %s", g.Name(), configurationPath)
		}
	}

	return configurationPath
}

func (g *GenericConfig) ExecArgs() []string {
	return g.executeParameters
}

func (g *GenericConfig) ValidatePreconditions() bool {
	return true
}

func (g *GenericConfig) SetStatus(state int, message string) {
	backendStatus.Set(state, message)
}

func (g *GenericConfig) Status() system.Status {
	return backendStatus
}

func validDriver(name string) bool {
	switch name {
	case
		"exec",
		"svc":
		return true
	}
	return false
}

