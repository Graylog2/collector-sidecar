// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"os"
	"regexp"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

// envPattern matches ${VAR} patterns for environment variable expansion.
var envPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Load loads configuration from a YAML file, expanding environment variables
// and merging with defaults.
func Load(path string) (Config, error) {
	if path == "" {
		return Config{}, errors.New("config path cannot be empty")
	}

	k := koanf.New(".")

	// Load defaults first using structs provider
	defaults := DefaultConfig()
	if err := k.Load(structs.Provider(defaults, "koanf"), nil); err != nil {
		return Config{}, err
	}

	// Load from file (merges with defaults, file values take precedence)
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, err
	}

	// Expand environment variables in string fields
	expandEnvVars(&cfg)

	return cfg, nil
}

// expandEnvVars expands ${VAR} patterns in config string fields.
func expandEnvVars(cfg *Config) {
	expand := func(s string) string {
		return envPattern.ReplaceAllStringFunc(s, func(match string) string {
			varName := strings.TrimPrefix(strings.TrimSuffix(match, "}"), "${")
			if val := os.Getenv(varName); val != "" {
				return val
			}
			return match
		})
	}

	cfg.Server.Endpoint = expand(cfg.Server.Endpoint)
	cfg.Auth.EnrollmentToken = expand(cfg.Auth.EnrollmentToken)
	cfg.Auth.TokenFile = expand(cfg.Auth.TokenFile)
	cfg.Agent.Executable = expand(cfg.Agent.Executable)
	cfg.Persistence.Dir = expand(cfg.Persistence.Dir)
	cfg.Packages.StorageDir = expand(cfg.Packages.StorageDir)

	// Expand headers
	for k, v := range cfg.Server.Headers {
		cfg.Server.Headers[k] = expand(v)
	}

	// Expand args
	for i, arg := range cfg.Agent.Args {
		cfg.Agent.Args[i] = expand(arg)
	}

	// Expand env vars in agent env
	for k, v := range cfg.Agent.Env {
		cfg.Agent.Env[k] = expand(v)
	}
}
