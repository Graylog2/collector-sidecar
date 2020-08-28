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
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/api/rest"
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/daemon"
	"github.com/Graylog2/collector-sidecar/logger"
	"github.com/Graylog2/collector-sidecar/system"
)

var (
	log                   = logger.Log()
	configurationOverride = false
)

func RequestBackendList(httpClient *http.Client, checksum string, ctx *context.Ctx) (graylog.ResponseBackendList, error) {
	c := rest.NewClient(httpClient, ctx)
	c.BaseURL = ctx.ServerUrl

	r, err := c.NewRequest("GET", "/sidecar/collectors", nil, nil)
	if err != nil {
		msg := "Can not initialize REST request"
		system.GlobalStatus.Set(backends.StatusError, msg)
		log.Errorf("[RequestCollectorList] %s", msg)
		return graylog.ResponseBackendList{}, err
	}

	if checksum != "" {
		r.Header.Add("If-None-Match", "\""+checksum+"\"")
	}

	backendResponse := graylog.ResponseBackendList{}
	resp, err := c.Do(r, &backendResponse)
	if err != nil && resp == nil {
		msg := "Fetching backend list"
		system.GlobalStatus.Set(backends.StatusError, msg)
		log.Errorf("[RequestBackendList] %s: %v", msg, err)
		return graylog.ResponseBackendList{}, err
	}

	if resp != nil {
		// preserve Etag as checksum for the next request. Empty string if header is not available
		backendResponse.Checksum = resp.Header.Get("Etag")
		switch {
		case resp.StatusCode == 304:
			backendResponse.NotModified = true
			log.Debug("[RequestBackendList] No update available.")
		case resp.StatusCode != 200:
			msg := "Bad response status from Graylog server"
			system.GlobalStatus.Set(backends.StatusError, msg)
			log.Errorf("[RequestBackendList] %s: %s", msg, resp.Status)
			return graylog.ResponseBackendList{}, err
		}
	}

	system.GlobalStatus.Set(backends.StatusRunning, "")
	return backendResponse, nil
}

func RequestConfiguration(
	httpClient *http.Client,
	configurationId string,
	checksum string,
	ctx *context.Ctx) (graylog.ResponseCollectorConfiguration, error) {
	c := rest.NewClient(httpClient, ctx)
	c.BaseURL = ctx.ServerUrl

	r, err := c.NewRequest("GET", "/sidecar/configurations/render/"+ctx.NodeId+"/"+configurationId, nil, nil)
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
		system.GlobalStatus.Set(backends.StatusError, msg+": "+err.Error())
		log.Errorf("[RequestConfiguration] %s: %v", msg, err)
	}

	if resp != nil {
		// preserve Etag as checksum for the next request. Empty string if header is not available
		configurationResponse.Checksum = resp.Header.Get("Etag")
		switch {
		case resp.StatusCode == 204:
			msg := "No configuration found!"
			system.GlobalStatus.Set(backends.StatusError, msg)
			log.Infof("[RequestConfiguration] %s", msg)
			return graylog.ResponseCollectorConfiguration{}, nil
		case resp.StatusCode == 304:
			log.Debug("[RequestConfiguration] No update available, skipping update.")
			configurationResponse.NotModified = true
		case resp.StatusCode != 200:
			msg := "Bad response status from Graylog server"
			system.GlobalStatus.Set(backends.StatusError, msg+": "+err.Error())
			log.Errorf("[RequestConfiguration] %s: %s", msg, resp.Status)
			return graylog.ResponseCollectorConfiguration{}, err
		}
	}

	system.GlobalStatus.Set(backends.StatusRunning, "")
	return configurationResponse, nil
}

func isUrlEOFError(err error) bool {
	if errVal, _ := err.(*url.Error); errVal.Err.Error() == "EOF" {
		return true
	}

	return false
}

func UpdateRegistration(httpClient *http.Client, checksum string, ctx *context.Ctx, status *graylog.StatusRequest) (graylog.ResponseCollectorRegistration, error) {
	c := rest.NewClient(httpClient, ctx)
	c.BaseURL = ctx.ServerUrl

	registration := graylog.RegistrationRequest{}

	registration.NodeName = ctx.UserConfig.NodeName
	registration.NodeDetails.OperatingSystem = common.GetSystemName()
	if ctx.UserConfig.SendStatus {
		metrics := &graylog.MetricsRequest{
			Disks75: common.GetFileSystemList75(),
			CpuIdle: common.GetCpuIdle(),
			Load1:   common.GetLoad1(),
		}
		registration.NodeDetails.IP = common.GetHostIP()
		registration.NodeDetails.Status = status
		registration.NodeDetails.Metrics = metrics
		if len(ctx.UserConfig.ListLogFiles) > 0 {
			fileList := common.ListFiles(ctx.UserConfig.ListLogFiles)
			buf := new(bytes.Buffer)
			if fileList != nil {
				json.NewEncoder(buf).Encode(fileList)
			}
			fileListSize := buf.Len()
			// Maximum MongoDB document size is 16793600 bytes so we leave some extra space for the rest of the request
			// before we skip to send the file list.
			if fileListSize < 10000000 {
				registration.NodeDetails.LogFileList = fileList
			} else {
				log.Warn("[UpdateRegistration] Maximum file list size exceeded, skip sending list of active log files!" +
					" Adjust list_log_file setting.")
			}
		}
	}

	r, err := c.NewRequest("PUT", "/sidecars/"+ctx.NodeId, nil, registration)
	if checksum != "" {
		r.Header.Add("If-None-Match", "\""+checksum+"\"")
	}
	if err != nil {
		log.Error("[UpdateRegistration] Can not initialize REST request")
		return graylog.ResponseCollectorRegistration{}, err
	}

	respBody := new(graylog.ResponseCollectorRegistration)
	resp, err := c.Do(r, &respBody)
	if resp != nil && resp.StatusCode == 400 && strings.Contains(err.Error(), "Unable to map property") {
		log.Error("[UpdateRegistration] Sending collector status failed. Disabling `send_status` as fallback! ", err)
		ctx.UserConfig.SendStatus = false
	} else if resp != nil && resp.StatusCode == 304 {
		log.Debug("[UpdateRegistration] No update available.")
		respBody.NotModified = true
	} else if resp != nil && resp.StatusCode != 202 {
		log.Error("[UpdateRegistration] Bad response from Graylog server: ", resp.Status)
		return graylog.ResponseCollectorRegistration{}, err
	} else if err != nil && err != io.EOF { // err is nil for GL 2.2 and EOF for 2.1 and earlier
		if !isUrlEOFError(err) {
			log.Error("[UpdateRegistration] Failed to report collector status to server: ", err)
		} else {
			log.Debug("[UpdateRegistration] Received EOF from server: ", err)
		}
		return graylog.ResponseCollectorRegistration{}, err
	}
	respBody.Checksum = resp.Header.Get("Etag")

	// Update configuration if provided
	if respBody.Configuration != (graylog.ResponseCollectorRegistrationConfiguration{}) {
		updateRuntimeConfiguration(respBody, ctx)
	}

	// Run collector actions if provided
	if len(respBody.CollectorActions) != 0 {
		daemon.HandleCollectorActions(respBody.CollectorActions)
	}

	return *respBody, nil
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
	statusRequest := graylog.StatusRequest{Backends: make([]graylog.StatusRequestBackend, 0)}
	combinedStatus := backends.StatusUnknown
	runningCount, stoppedCount, errorCount := 0, 0, 0

	for id, runner := range daemon.Daemon.Runner {
		backendStatus := runner.GetBackend().Status()
		statusRequest.Backends = append(statusRequest.Backends, graylog.StatusRequestBackend{
			Id:             id,
			Status:         backendStatus.Status,
			Message:        backendStatus.Message,
			VerboseMessage: backendStatus.VerboseMessage,
		})
		switch backendStatus.Status {
		case backends.StatusRunning:
			runningCount++
		case backends.StatusStopped:
			stoppedCount++
		case backends.StatusError:
			errorCount++
		}
	}

	switch {
	default:
		combinedStatus = backends.StatusRunning
	case stoppedCount != 0:
		combinedStatus = backends.StatusStopped
		fallthrough
	case errorCount != 0:
		combinedStatus = backends.StatusError
	}

	statusMessage := strconv.Itoa(runningCount) + " running / " +
		strconv.Itoa(stoppedCount) + " stopped / " +
		strconv.Itoa(errorCount) + " failing"

	if combinedStatus != backends.StatusRunning {
		statusRequest.Status = combinedStatus
		statusRequest.Message = statusMessage
	} else {
		statusRequest.Status = system.GlobalStatus.Status
		if len(system.GlobalStatus.Message) != 0 {
			statusRequest.Message = system.GlobalStatus.Message
		} else {
			statusRequest.Message = statusMessage
		}
	}

	return statusRequest
}
