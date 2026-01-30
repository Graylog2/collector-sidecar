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

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
)

func TestInjectOpAMPExtension(t *testing.T) {
	config := []byte(`
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: "0.0.0.0:4317"
exporters:
  debug: {}
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
`)

	result, err := InjectOpAMPExtension(config, "ws://localhost:4320/v1/opamp", "test-instance-123")
	require.NoError(t, err)

	// Parse result to verify structure
	var parsed map[string]interface{}
	err = yaml.Unmarshal(result, &parsed)
	require.NoError(t, err)

	// Verify extensions.opamp exists with correct settings
	extensions, ok := parsed["extensions"].(map[string]interface{})
	require.True(t, ok, "extensions should exist")

	opamp, ok := extensions["opamp"].(map[string]interface{})
	require.True(t, ok, "extensions.opamp should exist")

	server, ok := opamp["server"].(map[string]interface{})
	require.True(t, ok, "extensions.opamp.server should exist")

	ws, ok := server["ws"].(map[string]interface{})
	require.True(t, ok, "extensions.opamp.server.ws should exist")

	endpoint, ok := ws["endpoint"].(string)
	require.True(t, ok, "extensions.opamp.server.ws.endpoint should be a string")
	require.Equal(t, "ws://localhost:4320/v1/opamp", endpoint)

	instanceUID, ok := opamp["instance_uid"].(string)
	require.True(t, ok, "extensions.opamp.instance_uid should be a string")
	require.Equal(t, "test-instance-123", instanceUID)

	// Verify service.extensions includes opamp
	service, ok := parsed["service"].(map[string]interface{})
	require.True(t, ok, "service should exist")

	serviceExtensions, ok := service["extensions"].([]interface{})
	require.True(t, ok, "service.extensions should be a list")
	require.Contains(t, serviceExtensions, "opamp")

	// Verify original config is preserved
	require.Contains(t, parsed, "receivers")
	require.Contains(t, parsed, "exporters")
}

func TestInjectOpAMPExtension_ExistingExtensions(t *testing.T) {
	config := []byte(`
receivers:
  otlp: {}
exporters:
  debug: {}
extensions:
  health_check:
    endpoint: "0.0.0.0:13133"
service:
  extensions: [health_check]
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
`)

	result, err := InjectOpAMPExtension(config, "ws://localhost:4320/v1/opamp", "test-instance-456")
	require.NoError(t, err)

	// Parse result to verify structure
	var parsed map[string]interface{}
	err = yaml.Unmarshal(result, &parsed)
	require.NoError(t, err)

	// Verify both extensions exist
	extensions, ok := parsed["extensions"].(map[string]interface{})
	require.True(t, ok, "extensions should exist")

	_, ok = extensions["health_check"].(map[string]interface{})
	require.True(t, ok, "extensions.health_check should be preserved")

	opamp, ok := extensions["opamp"].(map[string]interface{})
	require.True(t, ok, "extensions.opamp should exist")

	// Verify opamp config is correct
	instanceUID, ok := opamp["instance_uid"].(string)
	require.True(t, ok)
	require.Equal(t, "test-instance-456", instanceUID)

	// Verify service.extensions includes both
	service, ok := parsed["service"].(map[string]interface{})
	require.True(t, ok)

	serviceExtensions, ok := service["extensions"].([]interface{})
	require.True(t, ok, "service.extensions should be a list")
	require.Contains(t, serviceExtensions, "health_check")
	require.Contains(t, serviceExtensions, "opamp")
}

func TestInjectOpAMPExtension_EmptyConfig(t *testing.T) {
	// Test with empty config
	result, err := InjectOpAMPExtension([]byte{}, "ws://localhost:4320/v1/opamp", "empty-test")
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = yaml.Unmarshal(result, &parsed)
	require.NoError(t, err)

	// Verify opamp extension is injected
	extensions, ok := parsed["extensions"].(map[string]interface{})
	require.True(t, ok)

	opamp, ok := extensions["opamp"].(map[string]interface{})
	require.True(t, ok)

	instanceUID, ok := opamp["instance_uid"].(string)
	require.True(t, ok)
	require.Equal(t, "empty-test", instanceUID)

	// Verify service.extensions includes opamp
	service, ok := parsed["service"].(map[string]interface{})
	require.True(t, ok)

	serviceExtensions, ok := service["extensions"].([]interface{})
	require.True(t, ok)
	require.Contains(t, serviceExtensions, "opamp")

	// Test with nil config
	result, err = InjectOpAMPExtension(nil, "ws://localhost:4320/v1/opamp", "nil-test")
	require.NoError(t, err)

	err = yaml.Unmarshal(result, &parsed)
	require.NoError(t, err)

	extensions, ok = parsed["extensions"].(map[string]interface{})
	require.True(t, ok)

	opamp, ok = extensions["opamp"].(map[string]interface{})
	require.True(t, ok)

	instanceUID, ok = opamp["instance_uid"].(string)
	require.True(t, ok)
	require.Equal(t, "nil-test", instanceUID)
}

func TestInjectOpAMPExtension_OpAMPAlreadyInServiceExtensions(t *testing.T) {
	// Config already has opamp in service.extensions but no extension definition
	config := []byte(`
receivers:
  otlp: {}
service:
  extensions: [opamp]
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
`)

	result, err := InjectOpAMPExtension(config, "ws://localhost:4320/v1/opamp", "test-123")
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = yaml.Unmarshal(result, &parsed)
	require.NoError(t, err)

	// Verify opamp extension is added
	extensions, ok := parsed["extensions"].(map[string]interface{})
	require.True(t, ok)

	opamp, ok := extensions["opamp"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "test-123", opamp["instance_uid"])

	// Verify service.extensions has opamp exactly once
	service := parsed["service"].(map[string]interface{})
	serviceExtensions := service["extensions"].([]interface{})

	count := 0
	for _, ext := range serviceExtensions {
		if ext == "opamp" {
			count++
		}
	}
	require.Equal(t, 1, count, "opamp should appear exactly once in service.extensions")
}

func TestInjectServiceExtension(t *testing.T) {
	config := []byte(`
receivers:
  otlp: {}
service:
  pipelines:
    traces:
      receivers: [otlp]
`)

	result, err := InjectServiceExtension(config, "opamp")
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = yaml.Unmarshal(result, &parsed)
	require.NoError(t, err)

	service := parsed["service"].(map[string]interface{})
	serviceExtensions := service["extensions"].([]interface{})
	require.Contains(t, serviceExtensions, "opamp")
}

func TestInjectServiceExtension_ExistingExtensions(t *testing.T) {
	config := []byte(`
service:
  extensions: [health_check, zpages]
`)

	result, err := InjectServiceExtension(config, "opamp")
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = yaml.Unmarshal(result, &parsed)
	require.NoError(t, err)

	service := parsed["service"].(map[string]interface{})
	serviceExtensions := service["extensions"].([]interface{})
	require.Contains(t, serviceExtensions, "health_check")
	require.Contains(t, serviceExtensions, "zpages")
	require.Contains(t, serviceExtensions, "opamp")
}

func TestInjectServiceExtension_AlreadyPresent(t *testing.T) {
	config := []byte(`
service:
  extensions: [opamp, health_check]
`)

	result, err := InjectServiceExtension(config, "opamp")
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = yaml.Unmarshal(result, &parsed)
	require.NoError(t, err)

	service := parsed["service"].(map[string]interface{})
	serviceExtensions := service["extensions"].([]interface{})

	// Count occurrences
	count := 0
	for _, ext := range serviceExtensions {
		if ext == "opamp" {
			count++
		}
	}
	require.Equal(t, 1, count, "opamp should appear exactly once")
}

func TestInjectServiceExtension_EmptyConfig(t *testing.T) {
	result, err := InjectServiceExtension([]byte{}, "opamp")
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = yaml.Unmarshal(result, &parsed)
	require.NoError(t, err)

	service := parsed["service"].(map[string]interface{})
	serviceExtensions := service["extensions"].([]interface{})
	require.Contains(t, serviceExtensions, "opamp")
}

func TestInjectHealthCheckExtension(t *testing.T) {
	config := []byte(`
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: "0.0.0.0:4317"
exporters:
  debug: {}
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
`)

	result, err := InjectHealthCheckExtension(config, HealthCheckConfig{
		Endpoint: "localhost:13133",
	})
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = yaml.Unmarshal(result, &parsed)
	require.NoError(t, err)

	// Verify extensions.health_check exists with correct settings
	extensions, ok := parsed["extensions"].(map[string]interface{})
	require.True(t, ok, "extensions should exist")

	healthCheck, ok := extensions["health_check"].(map[string]interface{})
	require.True(t, ok, "extensions.health_check should exist")

	endpoint, ok := healthCheck["endpoint"].(string)
	require.True(t, ok, "extensions.health_check.endpoint should be a string")
	require.Equal(t, "localhost:13133", endpoint)

	// Verify service.extensions includes health_check
	service, ok := parsed["service"].(map[string]interface{})
	require.True(t, ok, "service should exist")

	serviceExtensions, ok := service["extensions"].([]interface{})
	require.True(t, ok, "service.extensions should be a list")
	require.Contains(t, serviceExtensions, "health_check")
}

func TestInjectHealthCheckExtension_WithPath(t *testing.T) {
	config := []byte(`
receivers:
  otlp: {}
service:
  pipelines:
    traces:
      receivers: [otlp]
`)

	result, err := InjectHealthCheckExtension(config, HealthCheckConfig{
		Endpoint: "0.0.0.0:13133",
		Path:     "/health/status",
	})
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = yaml.Unmarshal(result, &parsed)
	require.NoError(t, err)

	extensions := parsed["extensions"].(map[string]interface{})
	healthCheck := extensions["health_check"].(map[string]interface{})

	require.Equal(t, "0.0.0.0:13133", healthCheck["endpoint"])
	require.Equal(t, "/health/status", healthCheck["path"])
}

func TestInjectHealthCheckExtension_ExistingExtensions(t *testing.T) {
	config := []byte(`
extensions:
  zpages:
    endpoint: "localhost:55679"
service:
  extensions: [zpages]
`)

	result, err := InjectHealthCheckExtension(config, HealthCheckConfig{
		Endpoint: "localhost:13133",
	})
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = yaml.Unmarshal(result, &parsed)
	require.NoError(t, err)

	extensions := parsed["extensions"].(map[string]interface{})

	// Verify zpages is preserved
	_, ok := extensions["zpages"].(map[string]interface{})
	require.True(t, ok, "extensions.zpages should be preserved")

	// Verify health_check is added
	_, ok = extensions["health_check"].(map[string]interface{})
	require.True(t, ok, "extensions.health_check should exist")

	// Verify service.extensions includes both
	service := parsed["service"].(map[string]interface{})
	serviceExtensions := service["extensions"].([]interface{})
	require.Contains(t, serviceExtensions, "zpages")
	require.Contains(t, serviceExtensions, "health_check")
}
