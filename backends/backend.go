package backends

import (
	"os/exec"
	"reflect"

	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/system"
)

type Backend struct {
	Enabled           *bool
	Id                string
	Name              string
	ServiceType       string
	OperatingSystem   string
	ExecutablePath    string
	ConfigurationPath string
	ExecuteParameters []string
	ValidationCommand string
	Template          string
	backendStatus     system.Status
}

func BackendFromResponse(response graylog.ResponseCollectorBackend) *Backend {
	return &Backend{
		Enabled:           common.NewTrue(),
		Id:                response.Id,
		Name:              response.Name,
		ServiceType:       response.ServiceType,
		OperatingSystem:   response.OperatingSystem,
		ExecutablePath:    response.ExecutablePath,
		ConfigurationPath: response.ConfigurationPath,
		ExecuteParameters: response.ExecuteParameters,
		ValidationCommand: response.ValidationCommand,
		backendStatus:     system.Status{},
	}
}

func (b *Backend) Equals(a *Backend) bool {
	return reflect.DeepEqual(a, b)
}

func (b *Backend) ValidatePreconditions() bool {
	return true
}

func (b *Backend) ValidateConfigurationFile() bool {
	output, err := exec.Command(b.ValidationCommand).CombinedOutput()
	soutput := string(output)
	if err != nil {
		log.Errorf("[%s] Error during configuration validation: %s", b.Name, soutput)
		return false
	}

	return true
}
