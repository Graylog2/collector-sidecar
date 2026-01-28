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

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadFromFile(t *testing.T) {
	cfg, err := Load("../testdata/config/valid.yaml")
	require.NoError(t, err)
	require.Equal(t, "wss://opamp.example.com/v1/opamp", cfg.Server.Endpoint)
	require.Equal(t, "/usr/local/bin/otelcol", cfg.Agent.Executable)
}

func TestLoadWithEnvOverride(t *testing.T) {
	os.Setenv("GLC_SERVER_ENDPOINT", "wss://env.example.com/v1/opamp")
	defer os.Unsetenv("GLC_SERVER_ENDPOINT")

	content := `
server:
  endpoint: "wss://file.example.com/v1/opamp"
agent:
  executable: /usr/local/bin/otelcol
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0644))

	cfg, err := Load(tmpFile)
	require.NoError(t, err)
	// Env var should override file value
	require.Equal(t, "wss://env.example.com/v1/opamp", cfg.Server.Endpoint)
}

func TestLoadMergesWithDefaults(t *testing.T) {
	content := `
server:
  endpoint: wss://opamp.example.com/v1/opamp
agent:
  executable: /usr/local/bin/otelcol
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0644))

	cfg, err := Load(tmpFile)
	require.NoError(t, err)
	// Check defaults are applied
	require.Equal(t, 5*time.Second, cfg.Agent.ConfigApplyTimeout)
	require.Equal(t, "json", cfg.Logging.Format)
}

func TestLoadInvalidPath(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	require.Error(t, err)
}

func TestLoadEmptyPath(t *testing.T) {
	_, err := Load("")
	require.Error(t, err)
}

func TestLoadEnvOverridesTakesPrecedence(t *testing.T) {
	os.Setenv("GLC_AGENT_EXECUTABLE", "/custom/otelcol")
	os.Setenv("GLC_PERSISTENCE_DIR", "/custom/state")
	defer func() {
		os.Unsetenv("GLC_AGENT_EXECUTABLE")
		os.Unsetenv("GLC_PERSISTENCE_DIR")
	}()

	content := `
server:
  endpoint: wss://opamp.example.com/v1/opamp
agent:
  executable: /usr/local/bin/otelcol
persistence:
  dir: /var/lib/superv
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0644))

	cfg, err := Load(tmpFile)
	require.NoError(t, err)
	// Env vars should override file values
	require.Equal(t, "/custom/otelcol", cfg.Agent.Executable)
	require.Equal(t, "/custom/state", cfg.Persistence.Dir)
	// File value should be preserved when no env override
	require.Equal(t, "wss://opamp.example.com/v1/opamp", cfg.Server.Endpoint)
}
