package context

import (
	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/common"
)

type BackendDefinition struct {
	Id                string
	Name              string
	ServiceType       string
	OperatingSystem   string
	Enabled           *bool
	BinaryPath        string
	ConfigurationPath string
	RunPath           string
}

func BackendFromResponse(response graylog.ResponseCollectorBackend) *BackendDefinition {
	return &BackendDefinition{
		Id:                response.Id,
		Name:              response.Name,
		ServiceType:       response.ServiceType,
		OperatingSystem:   response.OperatingSystem,
		Enabled:           common.NewTrue(),
		BinaryPath:        "/usr/bin/collector",
		ConfigurationPath: "/etc/graylog/collector-sidecar/config",
		RunPath:           "/var/run/collector/run",
	}
}
