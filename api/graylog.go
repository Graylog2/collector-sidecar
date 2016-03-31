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
	"encoding/json"
	"io"

	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/api/rest"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
)

var log = common.Log()

func RequestConfiguration(context *context.Ctx) (graylog.ResponseCollectorConfiguration, error) {
	c := rest.NewClient(nil)
	c.BaseURL = context.ServerUrl

	params := make(map[string]string)
	if len(context.UserConfig.Tags) != 0 {
		tags, err := json.Marshal(context.UserConfig.Tags)
		if err != nil {
			log.Error("Provided tags can not be send to Graylog server!")
		} else {
			params["tags"] = string(tags)
		}
	}

	r, err := c.NewRequest("GET", "/plugins/org.graylog.plugins.collector/"+context.CollectorId, params, nil)
	if err != nil {
		log.Error("[RequestConfiguration] Can not initialize REST request")
		return graylog.ResponseCollectorConfiguration{}, err
	}

	respBody := graylog.ResponseCollectorConfiguration{}
	resp, err := c.Do(r, &respBody)
	if err == nil && resp.StatusCode == 204 {
		log.Info("[RequestConfiguration] No configuration found for this collector!")
	} else if err == nil && resp.StatusCode != 200 {
		log.Error("[RequestConfiguration] Bad response status from Graylog server: ", resp.Status)
	}
	if err != nil {
		log.Error("[RequestConfiguration] Fetching configuration failed: ", err)
	}

	return respBody, err
}

func UpdateRegistration(context *context.Ctx) {
	c := rest.NewClient(nil)
	c.BaseURL = context.ServerUrl

	registration := graylog.RegistrationRequest{}
	registration.NodeId = context.UserConfig.NodeId
	registration.NodeDetails = make(map[string]string)
	registration.NodeDetails["operating_system"] = common.GetSystemName()

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
