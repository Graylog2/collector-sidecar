package context

import (
	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/common"
)

type BackendDefinition struct {
	Enabled           *bool
	Id                string
	Name              string
	ServiceType       string
	OperatingSystem   string
	ExecutablePath    string
	ConfigurationPath string
	ExecuteParameters []string
	ValidationCommand string
}

func BackendFromResponse(response graylog.ResponseCollectorBackend) *BackendDefinition {
	return &BackendDefinition{
		Enabled:           common.NewTrue(),
		Id:                response.Id,
		Name:              response.Name,
		ServiceType:       response.ServiceType,
		OperatingSystem:   response.OperatingSystem,
		ExecutablePath:    response.ExecutablePath,
		ConfigurationPath: response.ConfigurationPath,
		ExecuteParameters: response.ExecuteParameters,
		ValidationCommand: response.ValidationCommand,
	}
}
