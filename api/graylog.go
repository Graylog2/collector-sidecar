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

package api

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"strconv"

	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/api/rest"
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/daemon"
	"github.com/Graylog2/collector-sidecar/system"
)

var log = common.Log()

func RequestConfiguration(context *context.Ctx) (graylog.ResponseCollectorConfiguration, error) {
	c := rest.NewClient(nil, getTlsConfig(context))
	c.BaseURL = context.ServerUrl

	params := make(map[string]string)
	if len(context.UserConfig.Tags) != 0 {
		tags, err := json.Marshal(context.UserConfig.Tags)
		if err != nil {
			msg := "Provided tags can not be send to the Graylog server!"
			system.GlobalStatus.Set(backends.StatusUnknown, msg)
			log.Errorf("[RequestConfiguration] %s", msg)
		} else {
			params["tags"] = string(tags)
		}
	}

	r, err := c.NewRequest("GET", "/plugins/org.graylog.plugins.collector/"+context.CollectorId, params, nil)
	if err != nil {
		msg := "Can not initialize REST request"
		system.GlobalStatus.Set(backends.StatusError, msg)
		log.Errorf("[RequestConfiguration] %s", msg)
		return graylog.ResponseCollectorConfiguration{}, err
	}

	respBody := graylog.ResponseCollectorConfiguration{}
	resp, err := c.Do(r, &respBody)
	if resp != nil && resp.StatusCode == 204 {
		msg := "No configuration found for configured tags!"
		system.GlobalStatus.Set(backends.StatusError, msg)
		log.Infof("[RequestConfiguration] %s", msg)
		return graylog.ResponseCollectorConfiguration{}, nil
	} else if resp != nil && resp.StatusCode != 200 {
		msg := "Bad response status from Graylog server"
		system.GlobalStatus.Set(backends.StatusError, msg)
		log.Errorf("[RequestConfiguration] %s: ", msg, resp.Status)
		return graylog.ResponseCollectorConfiguration{}, nil
	} else if err != nil {
		msg := "Fetching configuration failed"
		system.GlobalStatus.Set(backends.StatusError, msg)
		log.Errorf("[RequestConfiguration] %s: ", msg, err)
	}

	system.GlobalStatus.Set(backends.StatusRunning, "")
	return respBody, err
}

func UpdateRegistration(context *context.Ctx, status *graylog.StatusRequest) {
	c := rest.NewClient(nil, getTlsConfig(context))
	c.BaseURL = context.ServerUrl

	metrics := &graylog.MetricsRequest{
		Disks75: common.GetFileSystemList75(),
		CpuIdle: common.GetCpuIdle(),
		Load1:   common.GetLoad1(),
	}

	registration := graylog.RegistrationRequest{}

	registration.NodeId = context.UserConfig.NodeId
	registration.NodeDetails.OperatingSystem = common.GetSystemName()
	if context.UserConfig.SendStatus {
		registration.NodeDetails.Tags = context.UserConfig.Tags
		registration.NodeDetails.Status = status
		registration.NodeDetails.Metrics = metrics
		if len(context.UserConfig.ListLogFiles) > 0 {
			registration.NodeDetails.LogFileList = common.ListFiles(context.UserConfig.ListLogFiles)
		}
	}

	r, err := c.NewRequest("PUT", "/plugins/org.graylog.plugins.collector/collectors/"+context.CollectorId, nil, registration)
	if err != nil {
		log.Error("[UpdateRegistration] Can not initialize REST request")
		return
	}

	respBody := new(graylog.ResponseCollectorRegistration)
	resp, err := c.Do(r, &respBody)
	if err == nil && resp.StatusCode != 202 {
		log.Error("[UpdateRegistration] Bad response from Graylog server: ", resp.Status)
	} else if err != io.EOF { // we expect an empty answer
		log.Error("[UpdateRegistration] Failed to report collector status to server: ", err)
	}
}

func getTlsConfig(context *context.Ctx) *tls.Config {
	var tlsConfig *tls.Config
	if context.UserConfig.TlsSkipVerify {
		tlsConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return tlsConfig
}

func NewStatusRequest() graylog.StatusRequest {
	statusRequest := graylog.StatusRequest{Backends: make(map[string]system.Status)}
	combined, count := system.GlobalStatus.Status, 0
	for name := range daemon.Daemon.Runner {
		backend := backends.Store.GetBackend(name)
		statusRequest.Backends[name] = backend.Status()
		if backend.Status().Status > combined {
			combined = backend.Status().Status
		}
		count++
	}

	if combined != backends.StatusRunning {
		statusRequest.Status = combined
		if len(system.GlobalStatus.Message) != 0 {
			statusRequest.Message = system.GlobalStatus.Message
		} else {
			statusRequest.Message = "At least one backend with errors"
		}
	} else {
		statusRequest.Status = system.GlobalStatus.Status
		statusRequest.Message = strconv.Itoa(count) + " collectors running"
	}

	return statusRequest
}
