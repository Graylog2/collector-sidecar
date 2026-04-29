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
	"runtime"
	"testing"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/auth"
	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestNewSupervisor(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.DefaultConfig()
	cfg.Server.Endpoint = "ws://localhost:4320/v1/opamp"
	cfg.Agent.Executable = "/bin/echo"
	cfg.Persistence.Dir = t.TempDir()

	sup, err := New(logger, cfg, uuid.New().String())
	require.NoError(t, err)
	require.NotNil(t, sup)
}

func TestSupervisor_GetInstanceUID(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.DefaultConfig()
	cfg.Server.Endpoint = "ws://localhost:4320/v1/opamp"
	cfg.Agent.Executable = "/bin/echo"
	cfg.Persistence.Dir = t.TempDir()

	testUID := uuid.New().String()
	sup, err := New(logger, cfg, testUID)
	require.NoError(t, err)

	uid := sup.InstanceUID()
	require.Equal(t, testUID, uid)
}

func TestSupervisor_IsRunning(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.DefaultConfig()
	cfg.Server.Endpoint = "ws://localhost:4320/v1/opamp"
	cfg.Agent.Executable = "/bin/echo"
	cfg.Persistence.Dir = t.TempDir()

	sup, err := New(logger, cfg, uuid.New().String())
	require.NoError(t, err)

	// Initially not running
	require.False(t, sup.IsRunning())
}

func TestSupervisor_ShouldAcceptLocalCollectorConnection(t *testing.T) {
	s := &Supervisor{}

	require.True(t, s.shouldAcceptLocalCollectorConnection())

	s.localServerDraining.Store(true)

	require.False(t, s.shouldAcceptLocalCollectorConnection())
}

func TestSupervisor_ConfigManagerIntegration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()

	executable := "/bin/sleep"
	args := []string{"1"}
	if runtime.GOOS == "windows" {
		executable = "powershell"
		args = []string{"-NoProfile", "-Command", "Start-Sleep -Seconds 1"}
	}

	cfg := config.Config{
		Server: config.ServerConfig{
			Endpoint: "ws://localhost:4320/v1/opamp",
		},
		LocalServer: config.LocalServer{
			Endpoint: "localhost:4321",
		},
		Agent: config.AgentConfig{
			Executable: executable,
			Args:       args,
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

	supervisor, err := New(logger, cfg, uuid.New().String())
	require.NoError(t, err)
	require.NotNil(t, supervisor)

	// Verify components are created
	require.NotNil(t, supervisor.configManager)
	require.NotNil(t, supervisor.healthMonitor)
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

		sup, err := New(logger, cfg, uuid.New().String())
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

		sup, err := New(logger, cfg, uuid.New().String())
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

		sup, err := New(logger, cfg, uuid.New().String())
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

		sup, err := New(logger, cfg, uuid.New().String())
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

func TestSupervisor_BuildCollectorEnv(t *testing.T) {
	keysDir := "/tmp/test-keys"
	authMgr := auth.NewManager(zaptest.NewLogger(t), auth.ManagerConfig{
		KeysDir: keysDir,
	})
	instanceUid, err := uuid.NewV7()
	require.NoError(t, err)

	t.Run("sets instance uid", func(t *testing.T) {
		s := &Supervisor{
			authManager:    authMgr,
			agentCfg:       config.AgentConfig{},
			instanceUID:    instanceUid.String(),
			persistenceDir: "/var/lib/graylog-sidecar",
		}

		env := s.buildCollectorEnv()

		require.Equal(t, instanceUid.String(), env["GLC_INTERNAL_INSTANCE_UID"])
	})

	t.Run("sets TLS paths from auth manager", func(t *testing.T) {
		s := &Supervisor{
			authManager: authMgr,
			agentCfg:    config.AgentConfig{},
			instanceUID: instanceUid.String(),
		}

		env := s.buildCollectorEnv()

		require.Equal(t, authMgr.GetSigningKeyPath(), env["GLC_INTERNAL_TLS_CLIENT_KEY_PATH"])
		require.Equal(t, authMgr.GetSigningCertPath(), env["GLC_INTERNAL_TLS_CLIENT_CERT_PATH"])
	})

	t.Run("sets persistence dir", func(t *testing.T) {
		s := &Supervisor{
			authManager:    authMgr,
			agentCfg:       config.AgentConfig{},
			instanceUID:    instanceUid.String(),
			persistenceDir: "/var/lib/graylog-sidecar/", // Check that trailing slash gets removed
		}

		env := s.buildCollectorEnv()

		if runtime.GOOS != "windows" {
			require.Equal(t, "/var/lib/graylog-sidecar", env["GLC_INTERNAL_PERSISTENCE_DIR"])
		} else {
			require.Equal(t, "\\var\\lib\\graylog-sidecar", env["GLC_INTERNAL_PERSISTENCE_DIR"])
		}
	})

	t.Run("sets storage path", func(t *testing.T) {
		s := &Supervisor{
			authManager: authMgr,
			agentCfg: config.AgentConfig{
				StorageDir: "/var/lib/graylog-sidecar/storage/", // Check that trailing slash gets removed
			},
			instanceUID:    instanceUid.String(),
			persistenceDir: "/var/lib/graylog-sidecar",
		}

		env := s.buildCollectorEnv()

		if runtime.GOOS != "windows" {
			require.Equal(t, "/var/lib/graylog-sidecar/storage", env["GLC_INTERNAL_STORAGE_PATH"])
		} else {
			require.Equal(t, "\\var\\lib\\graylog-sidecar\\storage", env["GLC_INTERNAL_STORAGE_PATH"])
		}
	})

	t.Run("merges user-configured env vars", func(t *testing.T) {
		s := &Supervisor{
			authManager: authMgr,
			agentCfg: config.AgentConfig{
				Env: map[string]string{"MY_VAR": "my-value"},
			},
		}

		env := s.buildCollectorEnv()

		require.Equal(t, "my-value", env["MY_VAR"])
		require.Equal(t, authMgr.GetSigningKeyPath(), env["GLC_INTERNAL_TLS_CLIENT_KEY_PATH"])
	})

	t.Run("user env overrides TLS paths", func(t *testing.T) {
		s := &Supervisor{
			authManager: authMgr,
			agentCfg: config.AgentConfig{
				Env: map[string]string{
					"GLC_INTERNAL_TLS_CLIENT_KEY_PATH": "/custom/key.pem",
				},
			},
		}

		env := s.buildCollectorEnv()

		require.Equal(t, "/custom/key.pem", env["GLC_INTERNAL_TLS_CLIENT_KEY_PATH"])
		require.Equal(t, authMgr.GetSigningCertPath(), env["GLC_INTERNAL_TLS_CLIENT_CERT_PATH"])
	})
}
