package backends

import (
	"reflect"

	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/system"
	"os/exec"
)

type Backend struct {
	Enabled              *bool
	Id                   string
	Name                 string
	ServiceType          string
	OperatingSystem      string
	ExecutablePath       string
	ConfigurationPath    string
	ExecuteParameters    []string
	ValidationParameters []string
	Template             string
	backendStatus        system.Status
}

func BackendFromResponse(response graylog.ResponseCollectorBackend) *Backend {
	return &Backend{
		Enabled:              common.NewTrue(),
		Id:                   response.Id,
		Name:                 response.Name,
		ServiceType:          response.ServiceType,
		OperatingSystem:      response.OperatingSystem,
		ExecutablePath:       response.ExecutablePath,
		ConfigurationPath:    response.ConfigurationPath,
		ExecuteParameters:    response.ExecuteParameters,
		ValidationParameters: response.ValidationParameters,
		backendStatus:        system.Status{},
	}
}

func (b *Backend) Equals(a *Backend) bool {
	return reflect.DeepEqual(a, b)
}

func (b *Backend) ValidatePreconditions() bool {
	return true
}

func (b *Backend) ValidateConfigurationFile() bool {
	if b.ValidationParameters == nil {
		log.Errorf("[%s] No parameters configured to validate configuration!", b.Name)
		return false
	}

	parameters, err := common.SprintfList(b.ValidationParameters, b.ConfigurationPath)
	if err != nil {
		log.Error("[%s] Validation parameters can't be parsed: %s", b.Name, b.ValidationParameters)
		return false
	}
	output, err := exec.Command(b.ExecutablePath, parameters...).CombinedOutput()
	if err != nil {
		soutput := string(output)
		log.Errorf("[%s] Error during configuration validation: %s %s", b.Name, soutput, err)
		return false
	}

	return true
}
