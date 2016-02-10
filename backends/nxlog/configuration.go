package nxlog

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"strconv"

	"github.com/Graylog2/nxlog-sidecar/api/graylog"
	"github.com/Graylog2/nxlog-sidecar/backends"
	"github.com/Graylog2/nxlog-sidecar/util"
	"github.com/Sirupsen/logrus"
)

const name = "nxlog"

type NxConfig struct {
	CollectorPath string
	Definitions   []nxdefinition
	Paths         []nxpath
	Extensions    []nxextension
	Inputs        []nxinput
	Outputs       []nxoutput
	Routes        []nxroute
	Matches       []nxmatch
	Snippets      []nxsnippet
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

func init() {
	if err := backends.RegisterBackend(name, New); err != nil {
		logrus.Fatal(err)
	}
}

func New(collectorPath string) backends.Backend {
	return NewCollectorConfig(collectorPath)
}

func NewCollectorConfig(collectorPath string) *NxConfig {
	nxc := &NxConfig{
		CollectorPath: collectorPath,
		Definitions:   []nxdefinition{{name: "ROOT", value: collectorPath}},
		Paths: []nxpath{{name: "Moduledir", path: "%ROOT%\\modules"},
			{name: "CacheDir", path: "%ROOT%\\data"},
			{name: "Pidfile", path: "%ROOT%\\data\\nxlog.pid"},
			{name: "SpoolDir", path: "%ROOT%\\data"},
			{name: "LogFile", path: "%ROOT%\\data\\nxlog.log"}},
		Extensions: []nxextension{{name: "gelf", properties: map[string]string{"Module": "xm_gelf"}}},
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
	}
}

func (nxc *NxConfig) GetCollectorPath() string {
	return nxc.CollectorPath
}

func (nxc *NxConfig) definitionsToString() string {
	var result bytes.Buffer
	for _, definition := range nxc.Definitions {
		result.WriteString("define " + definition.name + " " + definition.value)
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) pathsToString() string {
	var result bytes.Buffer
	for _, path := range nxc.Paths {
		result.WriteString(path.name + " " + path.path + "\n")
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) extensionsToString() string {
	var result bytes.Buffer
	for _, extension := range nxc.Extensions {
		result.WriteString("<Extension " + extension.name + ">\n")
		for propertyName, propertyValue := range extension.properties {
			result.WriteString("  " + propertyName + " " + propertyValue + "\n")
		}
		result.WriteString("</Extension>\n")
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) inputsToString() string {
	var result bytes.Buffer
	for _, input := range nxc.Inputs {
		result.WriteString("<Input " + input.name + ">\n")
		for propertyName, propertyValue := range input.properties {
			result.WriteString("  " + propertyName + " " + propertyValue + "\n")
		}
		result.WriteString("</Input>\n")
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) outputsToString() string {
	var result bytes.Buffer
	for _, output := range nxc.Outputs {
		result.WriteString("<Output " + output.name + ">\n")
		for propertyName, propertyValue := range output.properties {
			result.WriteString("  " + propertyName + " " + propertyValue + "\n")
		}
		result.WriteString("</Output>\n")
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) routesToString() string {
	var result bytes.Buffer
	for _, route := range nxc.Routes {
		result.WriteString("<Route " + route.name + ">\n")
		for propertyName, propertyValue := range route.properties {
			result.WriteString("  " + propertyName + " " + propertyValue + "\n")
		}
		result.WriteString("</Route>\n")
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) matchesToString() string {
	var result bytes.Buffer
	for _, match := range nxc.Matches {
		result.WriteString("<Match " + match.name + ">\n")
		for propertyName, propertyValue := range match.properties {
			result.WriteString("  " + propertyName + " " + propertyValue + "\n")
		}
		result.WriteString("</Match>\n")
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) snippetsToString() string {
	var result bytes.Buffer
	for _, snippet := range nxc.Snippets {
		result.WriteString(snippet.value)
		result.WriteString("\n")
	}
	return result.String()
}

func (nxc *NxConfig) Render() bytes.Buffer {
	var result bytes.Buffer
	result.WriteString(nxc.definitionsToString())
	result.WriteString(nxc.pathsToString())
	result.WriteString(nxc.extensionsToString())
	result.WriteString(nxc.inputsToString())
	result.WriteString(nxc.outputsToString())
	result.WriteString(nxc.routesToString())
	result.WriteString(nxc.matchesToString())
	result.WriteString(nxc.snippetsToString())
	return result
}

func (nxc *NxConfig) RenderToFile(path string) error {
	stringConfig := nxc.Render()
	err := ioutil.WriteFile(path, stringConfig.Bytes(), 0644)
	return err
}

func (nxc *NxConfig) Equals(a *NxConfig) bool {
	return reflect.DeepEqual(nxc, a)
}

func (nxc *NxConfig) Update(a *NxConfig) {
	nxc.CollectorPath = a.CollectorPath
	nxc.Definitions   = a.Definitions
	nxc.Paths         = a.Paths
	nxc.Extensions    = a.Extensions
	nxc.Inputs        = a.Inputs
	nxc.Outputs       = a.Outputs
	nxc.Routes        = a.Routes
	nxc.Matches       = a.Matches
	nxc.Snippets      = a.Snippets
}

func (nxc *NxConfig) RenderOnChange(json graylog.ResponseCollectorConfiguration) bool {
	jsonConfig := NewCollectorConfig(nxc.CollectorPath)
	sidecarPath, _ := util.GetSidecarPath()

	for _, output := range json.Outputs {
		if output.Type == "nxlog" {
			jsonConfig.Add("output", output.Name, output.Properties)
		}
	}
	for i, input := range json.Inputs {
		if input.Type == "nxlog" {
			jsonConfig.Add("input", input.Name, input.Properties)
			jsonConfig.Add("route", "route-"+strconv.Itoa(i), map[string]string{"Path": input.Name + " => " + input.ForwardTo})
		}
	}
	for _, snippet := range json.Snippets {
		if snippet.Type == "nxlog" {
			jsonConfig.Add("snippet", snippet.Name, snippet.Value)
		}
	}

	if !nxc.Equals(jsonConfig) {
		logrus.Info("Configuration change detected, rewriting configuration file.")
		nxc.Update(jsonConfig)
		nxc.RenderToFile(filepath.Join(sidecarPath, "nxlog", "nxlog.conf"))
		return true
	}

	return false
}
