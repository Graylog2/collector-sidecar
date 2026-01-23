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

func TestLoadWithEnvExpansion(t *testing.T) {
	os.Setenv("TEST_OPAMP_ENDPOINT", "wss://test.example.com/v1/opamp")
	defer os.Unsetenv("TEST_OPAMP_ENDPOINT")

	content := `
server:
  endpoint: "${TEST_OPAMP_ENDPOINT}"
agent:
  executable: /usr/local/bin/otelcol
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0644))

	cfg, err := Load(tmpFile)
	require.NoError(t, err)
	require.Equal(t, "wss://test.example.com/v1/opamp", cfg.Server.Endpoint)
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

func TestLoadWithEnvExpansionInMaps(t *testing.T) {
	os.Setenv("TEST_HEADER_VALUE", "secret-value")
	os.Setenv("TEST_ARG_VALUE", "custom-arg")
	defer func() {
		os.Unsetenv("TEST_HEADER_VALUE")
		os.Unsetenv("TEST_ARG_VALUE")
	}()

	content := `
server:
  endpoint: wss://opamp.example.com/v1/opamp
  headers:
    X-Secret: "${TEST_HEADER_VALUE}"
agent:
  executable: /usr/local/bin/otelcol
  args: ["--config", "${TEST_ARG_VALUE}"]
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0644))

	cfg, err := Load(tmpFile)
	require.NoError(t, err)
	require.Equal(t, "secret-value", cfg.Server.Headers["X-Secret"])
	require.Contains(t, cfg.Agent.Args, "custom-arg")
}
