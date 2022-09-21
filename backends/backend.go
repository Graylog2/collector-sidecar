// Copyright (C) 2020 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.

package backends

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
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
	ConfigId             string
	CollectorId          string
	Name                 string
	ServiceType          string
	OperatingSystem      string
	ExecutablePath       string
	ConfigurationPath    string
	ExecuteParameters    string
	ValidationParameters string
	Template             string
	backendStatus        system.VerboseStatus
}

func BackendFromResponse(response graylog.ResponseCollectorBackend, configId string, ctx *context.Ctx) *Backend {
	return &Backend{
		Enabled:              common.NewTrue(),
		Id:                   response.Id + "-" + configId,
		CollectorId:          response.Id,
		ConfigId:             configId,
		Name:                 response.Name + "-" + configId,
		ServiceType:          response.ServiceType,
		OperatingSystem:      response.OperatingSystem,
		ExecutablePath:       response.ExecutablePath,
		ConfigurationPath:    BuildConfigurationPath(response, configId, ctx),
		ExecuteParameters:    response.ExecuteParameters,
		ValidationParameters: response.ValidationParameters,
		backendStatus:        system.VerboseStatus{},
	}
}

func BuildConfigurationPath(response graylog.ResponseCollectorBackend, configId string, ctx *context.Ctx) string {
	return filepath.Join(ctx.UserConfig.CollectorConfigurationDirectory, configId, response.Name+".conf")
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
		ConfigId:             a.ConfigId,
		CollectorId:          a.CollectorId,
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

func (b *Backend) CheckExecutableAgainstAccesslist(context *context.Ctx) error {
	if len(context.UserConfig.CollectorBinariesAccesslist) <= 0 {
		return nil
	}
	isListed, err := common.PathMatch(b.ExecutablePath, context.UserConfig.CollectorBinariesAccesslist)
	if err != nil {
		return fmt.Errorf("Can not validate binary path: %s", err)
	}
	if !isListed.Match {
		if isListed.IsLink {
			msg := "Couldn't execute collector %s [%s], binary path is not included in `collector_binaries_accesslist' config option."
			return fmt.Errorf(msg, isListed.Path, b.ExecutablePath)
		} else {
			msg := "Couldn't execute collector %s, binary path is not included in `collector_binaries_accesslist' config option."
			return fmt.Errorf(msg, isListed.Path)
		}
	}
	return nil
}

func (b *Backend) CheckConfigPathAgainstAccesslist(context *context.Ctx) bool {
	configuration, err := common.PathMatch(b.ConfigurationPath, context.UserConfig.CollectorBinariesAccesslist)
	if err != nil {
		log.Errorf("Can not validate configuration path: %s", err)
		return false
	}
	if configuration.Match {
		b.SetStatusLogErrorf("Collector configuration %s is in executable path, exclude it from `collector_binaries_accesslist' config option.", b.ConfigurationPath)
		return false
	}
	return true
}

func (b *Backend) ValidateConfigurationFile(context *context.Ctx) (error, string) {
	if b.ValidationParameters == "" {
		log.Warnf("[%s] Skipping configuration test. No validation command configured.", b.Name)
		return nil, ""
	}
	if err := b.CheckExecutableAgainstAccesslist(context); err != nil {
		return err, ""
	}

	var err error
	var quotedArgs []string
	if runtime.GOOS == "windows" {
		quotedArgs = common.CommandLineToArgv(b.ValidationParameters)
	} else {
		quotedArgs, err = shlex.Split(b.ValidationParameters)
	}
	if err != nil {
		err = fmt.Errorf("Error during configuration validation: %s", err)
		return err, ""
	}
	cmd := exec.Command(b.ExecutablePath, quotedArgs...)

	var combinedOutputBuffer bytes.Buffer
	cmd.Stdout = &combinedOutputBuffer
	cmd.Stderr = &combinedOutputBuffer

	if err := cmd.Start(); err != nil {
		err = fmt.Errorf("Couldn't start validation command: %s", err)
		return err, string(combinedOutputBuffer.Bytes())
	}

	done := make(chan error)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(context.UserConfig.CollectorValidationTimeout):
		if err := cmd.Process.Kill(); err != nil {
			err = fmt.Errorf("Failed to kill validation process: %s", err)
			return err, ""
		}
		return fmt.Errorf("Unable to validate configuration, timeout <%v> reached", context.UserConfig.CollectorValidationTimeout), ""
	case err := <-done:
		if err != nil {
			close(done)
			return fmt.Errorf("Collector configuration file is not valid, waiting for the next update."),
				string(combinedOutputBuffer.Bytes())
		}
		return nil, ""
	}
}
