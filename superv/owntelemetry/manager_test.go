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

package owntelemetry

import (
	"context"
	"net/http"
	"testing"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestManager_Core_InitiallyDisabled(t *testing.T) {
	m := NewManager(config.TelemetryLogsConfig{})
	core := m.Core()
	assert.False(t, core.Enabled(zapcore.InfoLevel))
}

func TestManager_Shutdown_WhenNeverApplied(t *testing.T) {
	m := NewManager(config.TelemetryLogsConfig{})
	err := m.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestNewHTTPClient_UsesProxyHeaders(t *testing.T) {
	client, err := newHTTPClient(Settings{
		ProxyURL: "http://proxy.example:8080",
		ProxyHeaders: map[string]string{
			"Proxy-Authorization": "Basic abc123",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.Proxy)
	assert.Equal(t, "Basic abc123", transport.ProxyConnectHeader.Get("Proxy-Authorization"))
}

func TestNewHTTPClient_RejectsHeadersWithoutProxyURL(t *testing.T) {
	client, err := newHTTPClient(Settings{
		ProxyHeaders: map[string]string{"Proxy-Authorization": "Basic abc123"},
	})
	require.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "proxy headers require a proxy URL")
}

func TestIsGRPC(t *testing.T) {
	tests := []struct {
		endpoint string
		want     bool
	}{
		{"https://host:4317", true},
		{"https://host:4317/", true},
		{"https://host:4317?foo=bar", true},
		{"https://host:4317/?foo=bar", true},
		{"https://host:4317/v1/logs", false},
		{"https://host:43170", false},
		{"https://host:43179", false},
		{"https://host:4318", false},
		{"https://host:4318/v1/logs", false},
		{"https://host:443/v1/logs", false},
		{"https://host", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			assert.Equal(t, tt.want, isGRPC(tt.endpoint), "isGRPC(%q)", tt.endpoint)
		})
	}
}

func TestIsGRPC_MetricsPath(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     bool
	}{
		{"HTTP metrics path", "https://example.com:4318/v1/metrics", false},
		{"HTTP logs path", "https://example.com:4318/v1/logs", false},
		{"gRPC port", "https://example.com:4317", true},
		{"HTTP default", "https://example.com:4318", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGRPC(tt.endpoint); got != tt.want {
				t.Errorf("isGRPC(%q) = %v, want %v", tt.endpoint, got, tt.want)
			}
		})
	}
}

func TestBuildGRPCExporter_RejectsProxySettings(t *testing.T) {
	_, err := buildGRPCExporter(context.Background(), Settings{
		Endpoint:  "https://example.com:4317",
		ProxyURL:  "http://proxy.example:8080",
		Insecure:  false,
		Headers:   map[string]string{"Authorization": "Bearer token"},
		LogLevel:  "info",
		TLSConfig: nil,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "proxy settings are not supported for gRPC")
}
