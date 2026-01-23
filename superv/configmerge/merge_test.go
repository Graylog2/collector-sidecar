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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeConfigs_SimpleOverride(t *testing.T) {
	base := []byte(`
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: "0.0.0.0:4317"
exporters:
  debug: {}
`)
	override := []byte(`
exporters:
  otlp:
    endpoint: "http://collector:4317"
`)

	result, err := MergeConfigs(base, override)
	require.NoError(t, err)

	// Should have both exporters
	require.Contains(t, string(result), "debug")
	require.Contains(t, string(result), "otlp")
	require.Contains(t, string(result), "http://collector:4317")
}

func TestMergeConfigs_DeepMerge(t *testing.T) {
	base := []byte(`
processors:
  batch:
    timeout: 1s
    send_batch_size: 1000
`)
	override := []byte(`
processors:
  batch:
    timeout: 5s
`)

	result, err := MergeConfigs(base, override)
	require.NoError(t, err)

	// Override should win for timeout
	require.Contains(t, string(result), "5s")
	// But send_batch_size should be preserved
	require.Contains(t, string(result), "send_batch_size")
}

func TestMergeConfigs_MultipleOverrides(t *testing.T) {
	configs := [][]byte{
		[]byte(`a: 1`),
		[]byte(`b: 2`),
		[]byte(`c: 3`),
	}

	result, err := MergeMultiple(configs...)
	require.NoError(t, err)

	require.Contains(t, string(result), "a: 1")
	require.Contains(t, string(result), "b: 2")
	require.Contains(t, string(result), "c: 3")
}

func TestMergeConfigs_EmptyBase(t *testing.T) {
	base := []byte(``)
	override := []byte(`key: value`)

	result, err := MergeConfigs(base, override)
	require.NoError(t, err)
	require.Contains(t, string(result), "key: value")
}

func TestMergeConfigs_EmptyOverride(t *testing.T) {
	base := []byte(`key: value`)
	override := []byte(``)

	result, err := MergeConfigs(base, override)
	require.NoError(t, err)
	require.Contains(t, string(result), "key: value")
}

func TestInjectSettings_Basic(t *testing.T) {
	config := []byte(`receivers:
  otlp: {}
`)
	settings := map[string]interface{}{
		"exporters.debug": map[string]interface{}{},
	}

	result, err := InjectSettings(config, settings)
	require.NoError(t, err)
	require.Contains(t, string(result), "receivers")
	require.Contains(t, string(result), "exporters")
	require.Contains(t, string(result), "debug")
}

func TestInjectSettings_NestedKeyPath(t *testing.T) {
	config := []byte(`receivers:
  otlp: {}
`)
	settings := map[string]interface{}{
		"service.telemetry.logs.level": "debug",
	}

	result, err := InjectSettings(config, settings)
	require.NoError(t, err)
	require.Contains(t, string(result), "service")
	require.Contains(t, string(result), "telemetry")
	require.Contains(t, string(result), "logs")
	require.Contains(t, string(result), "level")
	require.Contains(t, string(result), "debug")
}

func TestInjectSettings_EmptyConfig(t *testing.T) {
	// Test with empty config
	settings := map[string]interface{}{
		"key": "value",
	}

	result, err := InjectSettings([]byte{}, settings)
	require.NoError(t, err)
	require.Contains(t, string(result), "key: value")

	// Test with nil config
	result, err = InjectSettings(nil, settings)
	require.NoError(t, err)
	require.Contains(t, string(result), "key: value")
}

func TestInjectSettings_OverwriteExisting(t *testing.T) {
	config := []byte(`service:
  telemetry:
    logs:
      level: info
`)
	settings := map[string]interface{}{
		"service.telemetry.logs.level": "debug",
	}

	result, err := InjectSettings(config, settings)
	require.NoError(t, err)
	// Should contain debug (the new value), not info (the old value)
	require.Contains(t, string(result), "debug")
	require.NotContains(t, string(result), "info")
}
