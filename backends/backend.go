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

func (b *Backend) EqualSettings(a *Backend) bool {
	executeParameters, _ := common.SprintfList(
		a.ExecuteParameters,
		a.ConfigurationPath)
	validationParameters, _ := common.SprintfList(
		a.ValidationParameters,
		a.ConfigurationPath)

	aBackend := &Backend{
		Enabled:              b.Enabled,
		Id:                   a.Id,
		Name:                 a.Name,
		ServiceType:          a.ServiceType,
		OperatingSystem:      a.OperatingSystem,
		ExecutablePath:       a.ExecutablePath,
		ConfigurationPath:    a.ConfigurationPath,
		ExecuteParameters:    executeParameters,
		ValidationParameters: validationParameters,
		Template:             b.Template,
		backendStatus:        b.Status(),
	}

	return b.Equals(aBackend)
}

func (b *Backend) ValidatePreconditions() bool {
	return true
}

func (b *Backend) ValidateConfigurationFile() (bool, string) {
	if b.ValidationParameters == nil {
		log.Errorf("[%s] No parameters configured to validate configuration!", b.Name)
		return false, ""
	}

	output, err := exec.Command(b.ExecutablePath, b.ValidationParameters...).CombinedOutput()
	if err != nil {
		soutput := string(output)
		log.Errorf("[%s] Error during configuration validation: %s %s", b.Name, soutput, err)
		return false, soutput
	}

	return true, ""
}
