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

package beats

import (
	"errors"
	"reflect"
	"strconv"

	"gopkg.in/yaml.v2"

	"github.com/Graylog2/collector-sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
)

var (
	// ErrPathCollision - Creating a path failed because an element collided with an existing value.
	ErrPathCollision = errors.New("encountered value collision whilst building path")
)

type BeatsConfig struct {
	Context             *context.Ctx
	UserConfig          *cfgfile.SidecarBackend
	Container           interface{}       // holds the configuration object for un/marshalling
	ContainerKeyMapping map[string]string // keys can be renamed before the configuration is rendered
	Snippets            []beatSnippet
	Version             []int // Beats collector version
}

type beatSnippet struct {
	Name  string
	Value string
}

func (bc *BeatsConfig) mapKey(key string) string {
	if mappedKey := bc.ContainerKeyMapping[key]; len(mappedKey) == 0 {
		return key
	} else {
		return mappedKey
	}
}

func (bc *BeatsConfig) Data() interface{} {
	return bc.Container
}

func (bc *BeatsConfig) Set(value interface{}, path ...string) error {
	if len(path) == 0 || value == nil {
		return nil
	}
	// Initialize configuration container if needed
	if bc.Container == nil {
		bc.Container = map[string]interface{}{}
	}
	// Unmarshal nested YAML
	if value != nil && reflect.TypeOf(value).Kind() == reflect.String {
		yaml.Unmarshal([]byte(value.(string)), &value)
	}

	cp := bc.Container
	for target := 0; target < len(path); target++ {
		if mmap, ok := cp.(map[string]interface{}); ok {
			if target == len(path)-1 {
				mmap[bc.mapKey(path[target])] = value
			} else if mmap[path[target]] == nil {
				mmap[path[target]] = map[string]interface{}{}
			}
			cp = mmap[path[target]]
		} else {
			return ErrPathCollision
		}
	}
	return nil
}

func (bc *BeatsConfig) Get(path ...string) interface{} {
	var object interface{}

	object = bc.Container
	for target := 0; target < len(path); target++ {
		if mmap, ok := object.(map[string]interface{}); ok {
			object = mmap[path[target]]
		}
	}
	return object
}

func (bc *BeatsConfig) AppendString(name string, value string) {
	addition := &beatSnippet{Name: name, Value: value}
	bc.Snippets = append(bc.Snippets, *addition)
}

func (bc *BeatsConfig) Update(a *BeatsConfig) {
	bc.Container = a.Container
	bc.Snippets = a.Snippets
}

func (bc *BeatsConfig) Equals(a *BeatsConfig) bool {
	return reflect.DeepEqual(bc, a)
}

func (bc *BeatsConfig) String() string {
	if bc.Container != nil {
		if bytes, err := yaml.Marshal(bc.Container); err == nil {
			return string(common.ConvertLineBreak(bytes))
		}
	}
	return "---"
}

func (bc *BeatsConfig) PropertyString(p interface{}, precision int) string {
	switch t := p.(type) {
	default:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		} else {
			return "false"
		}
	case int:
		return strconv.Itoa(t)
	case float64:
		return strconv.FormatFloat(t, 'f', precision, 64)
	}
}

func (bc *BeatsConfig) PropertyBool(p interface{}) bool {
	switch p.(type) {
	default:
		return false
	case bool:
		return p.(bool)
	case string:
		if s, err := strconv.ParseBool(p.(string)); len(p.(string)) > 0 && (s || err != nil) {
			return true
		}
		return false
	}
}

func (bc *BeatsConfig) RunMigrations(cachePath string) {
	if bc.Version[0] == 5 {
		// rename ssl properties
		cp := bc.Container
		configurationPath := []string{"output", "logstash", "tls", "certificate_key"}
		for target := 0; target < len(configurationPath); target++ {
			if mmap, ok := cp.(map[string]interface{}); ok {
				if target == len(configurationPath)-1 {
					if mmap["certificate_key"] != nil {
						mmap["key"] = mmap["certificate_key"]
						delete(mmap, "certificate_key")
					}
					if mmap["insecure"] == true {
						mmap["verification_mode"] = "none"
						delete(mmap, "insecure")
					}
				}
				cp = mmap[configurationPath[target]]
			}
		}

		// rename tls -> ssl
		cp = bc.Container
		configurationPath = []string{"output", "logstash", "tls"}
		for target := 0; target < len(configurationPath); target++ {
			if mmap, ok := cp.(map[string]interface{}); ok {
				if target == len(configurationPath)-1 {
					if mmap["tls"] != nil {
						mmap["ssl"] = mmap["tls"]
						delete(mmap, "tls")
					}
				}
				cp = mmap[configurationPath[target]]
			}
		}

		// remove shipper
		cp = bc.Container
		configurationPath = []string{"shipper", "tags"}
		for target := 0; target < len(configurationPath); target++ {
			if mmap, ok := cp.(map[string]interface{}); ok {
				if target == len(configurationPath)-1 {
					bc.Set(mmap["tags"], "tags")
					delete(bc.Container.(map[string]interface{}), "shipper")
				}
				cp = mmap[configurationPath[target]]
			}
		}

		// set cache data path
		bc.Set(cachePath, "path", "data")

		// configure log path
		bc.Set(bc.Context.UserConfig.LogPath, "path", "logs")

		if bc.Version[1] >= 5 {
			// in all prospectors
			cp = bc.Container
			configurationPath = []string{"filebeat", "prospectors"}
			for target := 0; target < len(configurationPath); target++ {
				if mmap, ok := cp.(map[string]interface{}); ok {
					cp = mmap[configurationPath[target]]
				}
			}
			if prospectors, ok := cp.([]map[string]interface{}); ok {
				for target := 0; target < len(prospectors); target++ {
					// rename input_type -> type
					if prospectors[target]["input_type"] != nil {
						prospectors[target]["type"] = prospectors[target]["input_type"]
						delete(prospectors[target], "input_type")
					}
					// rename document_type -> fields.type
					if prospectors[target]["document_type"] != nil {
						prospectors[target]["fields"].(map[string]interface{})["type"] = prospectors[target]["document_type"]
						delete(prospectors[target], "document_type")
					}

					cp.([]map[string]interface{})[target] = prospectors[target]
				}
			}
		}
	}
}