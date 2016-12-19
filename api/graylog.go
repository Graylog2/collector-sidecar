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
	"net/http"
	"strconv"
	"strings"

	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/api/rest"
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/daemon"
	"github.com/Graylog2/collector-sidecar/system"
	"github.com/Graylog2/collector-sidecar/logger"
)

var (
	log                   = logger.Log()
	configurationOverride = false
)

func RequestConfiguration(httpClient *http.Client, checksum string, ctx *context.Ctx) (graylog.ResponseCollectorConfiguration, error) {
	c := rest.NewClient(httpClient)
	c.BaseURL = ctx.ServerUrl

	params := make(map[string]string)
	if len(ctx.UserConfig.Tags) != 0 {
		tags, err := json.Marshal(ctx.UserConfig.Tags)
		if err != nil {
			msg := "Provided tags can not be send to the Graylog server!"
			system.GlobalStatus.Set(backends.StatusUnknown, msg)
			log.Errorf("[RequestConfiguration] %s", msg)
		} else {
			params["tags"] = string(tags)
		}
	}

	r, err := c.NewRequest("GET", "/plugins/org.graylog.plugins.collector/"+ctx.CollectorId, params, nil)
	if err != nil {
		msg := "Can not initialize REST request"
		system.GlobalStatus.Set(backends.StatusError, msg)
		log.Errorf("[RequestConfiguration] %s", msg)
		return graylog.ResponseCollectorConfiguration{}, err
	}

	if checksum != "" {
		r.Header.Add("If-None-Match", "\""+checksum+"\"")
	}

	configurationResponse := graylog.ResponseCollectorConfiguration{}
	resp, err := c.Do(r, &configurationResponse)
	if err != nil && resp == nil {
		msg := "Fetching configuration failed"
		system.GlobalStatus.Set(backends.StatusError, msg)
		log.Errorf("[RequestConfiguration] %s: %v", msg, err)
	}

	if resp != nil {
		// preserver Etag as checksum for the next request. Empty string if header is not available
		configurationResponse.Checksum = resp.Header.Get("Etag")
		switch {
		case resp.StatusCode == 204:
			msg := "No configuration found for configured tags!"
			system.GlobalStatus.Set(backends.StatusError, msg)
			log.Infof("[RequestConfiguration] %s", msg)
			return graylog.ResponseCollectorConfiguration{}, nil
		case resp.StatusCode == 304:
			log.Debug("[RequestConfiguration] No configuration update available, skipping update.")
		case resp.StatusCode != 200:
			msg := "Bad response status from Graylog server"
			system.GlobalStatus.Set(backends.StatusError, msg)
			log.Errorf("[RequestConfiguration] %s: %s", msg, resp.Status)
			return graylog.ResponseCollectorConfiguration{}, err
		}
	}

	system.GlobalStatus.Set(backends.StatusRunning, "")
	return configurationResponse, nil
}

func UpdateRegistration(httpClient *http.Client, ctx *context.Ctx, status *graylog.StatusRequest) {
	c := rest.NewClient(httpClient)
	c.BaseURL = ctx.ServerUrl

	metrics := &graylog.MetricsRequest{
		Disks75: common.GetFileSystemList75(),
		CpuIdle: common.GetCpuIdle(),
		Load1:   common.GetLoad1(),
	}

	registration := graylog.RegistrationRequest{}

	registration.NodeId = ctx.UserConfig.NodeId
	registration.NodeDetails.OperatingSystem = common.GetSystemName()
	if ctx.UserConfig.SendStatus {
		registration.NodeDetails.Tags = ctx.UserConfig.Tags
		registration.NodeDetails.IP = common.GetHostIP()
		registration.NodeDetails.Status = status
		registration.NodeDetails.Metrics = metrics
		if len(ctx.UserConfig.ListLogFiles) > 0 {
			registration.NodeDetails.LogFileList = common.ListFiles(ctx.UserConfig.ListLogFiles)
		}
	}

	r, err := c.NewRequest("PUT", "/plugins/org.graylog.plugins.collector/collectors/"+ctx.CollectorId, nil, registration)
	if err != nil {
		log.Error("[UpdateRegistration] Can not initialize REST request")
		return
	}

	respBody := new(graylog.ResponseCollectorRegistration)
	resp, err := c.Do(r, &respBody)
	if resp != nil && resp.StatusCode == 400 && strings.Contains(err.Error(), "Unable to map property") {
		log.Error("[UpdateRegistration] Sending collector status failed. Disabling `send_status` as fallback! ", err)
		ctx.UserConfig.SendStatus = false
	} else if resp != nil && resp.StatusCode != 202 {
		log.Error("[UpdateRegistration] Bad response from Graylog server: ", resp.Status)
	} else if err != nil && err != io.EOF { // err is nil for GL 2.2 and EOF for 2.1 and earlier
		log.Error("[UpdateRegistration] Failed to report collector status to server: ", err)
	}

	// Update configuration if provided
	if respBody.Configuration != (graylog.ResponseCollectorRegistrationConfiguration{}) {
		updateRuntimeConfiguration(respBody, ctx)
	}

	// Run collector actions if provided
	if len(respBody.CollectorActions) != 0 {
		daemon.HandleCollectorActions(respBody.CollectorActions)
	}
}

func updateRuntimeConfiguration(respBody *graylog.ResponseCollectorRegistration, ctx *context.Ctx) error {
	// API query interval
	if ctx.UserConfig.UpdateInterval != respBody.Configuration.UpdateInterval &&
		respBody.Configuration.UpdateInterval > 0 &&
		respBody.ConfigurationOverride == true {
		log.Infof("[ConfigurationUpdate] update_interval: %ds", respBody.Configuration.UpdateInterval)
		ctx.UserConfig.UpdateInterval = respBody.Configuration.UpdateInterval
		configurationOverride = true
	}
	// Send host status
	if ctx.UserConfig.SendStatus != respBody.Configuration.SendStatus &&
		respBody.ConfigurationOverride == true {
		log.Infof("[ConfigurationUpdate] send_status: %v", respBody.Configuration.SendStatus)
		ctx.UserConfig.SendStatus = respBody.Configuration.SendStatus
		configurationOverride = true
	}
	// Reset server overrides
	if respBody.ConfigurationOverride == false && configurationOverride == true {
		configFile := cfgfile.SidecarConfig{}
		err := cfgfile.Read(&configFile, "")
		if err != nil {
			log.Errorf("[ConfigurationUpdate] Failed to load default values from configuration file. Continuing with current values. %v", err)
			return err
		} else {
			log.Infof("[ConfigurationUpdate] Resetting update_interval: %ds", configFile.UpdateInterval)
			ctx.UserConfig.UpdateInterval = configFile.UpdateInterval
			log.Infof("[ConfigurationUpdate] Resetting send_status: %v", configFile.SendStatus)
			ctx.UserConfig.SendStatus = configFile.SendStatus
			configurationOverride = false
		}
	}
	return nil
}

func GetTlsConfig(ctx *context.Ctx) *tls.Config {
	var tlsConfig *tls.Config
	if ctx.UserConfig.TlsSkipVerify {
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
