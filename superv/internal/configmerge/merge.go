// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package configmerge

import (
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
)

// MergeConfigs merges two YAML configurations, with override taking precedence.
func MergeConfigs(base, override []byte) ([]byte, error) {
	k := koanf.New(".")

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
	k := koanf.New(".")

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
func InjectSettings(config []byte, settings map[string]interface{}) ([]byte, error) {
	k := koanf.New(".")

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
