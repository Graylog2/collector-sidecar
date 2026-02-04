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

package components

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseComponentsOutput(t *testing.T) {
	input := `buildinfo:
    command: graylog-collector
    description: Graylog Collector
    version: 2.0.0-alpha.0
receivers:
    - name: journald
      module: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/journaldreceiver v0.144.0
      stability:
        logs: Alpha
        metrics: Undefined
        traces: Undefined
    - name: otlp
      module: go.opentelemetry.io/collector/receiver/otlpreceiver v0.144.0
      stability:
        logs: Stable
        metrics: Stable
        traces: Stable
processors: []
exporters:
    - name: debug
      module: go.opentelemetry.io/collector/exporter/debugexporter v0.144.0
      stability:
        logs: Alpha
        metrics: Alpha
        traces: Alpha
connectors: []
extensions:
    - name: opamp
      module: github.com/open-telemetry/opentelemetry-collector-contrib/extension/opampextension v0.144.0
      stability:
        extension: Alpha
    - name: sidecar
      module: github.com/Graylog2/collector-sidecar/extension/sidecar v1.5.0
      stability:
        extension: Stable
providers:
    - scheme: env
      module: go.opentelemetry.io/collector/confmap/provider/envprovider v1.50.0
    - scheme: file
      module: go.opentelemetry.io/collector/confmap/provider/fileprovider v1.50.0
`

	components, err := ParseComponentsOutput([]byte(input))
	require.NoError(t, err)
	require.NotNil(t, components)

	// Check build info
	assert.Equal(t, "graylog-collector", components.BuildInfo.Command)
	assert.Equal(t, "Graylog Collector", components.BuildInfo.Description)
	assert.Equal(t, "2.0.0-alpha.0", components.BuildInfo.Version)

	// Check receivers
	assert.Len(t, components.Receivers, 2)
	assert.Equal(t, "journald", components.Receivers[0].Name)
	assert.Contains(t, components.Receivers[0].Module, "journaldreceiver")
	assert.Equal(t, "Alpha", components.Receivers[0].Stability.Logs)
	assert.Equal(t, "otlp", components.Receivers[1].Name)
	assert.Equal(t, "Stable", components.Receivers[1].Stability.Logs)

	// Check processors (empty)
	assert.Empty(t, components.Processors)

	// Check exporters
	assert.Len(t, components.Exporters, 1)
	assert.Equal(t, "debug", components.Exporters[0].Name)

	// Check extensions
	assert.Len(t, components.Extensions, 2)
	assert.Equal(t, "opamp", components.Extensions[0].Name)
	assert.Equal(t, "Alpha", components.Extensions[0].Stability.Extension)
	assert.Equal(t, "sidecar", components.Extensions[1].Name)
	assert.Equal(t, "Stable", components.Extensions[1].Stability.Extension)

	// Check connectors (empty)
	assert.Empty(t, components.Connectors)

	// Check providers
	assert.Len(t, components.Providers, 2)
	assert.Equal(t, "env", components.Providers[0].Scheme)
	assert.Equal(t, "file", components.Providers[1].Scheme)
}

func TestParseComponentsOutput_Empty(t *testing.T) {
	input := `buildinfo:
    command: test
    description: Test
    version: 1.0.0
receivers: []
processors: []
exporters: []
extensions: []
connectors: []
providers: []
`

	components, err := ParseComponentsOutput([]byte(input))
	require.NoError(t, err)
	require.NotNil(t, components)

	assert.Empty(t, components.Receivers)
	assert.Empty(t, components.Processors)
	assert.Empty(t, components.Exporters)
	assert.Empty(t, components.Extensions)
	assert.Empty(t, components.Connectors)
	assert.True(t, components.IsEmpty())
}

func TestComponents_ToProto(t *testing.T) {
	components := &Components{
		Receivers: []Component{
			{Name: "otlp", Module: "go.opentelemetry.io/collector/receiver/otlpreceiver v0.144.0", Stability: Stability{Logs: "Stable", Metrics: "Stable", Traces: "Stable"}},
			{Name: "prometheus", Module: "github.com/prometheus/prometheus v2.50.0"},
		},
		Processors: []Component{
			{Name: "batch", Module: "go.opentelemetry.io/collector/processor/batchprocessor v0.144.0"},
		},
		Exporters: []Component{
			{Name: "logging", Module: "go.opentelemetry.io/collector/exporter/loggingexporter v0.144.0"},
		},
		Extensions: []Component{
			{Name: "health_check", Module: "go.opentelemetry.io/collector/extension/healthcheckextension v0.144.0", Stability: Stability{Extension: "Stable"}},
		},
		Connectors: []Component{
			{Name: "forward", Module: "go.opentelemetry.io/collector/connector/forwardconnector v0.144.0"},
		},
	}

	proto := components.ToProto()
	require.NotNil(t, proto)
	require.NotNil(t, proto.Components)

	// Check receivers
	receiver, ok := proto.Components["receiver/otlp"]
	assert.True(t, ok)
	assert.NotNil(t, receiver.Metadata)
	// Check metadata contains package and version
	var hasPackage, hasVersion bool
	for _, kv := range receiver.Metadata {
		if kv.Key == "component.package" {
			hasPackage = true
			assert.Contains(t, kv.Value.GetStringValue(), "otlpreceiver")
		}
		if kv.Key == "component.version" {
			hasVersion = true
			assert.Equal(t, "v0.144.0", kv.Value.GetStringValue())
		}
	}
	assert.True(t, hasPackage, "should have component.package metadata")
	assert.True(t, hasVersion, "should have component.version metadata")

	_, ok = proto.Components["receiver/prometheus"]
	assert.True(t, ok)

	// Check processors
	_, ok = proto.Components["processor/batch"]
	assert.True(t, ok)

	// Check exporters
	_, ok = proto.Components["exporter/logging"]
	assert.True(t, ok)

	// Check extensions
	_, ok = proto.Components["extension/health_check"]
	assert.True(t, ok)

	// Check connectors
	_, ok = proto.Components["connector/forward"]
	assert.True(t, ok)

	// Total count
	assert.Len(t, proto.Components, 6)
}

func TestComponents_ToProto_Nil(t *testing.T) {
	var components *Components
	proto := components.ToProto()
	assert.Nil(t, proto)
}

func TestComponents_ToProto_Empty(t *testing.T) {
	components := &Components{}
	proto := components.ToProto()
	require.NotNil(t, proto)
	assert.Empty(t, proto.Components)
}

func TestComponents_Count(t *testing.T) {
	components := &Components{
		Receivers:  []Component{{Name: "a"}, {Name: "b"}},
		Processors: []Component{{Name: "c"}},
		Exporters:  []Component{{Name: "d"}, {Name: "e"}, {Name: "f"}},
		Extensions: []Component{},
	}

	assert.Equal(t, 6, components.Count())
}

func TestComponents_Count_Nil(t *testing.T) {
	var components *Components
	assert.Equal(t, 0, components.Count())
}

func TestComponents_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		comp     *Components
		expected bool
	}{
		{
			name:     "nil",
			comp:     nil,
			expected: true,
		},
		{
			name:     "empty struct",
			comp:     &Components{},
			expected: true,
		},
		{
			name:     "with receivers",
			comp:     &Components{Receivers: []Component{{Name: "a"}}},
			expected: false,
		},
		{
			name:     "with processors",
			comp:     &Components{Processors: []Component{{Name: "a"}}},
			expected: false,
		},
		{
			name:     "with exporters",
			comp:     &Components{Exporters: []Component{{Name: "a"}}},
			expected: false,
		},
		{
			name:     "with extensions",
			comp:     &Components{Extensions: []Component{{Name: "a"}}},
			expected: false,
		},
		{
			name:     "with connectors",
			comp:     &Components{Connectors: []Component{{Name: "a"}}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.comp.IsEmpty())
		})
	}
}

func TestComponents_Names(t *testing.T) {
	components := &Components{
		Receivers:  []Component{{Name: "otlp"}, {Name: "prometheus"}},
		Processors: []Component{{Name: "batch"}},
		Exporters:  []Component{{Name: "logging"}},
		Extensions: []Component{{Name: "health_check"}},
		Connectors: []Component{{Name: "forward"}},
	}

	assert.Equal(t, []string{"otlp", "prometheus"}, components.ReceiverNames())
	assert.Equal(t, []string{"batch"}, components.ProcessorNames())
	assert.Equal(t, []string{"logging"}, components.ExporterNames())
	assert.Equal(t, []string{"health_check"}, components.ExtensionNames())
	assert.Equal(t, []string{"forward"}, components.ConnectorNames())
}

func TestComponents_Names_Nil(t *testing.T) {
	var components *Components
	assert.Nil(t, components.ReceiverNames())
	assert.Nil(t, components.ProcessorNames())
	assert.Nil(t, components.ExporterNames())
	assert.Nil(t, components.ExtensionNames())
	assert.Nil(t, components.ConnectorNames())
}

func TestToProto_HashIsDeterministic(t *testing.T) {
	components := &Components{
		Receivers: []Component{
			{Name: "otlp", Module: "go.opentelemetry.io/collector/receiver/otlpreceiver v0.144.0"},
			{Name: "prometheus", Module: "github.com/prometheus/prometheus v2.50.0"},
		},
		Processors: []Component{
			{Name: "batch", Module: "go.opentelemetry.io/collector/processor/batchprocessor v0.144.0"},
		},
	}

	proto1 := components.ToProto()
	proto2 := components.ToProto()

	// Hash should be set
	assert.NotEmpty(t, proto1.Hash)
	assert.NotEmpty(t, proto2.Hash)

	// Hash should be deterministic
	assert.Equal(t, proto1.Hash, proto2.Hash)
}

func TestToProto_EmptyComponentsHasHash(t *testing.T) {
	components := &Components{}
	proto := components.ToProto()

	assert.NotNil(t, proto)
	assert.NotEmpty(t, proto.Hash, "empty components should still have a hash")
	assert.Empty(t, proto.Components)
}

func TestSplitModule(t *testing.T) {
	tests := []struct {
		module      string
		wantPkg     string
		wantVersion string
	}{
		{
			module:      "go.opentelemetry.io/collector/receiver/otlpreceiver v0.144.0",
			wantPkg:     "go.opentelemetry.io/collector/receiver/otlpreceiver",
			wantVersion: "v0.144.0",
		},
		{
			module:      "github.com/Graylog2/collector-sidecar/extension/sidecar v1.5.0",
			wantPkg:     "github.com/Graylog2/collector-sidecar/extension/sidecar",
			wantVersion: "v1.5.0",
		},
		{
			module:      "somepackage",
			wantPkg:     "somepackage",
			wantVersion: "",
		},
		{
			module:      "",
			wantPkg:     "",
			wantVersion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.module, func(t *testing.T) {
			pkg, version := splitModule(tt.module)
			assert.Equal(t, tt.wantPkg, pkg)
			assert.Equal(t, tt.wantVersion, version)
		})
	}
}
