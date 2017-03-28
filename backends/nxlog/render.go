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
	"bytes"
	"io/ioutil"
	"os/exec"
	"strconv"
	"text/template"

	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/common"
	"strings"
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
			result.WriteString("  " + propertyName + " " + nxc.propertyString(propertyValue, 0) + "\n")
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
			result.WriteString("  " + propertyName + " " + nxc.propertyString(propertyValue, 0) + "\n")
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
			result.WriteString("  " + propertyName + " " + nxc.propertyString(propertyValue, 0) + "\n")
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
			result.WriteString("  " + propertyName + " " + nxc.propertyString(propertyValue, 0) + "\n")
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
			result.WriteString("  " + propertyName + " " + nxc.propertyString(propertyValue, 0) + "\n")
		}
		result.WriteString("</Match>\n")
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) processorsToString() string {
	var result bytes.Buffer
	for _, processor := range nxc.Processors {
		result.WriteString("<Processor " + processor.name + ">\n")
		for propertyName, propertyValue := range processor.properties {
			result.WriteString("  " + propertyName + " " + nxc.propertyString(propertyValue, 0) + "\n")
		}
		result.WriteString("</Processor>\n")
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) snippetsToString() string {
	var result bytes.Buffer
	for _, snippet := range nxc.Snippets {
		snippetTemplate, err := template.New("snippet").Parse(snippet.value)
		if err != nil {
			result.WriteString(snippet.value)
		} else {
			var buffer bytes.Buffer
			snippetTemplate.Execute(&buffer, nxc.Context.Inventory)
			result.WriteString(buffer.String())
		}
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
			result.WriteString("	File '" + nxc.propertyString(can.properties["path"], 0) + "'\n")
			result.WriteString("	PollInterval " + nxc.propertyString(can.properties["poll_interval"], 0) + "\n")
			result.WriteString("	SavePos	" + nxc.propertyString(can.properties["save_position"], 0) + "\n")
			result.WriteString("	ReadFromLast " + nxc.propertyString(can.properties["read_last"], 0) + "\n")
			result.WriteString("	Recursive " + nxc.propertyString(can.properties["recursive"], 0) + "\n")
			result.WriteString("	RenameCheck " + nxc.propertyString(can.properties["rename_check"], 0) + "\n")
			result.WriteString("	Exec $FileName = file_name(); # Send file name with each message\n")
			if len(nxc.propertyStringMap(can.properties["fields"])) > 0 {
				for key, value := range nxc.propertyStringMap(can.properties["fields"]) {
					result.WriteString("	Exec $" + key + " = \"" + value.(string) + "\";\n")
				}
			}
			if nxc.isEnabled(can.properties["multiline"]) {
				result.WriteString("	InputType " + can.name + "-multiline\n")
			}
			if nxc.isEnabled(can.properties["verbatim"]) {
				var verbatim = nxc.propertyStringIndented(can.properties["verbatim"], 0)
				result.WriteString(common.EnsureLineBreak(verbatim))
			}
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
			result.WriteString("	PollInterval " + nxc.propertyString(can.properties["poll_interval"], 0) + "\n")
			result.WriteString("	SavePos	" + nxc.propertyString(can.properties["save_position"], 0) + "\n")
			result.WriteString("	ReadFromLast " + nxc.propertyString(can.properties["read_last"], 0) + "\n")
			if nxc.isEnabled(can.properties["channel"]) {
				result.WriteString("	Channel " + nxc.propertyString(can.properties["channel"], 0) + "\n")
			}
			if nxc.isEnabled(can.properties["query"]) {
				result.WriteString("	Query " + nxc.propertyString(can.properties["query"], 0) + "\n")
			}
			if len(nxc.propertyStringMap(can.properties["fields"])) > 0 {
				for key, value := range nxc.propertyStringMap(can.properties["fields"]) {
					result.WriteString("	Exec $" + key + " = \"" + value.(string) + "\";\n")
				}
			}
			if nxc.isEnabled(can.properties["verbatim"]) {
				var verbatim = nxc.propertyStringIndented(can.properties["verbatim"], 0)
				result.WriteString(common.EnsureLineBreak(verbatim))
			}
			result.WriteString("</Input>\n")
		}
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) udpSyslogInputsToString() string {
	var result bytes.Buffer
	for _, can := range nxc.Canned {
		if can.kind == "input-udp-syslog" {
			result.WriteString("<Input " + can.name + ">\n")
			result.WriteString("	Module im_udp\n")
			result.WriteString("	Host " + nxc.propertyString(can.properties["host"], 0) + "\n")
			result.WriteString("	Port " + nxc.propertyString(can.properties["port"], 0) + "\n")
			if len(nxc.propertyStringMap(can.properties["fields"])) > 0 {
				for key, value := range nxc.propertyStringMap(can.properties["fields"]) {
					result.WriteString("	Exec $" + key + " = \"" + value.(string) + "\";\n")
				}
			}
			result.WriteString("	Exec parse_syslog_bsd();\n")
			if nxc.isEnabled(can.properties["verbatim"]) {
				var verbatim = nxc.propertyStringIndented(can.properties["verbatim"], 0)
				result.WriteString(common.EnsureLineBreak(verbatim))
			}
			result.WriteString("</Input>\n")
		}
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) tcpSyslogInputsToString() string {
	var result bytes.Buffer
	for _, can := range nxc.Canned {
		if can.kind == "input-tcp-syslog" {
			result.WriteString("<Input " + can.name + ">\n")
			result.WriteString("	Module im_tcp\n")
			result.WriteString("	Host " + nxc.propertyString(can.properties["host"], 0) + "\n")
			result.WriteString("	Port " + nxc.propertyString(can.properties["port"], 0) + "\n")
			if len(nxc.propertyStringMap(can.properties["fields"])) > 0 {
				for key, value := range nxc.propertyStringMap(can.properties["fields"]) {
					result.WriteString("	Exec $" + key + " = \"" + value.(string) + "\";\n")
				}
			}
			result.WriteString("	Exec parse_syslog_bsd();\n")
			if nxc.isEnabled(can.properties["verbatim"]) {
				var verbatim = nxc.propertyStringIndented(can.properties["verbatim"], 0)
				result.WriteString(common.EnsureLineBreak(verbatim))
			}
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
			result.WriteString("	Host " + nxc.propertyString(can.properties["server"], 0) + "\n")
			result.WriteString("	Port " + nxc.propertyString(can.properties["port"], 0) + "\n")
			result.WriteString("	OutputType  GELF\n")
			result.WriteString("	Exec $short_message = $raw_event; # Avoids truncation of the short_message field.\n")
			result.WriteString("	Exec $gl2_source_collector = '" + nxc.Context.CollectorId + "';\n")
			result.WriteString("	Exec $collector_node_id = '" + nxc.Context.NodeId + "';\n")
			if nxc.isDisabled(can.properties["override_hostname"]) {
				result.WriteString("	Exec $Hostname = hostname_fqdn();\n")
			}
			if len(nxc.propertyStringMap(can.properties["fields"])) > 0 {
				for key, value := range nxc.propertyStringMap(can.properties["fields"]) {
					result.WriteString("	Exec $" + key + " = \"" + value.(string) + "\";\n")
				}
			}
			if nxc.isEnabled(can.properties["verbatim"]) {
				var verbatim = nxc.propertyStringIndented(can.properties["verbatim"], 0)
				result.WriteString(common.EnsureLineBreak(verbatim))
			}
			result.WriteString("</Output>\n")
		}
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) gelfTcpOutputsToString() string {
	var result bytes.Buffer
	for _, can := range nxc.Canned {
		if can.kind == "output-gelf-tcp" {
			result.WriteString("<Output " + can.name + ">\n")
			result.WriteString("	Module om_tcp\n")
			result.WriteString("	Host " + nxc.propertyString(can.properties["server"], 0) + "\n")
			result.WriteString("	Port " + nxc.propertyString(can.properties["port"], 0) + "\n")
			result.WriteString("	OutputType  GELF_TCP\n")
			result.WriteString("	Exec $short_message = $raw_event; # Avoids truncation of the short_message field.\n")
			result.WriteString("	Exec $gl2_source_collector = '" + nxc.Context.CollectorId + "';\n")
			result.WriteString("	Exec $collector_node_id = '" + nxc.Context.NodeId + "';\n")
			if nxc.isDisabled(can.properties["override_hostname"]) {
				result.WriteString("	Exec $Hostname = hostname_fqdn();\n")
			}
			if len(nxc.propertyStringMap(can.properties["fields"])) > 0 {
				for key, value := range nxc.propertyStringMap(can.properties["fields"]) {
					result.WriteString("	Exec $" + key + " = \"" + value.(string) + "\";\n")
				}
			}
			if nxc.isEnabled(can.properties["verbatim"]) {
				var verbatim = nxc.propertyStringIndented(can.properties["verbatim"], 0)
				result.WriteString(common.EnsureLineBreak(verbatim))
			}
			result.WriteString("</Output>\n")
		}
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) gelfTcpTlsOutputsToString() string {
	var result bytes.Buffer
	for _, can := range nxc.Canned {
		if can.kind == "output-gelf-tcp-tls" {
			result.WriteString("<Output " + can.name + ">\n")
			result.WriteString("	Module om_ssl\n")
			result.WriteString("	Host " + nxc.propertyString(can.properties["server"], 0) + "\n")
			result.WriteString("	Port " + nxc.propertyString(can.properties["port"], 0) + "\n")
			result.WriteString("	OutputType GELF_TCP\n")
			if nxc.isEnabled(can.properties["ca_file"]) {
				result.WriteString("	CAFile " + nxc.propertyString(can.properties["ca_file"], 0) + "\n")
			}
			if nxc.isEnabled(can.properties["cert_file"]) {
				result.WriteString("	CertFile " + nxc.propertyString(can.properties["cert_file"], 0) + "\n")
			}
			if nxc.isEnabled(can.properties["cert_key_file"]) {
				result.WriteString("	CertKeyFile " + nxc.propertyString(can.properties["cert_key_file"], 0) + "\n")
			}
			if nxc.isEnabled(can.properties["allow_untrusted"]) {
				result.WriteString("	AllowUntrusted " + nxc.propertyString(can.properties["allow_untrusted"], 0) + "\n")
			}
			result.WriteString("	Exec $short_message = $raw_event; # Avoids truncation of the short_message field.\n")
			result.WriteString("	Exec $gl2_source_collector = '" + nxc.Context.CollectorId + "';\n")
			result.WriteString("	Exec $collector_node_id = '" + nxc.Context.NodeId + "';\n")
			if nxc.isDisabled(can.properties["override_hostname"]) {
				result.WriteString("	Exec $Hostname = hostname_fqdn();\n")
			}
			if len(nxc.propertyStringMap(can.properties["fields"])) > 0 {
				for key, value := range nxc.propertyStringMap(can.properties["fields"]) {
					result.WriteString("	Exec $" + key + " = \"" + value.(string) + "\";\n")
				}
			}
			if nxc.isEnabled(can.properties["verbatim"]) {
				var verbatim = nxc.propertyStringIndented(can.properties["verbatim"], 0)
				result.WriteString(common.EnsureLineBreak(verbatim))
			}
			result.WriteString("</Output>\n")
		}
	}
	result.WriteString("\n")
	return result.String()
}

func (nxc *NxConfig) memBufferProperties() map[string]string {
	return map[string]string{
		"Module":  "pm_buffer",
		"MaxSize": "16384",
		"Type":    "Mem",
	}
}

func (nxc *NxConfig) Render() []byte {
	var result bytes.Buffer
	result.WriteString(nxc.definitionsToString())
	result.WriteString(nxc.pathsToString())
	result.WriteString(nxc.extensionsToString())
	result.WriteString(nxc.processorsToString())
	result.WriteString(nxc.snippetsToString())
	result.WriteString(nxc.inputsToString())
	result.WriteString(nxc.outputsToString())
	// pre-canned types
	result.WriteString(nxc.fileInputsToString())
	result.WriteString(nxc.windowsEventLogInputsToString())
	result.WriteString(nxc.udpSyslogInputsToString())
	result.WriteString(nxc.tcpSyslogInputsToString())
	result.WriteString(nxc.gelfUdpOutputsToString())
	result.WriteString(nxc.gelfTcpOutputsToString())
	result.WriteString(nxc.gelfTcpTlsOutputsToString())
	//
	result.WriteString(nxc.routesToString())
	result.WriteString(nxc.matchesToString())

	return common.ConvertLineBreak(result.Bytes())
}

func (nxc *NxConfig) RenderToFile() error {
	stringConfig := nxc.Render()
	err := common.CreatePathToFile(nxc.UserConfig.ConfigurationPath)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(nxc.UserConfig.ConfigurationPath, stringConfig, 0644)
	return err
}

func (nxc *NxConfig) RenderOnChange(json graylog.ResponseCollectorConfiguration) bool {
	jsonConfig := NewCollectorConfig(nxc.Context)

	for _, output := range json.Outputs {
		if output.Backend == "nxlog" {
			if len(output.Type) > 0 {
				jsonConfig.Add("output-"+output.Type, output.Id, output.Properties)
			} else {
				jsonConfig.Add("output", output.Id, output.Properties)
			}
		}
	}
	for i, input := range json.Inputs {
		if input.Backend == "nxlog" {
			// input has a special type like udp-syslog
			if len(input.Type) > 0 {
				jsonConfig.Add("input-"+input.Type, input.Id, input.Properties)
				output, err := jsonConfig.findCannedOutputByName(input.ForwardTo)
				if err != nil {
					log.Errorf("[%s] Could not find output for %s: %s", nxc.Name(), input.Name, err)
					continue
				}
				// add buffer processor if corresponding output is marked as buffered
				if nxc.isEnabled(output.properties["buffered"]) {
					jsonConfig.Add("processor", input.Id+"-buffer", nxc.memBufferProperties())
					// add buffered route
					jsonConfig.Add("route",
						"route-"+strconv.Itoa(i),
						map[string]string{"Path": input.Id + " => " + input.Id + "-buffer" + " => " + output.name})
				} else {
					// add un-buffered route
					jsonConfig.Add("route", "route-"+strconv.Itoa(i), map[string]string{"Path": input.Id + " => " + input.ForwardTo})
				}
				// input is generic
			} else {
				jsonConfig.Add("input", input.Id, input.Properties)
				jsonConfig.Add("route", "route-"+strconv.Itoa(i), map[string]string{"Path": input.Id + " => " + input.ForwardTo})
			}
		}
	}
	for _, snippet := range json.Snippets {
		if snippet.Backend == "nxlog" {
			jsonConfig.Add("snippet", snippet.Id, snippet.Value)
		}
	}

	if !nxc.Equals(jsonConfig) {
		log.Infof("[%s] Configuration change detected, rewriting configuration file.", nxc.Name())
		nxc.Update(jsonConfig)
		nxc.RenderToFile()
		return true
	}

	return false
}

func (nxc *NxConfig) ValidateConfigurationFile() bool {
	output, err := exec.Command(nxc.ExecPath(), "-v", "-c", nxc.UserConfig.ConfigurationPath).CombinedOutput()
	soutput := string(output)
	if err != nil || !strings.Contains(soutput, "configuration OK") {
		log.Errorf("[%s] Error during configuration validation: %s", nxc.Name(), soutput)
		return false
	}

	return true
}
