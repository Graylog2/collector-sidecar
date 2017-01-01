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
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"errors"
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
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
	Processors  []nxprocessor
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

type nxprocessor struct {
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
	backendIndex, err := context.UserConfig.GetBackendIndexByName(name)
	if err == nil {
		nxc.UserConfig = &context.UserConfig.Backends[backendIndex]
		nxc.Definitions = []nxdefinition{{name: "ROOT", value: filepath.Dir(context.UserConfig.Backends[backendIndex].BinaryPath)}}
	}
	return nxc
}

func (nxc *NxConfig) findCannedOutputByName(name string) (nxcanned, error) {
	for _, can := range nxc.Canned {
		if can.name == name && strings.HasPrefix(can.kind, "output") {
			return can, nil
		}
	}
	return nxcanned{}, errors.New("Output " + name + " not found.")
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
	case "processor":
		addition := &nxprocessor{name: name, properties: value.(map[string]string)}
		nxc.Processors = append(nxc.Processors, *addition)
	case "snippet":
		snippet := &nxsnippet{name: name, value: value.(string)}
		if !nxc.Exists("snippet", snippet) {
			nxc.Snippets = append(nxc.Snippets, *snippet)
		} else {
			msg := fmt.Sprintf("Skipping snippet %s till it already exist in configuration.", name)
			nxc.SetStatus(backends.StatusUnknown, msg)
			log.Warnf("[%s] %s", nxc.Name(), msg)
		}
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
		extension := &nxextension{name: "syslog", properties: map[string]string{"Module": "xm_syslog"}}
		if !nxc.Exists("extension", extension) {
			nxc.Extensions = append(nxc.Extensions, *extension)
		}
	case "input-tcp-syslog":
		addition := &nxcanned{name: name, kind: class, properties: value.(map[string]interface{})}
		nxc.Canned = append(nxc.Canned, *addition)
		extension := &nxextension{name: "syslog", properties: map[string]string{"Module": "xm_syslog"}}
		if !nxc.Exists("extension", extension) {
			nxc.Extensions = append(nxc.Extensions, *extension)
		}
	}
}

func (nxc *NxConfig) Exists(class string, c interface{}) bool {
	result := false
	switch class {
	case "extension":
		comparator := c.(*nxextension)
		for _, entity := range nxc.Extensions {
			if entity.name == comparator.name && reflect.DeepEqual(entity.properties, comparator.properties) {
				result = true
			}
		}
	case "input":
		comparator := c.(*nxinput)
		for _, entity := range nxc.Inputs {
			log.Info(entity.properties)
			if entity.name == comparator.name && reflect.DeepEqual(entity.properties, comparator.properties) {
				result = true
			}
		}
	case "output":
		comparator := c.(*nxoutput)
		for _, entity := range nxc.Outputs {
			if entity.name == comparator.name && reflect.DeepEqual(entity.properties, comparator.properties) {
				result = true
			}
		}
	case "route":
		comparator := c.(*nxroute)
		for _, entity := range nxc.Routes {
			if entity.name == comparator.name && reflect.DeepEqual(entity.properties, comparator.properties) {
				result = true
			}
		}
	case "match":
		comparator := c.(*nxmatch)
		for _, entity := range nxc.Matches {
			if entity.name == comparator.name && reflect.DeepEqual(entity.properties, comparator.properties) {
				result = true
			}
		}
	case "snippet":
		comparator := c.(*nxsnippet)
		for _, entity := range nxc.Snippets {
			if entity.value == comparator.value {
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
	nxc.Processors = a.Processors
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

func (nxc *NxConfig) propertyStringIndented(p interface{}, precision int) string {
	var result string
	lines := strings.Split(nxc.propertyString(p, precision), "\n")
	for _, line := range lines {
		result = result + "	" + line + "\n"
	}
	return result
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

func (nxc *NxConfig) isDisabled(p interface{}) bool {
	return !nxc.isEnabled(p)
}
