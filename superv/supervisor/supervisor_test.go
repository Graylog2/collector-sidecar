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

package supervisor

import (
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/persistence"
)

func TestNewSupervisor(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.DefaultConfig()
	cfg.Server.Endpoint = "ws://localhost:4320/v1/opamp"
	cfg.Agent.Executable = "/bin/echo"
	cfg.Persistence.Dir = t.TempDir()

	sup, err := New(logger, cfg)
	require.NoError(t, err)
	require.NotNil(t, sup)
}

func TestSupervisor_GetInstanceUID(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.DefaultConfig()
	cfg.Server.Endpoint = "ws://localhost:4320/v1/opamp"
	cfg.Agent.Executable = "/bin/echo"
	cfg.Persistence.Dir = t.TempDir()

	sup, err := New(logger, cfg)
	require.NoError(t, err)

	uid := sup.InstanceUID()
	require.NotEmpty(t, uid)

	// Second call should return same UID
	uid2 := sup.InstanceUID()
	require.Equal(t, uid, uid2)
}

func TestSupervisor_IsRunning(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.DefaultConfig()
	cfg.Server.Endpoint = "ws://localhost:4320/v1/opamp"
	cfg.Agent.Executable = "/bin/echo"
	cfg.Persistence.Dir = t.TempDir()

	sup, err := New(logger, cfg)
	require.NoError(t, err)

	// Initially not running
	require.False(t, sup.IsRunning())
}

func TestNewSupervisor_PersistenceError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.DefaultConfig()
	cfg.Server.Endpoint = "ws://localhost:4320/v1/opamp"
	cfg.Agent.Executable = "/bin/echo"
	// Set to a path that won't be writable
	cfg.Persistence.Dir = "/nonexistent/path/that/should/fail"

	sup, err := New(logger, cfg)
	require.Error(t, err)
	require.Nil(t, sup)
}

func TestSupervisor_ConfigManagerIntegration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()

	cfg := config.Config{
		Server: config.ServerConfig{
			Endpoint: "ws://localhost:4320/v1/opamp",
		},
		LocalOpAMP: config.LocalOpAMPConfig{
			Endpoint: "localhost:4321",
		},
		Agent: config.AgentConfig{
			Executable: "/bin/sleep",
			Args:       []string{"1"},
			Health: config.HealthConfig{
				Endpoint: "http://localhost:13133/health",
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
		Persistence: config.PersistenceConfig{
			Dir: dir,
		},
	}

	supervisor, err := New(logger, cfg)
	require.NoError(t, err)
	require.NotNil(t, supervisor)

	// Verify components are created
	require.NotNil(t, supervisor.configManager)
	require.NotNil(t, supervisor.healthMonitor)
}

func TestSupervisor_OnOpampConnectionSettings_UpdatesEndpoint(t *testing.T) {
	// This is an integration test that verifies the callback behavior.
	// Full testing requires a mock OpAMP server to receive connection settings
	// and verify the client reconnects with new settings.
	t.Skip("Requires integration test setup with mock OpAMP server")
}

func TestConvertProtoHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    *protobufs.Headers
		expected map[string]string
	}{
		{
			name:     "nil headers",
			input:    nil,
			expected: nil,
		},
		{
			name: "empty headers",
			input: &protobufs.Headers{
				Headers: []*protobufs.Header{},
			},
			expected: map[string]string{},
		},
		{
			name: "single header",
			input: &protobufs.Headers{
				Headers: []*protobufs.Header{
					{Key: "Authorization", Value: "Bearer token"},
				},
			},
			expected: map[string]string{
				"Authorization": "Bearer token",
			},
		},
		{
			name: "multiple headers",
			input: &protobufs.Headers{
				Headers: []*protobufs.Header{
					{Key: "Authorization", Value: "Bearer token"},
					{Key: "X-Custom", Value: "value"},
				},
			},
			expected: map[string]string{
				"Authorization": "Bearer token",
				"X-Custom":      "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertProtoHeaders(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestHeadersEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        map[string]string
		b        map[string]string
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "one nil one empty",
			a:        nil,
			b:        map[string]string{},
			expected: true,
		},
		{
			name:     "both empty",
			a:        map[string]string{},
			b:        map[string]string{},
			expected: true,
		},
		{
			name:     "equal single",
			a:        map[string]string{"k": "v"},
			b:        map[string]string{"k": "v"},
			expected: true,
		},
		{
			name:     "different values",
			a:        map[string]string{"k": "v1"},
			b:        map[string]string{"k": "v2"},
			expected: false,
		},
		{
			name:     "different keys",
			a:        map[string]string{"k1": "v"},
			b:        map[string]string{"k2": "v"},
			expected: false,
		},
		{
			name:     "different lengths",
			a:        map[string]string{"k1": "v1"},
			b:        map[string]string{"k1": "v1", "k2": "v2"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := headersEqual(tt.a, tt.b)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestSupervisor_LoadPersistedConnectionSettings(t *testing.T) {
	tmpDir := t.TempDir()
	logger := zaptest.NewLogger(t)

	// Save some settings first
	settings := &persistence.OpAMPSettings{
		Endpoint:  "wss://persisted-server.example.com/opamp",
		Headers:   map[string]string{"X-Custom": "value"},
		UpdatedAt: time.Now(),
	}
	require.NoError(t, persistence.SaveOpAMPSettings(tmpDir, settings))

	// Create supervisor with different initial endpoint
	sup, err := New(logger, config.Config{
		Persistence: config.PersistenceConfig{Dir: tmpDir},
		Server:      config.ServerConfig{Endpoint: "wss://initial.example.com/opamp"},
		Agent: config.AgentConfig{
			Executable: "/bin/true",
		},
	})
	require.NoError(t, err)

	// Load persisted settings
	err = sup.loadPersistedConnectionSettings()
	require.NoError(t, err)

	// Verify endpoint was updated from persisted settings
	assert.Equal(t, "wss://persisted-server.example.com/opamp", sup.cfg.Server.Endpoint)
	assert.Equal(t, "value", sup.cfg.Server.Headers["X-Custom"])
}

func TestSupervisor_LoadPersistedConnectionSettings_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	logger := zaptest.NewLogger(t)

	// Create supervisor without persisted settings
	sup, err := New(logger, config.Config{
		Persistence: config.PersistenceConfig{Dir: tmpDir},
		Server:      config.ServerConfig{Endpoint: "wss://initial.example.com/opamp"},
		Agent: config.AgentConfig{
			Executable: "/bin/true",
		},
	})
	require.NoError(t, err)

	// Load persisted settings (should succeed with no file)
	err = sup.loadPersistedConnectionSettings()
	require.NoError(t, err)

	// Verify initial endpoint unchanged
	assert.Equal(t, "wss://initial.example.com/opamp", sup.cfg.Server.Endpoint)
}

func TestSupervisor_LoadPersistedConnectionSettings_TLSSettings(t *testing.T) {
	tmpDir := t.TempDir()
	logger := zaptest.NewLogger(t)

	// Save settings with CA cert
	settings := &persistence.OpAMPSettings{
		Endpoint:  "wss://secure-server.example.com/opamp",
		CACertPEM: "-----BEGIN CERTIFICATE-----\ntest cert\n-----END CERTIFICATE-----",
		UpdatedAt: time.Now(),
	}
	require.NoError(t, persistence.SaveOpAMPSettings(tmpDir, settings))

	// Create supervisor
	sup, err := New(logger, config.Config{
		Persistence: config.PersistenceConfig{Dir: tmpDir},
		Server:      config.ServerConfig{Endpoint: "wss://initial.example.com/opamp"},
		Agent: config.AgentConfig{
			Executable: "/bin/true",
		},
	})
	require.NoError(t, err)

	// Load persisted settings
	err = sup.loadPersistedConnectionSettings()
	require.NoError(t, err)

	// Verify TLS settings were applied
	assert.Equal(t, "wss://secure-server.example.com/opamp", sup.cfg.Server.Endpoint)
	assert.Equal(t, "-----BEGIN CERTIFICATE-----\ntest cert\n-----END CERTIFICATE-----", sup.cfg.Server.TLS.CACert)
}

func TestSupervisor_LoadPersistedConnectionSettings_PartialSettings(t *testing.T) {
	tmpDir := t.TempDir()
	logger := zaptest.NewLogger(t)

	// Save settings with only headers (no endpoint)
	settings := &persistence.OpAMPSettings{
		Headers:   map[string]string{"X-Token": "secret"},
		UpdatedAt: time.Now(),
	}
	require.NoError(t, persistence.SaveOpAMPSettings(tmpDir, settings))

	// Create supervisor with initial endpoint
	sup, err := New(logger, config.Config{
		Persistence: config.PersistenceConfig{Dir: tmpDir},
		Server:      config.ServerConfig{Endpoint: "wss://initial.example.com/opamp"},
		Agent: config.AgentConfig{
			Executable: "/bin/true",
		},
	})
	require.NoError(t, err)

	// Load persisted settings
	err = sup.loadPersistedConnectionSettings()
	require.NoError(t, err)

	// Verify initial endpoint unchanged but headers applied
	assert.Equal(t, "wss://initial.example.com/opamp", sup.cfg.Server.Endpoint)
	assert.Equal(t, "secret", sup.cfg.Server.Headers["X-Token"])
}
