# Bootstrap Config Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ensure a valid collector config exists before the collector starts. On first
run, write a bootstrap config with opamp + health_check extensions and a minimal nop
pipeline. On subsequent runs, re-inject extensions and inject a nop pipeline if none
exists. Also fix the pre-existing issue where `ApplyRemoteConfig` used the static
`localhost:0` endpoint instead of the runtime-resolved one.

**Architecture:** A `resolveLocalEndpoint` helper in the supervisor package normalizes
the bound address into a dialable `ws://` URL. New `SetLocalEndpoint` and
`EnsureBootstrapConfig` methods on `configmanager.Manager`. Called in
`supervisor.Start()` after the local OpAMP server starts but before the OpAMP client
starts (no race with remote config callbacks).

**Tech Stack:** Go, testify, configmerge (koanf/yaml), persistence.WriteFile

---

### Task 1: resolveLocalEndpoint — tests and implementation

**Files:**
- Create: `superv/supervisor/endpoint.go`
- Create: `superv/supervisor/endpoint_test.go`

**Step 1: Write the test file**

Create `superv/supervisor/endpoint_test.go`:

```go
package supervisor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveLocalEndpoint(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{
			name: "explicit loopback with port",
			addr: "127.0.0.1:54321",
			want: "ws://127.0.0.1:54321/v1/opamp",
		},
		{
			name: "localhost with port",
			addr: "localhost:54321",
			want: "ws://localhost:54321/v1/opamp",
		},
		{
			name: "wildcard IPv4",
			addr: "0.0.0.0:54321",
			want: "ws://127.0.0.1:54321/v1/opamp",
		},
		{
			name: "wildcard IPv6 bracketed",
			addr: "[::]:54321",
			want: "ws://[::1]:54321/v1/opamp",
		},
		{
			name: "explicit IPv6 loopback",
			addr: "[::1]:54321",
			want: "ws://[::1]:54321/v1/opamp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveLocalEndpoint(tt.addr)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveLocalEndpoint_Error(t *testing.T) {
	_, err := resolveLocalEndpoint(":::54321")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot parse")
}
```

**Step 2: Write the implementation**

Create `superv/supervisor/endpoint.go`:

```go
package supervisor

import (
	"fmt"
	"net"
	"net/netip"

	"github.com/Graylog2/collector-sidecar/superv/config"
)

// resolveLocalEndpoint converts a net.Addr.String() result (e.g. "0.0.0.0:54321")
// into a dialable WebSocket URL for the collector's OpAMP extension.
//
// Unspecified addresses are replaced with family-aware loopback:
// 0.0.0.0 → 127.0.0.1, [::] → [::1].
//
// Returns an error if the address cannot be parsed.
func resolveLocalEndpoint(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("cannot parse local server address %q: %w", addr, err)
	}

	// Replace unspecified addresses with family-aware loopback.
	if ip, err := netip.ParseAddr(host); err == nil && ip.IsUnspecified() {
		if ip.Is4() {
			host = "127.0.0.1"
		} else {
			host = "::1"
		}
	}

	return fmt.Sprintf("ws://%s%s", net.JoinHostPort(host, port), config.DefaultOpAMPPath), nil
}
```

**Step 3: Run tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./supervisor/ -run TestResolveLocalEndpoint -v`
Expected: All 6 tests PASS (5 success cases + 1 error case)

---

### Task 2: SetLocalEndpoint and EnsureBootstrapConfig — tests

**Files:**
- Modify: `superv/configmanager/manager_test.go`

**Step 1: Add tests at the end of `manager_test.go`**

```go
func TestSetLocalEndpoint(t *testing.T) {
	mgr := New(zaptest.NewLogger(t), Config{
		LocalEndpoint: "localhost:0",
	})

	mgr.SetLocalEndpoint("ws://127.0.0.1:54321/v1/opamp")
	assert.Equal(t, "ws://127.0.0.1:54321/v1/opamp", mgr.cfg.LocalEndpoint)
}

func TestEnsureBootstrapConfig_WritesWhenMissing(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config", "collector.yaml")

	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:     filepath.Join(dir, "config"),
		OutputPath:    outputPath,
		LocalEndpoint: "ws://127.0.0.1:54321/v1/opamp",
		InstanceUID:   "test-uid-bootstrap",
		HealthCheck: configmerge.HealthCheckConfig{
			Endpoint: "localhost:13133",
			Path:     "/health",
		},
	})

	err := mgr.EnsureBootstrapConfig()
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "opamp")
	assert.Contains(t, s, "ws://127.0.0.1:54321/v1/opamp")
	assert.Contains(t, s, "test-uid-bootstrap")
	assert.Contains(t, s, "health_check")
	assert.Contains(t, s, "localhost:13133")

	// Bootstrap includes a nop pipeline so the collector accepts the config
	assert.Contains(t, s, "nop")
	assert.Contains(t, s, "logs/bootstrap")
}

func TestEnsureBootstrapConfig_ReinjectsExtensionsWhenExists(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	outputPath := filepath.Join(configDir, "collector.yaml")

	require.NoError(t, os.MkdirAll(configDir, 0o755))
	// Cached config with an old OpAMP endpoint and a real pipeline (simulates port change on restart)
	existing := []byte("extensions:\n  opamp:\n    server:\n      ws:\n        endpoint: ws://127.0.0.1:11111/v1/opamp\n    instance_uid: test-uid\nreceivers:\n  otlp:\n    protocols:\n      grpc: {}\nexporters:\n  debug: {}\nservice:\n  extensions:\n    - opamp\n  pipelines:\n    logs:\n      receivers:\n        - otlp\n      exporters:\n        - debug\n")
	require.NoError(t, os.WriteFile(outputPath, existing, 0o600))

	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:     configDir,
		OutputPath:    outputPath,
		LocalEndpoint: "ws://127.0.0.1:22222/v1/opamp",
		InstanceUID:   "test-uid",
		HealthCheck: configmerge.HealthCheckConfig{
			Endpoint: "localhost:13133",
		},
	})

	err := mgr.EnsureBootstrapConfig()
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	s := string(content)
	// Should have the NEW endpoint, not the old one
	assert.Contains(t, s, "ws://127.0.0.1:22222/v1/opamp")
	assert.NotContains(t, s, "ws://127.0.0.1:11111/v1/opamp")
	// Should also have health_check injected
	assert.Contains(t, s, "health_check")
	assert.Contains(t, s, "localhost:13133")
	// Cached config with pipelines should NOT get a nop bootstrap pipeline
	assert.NotContains(t, s, "logs/bootstrap")
}

func TestEnsureBootstrapConfig_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	// Nested dir that doesn't exist yet
	outputPath := filepath.Join(dir, "deep", "nested", "collector.yaml")

	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:     filepath.Join(dir, "deep", "nested"),
		OutputPath:    outputPath,
		LocalEndpoint: "ws://127.0.0.1:9999/v1/opamp",
		InstanceUID:   "test-uid-dir",
		HealthCheck: configmerge.HealthCheckConfig{
			Endpoint: "localhost:13133",
		},
	})

	err := mgr.EnsureBootstrapConfig()
	require.NoError(t, err)

	_, err = os.Stat(outputPath)
	require.NoError(t, err)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./configmanager/ -run 'TestEnsureBootstrapConfig|TestSetLocalEndpoint' -v`
Expected: Compilation failure — methods undefined

---

### Task 3: SetLocalEndpoint and EnsureBootstrapConfig — implementation

**Files:**
- Modify: `superv/configmanager/manager.go`

**Step 1: Add SetLocalEndpoint after the `OutputPath()` method**

```go
// SetLocalEndpoint updates the local OpAMP endpoint used for extension injection.
// Call this after the local OpAMP server starts, to replace the static config
// value (which may be "localhost:0") with the actual bound address.
func (m *Manager) SetLocalEndpoint(endpoint string) {
	m.cfg.LocalEndpoint = endpoint
}
```

**Step 2: Add EnsureBootstrapConfig after SetLocalEndpoint**

```go
// EnsureBootstrapConfig ensures a valid collector config exists at OutputPath
// before the collector starts.
//
// If no config file exists (first run), it writes a minimal bootstrap config
// containing only the opamp and health_check extensions.
//
// If a config file already exists (cached from a previous run), it re-injects
// the opamp and health_check extensions to update the local OpAMP endpoint,
// which may have changed if the local server binds to an ephemeral port.
func (m *Manager) EnsureBootstrapConfig() error {
	// Ensure the config directory exists.
	dir := filepath.Dir(m.cfg.OutputPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}

	// Load existing config or start from empty.
	config, err := os.ReadFile(m.cfg.OutputPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to read existing config %s: %w", m.cfg.OutputPath, err)
	}

	bootstrap := len(config) == 0
	if bootstrap {
		m.logger.Info("no existing config found, writing bootstrap config",
			zap.String("path", m.cfg.OutputPath))
	} else {
		m.logger.Info("re-injecting extensions into cached config",
			zap.String("path", m.cfg.OutputPath))
	}

	// Inject OpAMP extension (updates endpoint on every start).
	if m.cfg.LocalEndpoint != "" && m.cfg.InstanceUID != "" {
		config, err = configmerge.InjectOpAMPExtension(config, m.cfg.LocalEndpoint, m.cfg.InstanceUID)
		if err != nil {
			return fmt.Errorf("failed to inject OpAMP extension: %w", err)
		}
	}

	// Inject health_check extension.
	if m.cfg.HealthCheck.Endpoint != "" {
		config, err = configmerge.InjectHealthCheckExtension(config, m.cfg.HealthCheck)
		if err != nil {
			return fmt.Errorf("failed to inject health_check extension: %w", err)
		}
	}

	// The collector requires at least one pipeline. If the config has none
	// (fresh bootstrap or a cached config without pipelines), inject a
	// minimal nop pipeline so the collector can start.
	if !configmerge.HasPipelines(config) {
		config, err = configmerge.InjectSettings(config, map[string]any{
			"receivers::nop":  nil,
			"exporters::nop":  nil,
			"service::pipelines::logs/bootstrap": map[string]any{
				"receivers": []string{"nop"},
				"exporters": []string{"nop"},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to inject bootstrap pipeline: %w", err)
		}
	}

	if err := persistence.WriteFile(m.cfg.OutputPath, config, 0o600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	if bootstrap {
		m.logger.Info("wrote bootstrap config", zap.String("path", m.cfg.OutputPath))
	} else {
		m.logger.Info("updated cached config with current extensions", zap.String("path", m.cfg.OutputPath))
	}
	return nil
}
```

**Step 3: Run tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./configmanager/ -v`
Expected: All tests PASS (including existing tests)

---

### Task 4: Wire into supervisor.Start()

**Files:**
- Modify: `superv/supervisor/supervisor.go`

**Step 1: Insert after `opampServer.Start()`, before worker initialization**

After line 309 (`s.opampServer.Start(ctx)`) and before the worker queue setup, add:

```go
	// Resolve the runtime-bound local OpAMP endpoint and update the config
	// manager. This replaces the static config value (e.g. "localhost:0") with
	// the actual ws:// URL, used by both EnsureBootstrapConfig and
	// ApplyRemoteConfig for extension injection.
	localEndpoint, err := resolveLocalEndpoint(s.opampServer.Addr())
	if err != nil {
		if stopErr := s.opampServer.Stop(ctx); stopErr != nil {
			s.logger.Warn("Failed to stop local OpAMP server", zap.Error(stopErr))
		}
		return fmt.Errorf("failed to resolve local OpAMP endpoint: %w", err)
	}
	s.configManager.SetLocalEndpoint(localEndpoint)

	// Ensure a valid config exists before starting the collector. On first run
	// this writes a minimal bootstrap config; on subsequent runs it re-injects
	// the opamp and health_check extensions to update the local endpoint
	// (which may have changed if the server binds to an ephemeral port).
	if err := s.configManager.EnsureBootstrapConfig(); err != nil {
		if stopErr := s.opampServer.Stop(ctx); stopErr != nil {
			s.logger.Warn("Failed to stop local OpAMP server", zap.Error(stopErr))
		}
		return fmt.Errorf("failed to ensure bootstrap config: %w", err)
	}
```

Both error paths stop only the local OpAMP server — at this point no other components
have been started.

**Step 2: Run full test suite**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./... -count=1`
Expected: All tests PASS

**Step 3: Run go vet and go fix**

Run: `cd /home/bernd/graylog/sidecar/superv && go vet ./... && go fix ./...`
Expected: No issues
