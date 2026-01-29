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
	"crypto/tls"
	"net/http"
	"time"
)

// Config is the top-level supervisor configuration.
type Config struct {
	Server      ServerConfig      `koanf:"server"`
	Auth        AuthConfig        `koanf:"auth"`
	Keys        KeysConfig        `koanf:"keys"`
	LocalOpAMP  LocalOpAMPConfig  `koanf:"local_opamp"`
	Agent       AgentConfig       `koanf:"agent"`
	Packages    PackagesConfig    `koanf:"packages"`
	Persistence PersistenceConfig `koanf:"persistence"`
	Logging     LoggingConfig     `koanf:"logging"`
}

// ServerConfig configures the upstream OpAMP server connection.
type ServerConfig struct {
	Endpoint   string            `koanf:"endpoint"`
	Transport  string            `koanf:"transport"` // websocket | http | auto
	Headers    map[string]string `koanf:"headers"`
	TLS        TLSConfig         `koanf:"tls"`
	Connection ConnectionConfig  `koanf:"connection"`
}

// TLSConfig configures TLS for server connection.
type TLSConfig struct {
	Insecure   bool   `koanf:"insecure"`
	CACert     string `koanf:"ca_cert"`
	ClientCert string `koanf:"client_cert"`
	ClientKey  string `koanf:"client_key"`
	MinVersion string `koanf:"min_version"`
}

// ConnectionConfig configures connection retry behavior.
type ConnectionConfig struct {
	RetryBackoff BackoffConfig `koanf:"retry_backoff"`
}

// BackoffConfig configures exponential backoff.
type BackoffConfig struct {
	Initial    time.Duration `koanf:"initial"`
	Max        time.Duration `koanf:"max"`
	Multiplier float64       `koanf:"multiplier"`
}

// AuthConfig configures authentication.
type AuthConfig struct {
	EnrollmentURL string        `koanf:"enrollment_url"`
	InsecureTLS   bool          `koanf:"insecure_tls"`
	JWTLifetime   time.Duration `koanf:"jwt_lifetime"`
}

// KeysConfig configures key storage.
type KeysConfig struct {
	Dir        string           `koanf:"dir"`
	Encrypted  bool             `koanf:"encrypted"`
	Passphrase PassphraseConfig `koanf:"passphrase"`
}

// PassphraseConfig configures passphrase source for encrypted keys.
type PassphraseConfig struct {
	Env  string   `koanf:"env"`
	File string   `koanf:"file"`
	Cmd  []string `koanf:"cmd"`
}

// LocalOpAMPConfig configures the local OpAMP server for the collector.
type LocalOpAMPConfig struct {
	Endpoint string `koanf:"endpoint"`
}

// AgentConfig configures the managed collector agent.
type AgentConfig struct {
	Executable         string            `koanf:"executable"`
	Args               []string          `koanf:"args"`
	Env                map[string]string `koanf:"env"`
	ConfigApplyTimeout time.Duration     `koanf:"config_apply_timeout"`
	BootstrapTimeout   time.Duration     `koanf:"bootstrap_timeout"`
	PassthroughLogs    bool              `koanf:"passthrough_logs"`
	Config             AgentConfigMerge  `koanf:"config"`
	Health             HealthConfig      `koanf:"health"`
	Reload             ReloadConfig      `koanf:"reload"`
	Restart            RestartConfig     `koanf:"restart"`
	Shutdown           ShutdownConfig    `koanf:"shutdown"`
}

// AgentConfigMerge configures how agent configs are merged.
type AgentConfigMerge struct {
	MergeStrategy  string   `koanf:"merge_strategy"` // deep
	LocalOverrides []string `koanf:"local_overrides"`
}

// HealthConfig configures health monitoring.
type HealthConfig struct {
	Endpoint string        `koanf:"endpoint"`
	Interval time.Duration `koanf:"interval"`
	Timeout  time.Duration `koanf:"timeout"`
}

// ReloadConfig configures config reload behavior.
type ReloadConfig struct {
	Method                 string `koanf:"method"` // auto | signal | restart
	WindowsReloadEvent     string `koanf:"windows_reload_event"`
	RestartOnReloadFailure bool   `koanf:"restart_on_reload_failure"`
}

// RestartConfig configures crash recovery.
type RestartConfig struct {
	MaxRetries int             `koanf:"max_retries"`
	Backoff    []time.Duration `koanf:"backoff"`
}

// ShutdownConfig configures graceful shutdown.
type ShutdownConfig struct {
	GracefulTimeout time.Duration `koanf:"graceful_timeout"`
}

// PackagesConfig configures package management.
type PackagesConfig struct {
	StorageDir   string             `koanf:"storage_dir"`
	KeepVersions int                `koanf:"keep_versions"`
	Verification VerificationConfig `koanf:"verification"`
}

// VerificationConfig configures package verification.
type VerificationConfig struct {
	PublisherSignature PublisherSignatureConfig `koanf:"publisher_signature"`
}

// PublisherSignatureConfig configures publisher signature verification.
type PublisherSignatureConfig struct {
	Enabled     bool     `koanf:"enabled"`
	Format      string   `koanf:"format"` // cosign | gpg | minisign
	TrustedKeys []string `koanf:"trusted_keys"`
}

// PersistenceConfig configures state persistence.
type PersistenceConfig struct {
	Dir string `koanf:"dir"`
}

// LoggingConfig configures logging.
type LoggingConfig struct {
	Format string `koanf:"format"` // json | text
	Level  string `koanf:"level"`  // debug | info | warn | error
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Endpoint:  "ws://localhost:4320/v1/opamp",
			Transport: "auto",
			Connection: ConnectionConfig{
				RetryBackoff: BackoffConfig{
					Initial:    1 * time.Second,
					Max:        5 * time.Minute,
					Multiplier: 2.0,
				},
			},
		},
		Auth: AuthConfig{
			JWTLifetime: 5 * time.Minute,
		},
		Keys: KeysConfig{
			Dir:       "/var/lib/supervisor/keys",
			Encrypted: false,
		},
		LocalOpAMP: LocalOpAMPConfig{
			Endpoint: "localhost:0", // port 0 = random free port
		},
		Agent: AgentConfig{
			ConfigApplyTimeout: 5 * time.Second,
			BootstrapTimeout:   3 * time.Second,
			PassthroughLogs:    false,
			Config: AgentConfigMerge{
				MergeStrategy: "deep",
			},
			Health: HealthConfig{
				Endpoint: "http://localhost:13133/health",
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
			Reload: ReloadConfig{
				Method:                 "auto",
				RestartOnReloadFailure: true,
			},
			Restart: RestartConfig{
				MaxRetries: 5,
				Backoff: []time.Duration{
					1 * time.Second,
					2 * time.Second,
					4 * time.Second,
					8 * time.Second,
					16 * time.Second,
				},
			},
			Shutdown: ShutdownConfig{
				GracefulTimeout: 30 * time.Second,
			},
		},
		Packages: PackagesConfig{
			StorageDir:   "/var/lib/supervisor/packages",
			KeepVersions: 2,
			Verification: VerificationConfig{
				PublisherSignature: PublisherSignatureConfig{
					Enabled: false,
					Format:  "cosign",
				},
			},
		},
		Persistence: PersistenceConfig{
			Dir: "/var/lib/supervisor",
		},
		Logging: LoggingConfig{
			Format: "json",
			Level:  "info",
		},
	}
}

// ToHTTPHeaders converts config headers to http.Header.
func (s ServerConfig) ToHTTPHeaders() http.Header {
	h := make(http.Header)
	for k, v := range s.Headers {
		h.Set(k, v)
	}
	return h
}

// ToTLSConfig converts TLSConfig to *tls.Config.
// Returns nil if TLS is not configured.
func (t TLSConfig) ToTLSConfig() (*tls.Config, error) {
	if t.Insecure {
		return nil, nil
	}
	// TODO: Implement full TLS config loading
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
	}, nil
}
