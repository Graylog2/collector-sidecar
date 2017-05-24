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

package cfgfile

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/urso/ucfg"
	"github.com/urso/ucfg/yaml"

	"github.com/Graylog2/collector-sidecar/logger"
)

var log = logger.Log()
var configurationFile string

// Command line flags
var validateConfiguration *bool

func init() {
	validateConfiguration = flag.Bool("configtest", false, "Validate configuration file and exit.")
}

// Read reads the configuration from a yaml file into the given interface structure.
// In case path is not set this method reads from the default configuration file.
func Read(out interface{}, path string) error {

	if path == "" && configurationFile != "" {
		path = configurationFile
	}

	filecontent, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("[ConfigFile] Failed to read %s: %v. Exiting.", path, err)
	}
	filecontent = expandEnv(filecontent)

	config, err := yaml.NewConfig(filecontent, ucfg.PathSep("."))
	if err != nil {
		return fmt.Errorf("[ConfigFile] YAML config parsing failed on %s: %v. Exiting.", path, err)
	}

	err = config.Unpack(out, ucfg.PathSep("."))
	if err != nil {
		return fmt.Errorf("[ConfigFile] Failed to apply config %s: %v. Exiting. ", path, err)
	}
	return nil
}

// ValidateConfig returns whether or not this configuration is used for testing
func ValidateConfig() bool {
	return *validateConfiguration
}

// Preserve path to configuration file for reloading the same file during lifetime
func SetConfigPath(path string) {
	configurationFile = path
}

// expandEnv replaces ${var} or $var in config according to the values of the
// current environment variables. The replacement is case-sensitive. References
// to undefined variables are replaced by the empty string. A default value
// can be given by using the form ${var:default value}.
func expandEnv(config []byte) []byte {
	return []byte(os.Expand(string(config), func(key string) string {
		keyAndDefault := strings.SplitN(key, ":", 2)
		key = keyAndDefault[0]

		v := os.Getenv(key)
		if v == "" && len(keyAndDefault) == 2 {
			// Set value to the default.
			v = keyAndDefault[1]
			log.Infof("[ConfigFile] Replacing config environment variable '${%s}' with default '%s'",
				key, keyAndDefault[1])
		} else {
			log.Infof("[ConfigFile] Replacing config environment variable '${%s}' with '%s'",
				key, v)
		}

		return v
	}))
}
