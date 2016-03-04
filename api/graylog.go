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
	"net/http"
	"net/url"

	"gopkg.in/jmcvetta/napping.v3"

	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/util"
)

var log = util.Log()

func RequestConfiguration(context *context.Ctx) (graylog.ResponseCollectorConfiguration, error) {
	s := napping.Session{}
	api := context.ServerUrl.String() + "/plugins/org.graylog.plugins.collector/" + context.CollectorId
	res := graylog.ResponseCollectorConfiguration{}
	params := &url.Values{}

	if len(context.Tags) != 0 {
		tags, err := json.Marshal(context.Tags)
		if err != nil {
			log.Error("Provided tags can not be send to Graylog server!")
		} else {
			params.Add("tags", string(tags))
		}
	} else {
		params = nil
	}

	resp, err := s.Get(api, params, &res, nil)
	if err == nil && resp.Status() == 204 {
		log.Info("[RequestConfiguration] No configuration found for this collector!")
	} else if err == nil && resp.Status() != 200 {
		log.Error("[RequestConfiguration] Bad response status from Graylog server: ", resp.Status())
	}
	if err != nil {
		log.Error("[RequestConfiguration] Fetching configuration failed: ", err)
	}

	return res, err
}

func UpdateRegistration(context *context.Ctx) {
	s := napping.Session{}

	registration := graylog.RegistrationRequest{}
	registration.NodeId = context.NodeId
	registration.NodeDetails = make(map[string]string)
	registration.NodeDetails["operating_system"] = util.GetSystemName()

	h := http.Header{}
	h.Add("User-Agent", "Graylog Collector v"+util.CollectorVersion)
	h.Add("X-Graylog-Collector-Version", util.CollectorVersion)

	r := napping.Request{
		Url:     context.ServerUrl.String() + "/plugins/org.graylog.plugins.collector/collectors/" + context.CollectorId,
		Method:  "PUT",
		Payload: registration,
		Header:  &h,
	}

	resp, err := s.Send(&r)
	if err == nil && resp.Status() != 202 {
		log.Error("[UpdateRegistration] Bad response from Graylog server: ", resp.Status())
	} else if err != nil {
		log.Error("[UpdateRegistration] Failed to report collector status to server: ", err)
	}
}
