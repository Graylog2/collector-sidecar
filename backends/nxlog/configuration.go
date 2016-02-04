package nxlog

import (
	"bytes"
	"io/ioutil"
	"reflect"
)

type NxConfig struct {
	Nxpath      string
	Definitions []nxdefinition
	Paths       []nxpath
	Extensions  []nxextension
	Inputs      []nxinput
	Outputs     []nxoutput
	Routes      []nxroute
	Matches     []nxmatch
	Snippets    []nxsnippet
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

func NewNxConfig(nxPath string) *NxConfig {
	nxc := &NxConfig{
		Nxpath:      nxPath,
		Definitions: []nxdefinition{{name: "ROOT", value: nxPath}},
		Paths: []nxpath{{name: "Moduledir", path: "%ROOT%\\modules"},
			{name: "CacheDir", path: "%ROOT%\\data"},
			{name: "Pidfile", path: "%ROOT%\\data\\nxlog.pid"},
			{name: "SpoolDir", path: "%ROOT%\\data"},
			{name: "LogFile", path: "%ROOT%\\data\\nxlog.log"}},
		Extensions: []nxextension{{name: "gelf", properties: map[string]string{"Module": "xm_gelf"}}},
	}
	return nxc
}

func (nxc *NxConfig) AddExtension(extensionName string, extensionProperties map[string]string) {
	extension := &nxextension{name: extensionName, properties: extensionProperties}
	nxc.Extensions = append(nxc.Extensions, *extension)
}

func (nxc *NxConfig) AddInput(inputName string, inputProperties map[string]string) {
	input := &nxinput{name: inputName, properties: inputProperties}
	nxc.Inputs = append(nxc.Inputs, *input)
}

func (nxc *NxConfig) AddOutput(outputName string, outputProperties map[string]string) {
	output := &nxoutput{name: outputName, properties: outputProperties}
	nxc.Outputs = append(nxc.Outputs, *output)
}

func (nxc *NxConfig) AddRoute(routeName string, routeProperties map[string]string) {
	route := &nxroute{name: routeName, properties: routeProperties}
	nxc.Routes = append(nxc.Routes, *route)
}

func (nxc *NxConfig) AddMatch(matchName string, matchProperties map[string]string) {
	match := &nxmatch{name: matchName, properties: matchProperties}
	nxc.Matches = append(nxc.Matches, *match)
}

func (nxc *NxConfig) AddSnippet(snippetName string, snippetValue string) {
	snippet := &nxsnippet{name: snippetName, value: snippetValue}
	nxc.Snippets = append(nxc.Snippets, *snippet)
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
