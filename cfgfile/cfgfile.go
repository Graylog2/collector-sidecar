// Copyright (C) 2020 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.


package cfgfile

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/elastic/go-ucfg"
	"github.com/elastic/go-ucfg/yaml"

	"github.com/Graylog2/collector-sidecar/logger"
)

var log = logger.Log()
var configurationFile string

// Command line flags
var validateConfiguration *bool

func init() {
	validateConfiguration = flag.Bool("configtest", false, "Validate configuration file and exit.")
}

func ConfigDefaults() []byte {
	defaults := []byte(CommonDefaults)
	if runtime.GOOS == "windows" {
		defaults = append(defaults, WindowsDefaults...)
	}
	return defaults
}

// Read reads the configuration from a yaml file into the given interface structure.
// In case path is not set this method reads from the default configuration file.
func Read(out interface{}, path string) error {

	if path == "" && configurationFile != "" {
		path = configurationFile
	}

	configfile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("[ConfigFile] Failed to read %s: %v. Exiting.", path, err)
	}
	// start with configuration defaults
	filecontent := ConfigDefaults()

	// append configuration, but strip away possible yaml doc separators
	scanner := bufio.NewScanner(configfile)
	for scanner.Scan() {
		line := scanner.Text()
		if match, _ := regexp.Match("^---[ \t]*$", []byte(line)); !match {
			filecontent = append(filecontent, []byte(line+"\n")...)
		}
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
