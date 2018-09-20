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
	"bytes"
	"fmt"
	"os/exec"
	"reflect"
	"runtime"
	"time"

	"github.com/flynn-archive/go-shlex"

	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/system"
)

type Backend struct {
	Enabled              *bool
	Id                   string
	Name                 string
	ServiceType          string
	OperatingSystem      string
	ExecutablePath       string
	ConfigurationPath    string
	ExecuteParameters    string
	ValidationParameters string
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
	executeParameters, _ := common.Sprintf(
		a.ExecuteParameters,
		a.ConfigurationPath)
	validationParameters, _ := common.Sprintf(
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

func (b *Backend) CheckExecutableAgainstWhitelist(context *context.Ctx) error {
	if len(*context.UserConfig.CollectorBinariesWhitelist) <= 0 {
		return nil
	}
	whitelisted, err := common.PathMatch(b.ExecutablePath, *context.UserConfig.CollectorBinariesWhitelist)
	if err != nil {
		return fmt.Errorf("Can not validate binary path: %s", err)
	}
	if !whitelisted.Match {
		if whitelisted.IsLink {
			msg := "Couldn't execute collector %s [%s], binary path is not included in `collector_binaries_whitelist' config option."
			return fmt.Errorf(msg, whitelisted.Path, b.ExecutablePath)
		} else {
			msg := "Couldn't execute collector %s, binary path is not included in `collector_binaries_whitelist' config option."
			return fmt.Errorf(msg, whitelisted.Path)
		}
	}
	return nil
}

func (b *Backend) CheckConfigPathAgainstWhitelist(context *context.Ctx) bool {
	configuration, err := common.PathMatch(b.ConfigurationPath, *context.UserConfig.CollectorBinariesWhitelist)
	if err != nil {
		log.Errorf("Can not validate configuration path: %s", err)
		return false
	}
	if configuration.Match {
		b.SetStatusLogErrorf("Collector configuration %s is in executable path, exclude it from `collector_binaries_whitelist' config option.", b.ConfigurationPath)
		return false
	}
	return true
}

func (b *Backend) ValidateConfigurationFile(context *context.Ctx) (bool, string) {
	if b.ValidationParameters == "" {
		log.Warnf("[%s] Skipping configuration test. No validation command configured.", b.Name)
		return true, ""
	}
	if err := b.CheckExecutableAgainstWhitelist(context); err != nil {
		return false, err.Error()
	}

	var err error
	var quotedArgs []string
	if runtime.GOOS == "windows" {
		quotedArgs = common.CommandLineToArgv(b.ValidationParameters)
	} else {
		quotedArgs, err = shlex.Split(b.ValidationParameters)
	}
	if err != nil {
		log.Errorf("[%s] Error during configuration validation: %s", b.Name, err)
		return false, err.Error()
	}
	cmd := exec.Command(b.ExecutablePath, quotedArgs...)

	var combinedOutputBuffer bytes.Buffer
	cmd.Stdout = &combinedOutputBuffer
	cmd.Stderr = &combinedOutputBuffer

	if err := cmd.Start(); err != nil {
		log.Errorf("[%s] Couldn't start validation command: %s %s", b.Name, string(combinedOutputBuffer.Bytes()), err)
		return false, string(combinedOutputBuffer.Bytes())
	}

	done := make(chan error)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(time.Duration(30) * time.Second):
		if err := cmd.Process.Kill(); err != nil {
			log.Errorf("[%s] Failed to kill validation process: %s", b.Name, err)
			return false, err.Error()
		}
		log.Errorf("[%s] Timeout reached for validation command.", b.Name)
		return false, "Unable to validate configuration, timeout reached."
	case err := <-done:
		if err != nil {
			close(done)
			log.Errorf("[%s] Error during configuration validation: %s %s", b.Name, string(combinedOutputBuffer.Bytes()), err)
			return false, string(combinedOutputBuffer.Bytes())
		}
		return true, ""
	}
}
