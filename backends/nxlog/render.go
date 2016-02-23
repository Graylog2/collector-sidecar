package nxlog

import (
	"bytes"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/Graylog2/sidecar/api/graylog"
	"github.com/Graylog2/sidecar/util"
	"github.com/Sirupsen/logrus"
)

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

func (nxc *NxConfig) fileInputsToString() string {
	var result bytes.Buffer
	for _, can := range nxc.Canned {
		if can.kind == "input-file" {
			result.WriteString("<Input " + can.name + ">\n")
			result.WriteString("	Module im_file\n")
			result.WriteString("	File \"" + can.properties["path"] + "\"\n")
			result.WriteString("	SavePos	TRUE\n")
			result.WriteString("</Input>\n")
		}
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) windowsEventLogInputsToString() string {
	var result bytes.Buffer
	for _, can := range nxc.Canned {
		if can.kind == "input-windows-event-log" {
			result.WriteString("<Input " + can.name + ">\n")
			result.WriteString("	Module im_msvistalog\n")
			result.WriteString("</Input>\n")
		}
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) gelfUdpOutputsToString() string {
	var result bytes.Buffer
	for _, can := range nxc.Canned {
		if can.kind == "output-gelf-udp" {
			result.WriteString("<Output " + can.name + ">\n")
			result.WriteString("	Module om_udp\n")
			result.WriteString("	Host " + can.properties["server"] + "\n")
			result.WriteString("	Port " + can.properties["port"] + "\n")
			result.WriteString("	OutputType  GELF\n")
			result.WriteString("</Output>\n")
		}
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) Render() bytes.Buffer {
	var result bytes.Buffer
	result.WriteString(nxc.definitionsToString())
	result.WriteString(nxc.pathsToString())
	result.WriteString(nxc.extensionsToString())
	result.WriteString(nxc.inputsToString())
	result.WriteString(nxc.outputsToString())
	// pre-canned types
	result.WriteString(nxc.fileInputsToString())
	result.WriteString(nxc.windowsEventLogInputsToString())
	result.WriteString(nxc.gelfUdpOutputsToString())
	//
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

func (nxc *NxConfig) RenderOnChange(json graylog.ResponseCollectorConfiguration) bool {
	jsonConfig := NewCollectorConfig(nxc.CollectorPath)
	sidecarPath, _ := util.GetSidecarPath()

	for _, output := range json.Outputs {
		if output.Backend == "nxlog" {
			if len(output.Type) > 0 {
				jsonConfig.Add("output-"+output.Type, output.Name, output.Properties)
			} else {
				jsonConfig.Add("output", output.Name, output.Properties)
			}
		}
	}
	for i, input := range json.Inputs {
		if input.Backend == "nxlog" {
			if len(input.Type) > 0 {
				jsonConfig.Add("input-"+input.Type, input.Name, input.Properties)
			} else {
				jsonConfig.Add("input", input.Name, input.Properties)
			}
			jsonConfig.Add("route", "route-"+strconv.Itoa(i), map[string]string{"Path": input.Name + " => " + input.ForwardTo})
		}
	}
	for _, snippet := range json.Snippets {
		if snippet.Backend == "nxlog" {
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

func (nxc *NxConfig) ValidateConfigurationFile(configurationPath string) bool {
	cmd := exec.Command(nxc.ExecPath(), "-v", "-c", filepath.Join(configurationPath, "nxlog", "nxlog.conf"))
	err := cmd.Run()
	if err != nil {
		logrus.Error("Error during configuration validation: ", err)
		return false
	}

	return true
}
