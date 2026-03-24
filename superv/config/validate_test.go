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
	"time"

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
		// An empty endpoint is okay for config validation, it can be set later via stored connection settings.
		{"empty endpoint", "", false},
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

func TestValidateHealthEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  string
		expectErr bool
	}{
		{"valid_full_url", "http://localhost:13133/health", false},
		{"valid_https", "https://localhost:13133/health", false},
		{"valid_no_path", "http://localhost:13133", false},
		{"empty_allowed", "", false},
		{"missing_scheme", "localhost:13133/health", true},
		{"bare_hostname", "localhost", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Server.Endpoint = "ws://localhost:4320"
			cfg.Agent.Executable = "/bin/test"
			cfg.Agent.Health.Endpoint = tt.endpoint
			err := cfg.Validate()
			if tt.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "agent.health.endpoint")
			} else {
				require.NoError(t, err)
			}
		})
	}
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

func TestValidateTelemetryLogsDefaultLevel(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		expectErr bool
	}{
		{"debug", "debug", false},
		{"info", "info", false},
		{"warn", "warn", false},
		{"error", "error", false},
		{"empty", "", true},
		{"invalid", "trace", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Server.Endpoint = "ws://localhost:4320"
			cfg.Agent.Executable = "/bin/test"
			cfg.Telemetry.Logs.DefaultLevel = tt.level
			err := cfg.Validate()
			if tt.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "telemetry.logs.default_level")
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

func TestValidateRenewalFraction(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		wantErr string
	}{
		{name: "valid 0.75", value: 0.75},
		{name: "valid 0.5", value: 0.5},
		{name: "valid 0.01", value: 0.01},
		{name: "valid 0.99", value: 0.99},
		{name: "zero defaults to 0.75", value: 0},
		{name: "negative", value: -0.5, wantErr: "renewal_fraction"},
		{name: "one", value: 1.0, wantErr: "renewal_fraction"},
		{name: "greater than one", value: 1.5, wantErr: "renewal_fraction"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := AuthConfig{
				JWTLifetime:     5 * time.Minute,
				RenewalFraction: tt.value,
				RenewalInterval: 1 * time.Hour,
			}
			err := cfg.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateRenewalInterval(t *testing.T) {
	tests := []struct {
		name    string
		value   time.Duration
		wantErr string
	}{
		{name: "valid 1h", value: 1 * time.Hour},
		{name: "valid 10s", value: 10 * time.Second},
		{name: "zero", value: 0, wantErr: "renewal_interval"},
		{name: "negative", value: -1 * time.Second, wantErr: "renewal_interval"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := AuthConfig{
				JWTLifetime:     5 * time.Minute,
				RenewalInterval: tt.value,
			}
			err := cfg.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateBackoffConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     BackoffConfig
		wantErr string
	}{
		{
			name: "valid defaults",
			cfg:  BackoffConfig{Initial: 1 * time.Second, Max: 5 * time.Minute, Multiplier: 2.0},
		},
		{
			name: "multiplier exactly 1",
			cfg:  BackoffConfig{Initial: 1 * time.Second, Max: 1 * time.Second, Multiplier: 1.0},
		},
		{
			name:    "zero initial",
			cfg:     BackoffConfig{Initial: 0, Max: 5 * time.Minute, Multiplier: 2.0},
			wantErr: "initial",
		},
		{
			name:    "negative initial",
			cfg:     BackoffConfig{Initial: -1 * time.Second, Max: 5 * time.Minute, Multiplier: 2.0},
			wantErr: "initial",
		},
		{
			name:    "zero max",
			cfg:     BackoffConfig{Initial: 1 * time.Second, Max: 0, Multiplier: 2.0},
			wantErr: "max",
		},
		{
			name:    "max less than initial",
			cfg:     BackoffConfig{Initial: 5 * time.Minute, Max: 1 * time.Second, Multiplier: 2.0},
			wantErr: "max",
		},
		{
			name:    "zero multiplier",
			cfg:     BackoffConfig{Initial: 1 * time.Second, Max: 5 * time.Minute, Multiplier: 0},
			wantErr: "multiplier",
		},
		{
			name:    "multiplier less than 1",
			cfg:     BackoffConfig{Initial: 1 * time.Second, Max: 5 * time.Minute, Multiplier: 0.5},
			wantErr: "multiplier",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate("test.backoff")
			if tt.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.wantErr)
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
