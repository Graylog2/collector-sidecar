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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
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
		LocalServer: config.LocalServer{
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

func TestNewSupervisor_ConnectionSettingsBootstrap(t *testing.T) {
	t.Run("uses persisted settings when server endpoint is not configured", func(t *testing.T) {
		dir := t.TempDir()
		writePersistedConnectionSettings(t, dir, connection.Settings{
			Endpoint: "wss://persisted.example.com/v1/opamp",
			Headers:  map[string]string{"X-Persisted": "true"},
		})

		logger := zaptest.NewLogger(t)
		cfg := config.DefaultConfig()
		cfg.Persistence.Dir = dir
		cfg.Keys.Dir = filepath.Join(dir, "keys")
		cfg.Agent.Executable = "/bin/echo"
		cfg.Server.Endpoint = ""

		sup, err := New(logger, cfg)
		require.NoError(t, err)

		current := sup.connectionSettingsManager.GetCurrent()
		require.Equal(t, "wss://persisted.example.com/v1/opamp", current.Endpoint)
		require.Equal(t, map[string]string{"X-Persisted": "true"}, current.Headers)
	})

	t.Run("configured endpoint overrides persisted endpoint and headers", func(t *testing.T) {
		dir := t.TempDir()
		writePersistedConnectionSettings(t, dir, connection.Settings{
			Endpoint: "wss://persisted.example.com/v1/opamp",
			Headers:  map[string]string{"X-Persisted": "true"},
		})

		logger := zaptest.NewLogger(t)
		cfg := config.DefaultConfig()
		cfg.Persistence.Dir = dir
		cfg.Keys.Dir = filepath.Join(dir, "keys")
		cfg.Agent.Executable = "/bin/echo"
		cfg.Server.Endpoint = "wss://configured.example.com/v1/opamp"
		cfg.Server.Headers = map[string]string{"X-Configured": "true"}

		sup, err := New(logger, cfg)
		require.NoError(t, err)

		current := sup.connectionSettingsManager.GetCurrent()
		require.Equal(t, "wss://configured.example.com/v1/opamp", current.Endpoint)
		require.Equal(t, map[string]string{"X-Configured": "true"}, current.Headers)
	})

	t.Run("uses enrollment endpoint when no server or persisted endpoint exists", func(t *testing.T) {
		dir := t.TempDir()
		logger := zaptest.NewLogger(t)
		cfg := config.DefaultConfig()
		cfg.Persistence.Dir = dir
		cfg.Keys.Dir = filepath.Join(dir, "keys")
		cfg.Agent.Executable = "/bin/echo"
		cfg.Server.Endpoint = ""
		cfg.Server.Auth.EnrollmentEndpoint = "wss://enroll.example.com/v1/opamp"
		cfg.Server.Auth.EnrollmentHeaders = map[string]string{"X-Enrollment": "true"}

		sup, err := New(logger, cfg)
		require.NoError(t, err)

		current := sup.connectionSettingsManager.GetCurrent()
		require.Equal(t, "wss://enroll.example.com/v1/opamp", current.Endpoint)
		require.Equal(t, map[string]string{"X-Enrollment": "true"}, current.Headers)
	})

	t.Run("returns error when no endpoint can be resolved", func(t *testing.T) {
		dir := t.TempDir()
		logger := zaptest.NewLogger(t)
		cfg := config.DefaultConfig()
		cfg.Persistence.Dir = dir
		cfg.Keys.Dir = filepath.Join(dir, "keys")
		cfg.Agent.Executable = "/bin/echo"
		cfg.Server.Endpoint = ""
		cfg.Server.Auth.EnrollmentEndpoint = ""

		sup, err := New(logger, cfg)
		require.Error(t, err)
		require.Nil(t, sup)
		require.ErrorContains(t, err, "no server endpoint configured and no persisted connection settings found")
	})
}

func writePersistedConnectionSettings(t *testing.T, dir string, settings connection.Settings) {
	t.Helper()

	manager := connection.NewSettingsManager(zap.NewNop(), dir)
	manager.SetCurrent(connection.Settings{Endpoint: "wss://bootstrap.example.com/v1/opamp"})

	stage, err := manager.StageNext(settings)
	require.NoError(t, err)
	require.NoError(t, stage.Commit())
}
