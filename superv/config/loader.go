// Copyright (C)  2026 Graylog, Inc.
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
//
// SPDX-License-Identifier: SSPL-1.0

package config

import (
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

const envPrefix = "GLC_"

func DefaultConfigPaths() []string {
	return []string{"/etc/graylog/collector/supervisor.yaml", "./supervisor.yaml"}
}

// Load loads configuration from a YAML file, merging with defaults.
// Environment variables with the GLC_ prefix override config values
// (e.g., GLC_SERVER_ENDPOINT overrides server.endpoint).
func Load(path string) (Config, error) {
	k := koanf.New("::")

	// Load defaults first using structs provider
	defaults := DefaultConfig()
	if err := k.Load(structs.Provider(defaults, "koanf"), nil); err != nil {
		return Config{}, err
	}

	if path != "" {
		// Load from file (merges with defaults, file values take precedence)
		if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
			// It's okay to run the supervisor without config file
			if !os.IsNotExist(err) {
				return Config{}, err
			}
		}
	}

	// Load environment variables (highest precedence)
	// GLC_SERVER__ENDPOINT -> server::endpoint
	if err := k.Load(env.Provider("::", env.Opt{
		Prefix: envPrefix,
		TransformFunc: func(k, v string) (string, any) {
			return strings.Replace(
				strings.ToLower(strings.TrimPrefix(k, envPrefix)),
				"__", "::", -1), v
		},
	}), nil); err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
