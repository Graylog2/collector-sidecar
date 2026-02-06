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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateServerEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  string
		expectErr bool
	}{
		{"valid ws", "ws://localhost:4320/v1/opamp", false},
		{"valid wss", "wss://opamp.example.com/v1/opamp", false},
		{"valid http", "http://localhost:4320/v1/opamp", false},
		{"valid https", "https://opamp.example.com/v1/opamp", false},
		{"empty endpoint", "", true},
		{"invalid scheme", "ftp://localhost/v1/opamp", true},
		{"missing scheme", "not-a-url", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Server.Endpoint = tt.endpoint
			cfg.Agent.Executable = "/bin/test" // satisfy other validation
			err := cfg.Validate()
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateAgentExecutable(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Endpoint = "ws://localhost:4320"
	cfg.Agent.Executable = ""
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent.executable")
}

func TestValidateKeysConfig(t *testing.T) {
	tests := []struct {
		name       string
		encrypted  bool
		passphrase PassphraseConfig
		expectErr  bool
	}{
		{"unencrypted", false, PassphraseConfig{}, false},
		{"encrypted_with_env", true, PassphraseConfig{Env: "KEY_PASS"}, false},
		{"encrypted_with_file", true, PassphraseConfig{File: "/run/secrets/pass"}, false},
		{"encrypted_with_cmd", true, PassphraseConfig{Cmd: []string{"vault", "read"}}, false},
		{"encrypted_no_source", true, PassphraseConfig{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Server.Endpoint = "ws://localhost:4320"
			cfg.Agent.Executable = "/bin/test"
			cfg.Keys.Encrypted = tt.encrypted
			cfg.Keys.Passphrase = tt.passphrase
			err := cfg.Validate()
			if tt.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "keys.")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateLoggingLevel(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		expectErr bool
	}{
		{"debug", "debug", false},
		{"info", "info", false},
		{"warn", "warn", false},
		{"error", "error", false},
		{"invalid", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Server.Endpoint = "ws://localhost:4320"
			cfg.Agent.Executable = "/bin/test"
			cfg.Logging.Level = tt.level
			err := cfg.Validate()
			if tt.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "logging.")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateLoggingFormat(t *testing.T) {
	tests := []struct {
		name      string
		format    string
		expectErr bool
	}{
		{"json", "json", false},
		{"text", "text", false},
		{"invalid", "xml", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Server.Endpoint = "ws://localhost:4320"
			cfg.Agent.Executable = "/bin/test"
			cfg.Logging.Format = tt.format
			err := cfg.Validate()
			if tt.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "logging.")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateTransport(t *testing.T) {
	tests := []struct {
		name      string
		transport string
		expectErr bool
	}{
		{"websocket", "websocket", false},
		{"http", "http", false},
		{"auto", "auto", false},
		{"empty", "", true},
		{"invalid", "tcp", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Server.Endpoint = "ws://localhost:4320"
			cfg.Agent.Executable = "/bin/test"
			cfg.Server.Transport = tt.transport
			err := cfg.Validate()
			if tt.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "server.transport")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateReloadMethod(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		expectErr bool
	}{
		{"auto", "auto", false},
		{"signal", "signal", false},
		{"restart", "restart", false},
		{"invalid", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Server.Endpoint = "ws://localhost:4320"
			cfg.Agent.Executable = "/bin/test"
			cfg.Agent.Reload.Method = tt.method
			err := cfg.Validate()
			if tt.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "agent.reload.method")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
