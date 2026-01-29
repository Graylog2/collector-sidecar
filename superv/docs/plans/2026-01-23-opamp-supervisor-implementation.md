# OpAMP Supervisor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a production-ready OpAMP supervisor that manages an OpenTelemetry Collector with full remote management capabilities.

**Architecture:** Dual OpAMP role - client connecting upstream to management server (WebSocket/HTTP) and server for downstream collector on localhost. Core engine coordinates configuration layering, CSR-based authentication with supervisor-signed JWTs, process management with SIGHUP reload, and package management with signature verification.

**Tech Stack:** Go 1.25+, opamp-go (client + server), koanf (config merging), goccy/go-yaml, zap (logging), OTEL SDK (self-telemetry)

**Reference:** Design doc at `docs/plans/2026-01-23-opamp-supervisor-design.md`

---

## Phase 1: Project Foundation

### Task 1.1: Initialize Go Module

**Files:**
- Create: `go.mod`
- Create: `go.sum`
- Create: `cmd/supervisor/main.go`
- Create: `version/version.go`

**Step 1: Initialize Go module**

Run: `go mod init github.com/Graylog2/collector-sidecar/superv`
Expected: Creates go.mod file

**Step 2: Create main entry point**

Create `cmd/supervisor/main.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/Graylog2/collector-sidecar/superv/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version.Version())
		os.Exit(0)
	}
	fmt.Println("OpAMP Supervisor starting...")
}
```

**Step 3: Create version package**

Create `version/version.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package version

var (
	version = "0.1.0-dev"
	commit  = "unknown"
)

func Version() string {
	return version + " (" + commit + ")"
}
```

**Step 4: Verify build works**

Run: `go build -o supervisor ./cmd/supervisor`
Expected: Binary created successfully

**Step 5: Run version check**

Run: `./supervisor --version`
Expected: `0.1.0-dev (unknown)`

**Step 6: Commit**

```bash
git add .
git commit -m "feat: initialize Go module with main entry point"
```

---

### Task 1.2: Add Core Dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Add opamp-go dependencies**

Run: `go get github.com/open-telemetry/opamp-go@latest`
Expected: Downloads opamp-go and updates go.mod/go.sum

**Step 2: Add logging dependency**

Run: `go get go.uber.org/zap@latest`
Expected: Downloads zap and updates go.mod/go.sum

**Step 3: Add config merging dependency**

Run: `go get github.com/knadh/koanf/v2@latest`
Expected: Downloads koanf and updates go.mod/go.sum

**Step 4: Add YAML support for koanf**

Run: `go get github.com/knadh/koanf/parsers/yaml@latest && go get github.com/knadh/koanf/providers/file@latest`
Expected: Downloads koanf YAML support

**Step 5: Add UUID support**

Run: `go get github.com/google/uuid@latest`
Expected: Downloads uuid library

**Step 6: Add testify for testing**

Run: `go get github.com/stretchr/testify@latest`
Expected: Downloads testify

**Step 7: Tidy dependencies**

Run: `go mod tidy`
Expected: go.mod and go.sum are clean

**Step 8: Commit**

```bash
git add go.mod go.sum
git commit -m "feat: add core dependencies (opamp-go, zap, koanf, testify)"
```

---

## Phase 2: Configuration System

### Task 2.1: Define Configuration Types

**Files:**
- Create: `config/types.go`
- Create: `config/types_test.go`

**Step 1: Write test for config struct existence**

Create `config/types_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigStructExists(t *testing.T) {
	cfg := Config{}
	require.NotNil(t, &cfg)
}

func TestServerConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	require.NotEmpty(t, cfg.Server.Endpoint)
}

func TestAgentConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	require.Greater(t, cfg.Agent.ConfigApplyTimeout, time.Duration(0))
	require.Greater(t, cfg.Agent.BootstrapTimeout, time.Duration(0))
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./config/... -v`
Expected: FAIL with "package not found" or similar

**Step 3: Create config types**

Create `config/types.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

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
	Method                  string `koanf:"method"` // auto | signal | restart
	WindowsReloadEvent      string `koanf:"windows_reload_event"`
	RestartOnReloadFailure  bool   `koanf:"restart_on_reload_failure"`
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
			Endpoint: "localhost:4320",
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
```

**Step 4: Run tests to verify they pass**

Run: `go test ./config/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add config/
git commit -m "feat(config): add configuration types with defaults"
```

---

### Task 2.2: Implement Configuration Loading

**Files:**
- Create: `config/loader.go`
- Create: `config/loader_test.go`
- Create: `testdata/config/valid.yaml`

**Step 1: Write test for config loading**

Create `config/loader_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadFromFile(t *testing.T) {
	cfg, err := Load("../../testdata/config/valid.yaml")
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
```

**Step 2: Create test fixture**

Create `testdata/config/valid.yaml`:
```yaml
server:
  endpoint: wss://opamp.example.com/v1/opamp
  transport: websocket
  headers:
    X-Custom-Header: test-value

auth:
  enrollment_url: "${ENROLLMENT_URL}"
  jwt_lifetime: 5m

keys:
  dir: /var/lib/supervisor/keys
  encrypted: false

agent:
  executable: /usr/local/bin/otelcol
  args: ["--config", "{{.ConfigPath}}"]
  config:
    merge_strategy: deep
    local_overrides:
      - /etc/supervisor/compliance.yaml

persistence:
  dir: /var/lib/supervisor

logging:
  format: json
  level: info
```

**Step 3: Run test to verify it fails**

Run: `go test ./config/... -v -run TestLoad`
Expected: FAIL (Load function not defined)

**Step 4: Implement config loader**

Create `config/loader.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"os"
	"regexp"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Load loads configuration from a YAML file, expanding environment variables
// and merging with defaults.
func Load(path string) (Config, error) {
	if path == "" {
		return Config{}, errors.New("config path cannot be empty")
	}

	k := koanf.New(".")

	// Load defaults first
	defaults := DefaultConfig()
	if err := k.Load(structProvider{defaults}, nil); err != nil {
		return Config{}, err
	}

	// Load from file
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, err
	}

	// Expand environment variables in string fields
	expandEnvVars(&cfg)

	return cfg, nil
}

// structProvider implements koanf.Provider for loading from a struct.
type structProvider struct {
	cfg Config
}

func (s structProvider) ReadBytes() ([]byte, error) {
	return nil, errors.New("ReadBytes not supported")
}

func (s structProvider) Read() (map[string]interface{}, error) {
	// For simplicity, we'll return an empty map and let defaults be set via Unmarshal
	// A proper implementation would convert struct to map
	return map[string]interface{}{}, nil
}

// expandEnvVars expands ${VAR} patterns in config string fields.
func expandEnvVars(cfg *Config) {
	envPattern := regexp.MustCompile(`\$\{([^}]+)\}`)

	expand := func(s string) string {
		return envPattern.ReplaceAllStringFunc(s, func(match string) string {
			varName := strings.TrimPrefix(strings.TrimSuffix(match, "}"), "${")
			if val := os.Getenv(varName); val != "" {
				return val
			}
			return match
		})
	}

	cfg.Server.Endpoint = expand(cfg.Server.Endpoint)
	cfg.Auth.EnrollmentToken = expand(cfg.Auth.EnrollmentToken)
	cfg.Auth.TokenFile = expand(cfg.Auth.TokenFile)
	cfg.Agent.Executable = expand(cfg.Agent.Executable)
	cfg.Persistence.Dir = expand(cfg.Persistence.Dir)
	cfg.Packages.StorageDir = expand(cfg.Packages.StorageDir)

	// Expand headers
	for k, v := range cfg.Server.Headers {
		cfg.Server.Headers[k] = expand(v)
	}

	// Expand args
	for i, arg := range cfg.Agent.Args {
		cfg.Agent.Args[i] = expand(arg)
	}

	// Expand env vars in agent env
	for k, v := range cfg.Agent.Env {
		cfg.Agent.Env[k] = expand(v)
	}
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./config/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add config/loader.go config/loader_test.go testdata/
git commit -m "feat(config): implement configuration loading with env expansion"
```

---

### Task 2.3: Add Configuration Validation

**Files:**
- Create: `config/validate.go`
- Create: `config/validate_test.go`

**Step 1: Write validation tests**

Create `config/validate_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

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
		{"invalid url", "not-a-url", true},
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
	require.Contains(t, err.Error(), "executable")
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
				require.Contains(t, err.Error(), "keys")
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
				require.Contains(t, err.Error(), "logging")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./config/... -v -run TestValidate`
Expected: FAIL (Validate method not defined)

**Step 3: Implement validation**

Create `config/validate.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
)

var (
	validSchemes       = []string{"ws", "wss", "http", "https"}
	validLogLevels     = []string{"debug", "info", "warn", "error"}
	validLogFormats    = []string{"json", "text"}
	validReloadMethods = []string{"auto", "signal", "restart"}
	validTransports    = []string{"websocket", "http", "auto", ""}
)

// Validate checks the configuration for errors.
func (c Config) Validate() error {
	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("server: %w", err)
	}

	if err := c.Keys.Validate(); err != nil {
		return fmt.Errorf("keys: %w", err)
	}

	if err := c.Agent.Validate(); err != nil {
		return fmt.Errorf("agent: %w", err)
	}

	if err := c.Logging.Validate(); err != nil {
		return fmt.Errorf("logging: %w", err)
	}

	return nil
}

// Validate checks ServerConfig for errors.
func (s ServerConfig) Validate() error {
	if s.Endpoint == "" {
		return errors.New("endpoint is required")
	}

	u, err := url.Parse(s.Endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	if !slices.Contains(validSchemes, u.Scheme) {
		return fmt.Errorf("endpoint scheme must be one of %v, got %q", validSchemes, u.Scheme)
	}

	if !slices.Contains(validTransports, s.Transport) {
		return fmt.Errorf("transport must be one of %v, got %q", validTransports, s.Transport)
	}

	return nil
}

// Validate checks KeysConfig for errors.
func (k KeysConfig) Validate() error {
	if k.Encrypted && k.Passphrase.Env == "" && k.Passphrase.File == "" && len(k.Passphrase.Cmd) == 0 {
		return errors.New("passphrase source required when keys are encrypted")
	}
	return nil
}

// Validate checks AgentConfig for errors.
func (a AgentConfig) Validate() error {
	if a.Executable == "" {
		return errors.New("executable is required")
	}

	if !slices.Contains(validReloadMethods, a.Reload.Method) {
		return fmt.Errorf("reload.method must be one of %v, got %q", validReloadMethods, a.Reload.Method)
	}

	return nil
}

// Validate checks LoggingConfig for errors.
func (l LoggingConfig) Validate() error {
	if !slices.Contains(validLogLevels, l.Level) {
		return fmt.Errorf("level must be one of %v, got %q", validLogLevels, l.Level)
	}

	if !slices.Contains(validLogFormats, l.Format) {
		return fmt.Errorf("format must be one of %v, got %q", validLogFormats, l.Format)
	}

	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./config/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add config/validate.go config/validate_test.go
git commit -m "feat(config): add configuration validation"
```

---

## Phase 3: Persistence Layer

### Task 3.1: Implement Instance UID Persistence

**Files:**
- Create: `persistence/instance.go`
- Create: `persistence/instance_test.go`

**Step 1: Write tests for instance UID**

Create `persistence/instance_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestLoadOrCreateInstanceUID_CreatesNew(t *testing.T) {
	dir := t.TempDir()
	uid, err := LoadOrCreateInstanceUID(dir)
	require.NoError(t, err)
	require.NotEmpty(t, uid)

	// Verify it's a valid UUID
	_, err = uuid.Parse(uid)
	require.NoError(t, err)

	// Verify file was created
	filePath := filepath.Join(dir, "instance_uid.yaml")
	_, err = os.Stat(filePath)
	require.NoError(t, err)
}

func TestLoadOrCreateInstanceUID_LoadsExisting(t *testing.T) {
	dir := t.TempDir()

	// Create first
	uid1, err := LoadOrCreateInstanceUID(dir)
	require.NoError(t, err)

	// Load again - should return same UID
	uid2, err := LoadOrCreateInstanceUID(dir)
	require.NoError(t, err)
	require.Equal(t, uid1, uid2)
}

func TestLoadOrCreateInstanceUID_FileIsReadOnly(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadOrCreateInstanceUID(dir)
	require.NoError(t, err)

	// Check file permissions are read-only
	filePath := filepath.Join(dir, "instance_uid.yaml")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0444), info.Mode().Perm())
}

func TestLoadOrCreateInstanceUID_PreservesCreatedAt(t *testing.T) {
	dir := t.TempDir()

	// Create instance
	_, err := LoadOrCreateInstanceUID(dir)
	require.NoError(t, err)

	// Read the file to get created_at
	data, err := LoadInstanceData(dir)
	require.NoError(t, err)
	originalCreatedAt := data.CreatedAt

	// Load again
	_, err = LoadOrCreateInstanceUID(dir)
	require.NoError(t, err)

	// Verify created_at is unchanged
	data2, err := LoadInstanceData(dir)
	require.NoError(t, err)
	require.Equal(t, originalCreatedAt, data2.CreatedAt)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./persistence/... -v`
Expected: FAIL (package not found)

**Step 3: Implement instance UID persistence**

Create `persistence/instance.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package persistence

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/goccy/go-yaml"
)

const instanceUIDFile = "instance_uid.yaml"

// InstanceData represents the persisted instance identity.
type InstanceData struct {
	InstanceUID string    `yaml:"instance_uid"`
	CreatedAt   time.Time `yaml:"created_at"`
}

// LoadOrCreateInstanceUID loads the instance UID from disk, or creates a new one
// if it doesn't exist. The file is created with read-only permissions (0444).
func LoadOrCreateInstanceUID(dir string) (string, error) {
	filePath := filepath.Join(dir, instanceUIDFile)

	// Try to load existing
	data, err := LoadInstanceData(dir)
	if err == nil {
		return data.InstanceUID, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	// Create new instance
	data = &InstanceData{
		InstanceUID: uuid.New().String(),
		CreatedAt:   time.Now().UTC(),
	}

	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	// Marshal to YAML
	content, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}

	// Write file with read-only permissions
	if err := os.WriteFile(filePath, content, 0444); err != nil {
		return "", err
	}

	return data.InstanceUID, nil
}

// LoadInstanceData loads the instance data from disk.
func LoadInstanceData(dir string) (*InstanceData, error) {
	filePath := filepath.Join(dir, instanceUIDFile)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var data InstanceData
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	return &data, nil
}
```

**Step 4: Add yaml.v3 dependency**

Run: `go get github.com/goccy/go-yaml@latest && go mod tidy`
Expected: Downloads yaml.v3

**Step 5: Run tests to verify they pass**

Run: `go test ./persistence/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add persistence/ go.mod go.sum
git commit -m "feat(persistence): implement instance UID persistence"
```

---

### Task 3.2: Implement Key & Certificate Persistence

**Files:**
- Create: `persistence/keys.go`
- Create: `persistence/keys_test.go`

**Step 1: Write tests for key persistence**

Create `persistence/keys_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package persistence

import (
	"crypto/ed25519"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSaveAndLoadSigningKey(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	// Generate a test keypair
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	err = SaveSigningKey(keysDir, priv)
	require.NoError(t, err)

	loaded, err := LoadSigningKey(keysDir)
	require.NoError(t, err)
	require.Equal(t, priv, loaded)
	require.Equal(t, pub, loaded.Public())
}

func TestSaveSigningKey_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	err = SaveSigningKey(keysDir, priv)
	require.NoError(t, err)

	filePath := filepath.Join(keysDir, "signing.key")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestLoadSigningKey_NotExists(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadSigningKey(dir)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestSaveAndLoadCertificate(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	// Create a minimal test certificate (self-signed for testing)
	cert := createTestCertificate(t)

	err := SaveCertificate(keysDir, cert)
	require.NoError(t, err)

	loaded, err := LoadCertificate(keysDir)
	require.NoError(t, err)
	require.Equal(t, cert.Raw, loaded.Raw)
}

func TestSaveCertificate_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	cert := createTestCertificate(t)

	err := SaveCertificate(keysDir, cert)
	require.NoError(t, err)

	filePath := filepath.Join(keysDir, "signing.crt")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	// Certificates are public, so 0644 is fine
	require.Equal(t, os.FileMode(0644), info.Mode().Perm())
}

func TestKeysExist(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	// Initially no keys exist
	require.False(t, SigningKeyExists(keysDir))
	require.False(t, CertificateExists(keysDir))

	// Create signing key
	_, priv, _ := ed25519.GenerateKey(nil)
	_ = SaveSigningKey(keysDir, priv)
	require.True(t, SigningKeyExists(keysDir))

	// Create certificate
	cert := createTestCertificate(t)
	_ = SaveCertificate(keysDir, cert)
	require.True(t, CertificateExists(keysDir))
}

func TestCertificateFingerprint(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	cert := createTestCertificate(t)
	_ = SaveCertificate(keysDir, cert)

	fp, err := CertificateFingerprint(keysDir)
	require.NoError(t, err)
	require.NotEmpty(t, fp)
	// SHA-256 fingerprint should be 64 hex chars
	require.Len(t, fp, 64)
}

// createTestCertificate creates a self-signed certificate for testing
func createTestCertificate(t *testing.T) *x509.Certificate {
	t.Helper()
	// Implementation uses x509.CreateCertificate with test data
	// ... (test helper implementation)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./persistence/... -v -run TestSaveAndLoadSigningKey`
Expected: FAIL (functions not defined)

**Step 3: Implement key persistence**

Create `persistence/keys.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package persistence

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
)

const (
	signingKeyFile  = "signing.key"
	signingCertFile = "signing.crt"
	encryptionKeyFile = "encryption.key"
)

// SaveSigningKey saves an Ed25519 private key to disk in PEM format.
func SaveSigningKey(keysDir string, key ed25519.PrivateKey) error {
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return err
	}

	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}

	block := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8,
	}

	filePath := filepath.Join(keysDir, signingKeyFile)
	return os.WriteFile(filePath, pem.EncodeToMemory(block), 0600)
}

// LoadSigningKey loads an Ed25519 private key from disk.
func LoadSigningKey(keysDir string) (ed25519.PrivateKey, error) {
	filePath := filepath.Join(keysDir, signingKeyFile)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(content)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	ed25519Key, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("key is not Ed25519")
	}

	return ed25519Key, nil
}

// SaveCertificate saves an X.509 certificate to disk in PEM format.
func SaveCertificate(keysDir string, cert *x509.Certificate) error {
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return err
	}

	block := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}

	filePath := filepath.Join(keysDir, signingCertFile)
	return os.WriteFile(filePath, pem.EncodeToMemory(block), 0644)
}

// LoadCertificate loads an X.509 certificate from disk.
func LoadCertificate(keysDir string) (*x509.Certificate, error) {
	filePath := filepath.Join(keysDir, signingCertFile)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(content)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	return x509.ParseCertificate(block.Bytes)
}

// SigningKeyExists returns true if the signing key file exists.
func SigningKeyExists(keysDir string) bool {
	filePath := filepath.Join(keysDir, signingKeyFile)
	_, err := os.Stat(filePath)
	return err == nil
}

// CertificateExists returns true if the certificate file exists.
func CertificateExists(keysDir string) bool {
	filePath := filepath.Join(keysDir, signingCertFile)
	_, err := os.Stat(filePath)
	return err == nil
}

// CertificateFingerprint returns the SHA-256 fingerprint of the certificate.
func CertificateFingerprint(keysDir string) (string, error) {
	cert, err := LoadCertificate(keysDir)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:]), nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./persistence/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add persistence/
git commit -m "feat(persistence): implement Ed25519 key and certificate persistence"
```

---

### Task 3.3: Implement Connection State Persistence

**Files:**
- Create: `persistence/connection.go`
- Create: `persistence/connection_test.go`

**Step 1: Write tests for connection state**

Create `persistence/connection_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package persistence

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSaveAndLoadConnectionState(t *testing.T) {
	dir := t.TempDir()

	state := &ConnectionState{
		Server: ServerState{
			Endpoint:        "wss://opamp.example.com/v1/opamp",
			LastConnected:   time.Now().UTC().Truncate(time.Second),
			LastSequenceNum: 42,
		},
		RemoteConfig: RemoteConfigState{
			Hash:       "sha256:abc123",
			ReceivedAt: time.Now().UTC().Truncate(time.Second),
			Status:     "APPLIED",
		},
	}

	err := SaveConnectionState(dir, state)
	require.NoError(t, err)

	loaded, err := LoadConnectionState(dir)
	require.NoError(t, err)
	require.Equal(t, state.Server.Endpoint, loaded.Server.Endpoint)
	require.Equal(t, state.Server.LastSequenceNum, loaded.Server.LastSequenceNum)
	require.Equal(t, state.RemoteConfig.Hash, loaded.RemoteConfig.Hash)
	require.Equal(t, state.RemoteConfig.Status, loaded.RemoteConfig.Status)
}

func TestLoadConnectionState_NotExists(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadConnectionState(dir)
	require.Error(t, err)
}

func TestConnectionState_FilePermissions(t *testing.T) {
	dir := t.TempDir()

	state := &ConnectionState{
		Server: ServerState{
			Endpoint: "wss://opamp.example.com/v1/opamp",
		},
	}

	err := SaveConnectionState(dir, state)
	require.NoError(t, err)

	filePath := filepath.Join(dir, "connection.yaml")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./persistence/... -v -run TestSaveAndLoadConnectionState`
Expected: FAIL

**Step 3: Implement connection state persistence**

Create `persistence/connection.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package persistence

import (
	"os"
	"path/filepath"
	"time"

	"github.com/goccy/go-yaml"
)

const connectionFile = "connection.yaml"

// ConnectionState represents the persisted connection state.
type ConnectionState struct {
	Server       ServerState       `yaml:"server"`
	RemoteConfig RemoteConfigState `yaml:"remote_config"`
}

// ServerState represents the persisted server connection state.
type ServerState struct {
	Endpoint        string    `yaml:"endpoint"`
	LastConnected   time.Time `yaml:"last_connected"`
	LastSequenceNum uint64    `yaml:"last_sequence_num"`
}

// RemoteConfigState represents the persisted remote config state.
type RemoteConfigState struct {
	Hash       string    `yaml:"hash"`
	ReceivedAt time.Time `yaml:"received_at"`
	Status     string    `yaml:"status"`
	Error      string    `yaml:"error,omitempty"`
}

// SaveConnectionState saves the connection state to disk.
func SaveConnectionState(dir string, state *ConnectionState) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	content, err := yaml.Marshal(state)
	if err != nil {
		return err
	}

	filePath := filepath.Join(dir, connectionFile)
	return os.WriteFile(filePath, content, 0600)
}

// LoadConnectionState loads the connection state from disk.
func LoadConnectionState(dir string) (*ConnectionState, error) {
	filePath := filepath.Join(dir, connectionFile)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var state ConnectionState
	if err := yaml.Unmarshal(content, &state); err != nil {
		return nil, err
	}

	return &state, nil
}
```

**Step 4: Add missing import to test**

Update `persistence/connection_test.go` to add os import:
```go
import (
	"os"
	"path/filepath"
	// ... rest of imports
)
```

**Step 5: Run tests to verify they pass**

Run: `go test ./persistence/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add persistence/
git commit -m "feat(persistence): implement connection state persistence"
```

---

## Phase 4: Process Management

### Task 4.1: Implement Commander Keen (Process Controller)

**Files:**
- Create: `keen/keen.go`
- Create: `keen/keen_test.go`
- Create: `keen/signals_unix.go`
- Create: `keen/signals_windows.go`

**Step 1: Write tests for commander**

Create `keen/keen_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package keen

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestCommander_StartStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - need different test binary")
	}

	logger := zaptest.NewLogger(t)
	logsDir := t.TempDir()

	cmd, err := New(logger, logsDir, Config{
		Executable: "/bin/sleep",
		Args:       []string{"60"},
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.Start(ctx)
	require.NoError(t, err)
	require.True(t, cmd.IsRunning())
	require.Greater(t, cmd.Pid(), 0)

	err = cmd.Stop(ctx)
	require.NoError(t, err)
	require.False(t, cmd.IsRunning())
}

func TestCommander_StartAlreadyRunning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	logger := zaptest.NewLogger(t)
	logsDir := t.TempDir()

	cmd, err := New(logger, logsDir, Config{
		Executable: "/bin/sleep",
		Args:       []string{"60"},
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.Start(ctx)
	require.NoError(t, err)
	defer cmd.Stop(ctx)

	// Second start should be no-op
	err = cmd.Start(ctx)
	require.NoError(t, err)
}

func TestCommander_StopNotRunning(t *testing.T) {
	logger := zaptest.NewLogger(t)
	logsDir := t.TempDir()

	cmd, err := New(logger, logsDir, Config{
		Executable: "/bin/sleep",
		Args:       []string{"60"},
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.Stop(ctx)
	require.NoError(t, err)
}

func TestCommander_ExitedChannel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	logger := zaptest.NewLogger(t)
	logsDir := t.TempDir()

	cmd, err := New(logger, logsDir, Config{
		Executable: "/bin/sh",
		Args:       []string{"-c", "exit 0"},
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.Start(ctx)
	require.NoError(t, err)

	select {
	case <-cmd.Exited():
		// Expected
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for process to exit")
	}

	require.False(t, cmd.IsRunning())
}

func TestCommander_Restart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	logger := zaptest.NewLogger(t)
	logsDir := t.TempDir()

	cmd, err := New(logger, logsDir, Config{
		Executable: "/bin/sleep",
		Args:       []string{"60"},
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.Start(ctx)
	require.NoError(t, err)

	pid1 := cmd.Pid()

	err = cmd.Restart(ctx)
	require.NoError(t, err)
	require.True(t, cmd.IsRunning())

	pid2 := cmd.Pid()
	require.NotEqual(t, pid1, pid2, "PID should change after restart")

	cmd.Stop(ctx)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./keen/... -v`
Expected: FAIL (package not found)

**Step 3: Create commander implementation**

Create `keen/keen.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package keen

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Config holds the configuration for the commander.
type Config struct {
	Executable      string
	Args            []string
	Env             map[string]string
	PassthroughLogs bool
}

// Commander manages the lifecycle of an agent process.
type Commander struct {
	logger  *zap.Logger
	logsDir string
	cfg     Config
	cmd     *exec.Cmd
	running atomic.Bool
	doneCh  chan struct{}
	exitCh  chan struct{}
}

// New creates a new Commander instance.
func New(logger *zap.Logger, logsDir string, cfg Config) (*Commander, error) {
	return &Commander{
		logger:  logger,
		logsDir: logsDir,
		cfg:     cfg,
		doneCh:  make(chan struct{}, 1),
		exitCh:  make(chan struct{}, 1),
	}, nil
}

// Start starts the agent process.
func (c *Commander) Start(ctx context.Context) error {
	if c.running.Load() {
		return nil
	}

	// Drain channels from previous runs
	select {
	case <-c.doneCh:
	default:
	}
	select {
	case <-c.exitCh:
	default:
	}

	c.logger.Debug("Starting agent", zap.String("executable", c.cfg.Executable))

	c.cmd = exec.CommandContext(ctx, c.cfg.Executable, c.cfg.Args...)
	c.cmd.Env = c.buildEnv()
	c.cmd.SysProcAttr = sysProcAttrs()

	if c.cfg.PassthroughLogs {
		return c.startWithPassthroughLogging()
	}
	return c.startNormal()
}

func (c *Commander) buildEnv() []string {
	if c.cfg.Env == nil {
		return nil
	}
	env := os.Environ()
	for k, v := range c.cfg.Env {
		env = append(env, k+"="+v)
	}
	return env
}

func (c *Commander) startNormal() error {
	logFilePath := filepath.Join(c.logsDir, "agent.log")
	logFile, err := os.Create(logFilePath)
	if err != nil {
		return fmt.Errorf("cannot create log file %s: %w", logFilePath, err)
	}

	c.cmd.Stdout = logFile
	c.cmd.Stderr = logFile

	if err := c.cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start agent: %w", err)
	}

	c.logger.Debug("Agent process started", zap.Int("pid", c.cmd.Process.Pid))
	c.running.Store(true)

	go func() {
		defer logFile.Close()
		c.watch()
	}()

	return nil
}

func (c *Commander) startWithPassthroughLogging() error {
	stdoutPipe, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	stderrPipe, err := c.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	c.running.Store(true)

	agentLogger := c.logger.Named("agent")

	go c.pipeOutput(stdoutPipe, agentLogger, false)
	go c.pipeOutput(stderrPipe, agentLogger, true)

	c.logger.Debug("Agent process started", zap.Int("pid", c.cmd.Process.Pid))

	go c.watch()

	return nil
}

func (c *Commander) pipeOutput(pipe io.ReadCloser, logger *zap.Logger, isStderr bool) {
	reader := bufio.NewReader(pipe)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF && !errors.Is(err, os.ErrClosed) {
				c.logger.Error("Error reading agent output", zap.Error(err))
			}
			if line != "" {
				line = strings.TrimRight(line, "\r\n")
				if isStderr {
					logger.Error(line)
				} else {
					logger.Info(line)
				}
			}
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if isStderr {
			logger.Error(line)
		} else {
			logger.Info(line)
		}
	}
}

func (c *Commander) watch() {
	err := c.cmd.Wait()

	var exitError *exec.ExitError
	if err != nil && !errors.As(err, &exitError) {
		c.logger.Error("Error watching agent process", zap.Error(err))
	}

	c.running.Store(false)

	select {
	case c.doneCh <- struct{}{}:
	default:
	}
	select {
	case c.exitCh <- struct{}{}:
	default:
	}
}

// Stop stops the agent process gracefully.
func (c *Commander) Stop(ctx context.Context) error {
	if !c.running.Load() {
		return nil
	}

	pid := c.cmd.Process.Pid
	c.logger.Debug("Stopping agent process", zap.Int("pid", pid))

	if err := sendShutdownSignal(c.cmd.Process); err != nil {
		return err
	}

	// Wait with timeout for graceful shutdown
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	go func() {
		<-waitCtx.Done()
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			c.logger.Debug("Agent not responding to SIGTERM, sending SIGKILL", zap.Int("pid", pid))
			c.cmd.Process.Kill()
		}
	}()

	<-c.doneCh
	c.running.Store(false)

	return nil
}

// Restart restarts the agent process.
func (c *Commander) Restart(ctx context.Context) error {
	c.logger.Debug("Restarting agent")
	if err := c.Stop(ctx); err != nil {
		return err
	}
	return c.Start(ctx)
}

// ReloadConfig sends SIGHUP to the agent to reload configuration.
func (c *Commander) ReloadConfig() error {
	if c.cmd == nil || c.cmd.Process == nil {
		return errors.New("agent process is not running")
	}
	return sendReloadSignal(c.cmd.Process)
}

// Exited returns a channel that signals when the agent process exits.
func (c *Commander) Exited() <-chan struct{} {
	return c.exitCh
}

// Pid returns the agent process PID, or 0 if not running.
func (c *Commander) Pid() int {
	if c.cmd == nil || c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

// ExitCode returns the agent process exit code, or 0 if not exited.
func (c *Commander) ExitCode() int {
	if c.cmd == nil || c.cmd.ProcessState == nil {
		return 0
	}
	return c.cmd.ProcessState.ExitCode()
}

// IsRunning returns true if the agent process is running.
func (c *Commander) IsRunning() bool {
	return c.running.Load()
}
```

**Step 4: Create Unix signal handling**

Create `keen/signals_unix.go`:
```go
//go:build !windows

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package keen

import (
	"os"
	"syscall"
)

func sysProcAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func sendShutdownSignal(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}

func sendReloadSignal(process *os.Process) error {
	return process.Signal(syscall.SIGHUP)
}
```

**Step 5: Create Windows signal handling**

Create `keen/signals_windows.go`:
```go
//go:build windows

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package keen

import (
	"errors"
	"os"
	"syscall"
)

func sysProcAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func sendShutdownSignal(process *os.Process) error {
	// On Windows, we use CTRL_BREAK_EVENT to signal shutdown
	return process.Signal(os.Interrupt)
}

func sendReloadSignal(process *os.Process) error {
	// SIGHUP not available on Windows - caller should restart instead
	return errors.New("SIGHUP not supported on Windows, use restart instead")
}
```

**Step 6: Run tests to verify they pass**

Run: `go test ./keen/... -v`
Expected: PASS (some tests may be skipped on Windows)

**Step 7: Commit**

```bash
git add keen/
git commit -m "feat(keen): implement Commander Keen process management with platform-specific signals"
```

---

## Phase 5: OpAMP Integration

### Task 5.1: Implement OpAMP Client Wrapper

**Files:**
- Create: `opamp/client.go`
- Create: `opamp/client_test.go`
- Create: `opamp/callbacks.go`

**Step 1: Write tests for OpAMP client**

Create `opamp/client_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opamp

import (
	"testing"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewClient(t *testing.T) {
	logger := zaptest.NewLogger(t)
	callbacks := &Callbacks{}

	client, err := NewClient(logger, ClientConfig{
		Endpoint:    "ws://localhost:4320/v1/opamp",
		InstanceUID: "test-instance-uid",
	}, callbacks)
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestClientConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       ClientConfig
		expectErr bool
	}{
		{
			name: "valid websocket",
			cfg: ClientConfig{
				Endpoint:    "ws://localhost:4320/v1/opamp",
				InstanceUID: "test-uid",
			},
			expectErr: false,
		},
		{
			name: "valid wss",
			cfg: ClientConfig{
				Endpoint:    "wss://opamp.example.com/v1/opamp",
				InstanceUID: "test-uid",
			},
			expectErr: false,
		},
		{
			name: "missing endpoint",
			cfg: ClientConfig{
				InstanceUID: "test-uid",
			},
			expectErr: true,
		},
		{
			name: "missing instance UID",
			cfg: ClientConfig{
				Endpoint: "ws://localhost:4320/v1/opamp",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCapabilitiesToProto(t *testing.T) {
	caps := Capabilities{
		ReportsStatus:          true,
		AcceptsRemoteConfig:    true,
		ReportsEffectiveConfig: true,
		ReportsHealth:          true,
	}

	proto := caps.ToProto()
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_AcceptsRemoteConfig != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsEffectiveConfig != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsHealth != 0)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./opamp/... -v`
Expected: FAIL (package not found)

**Step 3: Implement callbacks**

Create `opamp/callbacks.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opamp

import (
	"context"

	"github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
)

// Callbacks handles OpAMP client callbacks.
type Callbacks struct {
	OnConnect               func(ctx context.Context)
	OnConnectFailed         func(ctx context.Context, err error)
	OnError                 func(ctx context.Context, err *protobufs.ServerErrorResponse)
	OnRemoteConfig          func(ctx context.Context, config *protobufs.AgentRemoteConfig) bool
	OnOpampConnectionSettings func(ctx context.Context, settings *protobufs.OpAMPConnectionSettings) bool
	OnPackagesAvailable     func(ctx context.Context, packages *protobufs.PackagesAvailable) bool
	OnCommand               func(ctx context.Context, command *protobufs.ServerToAgentCommand) bool
}

// Ensure Callbacks implements types.Callbacks
var _ types.Callbacks = (*Callbacks)(nil)

func (c *Callbacks) OnConnectFunc(ctx context.Context) {
	if c.OnConnect != nil {
		c.OnConnect(ctx)
	}
}

func (c *Callbacks) OnConnectFailedFunc(ctx context.Context, err error) {
	if c.OnConnectFailed != nil {
		c.OnConnectFailed(ctx, err)
	}
}

func (c *Callbacks) OnErrorFunc(ctx context.Context, err *protobufs.ServerErrorResponse) {
	if c.OnError != nil {
		c.OnError(ctx, err)
	}
}

func (c *Callbacks) OnMessageFunc(ctx context.Context, msg *types.MessageData) {
	// Handle remote config
	if msg.RemoteConfig != nil && c.OnRemoteConfig != nil {
		c.OnRemoteConfig(ctx, msg.RemoteConfig)
	}

	// Handle connection settings
	if msg.OwnConnectionSettings != nil && c.OnOpampConnectionSettings != nil {
		c.OnOpampConnectionSettings(ctx, msg.OwnConnectionSettings)
	}

	// Handle packages
	if msg.PackagesAvailable != nil && c.OnPackagesAvailable != nil {
		c.OnPackagesAvailable(ctx, msg.PackagesAvailable)
	}

	// Handle commands
	if msg.Command != nil && c.OnCommand != nil {
		c.OnCommand(ctx, msg.Command)
	}
}

func (c *Callbacks) SaveRemoteConfigStatusFunc(ctx context.Context, status *protobufs.RemoteConfigStatus) {
	// Status saved by caller
}

func (c *Callbacks) GetEffectiveConfigFunc(ctx context.Context) (*protobufs.EffectiveConfig, error) {
	return nil, nil
}
```

**Step 4: Implement OpAMP client wrapper**

Create `opamp/client.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opamp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/open-telemetry/opamp-go/client"
	"github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

// ClientConfig holds configuration for the OpAMP client.
type ClientConfig struct {
	Endpoint    string
	InstanceUID string
	Headers     http.Header
	TLSConfig   *types.TLSConfig
	Capabilities Capabilities
}

// Validate validates the client configuration.
func (c ClientConfig) Validate() error {
	if c.Endpoint == "" {
		return errors.New("endpoint is required")
	}
	if c.InstanceUID == "" {
		return errors.New("instance UID is required")
	}
	return nil
}

// Capabilities represents the supervisor's OpAMP capabilities.
type Capabilities struct {
	ReportsStatus                  bool
	AcceptsRemoteConfig            bool
	ReportsEffectiveConfig         bool
	AcceptsPackages                bool
	ReportsPackageStatuses         bool
	ReportsOwnTraces               bool
	ReportsOwnMetrics              bool
	ReportsOwnLogs                 bool
	AcceptsOpAMPConnectionSettings bool
	AcceptsRestartCommand          bool
	ReportsHealth                  bool
	ReportsRemoteConfig            bool
}

// ToProto converts capabilities to protobuf format.
func (c Capabilities) ToProto() protobufs.AgentCapabilities {
	caps := protobufs.AgentCapabilities(0)

	if c.ReportsStatus {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus
	}
	if c.AcceptsRemoteConfig {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_AcceptsRemoteConfig
	}
	if c.ReportsEffectiveConfig {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsEffectiveConfig
	}
	if c.AcceptsPackages {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_AcceptsPackages
	}
	if c.ReportsPackageStatuses {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsPackageStatuses
	}
	if c.ReportsOwnTraces {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsOwnTraces
	}
	if c.ReportsOwnMetrics {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsOwnMetrics
	}
	if c.ReportsOwnLogs {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsOwnLogs
	}
	if c.AcceptsOpAMPConnectionSettings {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_AcceptsOpAMPConnectionSettings
	}
	if c.AcceptsRestartCommand {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_AcceptsRestartCommand
	}
	if c.ReportsHealth {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsHealth
	}
	if c.ReportsRemoteConfig {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsRemoteConfig
	}

	return caps
}

// Client wraps the opamp-go client with supervisor-specific functionality.
type Client struct {
	logger     *zap.Logger
	cfg        ClientConfig
	callbacks  *Callbacks
	opampClient client.OpAMPClient
}

// NewClient creates a new OpAMP client wrapper.
func NewClient(logger *zap.Logger, cfg ClientConfig, callbacks *Callbacks) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Client{
		logger:    logger,
		cfg:       cfg,
		callbacks: callbacks,
	}, nil
}

// Start starts the OpAMP client connection.
func (c *Client) Start(ctx context.Context) error {
	u, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return err
	}

	var opampClient client.OpAMPClient

	if strings.HasPrefix(u.Scheme, "ws") {
		opampClient = client.NewWebSocket(c.logger.Sugar())
	} else {
		opampClient = client.NewHTTP(c.logger.Sugar())
	}

	settings := types.StartSettings{
		OpAMPServerURL: c.cfg.Endpoint,
		InstanceUid:    types.InstanceUid(c.cfg.InstanceUID),
		Callbacks:      c.callbacks,
		Header:         c.cfg.Headers,
		TLSConfig:      c.cfg.TLSConfig,
		Capabilities:   c.cfg.Capabilities.ToProto(),
	}

	if err := opampClient.Start(ctx, settings); err != nil {
		return err
	}

	c.opampClient = opampClient
	return nil
}

// Stop stops the OpAMP client connection.
func (c *Client) Stop(ctx context.Context) error {
	if c.opampClient == nil {
		return nil
	}
	return c.opampClient.Stop(ctx)
}

// SetAgentDescription updates the agent description.
func (c *Client) SetAgentDescription(desc *protobufs.AgentDescription) error {
	if c.opampClient == nil {
		return errors.New("client not started")
	}
	return c.opampClient.SetAgentDescription(desc)
}

// SetHealth updates the agent health status.
func (c *Client) SetHealth(health *protobufs.ComponentHealth) error {
	if c.opampClient == nil {
		return errors.New("client not started")
	}
	return c.opampClient.SetHealth(health)
}

// UpdateEffectiveConfig updates the effective configuration.
func (c *Client) UpdateEffectiveConfig(ctx context.Context) error {
	if c.opampClient == nil {
		return errors.New("client not started")
	}
	return c.opampClient.UpdateEffectiveConfig(ctx)
}

// SetRemoteConfigStatus sets the remote config status.
func (c *Client) SetRemoteConfigStatus(status *protobufs.RemoteConfigStatus) error {
	if c.opampClient == nil {
		return errors.New("client not started")
	}
	return c.opampClient.SetRemoteConfigStatus(status)
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./opamp/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add opamp/
git commit -m "feat(opamp): implement OpAMP client wrapper with callbacks"
```

---

### Task 5.2: Implement Local OpAMP Server

**Files:**
- Create: `opamp/server.go`
- Create: `opamp/server_test.go`

**Step 1: Write tests for local OpAMP server**

Create `opamp/server_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opamp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewServer(t *testing.T) {
	logger := zaptest.NewLogger(t)
	callbacks := &ServerCallbacks{}

	server, err := NewServer(logger, ServerConfig{
		ListenEndpoint: "localhost:0",
	}, callbacks)
	require.NoError(t, err)
	require.NotNil(t, server)
}

func TestServer_StartStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	callbacks := &ServerCallbacks{}

	server, err := NewServer(logger, ServerConfig{
		ListenEndpoint: "localhost:0",
	}, callbacks)
	require.NoError(t, err)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	addr := server.Addr()
	require.NotEmpty(t, addr)

	err = server.Stop(ctx)
	require.NoError(t, err)
}

func TestServerConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       ServerConfig
		expectErr bool
	}{
		{
			name: "valid",
			cfg: ServerConfig{
				ListenEndpoint: "localhost:4320",
			},
			expectErr: false,
		},
		{
			name:      "empty endpoint",
			cfg:       ServerConfig{},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./opamp/... -v -run TestNewServer`
Expected: FAIL

**Step 3: Implement local OpAMP server**

Create `opamp/server.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opamp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server"
	"github.com/open-telemetry/opamp-go/server/types"
	"go.uber.org/zap"
)

// ServerConfig holds configuration for the local OpAMP server.
type ServerConfig struct {
	ListenEndpoint string
}

// Validate validates the server configuration.
func (c ServerConfig) Validate() error {
	if c.ListenEndpoint == "" {
		return errors.New("listen endpoint is required")
	}
	return nil
}

// ServerCallbacks handles OpAMP server callbacks.
type ServerCallbacks struct {
	OnConnect              func(conn types.Connection)
	OnDisconnect           func(conn types.Connection)
	OnMessage              func(conn types.Connection, msg *protobufs.AgentToServer)
}

// Server wraps the opamp-go server for local collector communication.
type Server struct {
	logger      *zap.Logger
	cfg         ServerConfig
	callbacks   *ServerCallbacks
	opampServer server.OpAMPServer
	httpServer  *http.Server
	listener    net.Listener
	mu          sync.Mutex
	connections map[types.Connection]struct{}
}

// NewServer creates a new local OpAMP server.
func NewServer(logger *zap.Logger, cfg ServerConfig, callbacks *ServerCallbacks) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Server{
		logger:      logger,
		cfg:         cfg,
		callbacks:   callbacks,
		connections: make(map[types.Connection]struct{}),
	}, nil
}

// Start starts the local OpAMP server.
func (s *Server) Start(ctx context.Context) error {
	s.opampServer = server.New(s.logger.Sugar())

	// Create listener
	listener, err := net.Listen("tcp", s.cfg.ListenEndpoint)
	if err != nil {
		return err
	}
	s.listener = listener

	settings := server.StartSettings{
		Settings: server.Settings{
			Callbacks: s,
		},
		ListenEndpoint: listener.Addr().String(),
	}

	return s.opampServer.Start(settings)
}

// Stop stops the local OpAMP server.
func (s *Server) Stop(ctx context.Context) error {
	if s.opampServer == nil {
		return nil
	}
	s.opampServer.Stop(ctx)
	if s.listener != nil {
		s.listener.Close()
	}
	return nil
}

// Addr returns the server's listen address.
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// SendToAgent sends a message to a connected agent.
func (s *Server) SendToAgent(conn types.Connection, msg *protobufs.ServerToAgent) error {
	return conn.Send(context.Background(), msg)
}

// Broadcast sends a message to all connected agents.
func (s *Server) Broadcast(msg *protobufs.ServerToAgent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for conn := range s.connections {
		if err := conn.Send(context.Background(), msg); err != nil {
			s.logger.Error("Failed to send to agent", zap.Error(err))
		}
	}
}

// Server callback implementations

func (s *Server) OnConnecting(request *http.Request) types.ConnectionResponse {
	return types.ConnectionResponse{
		Accept: true,
	}
}

func (s *Server) OnConnected(ctx context.Context, conn types.Connection) {
	s.mu.Lock()
	s.connections[conn] = struct{}{}
	s.mu.Unlock()

	s.logger.Debug("Agent connected")
	if s.callbacks.OnConnect != nil {
		s.callbacks.OnConnect(conn)
	}
}

func (s *Server) OnMessage(ctx context.Context, conn types.Connection, msg *protobufs.AgentToServer) *protobufs.ServerToAgent {
	if s.callbacks.OnMessage != nil {
		s.callbacks.OnMessage(conn, msg)
	}
	return nil
}

func (s *Server) OnConnectionClose(conn types.Connection) {
	s.mu.Lock()
	delete(s.connections, conn)
	s.mu.Unlock()

	s.logger.Debug("Agent disconnected")
	if s.callbacks.OnDisconnect != nil {
		s.callbacks.OnDisconnect(conn)
	}
}

// Ensure Server implements server.Callbacks
var _ server.Callbacks = (*Server)(nil)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./opamp/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add opamp/server.go opamp/server_test.go
git commit -m "feat(opamp): implement local OpAMP server for collector communication"
```

---

## Phase 6: Core Supervisor Engine

### Task 6.1: Implement Supervisor Core

**Files:**
- Create: `supervisor/supervisor.go`
- Create: `supervisor/supervisor_test.go`

**Step 1: Write tests for supervisor**

Create `supervisor/supervisor_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"testing"

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
```

**Step 2: Run test to verify it fails**

Run: `go test ./supervisor/... -v`
Expected: FAIL (package not found)

**Step 3: Implement supervisor core**

Create `supervisor/supervisor.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/Graylog2/collector-sidecar/superv/keen"
	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/opamp"
	"github.com/Graylog2/collector-sidecar/superv/persistence"
)

// Supervisor coordinates the management of an OpenTelemetry Collector.
type Supervisor struct {
	logger      *zap.Logger
	cfg         config.Config
	instanceUID string
	keen        *keen.Commander
	opampClient *opamp.Client
	opampServer *opamp.Server
	mu          sync.RWMutex
	running     bool
}

// New creates a new Supervisor instance.
func New(logger *zap.Logger, cfg config.Config) (*Supervisor, error) {
	// Load or create instance UID
	uid, err := persistence.LoadOrCreateInstanceUID(cfg.Persistence.Dir)
	if err != nil {
		return nil, err
	}

	return &Supervisor{
		logger:      logger,
		cfg:         cfg,
		instanceUID: uid,
	}, nil
}

// InstanceUID returns the supervisor's unique instance identifier.
func (s *Supervisor) InstanceUID() string {
	return s.instanceUID
}

// Start starts the supervisor and begins managing the collector.
func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	s.logger.Info("Starting supervisor",
		zap.String("instance_uid", s.instanceUID),
		zap.String("endpoint", s.cfg.Server.Endpoint),
	)

	// Create commander for agent process management
	cmd, err := keen.New(s.logger, s.cfg.Persistence.Dir, keen.Config{
		Executable:      s.cfg.Agent.Executable,
		Args:            s.cfg.Agent.Args,
		Env:             s.cfg.Agent.Env,
		PassthroughLogs: s.cfg.Agent.PassthroughLogs,
	})
	if err != nil {
		return err
	}
	s.keen = cmd

	// Create local OpAMP server for collector
	serverCallbacks := &opamp.ServerCallbacks{
		OnConnect: func(conn interface{}) {
			s.logger.Info("Collector connected to local OpAMP server")
		},
		OnDisconnect: func(conn interface{}) {
			s.logger.Info("Collector disconnected from local OpAMP server")
		},
	}

	opampServer, err := opamp.NewServer(s.logger, opamp.ServerConfig{
		ListenEndpoint: s.cfg.LocalOpAMP.Endpoint,
	}, serverCallbacks)
	if err != nil {
		return err
	}
	s.opampServer = opampServer

	// Start local OpAMP server
	if err := s.opampServer.Start(ctx); err != nil {
		return err
	}

	// Create OpAMP client for upstream server
	clientCallbacks := &opamp.Callbacks{
		OnConnect: func(ctx context.Context) {
			s.logger.Info("Connected to OpAMP server")
		},
		OnConnectFailed: func(ctx context.Context, err error) {
			s.logger.Error("Failed to connect to OpAMP server", zap.Error(err))
		},
		OnRemoteConfig: func(ctx context.Context, cfg interface{}) bool {
			s.logger.Info("Received remote configuration")
			// TODO: Apply configuration
			return true
		},
		OnOpampConnectionSettings: func(ctx context.Context, settings interface{}) bool {
			s.logger.Info("Received connection settings update")
			// TODO: Handle token refresh
			return true
		},
	}

	opampClient, err := opamp.NewClient(s.logger, opamp.ClientConfig{
		Endpoint:    s.cfg.Server.Endpoint,
		InstanceUID: s.instanceUID,
		Headers:     s.cfg.Server.ToHTTPHeaders(),
		Capabilities: opamp.Capabilities{
			ReportsStatus:                  true,
			AcceptsRemoteConfig:            true,
			ReportsEffectiveConfig:         true,
			ReportsHealth:                  true,
			AcceptsOpAMPConnectionSettings: true,
			AcceptsRestartCommand:          true,
		},
	}, clientCallbacks)
	if err != nil {
		s.opampServer.Stop(ctx)
		return err
	}
	s.opampClient = opampClient

	// Start OpAMP client
	if err := s.opampClient.Start(ctx); err != nil {
		s.opampServer.Stop(ctx)
		return err
	}

	// Start the collector agent
	if err := s.keen.Start(ctx); err != nil {
		s.opampClient.Stop(ctx)
		s.opampServer.Stop(ctx)
		return err
	}

	s.running = true
	return nil
}

// Stop stops the supervisor and the managed collector.
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("Stopping supervisor")

	// Stop commander (agent)
	if s.keen != nil {
		if err := s.keen.Stop(ctx); err != nil {
			s.logger.Error("Error stopping agent", zap.Error(err))
		}
	}

	// Stop OpAMP client
	if s.opampClient != nil {
		if err := s.opampClient.Stop(ctx); err != nil {
			s.logger.Error("Error stopping OpAMP client", zap.Error(err))
		}
	}

	// Stop OpAMP server
	if s.opampServer != nil {
		if err := s.opampServer.Stop(ctx); err != nil {
			s.logger.Error("Error stopping OpAMP server", zap.Error(err))
		}
	}

	s.running = false
	return nil
}

// IsRunning returns true if the supervisor is running.
func (s *Supervisor) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./supervisor/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add supervisor/
git commit -m "feat(supervisor): implement core supervisor engine"
```

---

### Task 6.2: Wire Up Main Entry Point

**Files:**
- Modify: `cmd/supervisor/main.go`

**Step 1: Update main.go with full CLI**

Modify `cmd/supervisor/main.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/supervisor"
	"github.com/Graylog2/collector-sidecar/superv/version"
)

func main() {
	var (
		configPath    string
		showVersion   bool
		enrollmentURL string
	)

	flag.StringVar(&configPath, "config", "", "Path to configuration file")
	flag.BoolVar(&showVersion, "version", false, "Show version and exit")
	flag.StringVar(&enrollmentURL, "enrollment-url", "", "Enrollment URL for zero-touch bootstrap (e.g., https://server/opamp/enroll/<JWT>)")
	flag.Parse()

	if showVersion {
		fmt.Println(version.Version())
		os.Exit(0)
	}

	// Initialize logger
	logger, err := initLogger("info", "json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Load configuration
	var cfg config.Config
	if configPath != "" {
		cfg, err = config.Load(configPath)
		if err != nil {
			logger.Fatal("Failed to load configuration", zap.Error(err))
		}
	} else {
		cfg = config.DefaultConfig()
	}

	// Override with enrollment URL if provided
	if enrollmentURL != "" {
		cfg.Auth.EnrollmentURL = enrollmentURL
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration", zap.Error(err))
	}

	// Create supervisor
	sup, err := supervisor.New(logger, cfg)
	if err != nil {
		logger.Fatal("Failed to create supervisor", zap.Error(err))
	}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("Received signal, shutting down", zap.String("signal", sig.String()))
		cancel()
	}()

	// Start supervisor
	if err := sup.Start(ctx); err != nil {
		logger.Fatal("Failed to start supervisor", zap.Error(err))
	}

	// Wait for shutdown signal
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Agent.Shutdown.GracefulTimeout)
	defer shutdownCancel()

	if err := sup.Stop(shutdownCtx); err != nil {
		logger.Error("Error during shutdown", zap.Error(err))
	}

	logger.Info("Supervisor stopped")
}

func initLogger(level, format string) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	var cfg zap.Config
	if format == "json" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
	}
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)

	return cfg.Build()
}
```

**Step 2: Verify build works**

Run: `go build -o supervisor ./cmd/supervisor`
Expected: Binary created successfully

**Step 3: Test help output**

Run: `./supervisor -h`
Expected: Shows usage with flags

**Step 4: Commit**

```bash
git add cmd/supervisor/main.go
git commit -m "feat(cli): wire up main entry point with full CLI support"
```

---

## Phase 7: Configuration Merging & Auth

### Task 7.1: Implement Configuration Merging

**Files:**
- Create: `configmerge/merge.go`
- Create: `configmerge/merge_test.go`

**Step 1: Write tests for config merging**

Create `configmerge/merge_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package configmerge

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeConfigs_SimpleOverride(t *testing.T) {
	base := []byte(`
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: "0.0.0.0:4317"
exporters:
  debug: {}
`)
	override := []byte(`
exporters:
  otlp:
    endpoint: "http://collector:4317"
`)

	result, err := MergeConfigs(base, override)
	require.NoError(t, err)

	// Should have both exporters
	require.Contains(t, string(result), "debug")
	require.Contains(t, string(result), "otlp")
	require.Contains(t, string(result), "http://collector:4317")
}

func TestMergeConfigs_DeepMerge(t *testing.T) {
	base := []byte(`
processors:
  batch:
    timeout: 1s
    send_batch_size: 1000
`)
	override := []byte(`
processors:
  batch:
    timeout: 5s
`)

	result, err := MergeConfigs(base, override)
	require.NoError(t, err)

	// Override should win for timeout
	require.Contains(t, string(result), "5s")
	// But send_batch_size should be preserved
	require.Contains(t, string(result), "send_batch_size")
}

func TestMergeConfigs_MultipleOverrides(t *testing.T) {
	configs := [][]byte{
		[]byte(`a: 1`),
		[]byte(`b: 2`),
		[]byte(`c: 3`),
	}

	result, err := MergeMultiple(configs...)
	require.NoError(t, err)

	require.Contains(t, string(result), "a: 1")
	require.Contains(t, string(result), "b: 2")
	require.Contains(t, string(result), "c: 3")
}

func TestMergeConfigs_EmptyBase(t *testing.T) {
	base := []byte(``)
	override := []byte(`key: value`)

	result, err := MergeConfigs(base, override)
	require.NoError(t, err)
	require.Contains(t, string(result), "key: value")
}

func TestMergeConfigs_EmptyOverride(t *testing.T) {
	base := []byte(`key: value`)
	override := []byte(``)

	result, err := MergeConfigs(base, override)
	require.NoError(t, err)
	require.Contains(t, string(result), "key: value")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./configmerge/... -v`
Expected: FAIL (package not found)

**Step 3: Implement config merging**

Create `configmerge/merge.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package configmerge

import (
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
)

// MergeConfigs merges two YAML configurations, with override taking precedence.
func MergeConfigs(base, override []byte) ([]byte, error) {
	k := koanf.New(".")

	// Load base config
	if len(base) > 0 {
		if err := k.Load(rawbytes.Provider(base), yaml.Parser()); err != nil {
			return nil, err
		}
	}

	// Merge override config
	if len(override) > 0 {
		if err := k.Load(rawbytes.Provider(override), yaml.Parser()); err != nil {
			return nil, err
		}
	}

	// Marshal back to YAML
	return k.Marshal(yaml.Parser())
}

// MergeMultiple merges multiple YAML configurations in order.
// Later configs take precedence over earlier ones.
func MergeMultiple(configs ...[]byte) ([]byte, error) {
	k := koanf.New(".")

	for _, cfg := range configs {
		if len(cfg) > 0 {
			if err := k.Load(rawbytes.Provider(cfg), yaml.Parser()); err != nil {
				return nil, err
			}
		}
	}

	return k.Marshal(yaml.Parser())
}

// InjectSettings injects supervisor settings into a collector config.
func InjectSettings(config []byte, settings map[string]interface{}) ([]byte, error) {
	k := koanf.New(".")

	// Load existing config
	if len(config) > 0 {
		if err := k.Load(rawbytes.Provider(config), yaml.Parser()); err != nil {
			return nil, err
		}
	}

	// Inject settings
	for key, value := range settings {
		k.Set(key, value)
	}

	return k.Marshal(yaml.Parser())
}
```

**Step 4: Add rawbytes provider dependency**

Run: `go get github.com/knadh/koanf/providers/rawbytes@latest && go mod tidy`
Expected: Downloads rawbytes provider

**Step 5: Run tests to verify they pass**

Run: `go test ./configmerge/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add configmerge/ go.mod go.sum
git commit -m "feat(configmerge): implement YAML configuration merging"
```

---

### Task 7.2: Implement CSR Flow & Authentication

**Files:**
- Create: `auth/jwks.go`
- Create: `auth/jwks_test.go`
- Create: `auth/enrollment.go`
- Create: `auth/enrollment_test.go`
- Create: `auth/keypair.go`
- Create: `auth/keypair_test.go`
- Create: `auth/csr.go`
- Create: `auth/csr_test.go`
- Create: `auth/jwt.go`
- Create: `auth/jwt_test.go`

**Step 1: Write tests for JWKS fetching**

Create `auth/jwks_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchJWKS_Success(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)

	jwks := map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "OKP",
				"crv": "Ed25519",
				"kid": "test-key-1",
				"x":   base64.RawURLEncoding.EncodeToString(pub),
				"use": "sig",
			},
		},
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/.well-known/jwks.json", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	defer server.Close()

	keys, err := FetchJWKS(server.Client(), server.URL)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, "test-key-1", keys[0].KeyID)
}

func TestFetchJWKS_InvalidResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := FetchJWKS(server.Client(), server.URL)
	require.Error(t, err)
}

func TestGetKeyByID(t *testing.T) {
	pub1, _, _ := ed25519.GenerateKey(nil)
	pub2, _, _ := ed25519.GenerateKey(nil)

	keys := []JWK{
		{KeyID: "key-1", PublicKey: pub1},
		{KeyID: "key-2", PublicKey: pub2},
	}

	key, err := GetKeyByID(keys, "key-2")
	require.NoError(t, err)
	require.Equal(t, pub2, key.PublicKey)

	_, err = GetKeyByID(keys, "nonexistent")
	require.Error(t, err)
}
```

**Step 2: Implement JWKS fetching**

Create `auth/jwks.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// JWK represents a JSON Web Key.
type JWK struct {
	KeyID     string
	PublicKey ed25519.PublicKey
}

type jwksResponse struct {
	Keys []jwkEntry `json:"keys"`
}

type jwkEntry struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	X   string `json:"x"`
	Use string `json:"use"`
}

// FetchJWKS fetches the JWKS from the server's well-known endpoint.
func FetchJWKS(client *http.Client, baseURL string) ([]JWK, error) {
	resp, err := client.Get(baseURL + "/.well-known/jwks.json")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS request failed with status %d", resp.StatusCode)
	}

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to decode JWKS: %w", err)
	}

	var keys []JWK
	for _, entry := range jwks.Keys {
		if entry.Kty != "OKP" || entry.Crv != "Ed25519" {
			continue // Skip non-Ed25519 keys
		}

		pubBytes, err := base64.RawURLEncoding.DecodeString(entry.X)
		if err != nil {
			continue
		}

		keys = append(keys, JWK{
			KeyID:     entry.Kid,
			PublicKey: ed25519.PublicKey(pubBytes),
		})
	}

	return keys, nil
}

// GetKeyByID finds a key by its ID in the JWKS.
func GetKeyByID(keys []JWK, kid string) (*JWK, error) {
	for _, k := range keys {
		if k.KeyID == kid {
			return &k, nil
		}
	}
	return nil, errors.New("key not found in JWKS")
}
```

**Step 3: Write tests for enrollment JWT validation**

Create `auth/enrollment_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseEnrollmentURL(t *testing.T) {
	url := "https://opamp.example.com/opamp/enroll/eyJhbGciOiJFZERTQSJ9.eyJ0ZW5hbnRfaWQiOiJ0ZXN0In0.sig"

	hostname, jwt, err := ParseEnrollmentURL(url)
	require.NoError(t, err)
	require.Equal(t, "opamp.example.com", hostname)
	require.Equal(t, "eyJhbGciOiJFZERTQSJ9.eyJ0ZW5hbnRfaWQiOiJ0ZXN0In0.sig", jwt)
}

func TestParseEnrollmentURL_InvalidFormat(t *testing.T) {
	_, _, err := ParseEnrollmentURL("not-a-url")
	require.Error(t, err)

	_, _, err = ParseEnrollmentURL("https://example.com/no-jwt-here")
	require.Error(t, err)
}

func TestValidateEnrollmentJWT(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)

	claims := &EnrollmentClaims{
		TenantID:     "test-tenant",
		KeyAlgorithm: "Ed25519",
		AgentLabels:  map[string]string{"env": "test"},
	}

	token := createSignedJWT(t, priv, "test-kid", claims, time.Hour)

	keys := []JWK{{KeyID: "test-kid", PublicKey: pub}}

	validated, err := ValidateEnrollmentJWT(token, keys)
	require.NoError(t, err)
	require.Equal(t, "test-tenant", validated.TenantID)
	require.Equal(t, "Ed25519", validated.KeyAlgorithm)
}

func TestValidateEnrollmentJWT_Expired(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)

	claims := &EnrollmentClaims{TenantID: "test"}
	token := createSignedJWT(t, priv, "test-kid", claims, -time.Hour) // Expired

	keys := []JWK{{KeyID: "test-kid", PublicKey: pub}}

	_, err := ValidateEnrollmentJWT(token, keys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expired")
}

func TestValidateEnrollmentJWT_InvalidSignature(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	otherPub, _, _ := ed25519.GenerateKey(nil) // Different key

	claims := &EnrollmentClaims{TenantID: "test"}
	token := createSignedJWT(t, priv, "test-kid", claims, time.Hour)

	keys := []JWK{{KeyID: "test-kid", PublicKey: otherPub}}

	_, err := ValidateEnrollmentJWT(token, keys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "signature")
}
```

**Step 4: Implement enrollment JWT validation**

Create `auth/enrollment.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// EnrollmentClaims represents claims from an enrollment JWT.
type EnrollmentClaims struct {
	Issuer       string            `json:"iss"`
	TenantID     string            `json:"tenant_id"`
	KeyAlgorithm string            `json:"key_algorithm"`
	AgentLabels  map[string]string `json:"agent_labels"`
	ExpiresAt    time.Time         `json:"-"`
	Exp          int64             `json:"exp"`
}

// ParseEnrollmentURL extracts the hostname and JWT from an enrollment URL.
// URL format: https://server.example.com/opamp/enroll/<JWT>
func ParseEnrollmentURL(enrollmentURL string) (hostname string, jwt string, err error) {
	u, err := url.Parse(enrollmentURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid enrollment URL: %w", err)
	}

	if u.Scheme != "https" {
		return "", "", errors.New("enrollment URL must use HTTPS")
	}

	// Extract JWT from path (last segment after /enroll/)
	parts := strings.Split(u.Path, "/enroll/")
	if len(parts) != 2 || parts[1] == "" {
		return "", "", errors.New("enrollment URL must contain /enroll/<JWT>")
	}

	return u.Host, parts[1], nil
}

// ValidateEnrollmentJWT validates an enrollment JWT against the JWKS.
func ValidateEnrollmentJWT(token string, keys []JWK) (*EnrollmentClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid JWT format")
	}

	// Decode header to get key ID
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("failed to decode JWT header")
	}

	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, errors.New("failed to parse JWT header")
	}

	if header.Alg != "EdDSA" {
		return nil, fmt.Errorf("unsupported algorithm: %s", header.Alg)
	}

	// Find the key
	key, err := GetKeyByID(keys, header.Kid)
	if err != nil {
		return nil, err
	}

	// Verify signature
	signedContent := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("failed to decode signature")
	}

	if !ed25519.Verify(key.PublicKey, []byte(signedContent), signature) {
		return nil, errors.New("invalid JWT signature")
	}

	// Decode claims
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("failed to decode JWT claims")
	}

	var claims EnrollmentClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, errors.New("failed to parse JWT claims")
	}

	if claims.Exp > 0 {
		claims.ExpiresAt = time.Unix(claims.Exp, 0)
	}

	// Check expiration
	if time.Now().After(claims.ExpiresAt) {
		return nil, errors.New("JWT has expired")
	}

	return &claims, nil
}
```

**Step 5: Write tests for keypair generation**

Create `auth/keypair_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/curve25519"
)

func TestGenerateSigningKeypair(t *testing.T) {
	pub, priv, err := GenerateSigningKeypair()
	require.NoError(t, err)
	require.Len(t, pub, ed25519.PublicKeySize)
	require.Len(t, priv, ed25519.PrivateKeySize)

	// Verify the keypair works for signing
	msg := []byte("test message")
	sig := ed25519.Sign(priv, msg)
	require.True(t, ed25519.Verify(pub, msg, sig))
}

func TestGenerateEncryptionKeypair(t *testing.T) {
	pub, priv, err := GenerateEncryptionKeypair()
	require.NoError(t, err)
	require.Len(t, pub, curve25519.PointSize)
	require.Len(t, priv, curve25519.ScalarSize)

	// Verify ECDH works
	otherPub, otherPriv, _ := GenerateEncryptionKeypair()

	shared1, err := curve25519.X25519(priv, otherPub)
	require.NoError(t, err)

	shared2, err := curve25519.X25519(otherPriv, pub)
	require.NoError(t, err)

	require.Equal(t, shared1, shared2)
}
```

**Step 6: Implement keypair generation**

Create `auth/keypair.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/ed25519"
	"crypto/rand"

	"golang.org/x/crypto/curve25519"
)

// GenerateSigningKeypair generates a new Ed25519 keypair for signing.
func GenerateSigningKeypair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

// GenerateEncryptionKeypair generates a new X25519 keypair for encryption.
func GenerateEncryptionKeypair() (publicKey, privateKey []byte, err error) {
	privateKey = make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(privateKey); err != nil {
		return nil, nil, err
	}

	publicKey, err = curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		return nil, nil, err
	}

	return publicKey, privateKey, nil
}
```

**Step 7: Write tests for CSR creation**

Create `auth/csr_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/ed25519"
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateCSR(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	encPub, _, _ := GenerateEncryptionKeypair()

	instanceUID := "01HQ3K5V7X2M4N8P9R0S1T2U3V"

	csrDER, err := CreateCSR(priv, instanceUID, encPub)
	require.NoError(t, err)
	require.NotEmpty(t, csrDER)

	// Parse and verify the CSR
	csr, err := x509.ParseCertificateRequest(csrDER)
	require.NoError(t, err)
	require.Equal(t, instanceUID, csr.Subject.CommonName)
	require.NoError(t, csr.CheckSignature())
}

func TestCreateCSR_IncludesEncryptionKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	encPub, _, _ := GenerateEncryptionKeypair()

	csrDER, err := CreateCSR(priv, "test-uid", encPub)
	require.NoError(t, err)

	csr, err := x509.ParseCertificateRequest(csrDER)
	require.NoError(t, err)

	// Encryption key should be in extensions
	require.NotEmpty(t, csr.Extensions)
}
```

**Step 8: Implement CSR creation**

Create `auth/csr.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
)

// OID for X25519 encryption public key extension (custom OID - TBD)
var oidEncryptionPublicKey = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1, 1}

// CreateCSR creates a Certificate Signing Request with the instance UID as CN
// and the X25519 encryption public key as a custom extension.
func CreateCSR(signingKey ed25519.PrivateKey, instanceUID string, encryptionPubKey []byte) ([]byte, error) {
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: instanceUID,
		},
		SignatureAlgorithm: x509.PureEd25519,
		ExtraExtensions: []pkix.Extension{
			{
				Id:       oidEncryptionPublicKey,
				Critical: false,
				Value:    encryptionPubKey,
			},
		},
	}

	return x509.CreateCertificateRequest(rand.Reader, template, signingKey)
}

// ParseCSR parses a DER-encoded CSR.
func ParseCSR(csrDER []byte) (*x509.CertificateRequest, error) {
	return x509.ParseCertificateRequest(csrDER)
}
```

**Step 9: Write tests for supervisor-signed JWT**

Create `auth/jwt_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCreateSupervisorJWT(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)

	// Create a test certificate
	cert := createTestCertWithPublicKey(t, pub)
	certFingerprint := sha256.Sum256(cert.Raw)

	instanceUID := "01HQ3K5V7X2M4N8P9R0S1T2U3V"
	audience := "opamp.example.com"
	lifetime := 5 * time.Minute

	token, err := CreateSupervisorJWT(priv, cert, instanceUID, audience, lifetime)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// Parse and verify the token
	header, claims, err := ParseSupervisorJWT(token)
	require.NoError(t, err)

	require.Equal(t, "EdDSA", header.Alg)
	require.Equal(t, hex.EncodeToString(certFingerprint[:]), header.X5tS256)

	require.Equal(t, instanceUID, claims.Subject)
	require.Equal(t, audience, claims.Audience)
	require.WithinDuration(t, time.Now(), claims.IssuedAt, time.Second)
	require.WithinDuration(t, time.Now().Add(lifetime), claims.ExpiresAt, time.Second)
}

func TestVerifySupervisorJWT(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	cert := createTestCertWithPublicKey(t, pub)

	token, _ := CreateSupervisorJWT(priv, cert, "test-uid", "test-aud", 5*time.Minute)

	err := VerifySupervisorJWT(token, cert)
	require.NoError(t, err)
}

func TestVerifySupervisorJWT_WrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	otherPub, _, _ := ed25519.GenerateKey(nil)

	cert := createTestCertWithPublicKey(t, otherPub) // Different key

	token, _ := CreateSupervisorJWT(priv, cert, "test-uid", "test-aud", 5*time.Minute)

	err := VerifySupervisorJWT(token, cert)
	require.Error(t, err)
}
```

**Step 10: Implement supervisor-signed JWT**

Create `auth/jwt.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// JWTHeader represents the header of a supervisor-signed JWT.
type JWTHeader struct {
	Alg     string `json:"alg"`
	Typ     string `json:"typ"`
	X5tS256 string `json:"x5t#S256"` // Certificate SHA-256 fingerprint
}

// SupervisorClaims represents the claims in a supervisor-signed JWT.
type SupervisorClaims struct {
	Subject   string    `json:"sub"`
	Audience  string    `json:"aud"`
	IssuedAt  time.Time `json:"-"`
	ExpiresAt time.Time `json:"-"`
	Iat       int64     `json:"iat"`
	Exp       int64     `json:"exp"`
}

// CreateSupervisorJWT creates a JWT signed by the supervisor's private key.
func CreateSupervisorJWT(
	privateKey ed25519.PrivateKey,
	cert *x509.Certificate,
	instanceUID string,
	audience string,
	lifetime time.Duration,
) (string, error) {
	now := time.Now()

	// Calculate certificate fingerprint
	fingerprint := sha256.Sum256(cert.Raw)

	header := JWTHeader{
		Alg:     "EdDSA",
		Typ:     "JWT",
		X5tS256: hex.EncodeToString(fingerprint[:]),
	}

	claims := SupervisorClaims{
		Subject:  instanceUID,
		Audience: audience,
		Iat:      now.Unix(),
		Exp:      now.Add(lifetime).Unix(),
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signedContent := headerB64 + "." + claimsB64
	signature := ed25519.Sign(privateKey, []byte(signedContent))
	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)

	return signedContent + "." + signatureB64, nil
}

// ParseSupervisorJWT parses a supervisor-signed JWT without verifying.
func ParseSupervisorJWT(token string) (*JWTHeader, *SupervisorClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, nil, errors.New("invalid JWT format")
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, errors.New("failed to decode header")
	}

	var header JWTHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, nil, errors.New("failed to parse header")
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, errors.New("failed to decode claims")
	}

	var claims SupervisorClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, nil, errors.New("failed to parse claims")
	}

	claims.IssuedAt = time.Unix(claims.Iat, 0)
	claims.ExpiresAt = time.Unix(claims.Exp, 0)

	return &header, &claims, nil
}

// VerifySupervisorJWT verifies a supervisor-signed JWT against the certificate.
func VerifySupervisorJWT(token string, cert *x509.Certificate) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("invalid JWT format")
	}

	signedContent := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return errors.New("failed to decode signature")
	}

	pubKey, ok := cert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return errors.New("certificate does not contain Ed25519 key")
	}

	if !ed25519.Verify(pubKey, []byte(signedContent), signature) {
		return errors.New("invalid signature")
	}

	return nil
}

// BearerToken returns the token formatted for the Authorization header.
func BearerToken(token string) string {
	return "Bearer " + token
}
```

**Step 11: Add X25519 dependency**

Run: `go get golang.org/x/crypto/curve25519@latest && go mod tidy`
Expected: Downloads curve25519 package

**Step 12: Run all auth tests**

Run: `go test ./auth/... -v`
Expected: PASS

**Step 13: Commit**

```bash
git add auth/ go.mod go.sum
git commit -m "feat(auth): implement CSR flow with JWKS validation and supervisor-signed JWTs"
```

---

## Summary

This implementation plan covers:

1. **Phase 1: Project Foundation** - Go module setup, dependencies
2. **Phase 2: Configuration System** - Types, loading, validation
3. **Phase 3: Persistence Layer** - Instance UID, keys/certificates, connection state
4. **Phase 4: Process Management** - Commander Keen with platform-specific signals
5. **Phase 5: OpAMP Integration** - Client and server wrappers
6. **Phase 6: Core Supervisor Engine** - Main supervisor orchestration
7. **Phase 7: Configuration Merging & Auth** - YAML merging, CSR flow, supervisor-signed JWTs

Each task follows TDD principles with:
- Failing test first
- Minimal implementation
- Verify tests pass
- Commit

**Not covered in this plan (future work):**
- Package management with signature verification
- Health endpoint HTTP server
- Full agent description reporting
- Custom message relay
- Compliance override handling
- Windows service integration
- Config encryption (X25519 + AES-GCM)
- PKCS#8 encrypted key storage
- Certificate renewal automation
- Collector credential injection for OTLP
