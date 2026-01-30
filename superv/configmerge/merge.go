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

package configmerge

import (
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
)

// MergeConfigs merges two YAML configurations, with override taking precedence.
func MergeConfigs(base, override []byte) ([]byte, error) {
	k := koanf.New("::")

	// Load base config
	if len(base) > 0 {
		if err := k.Load(rawbytes.Provider(base), yaml.Parser()); err != nil {
			return nil, err
		}
	}

	// Merge override config
	if len(override) > 0 {
		if err := k.Load(rawbytes.Provider(override), yaml.Parser()); err != nil {
			return nil, err
		}
	}

	// Marshal back to YAML
	return k.Marshal(yaml.Parser())
}

// MergeMultiple merges multiple YAML configurations in order.
// Later configs take precedence over earlier ones.
func MergeMultiple(configs ...[]byte) ([]byte, error) {
	k := koanf.New("::")

	for _, cfg := range configs {
		if len(cfg) > 0 {
			if err := k.Load(rawbytes.Provider(cfg), yaml.Parser()); err != nil {
				return nil, err
			}
		}
	}

	return k.Marshal(yaml.Parser())
}

// InjectSettings injects supervisor settings into a collector config.
func InjectSettings(config []byte, settings map[string]any) ([]byte, error) {
	// TODO: Check how the reference implementation handles nested keys and arrays.
	//       Currently, this implementation will overwrite entire sections if nested keys are provided.
	k := koanf.New("::")

	// Load existing config
	if len(config) > 0 {
		if err := k.Load(rawbytes.Provider(config), yaml.Parser()); err != nil {
			return nil, err
		}
	}

	// Inject settings
	for key, value := range settings {
		k.Set(key, value)
	}

	return k.Marshal(yaml.Parser())
}
