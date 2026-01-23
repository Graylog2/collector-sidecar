# OpAMP Supervisor Phase 2 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the configuration management and health monitoring features that enable the supervisor to actually manage a collector's configuration lifecycle.

**Architecture:** Phase 2 builds on the Phase 1 foundation by adding: (1) a config manager that writes merged configs to disk, (2) remote config handling that triggers collector reload, (3) health monitoring that polls the collector and reports upstream, and (4) effective config reporting back to the server.

**Tech Stack:** Go 1.25+, koanf for config merging, goccy/go-yaml for YAML, opamp-go for protocol, net/http for health polling

---

## Phase 2 Overview

| Task | Component | Description |
|------|-----------|-------------|
| 2.1 | Config Writer | Write merged YAML configs to disk atomically |
| 2.2 | Supervisor Injections | Inject OpAMP extension config into collector config |
| 2.3 | Remote Config Handler | Handle OnRemoteConfig callback, merge, write, reload |
| 2.4 | Health Monitor | Poll collector health endpoint, report upstream |
| 2.5 | Effective Config Reporter | Report effective config to upstream server |
| 2.6 | Integration Wiring | Wire all components together in supervisor |

---

## Task 2.1: Config Writer

**Files:**
- Create: `configwriter/writer.go`
- Create: `configwriter/writer_test.go`

**Step 1: Write tests for config writer**

Create `configwriter/writer_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package configwriter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteConfig_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collector.yaml")

	content := []byte("receivers:\n  otlp:\n    protocols:\n      grpc: {}\n")

	err := WriteConfig(path, content)
	require.NoError(t, err)

	// Verify file exists and has correct content
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, data)
}

func TestWriteConfig_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collector.yaml")

	// Write initial content
	initial := []byte("initial: content\n")
	err := WriteConfig(path, initial)
	require.NoError(t, err)

	// Write new content - should be atomic
	updated := []byte("updated: content\n")
	err = WriteConfig(path, updated)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, updated, data)

	// No temp files should remain
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1) // Only the config file
}

func TestWriteConfig_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "nested", "collector.yaml")

	content := []byte("test: value\n")
	err := WriteConfig(path, content)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, data)
}

func TestWriteConfig_PreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collector.yaml")

	content := []byte("test: value\n")
	err := WriteConfig(path, content)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	// Should be readable by owner and group (0644)
	require.Equal(t, os.FileMode(0644), info.Mode().Perm())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./configwriter/... -v`
Expected: FAIL (package not found)

**Step 3: Implement config writer**

Create `configwriter/writer.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package configwriter

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteConfig writes configuration content to the specified path atomically.
// It creates parent directories if they don't exist.
// The write is atomic: content is written to a temp file first, then renamed.
func WriteConfig(path string, content []byte) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write to temp file first for atomic operation
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, path); err != nil {
		// Clean up temp file on failure
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename config: %w", err)
	}

	return nil
}

// ReadConfig reads configuration content from the specified path.
func ReadConfig(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	return content, nil
}

// ConfigExists checks if a configuration file exists.
func ConfigExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./configwriter/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add configwriter/
git commit -m "feat(configwriter): implement atomic config file writing"
```

---

## Task 2.2: Supervisor Injections

**Files:**
- Create: `configmerge/inject.go`
- Create: `configmerge/inject_test.go`

**Step 1: Write tests for supervisor injections**

Create `configmerge/inject_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package configmerge

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInjectOpAMPExtension(t *testing.T) {
	baseConfig := []byte(`
receivers:
  otlp:
    protocols:
      grpc: {}
exporters:
  debug: {}
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
`)

	result, err := InjectOpAMPExtension(baseConfig, "localhost:4321", "test-instance-uid")
	require.NoError(t, err)

	resultStr := string(result)
	// Should have opamp extension
	require.Contains(t, resultStr, "extensions:")
	require.Contains(t, resultStr, "opamp:")
	require.Contains(t, resultStr, "localhost:4321")
	require.Contains(t, resultStr, "test-instance-uid")
}

func TestInjectOpAMPExtension_ExistingExtensions(t *testing.T) {
	baseConfig := []byte(`
extensions:
  health_check: {}
receivers:
  otlp: {}
service:
  extensions: [health_check]
  pipelines:
    traces:
      receivers: [otlp]
`)

	result, err := InjectOpAMPExtension(baseConfig, "localhost:4321", "uid-123")
	require.NoError(t, err)

	resultStr := string(result)
	// Should preserve existing extension
	require.Contains(t, resultStr, "health_check")
	// Should add opamp extension
	require.Contains(t, resultStr, "opamp:")
}

func TestInjectOpAMPExtension_EmptyConfig(t *testing.T) {
	baseConfig := []byte(``)

	result, err := InjectOpAMPExtension(baseConfig, "localhost:4321", "uid-123")
	require.NoError(t, err)

	resultStr := string(result)
	require.Contains(t, resultStr, "opamp:")
	require.Contains(t, resultStr, "localhost:4321")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./configmerge/... -v -run TestInjectOpAMP`
Expected: FAIL (undefined: InjectOpAMPExtension)

**Step 3: Implement supervisor injections**

Create `configmerge/inject.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package configmerge

// InjectOpAMPExtension injects the OpAMP extension configuration into a collector config.
// This enables the collector to communicate with the supervisor's local OpAMP server.
func InjectOpAMPExtension(config []byte, endpoint string, instanceUID string) ([]byte, error) {
	// Build the OpAMP extension config
	opampExtension := map[string]interface{}{
		"extensions": map[string]interface{}{
			"opamp": map[string]interface{}{
				"server": map[string]interface{}{
					"ws": map[string]interface{}{
						"endpoint": endpoint,
					},
				},
				"instance_uid": instanceUID,
			},
		},
	}

	// Merge with existing config (opamp extension added/updated)
	injected, err := InjectSettings(config, opampExtension)
	if err != nil {
		return nil, err
	}

	return injected, nil
}

// InjectServiceExtension ensures the opamp extension is listed in service.extensions.
// This should be called after InjectOpAMPExtension.
func InjectServiceExtension(config []byte) ([]byte, error) {
	// This is a simplified implementation that adds opamp to service.extensions
	// A full implementation would parse and modify the YAML properly
	settings := map[string]interface{}{
		"service": map[string]interface{}{
			"extensions": []string{"opamp"},
		},
	}

	return InjectSettings(config, settings)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./configmerge/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add configmerge/inject.go configmerge/inject_test.go
git commit -m "feat(configmerge): add OpAMP extension injection for collector config"
```

---

## Task 2.3: Remote Config Handler

**Files:**
- Create: `configmanager/manager.go`
- Create: `configmanager/manager_test.go`

**Step 1: Write tests for config manager**

Create `configmanager/manager_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package configmanager

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestConfigManager_ApplyRemoteConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()

	mgr := New(logger, Config{
		ConfigDir:      dir,
		OutputPath:     filepath.Join(dir, "collector.yaml"),
		LocalOverrides: nil,
		LocalEndpoint:  "localhost:4321",
		InstanceUID:    "test-uid",
	})

	remoteConfig := &protobufs.AgentRemoteConfig{
		Config: &protobufs.AgentConfigMap{
			ConfigMap: map[string]*protobufs.AgentConfigFile{
				"collector.yaml": {
					Body: []byte("receivers:\n  otlp: {}\n"),
				},
			},
		},
		ConfigHash: []byte("hash123"),
	}

	result, err := mgr.ApplyRemoteConfig(context.Background(), remoteConfig)
	require.NoError(t, err)
	require.True(t, result.Changed)
	require.NotEmpty(t, result.EffectiveConfig)
}

func TestConfigManager_ApplyRemoteConfig_WithLocalOverrides(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()

	// Create a local override file
	overridePath := filepath.Join(dir, "override.yaml")
	overrideContent := []byte("exporters:\n  debug: {}\n")
	require.NoError(t, WriteTestFile(overridePath, overrideContent))

	mgr := New(logger, Config{
		ConfigDir:      dir,
		OutputPath:     filepath.Join(dir, "collector.yaml"),
		LocalOverrides: []string{overridePath},
		LocalEndpoint:  "localhost:4321",
		InstanceUID:    "test-uid",
	})

	remoteConfig := &protobufs.AgentRemoteConfig{
		Config: &protobufs.AgentConfigMap{
			ConfigMap: map[string]*protobufs.AgentConfigFile{
				"collector.yaml": {
					Body: []byte("receivers:\n  otlp: {}\n"),
				},
			},
		},
		ConfigHash: []byte("hash123"),
	}

	result, err := mgr.ApplyRemoteConfig(context.Background(), remoteConfig)
	require.NoError(t, err)

	// Should have both receivers and exporters
	require.Contains(t, string(result.EffectiveConfig), "otlp")
	require.Contains(t, string(result.EffectiveConfig), "debug")
}

func TestConfigManager_GetEffectiveConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "collector.yaml")

	// Write a config file
	content := []byte("test: config\n")
	require.NoError(t, WriteTestFile(outputPath, content))

	mgr := New(logger, Config{
		ConfigDir:      dir,
		OutputPath:     outputPath,
		LocalOverrides: nil,
		LocalEndpoint:  "localhost:4321",
		InstanceUID:    "test-uid",
	})

	effective, err := mgr.GetEffectiveConfig()
	require.NoError(t, err)
	require.Equal(t, content, effective)
}

// WriteTestFile is a helper for tests
func WriteTestFile(path string, content []byte) error {
	return writeFile(path, content)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./configmanager/... -v`
Expected: FAIL (package not found)

**Step 3: Implement config manager**

Create `configmanager/manager.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package configmanager

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"

	"github.com/Graylog2/collector-sidecar/superv/configmerge"
	"github.com/Graylog2/collector-sidecar/superv/configwriter"
)

// Config holds configuration for the config manager.
type Config struct {
	ConfigDir      string   // Directory to store remote configs
	OutputPath     string   // Path to write final merged config
	LocalOverrides []string // Paths to local override files
	LocalEndpoint  string   // Local OpAMP server endpoint for injection
	InstanceUID    string   // Instance UID for injection
}

// ApplyResult contains the result of applying a remote config.
type ApplyResult struct {
	Changed         bool   // Whether the config changed
	EffectiveConfig []byte // The final merged config
	ConfigHash      []byte // Hash of the applied config
}

// Manager handles configuration merging and writing.
type Manager struct {
	logger     *zap.Logger
	cfg        Config
	lastHash   []byte
}

// New creates a new config manager.
func New(logger *zap.Logger, cfg Config) *Manager {
	return &Manager{
		logger: logger,
		cfg:    cfg,
	}
}

// ApplyRemoteConfig applies a remote configuration from the OpAMP server.
// It merges remote config with local overrides, injects supervisor settings,
// and writes the result to disk.
func (m *Manager) ApplyRemoteConfig(ctx context.Context, remote *protobufs.AgentRemoteConfig) (*ApplyResult, error) {
	if remote == nil || remote.Config == nil {
		return &ApplyResult{Changed: false}, nil
	}

	// Check if config hash changed
	if bytes.Equal(m.lastHash, remote.ConfigHash) {
		return &ApplyResult{Changed: false, ConfigHash: remote.ConfigHash}, nil
	}

	// Get the main collector config from the map
	var baseConfig []byte
	for name, file := range remote.Config.ConfigMap {
		if name == "collector.yaml" || name == "" {
			baseConfig = file.Body
			break
		}
	}
	// If no specific file found, use the first one
	if baseConfig == nil {
		for _, file := range remote.Config.ConfigMap {
			baseConfig = file.Body
			break
		}
	}

	if baseConfig == nil {
		baseConfig = []byte{}
	}

	// Store raw remote configs
	if err := m.storeRemoteConfigs(remote.Config.ConfigMap); err != nil {
		m.logger.Warn("Failed to store remote configs", zap.Error(err))
	}

	// Merge with local overrides
	merged := baseConfig
	for _, overridePath := range m.cfg.LocalOverrides {
		override, err := os.ReadFile(overridePath)
		if err != nil {
			m.logger.Warn("Failed to read local override",
				zap.String("path", overridePath),
				zap.Error(err))
			continue
		}
		merged, err = configmerge.MergeConfigs(merged, override)
		if err != nil {
			return nil, fmt.Errorf("failed to merge override %s: %w", overridePath, err)
		}
	}

	// Inject OpAMP extension
	merged, err := configmerge.InjectOpAMPExtension(merged, m.cfg.LocalEndpoint, m.cfg.InstanceUID)
	if err != nil {
		return nil, fmt.Errorf("failed to inject OpAMP extension: %w", err)
	}

	// Write to disk atomically
	if err := configwriter.WriteConfig(m.cfg.OutputPath, merged); err != nil {
		return nil, fmt.Errorf("failed to write config: %w", err)
	}

	m.lastHash = remote.ConfigHash

	m.logger.Info("Applied remote configuration",
		zap.String("output", m.cfg.OutputPath),
		zap.Int("size", len(merged)))

	return &ApplyResult{
		Changed:         true,
		EffectiveConfig: merged,
		ConfigHash:      remote.ConfigHash,
	}, nil
}

// GetEffectiveConfig returns the current effective configuration.
func (m *Manager) GetEffectiveConfig() ([]byte, error) {
	return configwriter.ReadConfig(m.cfg.OutputPath)
}

// GetConfigHash returns the hash of the last applied config.
func (m *Manager) GetConfigHash() []byte {
	return m.lastHash
}

// storeRemoteConfigs stores raw remote configs for debugging/recovery.
func (m *Manager) storeRemoteConfigs(configMap map[string]*protobufs.AgentConfigFile) error {
	remoteDir := filepath.Join(m.cfg.ConfigDir, "remote")
	if err := os.MkdirAll(remoteDir, 0755); err != nil {
		return err
	}

	for name, file := range configMap {
		path := filepath.Join(remoteDir, name)
		if err := configwriter.WriteConfig(path, file.Body); err != nil {
			return err
		}
	}
	return nil
}

// writeFile is a helper for writing files (used by tests too).
func writeFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./configmanager/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add configmanager/
git commit -m "feat(configmanager): implement remote config handling with merge and write"
```

---

## Task 2.4: Health Monitor

**Files:**
- Create: `healthmonitor/monitor.go`
- Create: `healthmonitor/monitor_test.go`

**Step 1: Write tests for health monitor**

Create `healthmonitor/monitor_test.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package healthmonitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestHealthMonitor_CheckHealth_Healthy(t *testing.T) {
	// Create a mock healthy endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	monitor := New(logger, Config{
		Endpoint: server.URL,
		Timeout:  5 * time.Second,
	})

	status, err := monitor.CheckHealth(context.Background())
	require.NoError(t, err)
	require.True(t, status.Healthy)
}

func TestHealthMonitor_CheckHealth_Unhealthy(t *testing.T) {
	// Create a mock unhealthy endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"unhealthy"}`))
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	monitor := New(logger, Config{
		Endpoint: server.URL,
		Timeout:  5 * time.Second,
	})

	status, err := monitor.CheckHealth(context.Background())
	require.NoError(t, err)
	require.False(t, status.Healthy)
}

func TestHealthMonitor_CheckHealth_Unreachable(t *testing.T) {
	logger := zaptest.NewLogger(t)
	monitor := New(logger, Config{
		Endpoint: "http://localhost:99999/health", // Unreachable
		Timeout:  100 * time.Millisecond,
	})

	status, err := monitor.CheckHealth(context.Background())
	require.Error(t, err)
	require.False(t, status.Healthy)
}

func TestHealthMonitor_ToComponentHealth(t *testing.T) {
	status := &HealthStatus{
		Healthy:   true,
		Timestamp: time.Now(),
	}

	componentHealth := status.ToComponentHealth()
	require.NotNil(t, componentHealth)
	require.True(t, componentHealth.Healthy)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./healthmonitor/... -v`
Expected: FAIL (package not found)

**Step 3: Implement health monitor**

Create `healthmonitor/monitor.go`:
```go
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package healthmonitor

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

// Config holds configuration for the health monitor.
type Config struct {
	Endpoint string
	Timeout  time.Duration
	Interval time.Duration
}

// HealthStatus represents the health status of the collector.
type HealthStatus struct {
	Healthy      bool
	StatusCode   int
	ErrorMessage string
	Timestamp    time.Time
}

// ToComponentHealth converts HealthStatus to OpAMP ComponentHealth.
func (s *HealthStatus) ToComponentHealth() *protobufs.ComponentHealth {
	health := &protobufs.ComponentHealth{
		Healthy:            s.Healthy,
		StartTimeUnixNano:  uint64(s.Timestamp.UnixNano()),
		LastError:          s.ErrorMessage,
	}
	return health
}

// Monitor polls a health endpoint and reports status.
type Monitor struct {
	logger *zap.Logger
	cfg    Config
	client *http.Client
	last   *HealthStatus
}

// New creates a new health monitor.
func New(logger *zap.Logger, cfg Config) *Monitor {
	return &Monitor{
		logger: logger,
		cfg:    cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// CheckHealth performs a single health check.
func (m *Monitor) CheckHealth(ctx context.Context) (*HealthStatus, error) {
	status := &HealthStatus{
		Timestamp: time.Now(),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.cfg.Endpoint, nil)
	if err != nil {
		status.Healthy = false
		status.ErrorMessage = err.Error()
		m.last = status
		return status, err
	}

	resp, err := m.client.Do(req)
	if err != nil {
		status.Healthy = false
		status.ErrorMessage = fmt.Sprintf("health check failed: %v", err)
		m.last = status
		return status, err
	}
	defer resp.Body.Close()

	status.StatusCode = resp.StatusCode
	status.Healthy = resp.StatusCode >= 200 && resp.StatusCode < 300

	if !status.Healthy {
		status.ErrorMessage = fmt.Sprintf("unhealthy status code: %d", resp.StatusCode)
	}

	m.last = status
	return status, nil
}

// LastStatus returns the last health status.
func (m *Monitor) LastStatus() *HealthStatus {
	return m.last
}

// StartPolling starts periodic health checks in the background.
// Returns a channel that receives health status updates.
func (m *Monitor) StartPolling(ctx context.Context) <-chan *HealthStatus {
	updates := make(chan *HealthStatus, 1)

	go func() {
		defer close(updates)

		ticker := time.NewTicker(m.cfg.Interval)
		defer ticker.Stop()

		// Initial check
		if status, err := m.CheckHealth(ctx); err == nil {
			select {
			case updates <- status:
			default:
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status, _ := m.CheckHealth(ctx)
				select {
				case updates <- status:
				default:
					// Drop if channel full
				}
			}
		}
	}()

	return updates
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./healthmonitor/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add healthmonitor/
git commit -m "feat(healthmonitor): implement collector health endpoint polling"
```

---

## Task 2.5: Effective Config Reporter

**Files:**
- Modify: `opamp/client.go`
- Modify: `opamp/client_test.go`

**Step 1: Write tests for effective config reporting**

Add to `opamp/client_test.go`:
```go
func TestClient_SetEffectiveConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	callbacks := &Callbacks{}

	client, err := NewClient(logger, ClientConfig{
		Endpoint:    "ws://localhost:4320/v1/opamp",
		InstanceUID: "test-uid",
	}, callbacks)
	require.NoError(t, err)

	// Before start, should store for later
	config := map[string]*protobufs.AgentConfigFile{
		"collector.yaml": {
			Body: []byte("test: config"),
		},
	}
	err = client.SetEffectiveConfig(config)
	require.NoError(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./opamp/... -v -run TestClient_SetEffectiveConfig`
Expected: FAIL (undefined: SetEffectiveConfig)

**Step 3: Add SetEffectiveConfig method**

Add to `opamp/client.go`:
```go
// SetEffectiveConfig updates the effective configuration reported to the server.
// Can be called before Start() to set the initial effective config.
func (c *Client) SetEffectiveConfig(config map[string]*protobufs.AgentConfigFile) error {
	effectiveConfig := &protobufs.EffectiveConfig{
		ConfigMap: &protobufs.AgentConfigMap{
			ConfigMap: config,
		},
	}

	if c.opampClient == nil {
		// Store for use when Start() is called
		c.initialEffectiveConfig = effectiveConfig
		return nil
	}

	return c.opampClient.SetEffectiveConfig(effectiveConfig)
}
```

Also add field to Client struct:
```go
type Client struct {
	// ... existing fields ...
	initialEffectiveConfig *protobufs.EffectiveConfig
}
```

And apply in Start():
```go
// Apply initial effective config before starting
if c.initialEffectiveConfig != nil {
	if err := opampClient.SetEffectiveConfig(c.initialEffectiveConfig); err != nil {
		return err
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./opamp/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add opamp/client.go opamp/client_test.go
git commit -m "feat(opamp): add SetEffectiveConfig for reporting config to server"
```

---

## Task 2.6: Integration Wiring

**Files:**
- Modify: `supervisor/supervisor.go`
- Modify: `supervisor/supervisor_test.go`

**Step 1: Write integration test**

Add to `supervisor/supervisor_test.go`:
```go
func TestSupervisor_ConfigManagerIntegration(t *testing.T) {
	// This is a basic integration test verifying the components wire together
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
			Config: config.AgentConfigMerge{
				MergeStrategy: "deep",
			},
		},
		Persistence: config.PersistenceConfig{
			Dir: dir,
		},
	}

	supervisor, err := New(logger, cfg)
	require.NoError(t, err)
	require.NotNil(t, supervisor)

	// Verify config manager is created
	require.NotNil(t, supervisor.configManager)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./supervisor/... -v -run TestSupervisor_ConfigManager`
Expected: FAIL (supervisor.configManager undefined)

**Step 3: Wire components in supervisor**

Modify `supervisor/supervisor.go` to add:

1. Add imports:
```go
import (
	// ... existing imports ...
	"github.com/Graylog2/collector-sidecar/superv/configmanager"
	"github.com/Graylog2/collector-sidecar/superv/healthmonitor"
)
```

2. Add fields to Supervisor struct:
```go
type Supervisor struct {
	// ... existing fields ...
	configManager  *configmanager.Manager
	healthMonitor  *healthmonitor.Monitor
}
```

3. Initialize in New():
```go
func New(logger *zap.Logger, cfg config.Config) (*Supervisor, error) {
	// ... existing code ...

	// Initialize config manager
	configMgr := configmanager.New(logger, configmanager.Config{
		ConfigDir:      filepath.Join(cfg.Persistence.Dir, "config"),
		OutputPath:     filepath.Join(cfg.Persistence.Dir, "config", "collector.yaml"),
		LocalOverrides: cfg.Agent.Config.LocalOverrides,
		LocalEndpoint:  cfg.LocalOpAMP.Endpoint,
		InstanceUID:    uid,
	})

	// Initialize health monitor
	healthMon := healthmonitor.New(logger, healthmonitor.Config{
		Endpoint: cfg.Agent.Health.Endpoint,
		Timeout:  cfg.Agent.Health.Timeout,
		Interval: cfg.Agent.Health.Interval,
	})

	return &Supervisor{
		logger:         logger,
		cfg:            cfg,
		instanceUID:    uid,
		configManager:  configMgr,
		healthMonitor:  healthMon,
	}, nil
}
```

4. Update OnRemoteConfig callback in Start():
```go
OnRemoteConfig: func(ctx context.Context, cfg *protobufs.AgentRemoteConfig) bool {
	s.logger.Info("Received remote configuration")

	result, err := s.configManager.ApplyRemoteConfig(ctx, cfg)
	if err != nil {
		s.logger.Error("Failed to apply remote config", zap.Error(err))
		return false
	}

	if result.Changed {
		// Reload collector
		if err := s.commander.ReloadConfig(); err != nil {
			s.logger.Error("Failed to reload collector", zap.Error(err))
			// Try restart as fallback
			if s.cfg.Agent.Reload.RestartOnReloadFailure {
				if err := s.commander.Restart(ctx); err != nil {
					s.logger.Error("Failed to restart collector", zap.Error(err))
				}
			}
		}

		// Report effective config
		s.opampClient.SetEffectiveConfig(map[string]*protobufs.AgentConfigFile{
			"collector.yaml": {
				Body: result.EffectiveConfig,
			},
		})
	}

	return true
},
```

5. Start health monitoring in Start():
```go
// Start health monitoring
healthUpdates := s.healthMonitor.StartPolling(ctx)
go func() {
	for status := range healthUpdates {
		if err := s.opampClient.SetHealth(status.ToComponentHealth()); err != nil {
			s.logger.Warn("Failed to report health", zap.Error(err))
		}
	}
}()
```

**Step 4: Run tests to verify they pass**

Run: `go test ./supervisor/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add supervisor/
git commit -m "feat(supervisor): wire config manager and health monitor"
```

---

## Summary

Phase 2 implements the core configuration lifecycle:

1. **Config Writer** - Atomic file writes for safety
2. **Supervisor Injections** - Auto-inject OpAMP extension config
3. **Config Manager** - Merge remote + local + injections, write to disk
4. **Health Monitor** - Poll collector health, report upstream
5. **Effective Config** - Report merged config back to server
6. **Integration** - Wire all components in supervisor

After Phase 2, the supervisor can:
- Receive config from OpAMP server
- Merge with local compliance overrides
- Write atomically to disk
- Reload the collector (SIGHUP)
- Report health status upstream
- Report effective config upstream

**Not covered (Phase 3):**
- Package management
- Custom message relay
- Connection settings/token refresh
- Full offline operation
