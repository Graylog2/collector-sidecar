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

package nxlog

import (
	"path/filepath"
	"reflect"

	"github.com/Graylog2/collector-sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"strconv"
)

type NxConfig struct {
	Context     *context.Ctx
	UserConfig  *cfgfile.SidecarBackend
	Definitions []nxdefinition
	Paths       []nxpath
	Extensions  []nxextension
	Inputs      []nxinput
	Outputs     []nxoutput
	Routes      []nxroute
	Matches     []nxmatch
	Snippets    []nxsnippet
	Canned      []nxcanned
}

type nxdefinition struct {
	name  string
	value string
}

type nxpath struct {
	name string
	path string
}

type nxextension struct {
	name       string
	properties map[string]string
}

type nxinput struct {
	name       string
	properties map[string]string
}

type nxoutput struct {
	name       string
	properties map[string]string
}

type nxroute struct {
	name       string
	properties map[string]string
}

type nxmatch struct {
	name       string
	properties map[string]string
}

type nxsnippet struct {
	name  string
	value string
}

type nxcanned struct {
	name       string
	kind       string
	properties map[string]interface{}
}

func NewCollectorConfig(context *context.Ctx) *NxConfig {
	nxc := &NxConfig{
		Context:    context,
		Extensions: []nxextension{{name: "gelf", properties: map[string]string{"Module": "xm_gelf"}}},
	}
	backendIndex, err := context.UserConfig.GetIndexByName(name)
	if err == nil {
		nxc.UserConfig = &context.UserConfig.Backends[backendIndex]
		nxc.Definitions = []nxdefinition{{name: "ROOT", value: filepath.Dir(context.UserConfig.Backends[backendIndex].BinaryPath)}}
	}
	return nxc
}

func (nxc *NxConfig) Add(class string, name string, value interface{}) {
	switch class {
	case "extension":
		addition := &nxextension{name: name, properties: value.(map[string]string)}
		nxc.Extensions = append(nxc.Extensions, *addition)
	case "input":
		addition := &nxinput{name: name, properties: value.(map[string]string)}
		nxc.Inputs = append(nxc.Inputs, *addition)
	case "output":
		addition := &nxoutput{name: name, properties: value.(map[string]string)}
		nxc.Outputs = append(nxc.Outputs, *addition)
	case "route":
		addition := &nxroute{name: name, properties: value.(map[string]string)}
		nxc.Routes = append(nxc.Routes, *addition)
	case "match":
		addition := &nxmatch{name: name, properties: value.(map[string]string)}
		nxc.Matches = append(nxc.Matches, *addition)
	case "snippet":
		addition := &nxsnippet{name: name, value: value.(string)}
		nxc.Snippets = append(nxc.Snippets, *addition)
	//pre-canned configuration types
	case "output-gelf-udp":
		addition := &nxcanned{name: name, kind: class, properties: value.(map[string]interface{})}
		nxc.Canned = append(nxc.Canned, *addition)
	case "output-gelf-tcp":
		addition := &nxcanned{name: name, kind: class, properties: value.(map[string]interface{})}
		nxc.Canned = append(nxc.Canned, *addition)
	case "output-gelf-tcp-tls":
		addition := &nxcanned{name: name, kind: class, properties: value.(map[string]interface{})}
		nxc.Canned = append(nxc.Canned, *addition)
	case "input-file":
		input_properties := value.(map[string]interface{})
		addition := &nxcanned{name: name, kind: class, properties: input_properties}
		nxc.Canned = append(nxc.Canned, *addition)

		multiline := nxc.isEnabled(input_properties["multiline"])
		var multilineStart string
		var multilineStop string
		if input_properties["multiline_start"] != nil {
			multilineStart = common.EncloseWith(input_properties["multiline_start"].(string), "/")
		}
		if input_properties["multiline_stop"] != nil {
			multilineStop = common.EncloseWith(input_properties["multiline_stop"].(string), "/")
		}
		if multiline {
			extension := &nxextension{name: name + "-multiline", properties: map[string]string{"Module": "xm_multiline"}}
			if len(multilineStart) > 0 {
				extension.properties["HeaderLine"] = multilineStart
			}
			if len(multilineStop) > 0 {
				extension.properties["EndLine"] = multilineStop
			}
			nxc.Extensions = append(nxc.Extensions, *extension)
		}
	case "input-windows-event-log":
		addition := &nxcanned{name: name, kind: class, properties: value.(map[string]interface{})}
		nxc.Canned = append(nxc.Canned, *addition)
	case "input-udp-syslog":
		addition := &nxcanned{name: name, kind: class, properties: value.(map[string]interface{})}
		nxc.Canned = append(nxc.Canned, *addition)
		if !nxc.Exists("extension", "syslog") {
			extension := &nxextension{name: "syslog", properties: map[string]string{"Module": "xm_syslog"}}
			nxc.Extensions = append(nxc.Extensions, *extension)
		}
	case "input-tcp-syslog":
		addition := &nxcanned{name: name, kind: class, properties: value.(map[string]interface{})}
		nxc.Canned = append(nxc.Canned, *addition)
		if !nxc.Exists("extension", "syslog") {
			extension := &nxextension{name: "syslog", properties: map[string]string{"Module": "xm_syslog"}}
			nxc.Extensions = append(nxc.Extensions, *extension)
		}
	}
}

func (nxc *NxConfig) Exists(class string, name string) bool {
	result := false
	switch class {
	case "extension":
		for _, entity := range nxc.Extensions {
			if entity.name == name {
				result = true
			}
		}
	case "input":
		for _, entity := range nxc.Inputs {
			if entity.name == name {
				result = true
			}
		}
	case "output":
		for _, entity := range nxc.Outputs {
			if entity.name == name {
				result = true
			}
		}
	case "route":
		for _, entity := range nxc.Routes {
			if entity.name == name {
				result = true
			}
		}
	case "match":
		for _, entity := range nxc.Matches {
			if entity.name == name {
				result = true
			}
		}
	case "snippet":
		for _, entity := range nxc.Snippets {
			if entity.name == name {
				result = true
			}
		}
	}
	return result
}

func (nxc *NxConfig) Update(a *NxConfig) {
	nxc.Definitions = a.Definitions
	nxc.Paths = a.Paths
	nxc.Extensions = a.Extensions
	nxc.Inputs = a.Inputs
	nxc.Outputs = a.Outputs
	nxc.Routes = a.Routes
	nxc.Matches = a.Matches
	nxc.Snippets = a.Snippets
	nxc.Canned = a.Canned
}

func (nxc *NxConfig) Equals(a *NxConfig) bool {
	return reflect.DeepEqual(nxc, a)
}

func (nxc *NxConfig) propertyString(p interface{}, precision int) string {
	switch t := p.(type) {
	default:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "True"
		} else {
			return "False"
		}
	case int:
		return strconv.Itoa(t)
	case float64:
		return strconv.FormatFloat(t, 'f', precision, 64)
	}

}

func (nxc *NxConfig) propertyStringMap(p interface{}) map[string]interface{} {
	if p != nil {
		return p.(map[string]interface{})
	} else {
		return make(map[string]interface{})
	}
}

func (nxc *NxConfig) isEnabled(p interface{}) bool {
	if p == nil {
		return false
	}
	switch t := p.(type) {
	case string:
		if len(t) > 0 {
			return true
		}
	case bool:
		if t {
			return true
		}
	}
	return false
}
