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
	"fmt"
	"path/filepath"
	"runtime"
	"time"
)

// Config is the top-level supervisor configuration.
type Config struct {
	Server      ServerConfig      `koanf:"server"`
	Keys        KeysConfig        `koanf:"keys"`
	LocalServer LocalServer       `koanf:"local_server"`
	Agent       AgentConfig       `koanf:"agent"`
	Packages    PackagesConfig    `koanf:"packages"`
	Persistence PersistenceConfig `koanf:"persistence"`
	Logging     LoggingConfig     `koanf:"logging"`
	Telemetry   TelemetryConfig   `koanf:"telemetry"`
	Debug       bool              `koanf:"debug"`
}

// ServerConfig configures the upstream OpAMP server connection.
type ServerConfig struct {
	Endpoint             string            `koanf:"endpoint"`
	Transport            string            `koanf:"transport"` // websocket | http | auto
	Headers              map[string]string `koanf:"headers"`
	TLS                  TLSConfig         `koanf:"tls"`
	Connection           ConnectionConfig  `koanf:"connection"`
	Auth                 AuthConfig        `koanf:"auth"`
	MaxHeartbeatInterval time.Duration     `koanf:"max_heartbeat_interval"`
}

// TLSConfig configures TLS for server connection.
type TLSConfig struct {
	Insecure   *bool  `koanf:"insecure"`
	CACert     string `koanf:"ca_cert"`
	ClientCert string `koanf:"client_cert"`
	ClientKey  string `koanf:"client_key"`
	MinVersion string `koanf:"min_version"`
	MaxVersion string `koanf:"max_version"`
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
	EnrollmentEndpoint  string            `koanf:"enrollment_endpoint"`
	EnrollmentToken     string            `koanf:"enrollment_token"`
	EnrollmentHeaders   map[string]string `koanf:"enrollment_headers"`
	InsecureTLS         bool              `koanf:"insecure_tls"`
	JWTLifetime         time.Duration     `koanf:"jwt_lifetime"`
	RenewalFraction     float64           `koanf:"renewal_fraction"`
	RenewalInterval     time.Duration     `koanf:"renewal_interval"`
	ResetOnAuthRejection bool             `koanf:"reset_on_auth_rejection"`
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

// LocalServer configures the local OpAMP server for the collector.
type LocalServer struct {
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
	StorageDir         string            `koanf:"storage_dir"`
	Config             AgentConfigMerge  `koanf:"config"`
	Health             HealthConfig      `koanf:"health"`
	Reload             ReloadConfig      `koanf:"reload"`
	Restart            RestartConfig     `koanf:"restart"`
	Shutdown           ShutdownConfig    `koanf:"shutdown"`
	Sidecar            Sidecar           `koanf:"sidecar"`
}

// AgentConfigMerge configures how agent configs are merged.
type AgentConfigMerge struct {
	MergeStrategy  string   `koanf:"merge_strategy"` // deep
	LocalOverrides []string `koanf:"local_overrides"`
}

// HealthConfig configures health monitoring.
type HealthConfig struct {
	Endpoint           string        `koanf:"endpoint"`
	Interval           time.Duration `koanf:"interval"`
	Timeout            time.Duration `koanf:"timeout"`
	StartupGracePeriod time.Duration `koanf:"startup_grace_period"`
}

// ReloadConfig configures config reload behavior.
type ReloadConfig struct {
	Method                 string `koanf:"method"` // auto | signal | restart
	WindowsReloadEvent     string `koanf:"windows_reload_event"`
	RestartOnReloadFailure bool   `koanf:"restart_on_reload_failure"`
}

// RestartConfig configures crash recovery with exponential backoff.
type RestartConfig struct {
	// MaxRetries is the maximum number of restart attempts. 0 means unlimited.
	MaxRetries int `koanf:"max_retries"`

	// InitialInterval is the first backoff delay. Default: 1s.
	InitialInterval time.Duration `koanf:"initial_interval"`

	// MaxInterval is the maximum backoff delay. Default: 30s.
	MaxInterval time.Duration `koanf:"max_interval"`

	// Multiplier is the factor by which delay increases. Default: 2.0.
	Multiplier float64 `koanf:"multiplier"`

	// RandomizationFactor adds jitter to delays. 0.5 means ±50%. Default: 0.5.
	RandomizationFactor float64 `koanf:"randomization_factor"`

	// StableAfter is the duration after which a running process is considered
	// stable and backoff resets. Default: 30s. 0 disables stability tracking.
	StableAfter time.Duration `koanf:"stable_after"`
}

// ShutdownConfig configures graceful shutdown.
type ShutdownConfig struct {
	GracefulTimeout time.Duration `koanf:"graceful_timeout"`
}

type Sidecar struct {
	Enabled    bool `koanf:"enabled"`
	Autodetect bool `koanf:"autodetect"`
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

// TelemetryConfig configures the supervisor's own telemetry export.
type TelemetryConfig struct {
	Logs TelemetryLogsConfig `koanf:"logs"`
}

// TelemetryLogsConfig configures own-log export via OTLP.
type TelemetryLogsConfig struct {
	// DefaultLevel is the minimum log level to export unless overridden
	// by the OpAMP server. Default: "info".
	// Valid values: debug, info, warn, error.
	DefaultLevel string      `koanf:"default_level"`
	Batch        BatchConfig `koanf:"batch"`
}

// BatchConfig configures the OTel SDK BatchProcessor.
// Zero values use SDK defaults.
type BatchConfig struct {
	// MaxQueueSize is the ring buffer capacity. Default: 2048.
	MaxQueueSize int `koanf:"max_queue_size"`
	// ExportMaxBatchSize is the maximum number of records per export. Default: 512.
	ExportMaxBatchSize int `koanf:"export_max_batch_size"`
	// ExportInterval is how often batches are flushed. Default: 1s.
	ExportInterval time.Duration `koanf:"export_interval"`
	// ExportTimeout is the per-export timeout. Default: 30s.
	ExportTimeout time.Duration `koanf:"export_timeout"`
}

// LoggingConfig configures logging.
type LoggingConfig struct {
	Format string `koanf:"format"` // json | text
	Level  string `koanf:"level"`  // debug | info | warn | error
	Color  bool   `koanf:"color"`
}

var linuxDataPathPrefix = "/var/lib/graylog-collector"
var windowsDataPathPrefix = filepath.Join("C:", "ProgramData", "Graylog", "Collector")

type platformName string

const windows platformName = "windows"
const linux platformName = "linux"

func platformDefaultValue[T any](values map[platformName]T) T {
	if value, ok := values[platformName(runtime.GOOS)]; ok {
		return value
	}
	// Must not happen
	panic(fmt.Sprintf("platform %s not found", runtime.GOOS))
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Endpoint:             "", //ws://localhost:4320/v1/opamp",
			Transport:            "auto",
			MaxHeartbeatInterval: 15 * time.Minute,
			Connection: ConnectionConfig{
				RetryBackoff: BackoffConfig{
					Initial:    1 * time.Second,
					Max:        5 * time.Minute,
					Multiplier: 2.0,
				},
			},
			Auth: AuthConfig{
				JWTLifetime:     5 * time.Minute,
				RenewalFraction: 0.75,
				RenewalInterval: 1 * time.Hour,
			},
		},
		Keys: KeysConfig{
			// TODO: Branding
			Dir: platformDefaultValue(map[platformName]string{
				linux:   filepath.Join(linuxDataPathPrefix, "keys"),
				windows: filepath.Join(windowsDataPathPrefix, "keys"),
			}),
			Encrypted: false,
		},
		LocalServer: LocalServer{
			Endpoint: "localhost:0", // port 0 = random free port
		},
		Agent: AgentConfig{
			Args:               []string{"--config", "{{ .ConfigPath }}"},
			ConfigApplyTimeout: 5 * time.Second,
			BootstrapTimeout:   3 * time.Second,
			PassthroughLogs:    false,
			// TODO: Branding
			StorageDir: platformDefaultValue(map[platformName]string{
				linux:   filepath.Join(linuxDataPathPrefix, "storage"),
				windows: filepath.Join(windowsDataPathPrefix, "storage"),
			}),
			Config: AgentConfigMerge{
				MergeStrategy: "deep",
			},
			Health: HealthConfig{
				// TODO: Check if we can switch to a UNIX socket instead of opening a network port.
				Endpoint:           "http://localhost:13133/health",
				Interval:           10 * time.Second,
				Timeout:            5 * time.Second,
				StartupGracePeriod: 3 * time.Second,
			},
			Reload: ReloadConfig{
				Method:                 "auto",
				RestartOnReloadFailure: true,
			},
			Restart: RestartConfig{
				MaxRetries:          0, // Unlimited retries by default
				InitialInterval:     1 * time.Second,
				MaxInterval:         30 * time.Second,
				Multiplier:          2.0,
				RandomizationFactor: 0.5,
				StableAfter:         30 * time.Second,
			},
			Shutdown: ShutdownConfig{
				GracefulTimeout: 30 * time.Second,
			},
			Sidecar: Sidecar{
				Enabled:    false,
				Autodetect: true,
			},
		},
		Packages: PackagesConfig{
			// TODO: Branding
			StorageDir: platformDefaultValue(map[platformName]string{
				linux:   filepath.Join(linuxDataPathPrefix, "packages"),
				windows: filepath.Join(windowsDataPathPrefix, "packages"),
			}),
			KeepVersions: 2,
			Verification: VerificationConfig{
				PublisherSignature: PublisherSignatureConfig{
					Enabled: false,
					Format:  "cosign",
				},
			},
		},
		Persistence: PersistenceConfig{
			// TODO: Branding
			Dir: platformDefaultValue(map[platformName]string{
				linux:   filepath.Join(linuxDataPathPrefix, "supervisor"),
				windows: filepath.Join(windowsDataPathPrefix, "supervisor"),
			}),
		},
		Telemetry: TelemetryConfig{
			Logs: TelemetryLogsConfig{
				DefaultLevel: "info",
				Batch: BatchConfig{
					MaxQueueSize:       2048,
					ExportMaxBatchSize: 512,
					ExportInterval:     1 * time.Second,
					ExportTimeout:      30 * time.Second,
				},
			},
		},
		Logging: LoggingConfig{
			Format: "json",
			Level:  "info",
		},
	}
}

// SetInsecure configures the supervisor to not validate TLS certificates.
func (c *Config) SetInsecure() {
	c.Server.Auth.InsecureTLS = true
	c.Server.TLS.Insecure = new(true)
}

// IsInsecure returns true if any of the TLS verification settings is disabled.
func (c *Config) IsInsecure() bool {
	// TODO: We might want to refactor this to only have a single source of truth for TLS insecurity.
	if c.Server.TLS.Insecure != nil {
		return c.Server.Auth.InsecureTLS || *c.Server.TLS.Insecure
	}
	return c.Server.Auth.InsecureTLS
}
