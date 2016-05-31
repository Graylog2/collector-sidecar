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

package filebeat

import (
	"bytes"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os/exec"
	"text/template"

	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/common"
)

func (fbc *FileBeatConfig) snippetsToString() string {
	var buffer bytes.Buffer
	var result bytes.Buffer
	for _, snippet := range fbc.Beats.Snippets {
		snippetTemplate, err := template.New("snippet").Parse(snippet.Value)
		if err != nil {
			result.WriteString(snippet.Value)
		} else {
			snippetTemplate.Execute(&buffer, fbc.Beats.Context.Inventory)
			result.WriteString(buffer.String())
		}
		result.WriteString("\n")
	}
	return result.String()
}

func (fbc *FileBeatConfig) Render() bytes.Buffer {
	var result bytes.Buffer

	if fbc.Beats.Data() == nil {
		return result
	}

	result.WriteString(fbc.Beats.String())
	result.WriteString(fbc.snippetsToString())

	return result
}

func (fbc *FileBeatConfig) RenderToFile() error {
	stringConfig := fbc.Render()
	err := common.CreatePathToFile(fbc.Beats.UserConfig.ConfigurationPath)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(fbc.Beats.UserConfig.ConfigurationPath, stringConfig.Bytes(), 0644)
	return err
}

func (fbc *FileBeatConfig) RenderOnChange(response graylog.ResponseCollectorConfiguration) bool {
	newConfig := NewCollectorConfig(fbc.Beats.Context)

	// create prospector slice
	var prospector []map[string]interface{}

	for _, output := range response.Outputs {
		if output.Backend == "filebeat" {
			for property, value := range output.Properties {
				// ignore tls properties
				if property == "tls" ||
					property == "ca_file" ||
					property == "cert_file" ||
					property == "cert_key_file" {
					continue
				}
				newConfig.Beats.Set(value, "output", output.Type, property)
			}
			if output.Properties["tls"].(bool) {
				newConfig.Beats.Set([]string{output.Properties["ca_file"].(string)}, "output", "logstash", "tls", "certificate_authorities")
				newConfig.Beats.Set(output.Properties["cert_file"], "output", "logstash", "tls", "certificate")
				newConfig.Beats.Set(output.Properties["cert_key_file"], "output", "logstash", "tls", "certificate_key")
			}
		}
	}

	for _, input := range response.Inputs {
		if input.Backend == "filebeat" {
			prospector = append(prospector, make(map[string]interface{}))
			idx := len(prospector) - 1
			// we dont support stdin input type
			prospector[idx]["input_type"] = "log"
			for property, value := range input.Properties {
				// ignore include|exclude_lines if they are an empty array (default value)
				if (property == "include_lines" || property == "exclude_lines") && fbc.Beats.PropertyString(value, 0) == "[]" {
					continue
				}
				// ignore multiline fields
				if (property == "multiline" ||
					property == "multiline_pattern" ||
					property == "multiline_negate" ||
					property == "multiline_match") {
					continue

				}
				// ignore additional fields
				if property == "fields" {
					continue
				}

				// everything else get's rendered without transformation
				var vt interface{}
				err := yaml.Unmarshal([]byte(fbc.Beats.PropertyString(value, 0)), &vt)
				if err != nil {
					log.Errorf("[%s] Nested YAML is not parsable: '%s'", fbc.Name(), value)
				} else {
					prospector[idx][property] = vt
				}
			}
			// generate multiline.* structure if enabled
			if input.Properties["multiline"].(bool) {
				multiline := make(map[string]interface{})
				multiline["pattern"] = fbc.Beats.PropertyString(input.Properties["multiline_pattern"], 0)
				multiline["negate"] = input.Properties["multiline_negate"] .(bool)
				match := fbc.Beats.PropertyString(input.Properties["multiline_match"], 0)
				if match == "after" || match == "before" {
					multiline["match"] = match
				} else {
					log.Errorf("[%s] Multiline match can either be 'after' or 'before', but not '%s'", fbc.Name(), match)
				}
				prospector[idx]["multiline"] = multiline
			}
			// add additional fields if enabled
			if input.Properties["fields"] != nil {
				additionalFields := input.Properties["fields"].(map[string]interface{})
				if len(additionalFields) != 0 {
					prospector[idx]["fields"] = additionalFields
				}
			}
		}
	}
	newConfig.Beats.Set(prospector, "filebeat", "prospectors")

	for _, snippet := range response.Snippets {
		if snippet.Backend == "filebeat" {
			newConfig.Beats.AppendString(snippet.Id, snippet.Value)
		}
	}

	if !fbc.Beats.Equals(newConfig.Beats) {
		log.Infof("[%s] Configuration change detected, rewriting configuration file.", fbc.Name())
		fbc.Beats.Update(newConfig.Beats)
		fbc.RenderToFile()
		return true
	}

	return false
}

func (fbc *FileBeatConfig) ValidateConfigurationFile() bool {
	cmd := exec.Command(fbc.ExecPath(), "-configtest", "-c", fbc.Beats.UserConfig.ConfigurationPath)
	err := cmd.Run()
	if err != nil {
		log.Errorf("[%s] Error during configuration validation: %s", fbc.Name(), err)
		return false
	}

	return true
}
