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
	"github.com/knadh/koanf/maps"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
)

// deduplicateSlice removes duplicates from a slice while preserving order.
func deduplicateSlice(slice []any) []any {
	seen := make(map[any]struct{}, len(slice))
	result := make([]any, 0, len(slice))
	for _, v := range slice {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

// collectorConfigMerge is a custom merge function for koanf that handles
// OTel Collector config semantics. Extension lists in service.extensions
// are concatenated and deduplicated rather than overwritten.
func collectorConfigMerge(src, dest map[string]any) error {
	// Capture extension lists before standard merge overwrites them
	srcExt := maps.Search(src, []string{"service", "extensions"})
	destExt := maps.Search(dest, []string{"service", "extensions"})

	// Standard map merge (this overwrites arrays)
	maps.Merge(src, dest)

	// Restore concatenated, deduplicated extensions
	destSlice, _ := destExt.([]any)
	srcSlice, _ := srcExt.([]any)

	if len(destSlice) > 0 || len(srcSlice) > 0 {
		merged := deduplicateSlice(append(destSlice, srcSlice...))
		if service, ok := dest["service"].(map[string]any); ok {
			service["extensions"] = merged
		}
	}

	return nil
}

// MergeConfigs merges two YAML configurations with collector-aware semantics.
// Extension lists in service.extensions are concatenated and deduplicated.
func MergeConfigs(base, override []byte) ([]byte, error) {
	k := koanf.New("::")

	// Load base config
	if len(base) > 0 {
		if err := k.Load(rawbytes.Provider(base), yaml.Parser()); err != nil {
			return nil, err
		}
	}

	// Merge override config with custom merge function
	if len(override) > 0 {
		if err := k.Load(rawbytes.Provider(override), yaml.Parser(),
			koanf.WithMergeFunc(collectorConfigMerge)); err != nil {
			return nil, err
		}
	}

	// Marshal back to YAML
	return k.Marshal(yaml.Parser())
}

// MergeMultiple merges multiple YAML configurations in order with collector-aware semantics.
// Later configs take precedence over earlier ones.
// Extension lists in service.extensions are concatenated and deduplicated.
func MergeMultiple(configs ...[]byte) ([]byte, error) {
	k := koanf.New("::")

	for i, cfg := range configs {
		if len(cfg) == 0 {
			continue
		}

		var opts []koanf.Option
		if i > 0 {
			// Use custom merge for all configs after the first
			opts = append(opts, koanf.WithMergeFunc(collectorConfigMerge))
		}

		if err := k.Load(rawbytes.Provider(cfg), yaml.Parser(), opts...); err != nil {
			return nil, err
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
