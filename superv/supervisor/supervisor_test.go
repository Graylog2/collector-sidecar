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

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/Graylog2/collector-sidecar/superv/config"
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
