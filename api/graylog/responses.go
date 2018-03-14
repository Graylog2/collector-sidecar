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

package graylog

type ResponseCollectorRegistration struct {
	Configuration         ResponseCollectorRegistrationConfiguration `json:"configuration"`
	ConfigurationOverride bool                                       `json:"configuration_override"`
	CollectorActions      []ResponseCollectorAction                  `json:"actions,omitempty"`
	Assignments           []ResponseConfigurationRelation            `json:"assignments,omitempty"`
}

type ResponseCollectorAction struct {
	Backend    string                 `json:"backend"`
	Properties map[string]interface{} `json:"properties"`
}

type ResponseConfigurationRelation struct {
	BackendId       string `json:"backend_id"`
	ConfigurationId string `json:"configuration_id"`
}

type ResponseCollectorRegistrationConfiguration struct {
	UpdateInterval int  `json:"update_interval"`
	SendStatus     bool `json:"send_status"`
}

type ResponseBackendList struct {
	Backends []ResponseCollectorBackend `json:"backends"`
	Checksum string                     //Etag of the response
}

func (r *ResponseBackendList) IsEmpty() bool {
	if len(r.Backends) == 0 {
		return true
	}
	return false
}

type ResponseCollectorBackend struct {
	Id                string   `json:"id"`
	Name              string   `json:"name"`
	ServiceType       string   `json:"service_type"`
	OperatingSystem   string   `json:"node_operating_system"`
	ExecutablePath    string   `json:"executable_path"`
	ConfigurationPath string   `json:"configuration_path"`
	ExecuteParameters []string `json:"execute_parameters"`
	ValidationCommand string   `json:"validation_command"`
}

type ResponseCollectorConfiguration struct {
	Inputs   []ResponseCollectorInput   `json:"inputs"`
	Outputs  []ResponseCollectorOutput  `json:"outputs"`
	Snippets []ResponseCollectorSnippet `json:"snippets"`
	Checksum string                     //Etag of the response
}

func (r *ResponseCollectorConfiguration) IsEmpty() bool {
	if len(r.Inputs)+len(r.Outputs)+len(r.Snippets) == 0 {
		return true
	}
	return false
}

type ResponseCollectorInput struct {
	Id         string                 `json:"input_id"`
	Backend    string                 `json:"backend"`
	Type       string                 `json:"type"`
	Name       string                 `json:"name"`
	Properties map[string]interface{} `json:"properties"`
	ForwardTo  string                 `json:"forward_to"`
}

type ResponseCollectorOutput struct {
	Id         string                 `json:"output_id"`
	Backend    string                 `json:"backend"`
	Type       string                 `json:"type"`
	Name       string                 `json:"name"`
	Properties map[string]interface{} `json:"properties"`
}

type ResponseCollectorSnippet struct {
	Id      string `json:"snippet_id"`
	Backend string `json:"backend"`
	Name    string `json:"name"`
	Value   string `json:"snippet"`
}
