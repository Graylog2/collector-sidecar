# Collector Reload & Remote Config Status Reporting — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** After receiving a remote config, restart the collector and report APPLIED/FAILED status back to the OpAMP server, with config rollback on failure and status persistence across restarts.

**Architecture:** The config manager gains backup/rollback and status persistence responsibilities. The supervisor's `OnRemoteConfig` callback orchestrates the full flow: apply config, restart collector, report status. The OpAMP client wrapper gains an initial remote config status field for startup restore.

**Tech Stack:** Go, opamp-go v0.23.0, koanf (YAML persistence via `persistence.WriteYAMLFile`/`LoadYAMLFile`), testify

---

### Task 1: Config backup before write

Add `.bak` file creation to `ApplyRemoteConfig` so we can roll back if the collector restart fails.

**Files:**
- Modify: `configmanager/manager.go:144-148` (before `WriteFile` call)
- Test: `configmanager/manager_test.go` (new file)

**Step 1: Write the failing test**

```go
package configmanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func makeRemoteConfig(t *testing.T, body []byte, hash []byte) *protobufs.AgentRemoteConfig {
	t.Helper()
	return &protobufs.AgentRemoteConfig{
		Config: &protobufs.AgentConfigMap{
			ConfigMap: map[string]*protobufs.AgentConfigFile{
				"collector.yaml": {Body: body},
			},
		},
		ConfigHash: hash,
	}
}

func TestApplyRemoteConfig_CreatesBackupFile(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config", "collector.yaml")

	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  filepath.Join(dir, "config"),
		OutputPath: outputPath,
	})

	// First apply — no backup expected (nothing to back up)
	cfg1 := makeRemoteConfig(t, []byte("receivers:\n  otlp:\n"), []byte("hash1"))
	result1, err := mgr.ApplyRemoteConfig(context.Background(), cfg1)
	require.NoError(t, err)
	require.True(t, result1.Changed)

	_, err = os.Stat(outputPath + ".bak")
	require.ErrorIs(t, err, os.ErrNotExist, "no backup on first apply")

	// Second apply — backup of first config expected
	cfg2 := makeRemoteConfig(t, []byte("receivers:\n  otlp:\n  filelog:\n"), []byte("hash2"))
	result2, err := mgr.ApplyRemoteConfig(context.Background(), cfg2)
	require.NoError(t, err)
	require.True(t, result2.Changed)

	bakContent, err := os.ReadFile(outputPath + ".bak")
	require.NoError(t, err)
	require.Contains(t, string(bakContent), "otlp")
	require.NotContains(t, string(bakContent), "filelog")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./configmanager/ -run TestApplyRemoteConfig_CreatesBackupFile -v`
Expected: FAIL — no `.bak` file created

**Step 3: Write minimal implementation**

In `configmanager/manager.go`, add backup logic before the `WriteFile` call (around line 145):

```go
	// Back up current config for rollback (skip if no existing config)
	if existing, err := os.ReadFile(m.cfg.OutputPath); err == nil {
		if err := persistence.WriteFile(m.cfg.OutputPath+".bak", existing, 0o600); err != nil {
			return nil, fmt.Errorf("failed to back up current config: %w", err)
		}
		m.logger.Debug("backed up current config", zap.String("path", m.cfg.OutputPath+".bak"))
	}
```

Also add a `previousHash` field to the `Manager` struct and save the old hash before updating:

```go
type Manager struct {
	logger       *zap.Logger
	cfg          Config
	lastHash     []byte
	previousHash []byte
}
```

Before `m.lastHash = remote.GetConfigHash()` at line 152, add:

```go
	m.previousHash = m.lastHash
```

**Step 4: Run test to verify it passes**

Run: `go test ./configmanager/ -run TestApplyRemoteConfig_CreatesBackupFile -v`
Expected: PASS

**Step 5: Commit**

```
feat(configmanager): back up config before writing new version
```

---

### Task 2: RollbackConfig method

Add `RollbackConfig()` that restores the `.bak` file and resets the config hash.

**Files:**
- Modify: `configmanager/manager.go` (add method)
- Test: `configmanager/manager_test.go`

**Step 1: Write the failing tests**

```go
func TestRollbackConfig_RestoresPreviousConfig(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config", "collector.yaml")

	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  filepath.Join(dir, "config"),
		OutputPath: outputPath,
	})

	// Apply two configs so we have a .bak
	cfg1 := makeRemoteConfig(t, []byte("version: 1\n"), []byte("hash1"))
	_, err := mgr.ApplyRemoteConfig(context.Background(), cfg1)
	require.NoError(t, err)

	cfg2 := makeRemoteConfig(t, []byte("version: 2\n"), []byte("hash2"))
	_, err = mgr.ApplyRemoteConfig(context.Background(), cfg2)
	require.NoError(t, err)

	// Rollback
	err = mgr.RollbackConfig()
	require.NoError(t, err)

	// Output should contain version 1 config
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	require.Contains(t, string(content), "version: 1")

	// .bak should be removed
	_, err = os.Stat(outputPath + ".bak")
	require.ErrorIs(t, err, os.ErrNotExist)

	// Hash should be reset so next apply with hash1 is not skipped
	require.Equal(t, []byte("hash1"), mgr.GetConfigHash())
}

func TestRollbackConfig_ErrorsWhenNoBackup(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config", "collector.yaml")

	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  filepath.Join(dir, "config"),
		OutputPath: outputPath,
	})

	err := mgr.RollbackConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no backup")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./configmanager/ -run TestRollbackConfig -v`
Expected: FAIL — `RollbackConfig` method doesn't exist

**Step 3: Write minimal implementation**

```go
// RollbackConfig restores the previous config from the backup file.
// It resets lastHash to previousHash so the next remote config with
// the old hash is not skipped by the deduplication check.
func (m *Manager) RollbackConfig() error {
	bakPath := m.cfg.OutputPath + ".bak"

	bakContent, err := os.ReadFile(bakPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no backup config to roll back to")
		}
		return fmt.Errorf("failed to read backup config: %w", err)
	}

	if err := persistence.WriteFile(m.cfg.OutputPath, bakContent, 0o600); err != nil {
		return fmt.Errorf("failed to write rolled-back config: %w", err)
	}

	if err := os.Remove(bakPath); err != nil {
		m.logger.Warn("failed to remove backup file after rollback", zap.Error(err))
	}

	m.lastHash = m.previousHash
	m.previousHash = nil

	m.logger.Info("rolled back to previous config", zap.String("path", m.cfg.OutputPath))
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./configmanager/ -run TestRollbackConfig -v`
Expected: PASS

**Step 5: Commit**

```
feat(configmanager): add RollbackConfig for restart failure recovery
```

---

### Task 3: Remote config status persistence

Add `SaveRemoteConfigStatus()` and `LoadRemoteConfigStatus()` to the config manager.

**Files:**
- Modify: `configmanager/manager.go` (add methods + YAML struct)
- Test: `configmanager/manager_test.go`

**Step 1: Write the failing tests**

```go
func TestSaveAndLoadRemoteConfigStatus(t *testing.T) {
	dir := t.TempDir()
	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  dir,
		OutputPath: filepath.Join(dir, "collector.yaml"),
	})

	hash := []byte("abc123")
	err := mgr.SaveRemoteConfigStatus(
		protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
		"",
		hash,
	)
	require.NoError(t, err)

	status, err := mgr.LoadRemoteConfigStatus()
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED, status.GetStatus())
	require.Equal(t, hash, status.GetLastRemoteConfigHash())
	require.Empty(t, status.GetErrorMessage())
}

func TestSaveAndLoadRemoteConfigStatus_Failed(t *testing.T) {
	dir := t.TempDir()
	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  dir,
		OutputPath: filepath.Join(dir, "collector.yaml"),
	})

	hash := []byte("abc123")
	err := mgr.SaveRemoteConfigStatus(
		protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED,
		"restart failed: exit code 1",
		hash,
	)
	require.NoError(t, err)

	status, err := mgr.LoadRemoteConfigStatus()
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED, status.GetStatus())
	require.Equal(t, "restart failed: exit code 1", status.GetErrorMessage())
	require.Equal(t, hash, status.GetLastRemoteConfigHash())
}

func TestLoadRemoteConfigStatus_NoFile(t *testing.T) {
	dir := t.TempDir()
	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  dir,
		OutputPath: filepath.Join(dir, "collector.yaml"),
	})

	status, err := mgr.LoadRemoteConfigStatus()
	require.NoError(t, err)
	require.Nil(t, status)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./configmanager/ -run TestSaveAndLoadRemoteConfigStatus -v && go test ./configmanager/ -run TestLoadRemoteConfigStatus_NoFile -v`
Expected: FAIL — methods don't exist

**Step 3: Write minimal implementation**

Add the YAML struct and methods to `configmanager/manager.go`:

```go
import "encoding/base64"

// remoteConfigStatusYAML is the on-disk YAML representation of RemoteConfigStatus.
type remoteConfigStatusYAML struct {
	Status           string `koanf:"status"`
	ErrorMessage     string `koanf:"error_message"`
	LastConfigHash   string `koanf:"last_config_hash"` // base64-encoded
}

const remoteConfigStatusFile = "remote_config_status.yaml"

// SaveRemoteConfigStatus persists the remote config status to disk as YAML.
func (m *Manager) SaveRemoteConfigStatus(status protobufs.RemoteConfigStatuses, errorMessage string, configHash []byte) error {
	data := remoteConfigStatusYAML{
		Status:         status.String(),
		ErrorMessage:   errorMessage,
		LastConfigHash: base64.StdEncoding.EncodeToString(configHash),
	}

	path := filepath.Join(m.cfg.ConfigDir, remoteConfigStatusFile)
	return persistence.WriteYAMLFile(".", path, data)
}

// LoadRemoteConfigStatus loads the persisted remote config status from disk.
// Returns nil, nil if the file does not exist (fresh install).
func (m *Manager) LoadRemoteConfigStatus() (*protobufs.RemoteConfigStatus, error) {
	path := filepath.Join(m.cfg.ConfigDir, remoteConfigStatusFile)

	var data remoteConfigStatusYAML
	if err := persistence.LoadYAMLFile(".", path, &data); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load remote config status: %w", err)
	}

	hash, err := base64.StdEncoding.DecodeString(data.LastConfigHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode config hash: %w", err)
	}

	statusEnum, ok := protobufs.RemoteConfigStatuses_value[data.Status]
	if !ok {
		return nil, fmt.Errorf("unknown remote config status: %s", data.Status)
	}

	return &protobufs.RemoteConfigStatus{
		Status:               protobufs.RemoteConfigStatuses(statusEnum),
		ErrorMessage:         data.ErrorMessage,
		LastRemoteConfigHash: hash,
	}, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./configmanager/ -run "TestSaveAndLoadRemoteConfigStatus|TestLoadRemoteConfigStatus_NoFile" -v`
Expected: PASS

**Step 5: Run `go fix`**

Run: `go fix ./configmanager/`

**Step 6: Commit**

```
feat(configmanager): add remote config status YAML persistence
```

---

### Task 4: Initial remote config status on OpAMP client

Add a field to the `Client` wrapper so the supervisor can set the initial `RemoteConfigStatus` before `Start()`, and pass it into `StartSettings`.

**Files:**
- Modify: `opamp/client.go:162-169` (add field), `opamp/client.go:229-235` (pass in StartSettings)
- Test: `opamp/client_test.go`

**Step 1: Write the failing test**

Check what tests already exist in `opamp/client_test.go` for patterns. The key behavior: when `SetInitialRemoteConfigStatus` is called before `Start()`, the status should appear in `StartSettings.RemoteConfigStatus`. Since we can't easily inspect `StartSettings` without starting a real connection, test that the field is stored:

```go
func TestClient_SetInitialRemoteConfigStatus(t *testing.T) {
	// Verify the field is stored and accessible
	// (integration test will verify it reaches StartSettings)
	c := &Client{}
	status := &protobufs.RemoteConfigStatus{
		Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
		LastRemoteConfigHash: []byte("hash"),
	}
	c.SetInitialRemoteConfigStatus(status)
	require.Equal(t, status, c.remoteConfigStatus)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./opamp/ -run TestClient_SetInitialRemoteConfigStatus -v`
Expected: FAIL — field and method don't exist

**Step 3: Write minimal implementation**

In `opamp/client.go`, add the field to `Client` struct (after `effectiveConfig`):

```go
type Client struct {
	logger              *zap.Logger
	cfg                 ClientConfig
	callbacks           *Callbacks
	opampClient         client.OpAMPClient
	effectiveConfig     *protobufs.EffectiveConfig
	remoteConfigStatus  *protobufs.RemoteConfigStatus
	started             atomic.Bool
}
```

Add the method:

```go
// SetInitialRemoteConfigStatus sets the remote config status to include in
// StartSettings. Must be called before Start(). This restores persisted status
// so the server knows our last config state after a supervisor restart.
func (c *Client) SetInitialRemoteConfigStatus(status *protobufs.RemoteConfigStatus) {
	c.remoteConfigStatus = status
}
```

In `Start()`, after building `settings` (around line 235), add:

```go
	settings.RemoteConfigStatus = c.remoteConfigStatus
```

**Step 4: Run test to verify it passes**

Run: `go test ./opamp/ -run TestClient_SetInitialRemoteConfigStatus -v`
Expected: PASS

**Step 5: Commit**

```
feat(opamp): support initial RemoteConfigStatus in client StartSettings
```

---

### Task 5: Enable ReportsRemoteConfig capability and fix config path mismatch

Three prerequisite fixes needed before the OnRemoteConfig wiring:

1. **Enable `ReportsRemoteConfig` capability** — opamp-go requires `AgentCapabilities_ReportsRemoteConfig` to be set, otherwise `SetRemoteConfigStatus()` returns `ErrReportsRemoteConfigNotSet` (see `opamp-go/client/internal/clientcommon.go:375`).

2. **Fix config path mismatch** — `supervisor.go:241` uses `filepath.Join(s.persistenceDir, "effective.yaml")` as the collector config path passed to commander args, but the config manager writes to `filepath.Join(cfg.Persistence.Dir, "config", "collector.yaml")` (set at `supervisor.go:161`). The collector will never see the merged config because it's reading from the wrong path.

3. **Inject health_check extension in ApplyRemoteConfig** — `InjectHealthCheckExtension` exists in `configmerge/inject.go` but is never called from production code. The supervisor must inject it (like OpAMP extension) to guarantee the health endpoint stays reachable regardless of remote config content. Without this, `awaitCollectorHealthy` could false-rollback.

**Files:**
- Modify: `supervisor/supervisor.go:522-535` (add capability)
- Modify: `supervisor/supervisor.go:239-241` (fix config path)
- Modify: `configmanager/manager.go` (add health_check injection + config fields)
- Test: `configmanager/manager_test.go`

**Step 1: Enable ReportsRemoteConfig capability**

In `supervisor/supervisor.go`, in the `Capabilities` struct within `createAndStartClient` (line 522-535), add `ReportsRemoteConfig: true`:

```go
		Capabilities: opamp.Capabilities{
			AcceptsRemoteConfig:            true,
			ReportsEffectiveConfig:         true,
			ReportsRemoteConfig:            true,
			ReportsHealth:                  true,
			AcceptsOpAMPConnectionSettings: true,
			// ReportsConnectionSettingsStatus is disabled because opamp-go schedules APPLIED immediately after the callback
			// returns, but the actual reconnect happens asynchronously on the worker. The sender's in-flight status POST races
			// with client.Stop(), producing a spurious "context canceled" error. Enable once opamp-go exposes a public
			// SetConnectionSettingsStatus API so we can report the outcome after the async reconnect completes.
			ReportsConnectionSettingsStatus: false,
			AcceptsRestartCommand:           true,
			ReportsHeartbeat:                true,
			ReportsAvailableComponents:      true,
		},
```

**Step 2: Fix config path to use config manager's OutputPath**

In `supervisor/supervisor.go`, replace lines 239-241:

```go
	// Determine effective config path
	// TODO: Write actual merged config when remote config handling is implemented
	configPath := filepath.Join(s.persistenceDir, "effective.yaml")
```

With:

```go
	// Use the config manager's output path — this is where ApplyRemoteConfig writes
	// the merged effective config that the collector should read.
	configPath := s.configManager.OutputPath()
```

Add an `OutputPath()` accessor to `configmanager/manager.go`:

```go
// OutputPath returns the path where the effective config is written.
func (m *Manager) OutputPath() string {
	return m.cfg.OutputPath
}
```

**Step 3: Inject health_check extension in ApplyRemoteConfig**

Add a `HealthCheck` field to the config manager's `Config` struct. The health monitor's endpoint is a full URL (`http://localhost:13133/health`), but the OTel collector's `health_check` extension expects `host:port` as `endpoint` and an optional separate `path`. We parse the URL in the supervisor when wiring the config.

```go
type Config struct {
	ConfigDir       string                  // Directory to store remote configs
	OutputPath      string                  // Path to write final merged config
	LocalOverrides  []string                // Paths to local override files
	LocalEndpoint   string                  // Local OpAMP server endpoint for injection
	InstanceUID     string                  // Instance UID for injection
	HealthCheck     configmerge.HealthCheckConfig // Health check extension injection settings
}
```

In `configmanager/manager.go`, in `ApplyRemoteConfig`, after the OpAMP extension injection block (around line 143), add health_check injection:

```go
	// Inject health_check extension to guarantee it stays reachable.
	// This runs after merge so remote config cannot override the endpoint.
	if m.cfg.HealthCheck.Endpoint != "" {
		mergedConfig, err = configmerge.InjectHealthCheckExtension(mergedConfig, m.cfg.HealthCheck)
		if err != nil {
			return nil, fmt.Errorf("failed to inject health_check extension: %w", err)
		}
		m.logger.Debug("injected health_check extension",
			zap.String("endpoint", m.cfg.HealthCheck.Endpoint))
	}
```

Wire it in `supervisor.go` where configManager is created (around line 159). Parse the health URL into `host:port` + `path`:

```go
	// Parse the health monitor URL (e.g. "http://localhost:13133/health") into
	// the host:port and path components that the OTel health_check extension expects.
	healthCheck := configmerge.HealthCheckConfig{}
	if cfg.Agent.Health.Endpoint != "" {
		u, err := url.Parse(cfg.Agent.Health.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("invalid health endpoint URL: %w", err)
		}
		healthCheck.Endpoint = u.Host
		if u.Path != "" && u.Path != "/" {
			healthCheck.Path = u.Path
		}
	}

	configMgr := configmanager.New(logger.Named("config"), configmanager.Config{
		ConfigDir:      filepath.Join(cfg.Persistence.Dir, "config"),
		OutputPath:     filepath.Join(cfg.Persistence.Dir, "config", "collector.yaml"),
		LocalOverrides: cfg.Agent.Config.LocalOverrides,
		LocalEndpoint:  cfg.LocalServer.Endpoint,
		InstanceUID:    uid,
		HealthCheck:    healthCheck,
	})
```

**Step 4: Write targeted tests**

Add tests to `configmanager/manager_test.go`:

```go
func TestOutputPath(t *testing.T) {
	mgr := New(zaptest.NewLogger(t), Config{
		OutputPath: "/var/lib/superv/config/collector.yaml",
	})
	require.Equal(t, "/var/lib/superv/config/collector.yaml", mgr.OutputPath())
}

func TestApplyRemoteConfig_InjectsHealthCheckExtension(t *testing.T) {
	dir := t.TempDir()
	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  filepath.Join(dir, "config"),
		OutputPath: filepath.Join(dir, "config", "collector.yaml"),
		HealthCheck: configmerge.HealthCheckConfig{
			Endpoint: "localhost:13133",
			Path:     "/health",
		},
	})

	cfg := makeRemoteConfig(t, []byte("receivers:\n  otlp:\n"), []byte("hash1"))
	result, err := mgr.ApplyRemoteConfig(context.Background(), cfg)
	require.NoError(t, err)
	require.True(t, result.Changed)

	// Effective config should contain the health_check extension with correct settings
	require.Contains(t, string(result.EffectiveConfig), "health_check")
	require.Contains(t, string(result.EffectiveConfig), "localhost:13133")
	require.Contains(t, string(result.EffectiveConfig), "/health")
}
```

The `ReportsRemoteConfig` capability is already covered by the existing test in `opamp/client_test.go:124,141` which asserts that `ReportsRemoteConfig: true` maps to the correct protobuf bit. What we're adding here is that the supervisor actually *sets* it — verify this by grep/inspection since there are no supervisor unit tests.

**Step 5: Run tests**

Run: `go test ./configmanager/ -run "TestOutputPath|TestApplyRemoteConfig_InjectsHealthCheckExtension" -v && go test ./opamp/ -run TestCapabilities -v`
Expected: PASS

**Step 6: Run `go fix`**

Run: `go fix ./supervisor/ && go fix ./configmanager/`

**Step 7: Commit**

```
fix(supervisor): enable ReportsRemoteConfig, fix config path, inject health_check
```

---

### Task 6: Wire up OnRemoteConfig with restart, health confirmation, and status reporting

Replace the TODO in `supervisor.go` with the full flow: restart, confirm health, rollback on failure, status reporting.

**Context:** `Commander.Start()` returns immediately when crash recovery is enabled (`MaxRetries >= 1`) — the actual process start happens in a goroutine. We must poll `healthMonitor.CheckHealth()` after restart to confirm the collector is actually running with the new config before reporting APPLIED.

**Files:**
- Modify: `supervisor/supervisor.go:848-878` (OnRemoteConfig callback)

**Step 1: Read the current callback**

The current code at `supervisor/supervisor.go:848-878`. This is the main integration point.

**Step 2: Add a health confirmation helper**

Add a helper method to `supervisor.go` that polls health with a timeout. Uses the configurable `agent.config_apply_timeout` (default 5s) from `s.agentCfg.ConfigApplyTimeout`. Falls back to a process-alive check when the health HTTP endpoint returns connection refused (e.g. port conflict), to avoid false rollbacks when the health endpoint itself is affected.

```go
// awaitCollectorHealthy polls the health monitor until the collector reports
// healthy or the timeout expires. This is needed because Commander.Start()
// returns immediately when crash recovery is enabled (MaxRetries >= 1).
//
// If the health HTTP check fails with a connection error (endpoint unreachable),
// we fall back to checking whether the process is still running. This avoids
// false rollbacks when the health endpoint is temporarily unavailable.
func (s *Supervisor) awaitCollectorHealthy(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		status, err := s.healthMonitor.CheckHealth(ctx)
		if err == nil && status.Healthy {
			return nil
		}

		// Fallback: if health endpoint is unreachable but process is alive,
		// treat as healthy. The health_check extension is injected by us,
		// but a port conflict or slow bind could cause transient failures.
		if err != nil && s.commander.IsRunning() {
			s.logger.Debug("Health endpoint unreachable but process alive, waiting",
				zap.Error(err))
		}

		select {
		case <-ctx.Done():
			// Final check: if process is running, consider it alive even without health
			if s.commander.IsRunning() {
				s.logger.Warn("Health endpoint never became reachable, but process is running; treating as healthy")
				return nil
			}
			if status != nil {
				return fmt.Errorf("collector not healthy after %v: %s", timeout, status.ErrorMessage)
			}
			return fmt.Errorf("collector not healthy after %v", timeout)
		case <-ticker.C:
		}
	}
}
```

Note: `Commander.IsRunning()` checks `c.running.Load()` — this already exists (`keen.go`). If it doesn't, add it as a simple accessor on `Commander`.

**Step 3: Write the OnRemoteConfig implementation**

Replace the `OnRemoteConfig` callback body with:

```go
		OnRemoteConfig: func(ctx context.Context, cfg *protobufs.AgentRemoteConfig) bool {
			s.logger.Info("Received remote configuration")

			result, err := s.configManager.ApplyRemoteConfig(ctx, cfg)
			if err != nil {
				s.logger.Error("Failed to apply remote config", zap.Error(err))
				s.reportRemoteConfigStatus(ctx,
					protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED,
					err.Error(),
					cfg.GetConfigHash(),
				)
				return false
			}

			if !result.Changed {
				return true
			}

			// Restart collector with new config.
			// Restart = Stop + Start, so if Start fails the collector is down.
			// On failure we roll back the config file and re-start the collector
			// with the previous config to avoid leaving it stopped.
			s.logger.Info("Config changed, restarting collector")
			if err := s.commander.Restart(ctx); err != nil {
				s.logger.Error("Failed to restart collector with new config", zap.Error(err))
				s.rollbackAndRecover(ctx, result.ConfigHash, err)
				return false
			}

			// Confirm the collector is healthy with the new config.
			// Commander.Start() returns immediately when crash recovery is
			// enabled (MaxRetries >= 1), so we must poll health to confirm
			// the process actually started successfully.
			if err := s.awaitCollectorHealthy(ctx, s.agentCfg.ConfigApplyTimeout); err != nil {
				s.logger.Error("Collector unhealthy after restart", zap.Error(err))
				s.rollbackAndRecover(ctx, result.ConfigHash, err)
				return false
			}

			// Report effective config
			s.reportEffectiveConfig(ctx, result.EffectiveConfig)

			// Report success
			s.reportRemoteConfigStatus(ctx,
				protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
				"",
				result.ConfigHash,
			)

			return true
		},
```

**Step 4: Add helper methods to supervisor**

Add these private helpers to `supervisor.go`:

```go
// rollbackAndRecover rolls back to the previous config and restarts the
// collector. Used when the new config fails (either restart error or health
// check failure). Reports FAILED status to the server.
func (s *Supervisor) rollbackAndRecover(ctx context.Context, configHash []byte, originalErr error) {
	if rbErr := s.configManager.RollbackConfig(); rbErr != nil {
		s.logger.Error("Failed to roll back config", zap.Error(rbErr))
	}

	// Restart with rolled-back config. The collector may be stopped
	// (Restart = Stop + Start, failed Start leaves process down) or
	// running with a bad config (health check failed).
	if restartErr := s.commander.Restart(ctx); restartErr != nil {
		s.logger.Error("Failed to restart collector after rollback", zap.Error(restartErr))
	}

	s.reportRemoteConfigStatus(ctx,
		protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED,
		fmt.Sprintf("collector restart failed: %v", originalErr),
		configHash,
	)
}

// reportRemoteConfigStatus reports config status to the OpAMP server and persists it to disk.
func (s *Supervisor) reportRemoteConfigStatus(ctx context.Context, status protobufs.RemoteConfigStatuses, errMsg string, configHash []byte) {
	s.mu.RLock()
	client := s.opampClient
	s.mu.RUnlock()

	if client != nil {
		if err := client.SetRemoteConfigStatus(&protobufs.RemoteConfigStatus{
			Status:               status,
			ErrorMessage:         errMsg,
			LastRemoteConfigHash: configHash,
		}); err != nil {
			s.logger.Warn("Failed to report remote config status", zap.Error(err))
		}
	}

	if err := s.configManager.SaveRemoteConfigStatus(status, errMsg, configHash); err != nil {
		s.logger.Warn("Failed to persist remote config status", zap.Error(err))
	}
}

// reportEffectiveConfig reports the effective config to the OpAMP server.
func (s *Supervisor) reportEffectiveConfig(ctx context.Context, effectiveConfig []byte) {
	s.mu.RLock()
	client := s.opampClient
	s.mu.RUnlock()

	if client != nil {
		if err := client.SetEffectiveConfig(ctx, map[string]*protobufs.AgentConfigFile{
			"collector.yaml": {Body: effectiveConfig},
		}); err != nil {
			s.logger.Warn("Failed to report effective config", zap.Error(err))
		}
	}
}
```

**Step 5: Run existing tests**

Run: `go test ./supervisor/ -v -count=1`
Expected: PASS (existing tests should still work)

**Step 6: Run `go fix`**

Run: `go fix ./supervisor/`

**Step 7: Commit**

```
feat(supervisor): restart collector on remote config change with rollback
```

---

### Task 7: Load persisted status on startup

On startup, load the persisted remote config status and pass it to the OpAMP client.

**Files:**
- Modify: `supervisor/supervisor.go` — in `createAndStartClient` (around line 547-553) and `Start` method
- Modify: `opamp/client.go` — already done in Task 4

**Step 1: Write the implementation**

In `supervisor/supervisor.go`, in `createAndStartClient` (after `client.SetHealth` at line 551-553), add:

```go
	// Restore persisted remote config status so the server knows our
	// last config state after a supervisor restart.
	if status, err := s.configManager.LoadRemoteConfigStatus(); err != nil {
		s.logger.Warn("Failed to load persisted remote config status, starting with UNSET", zap.Error(err))
	} else if status != nil {
		client.SetInitialRemoteConfigStatus(status)
	}
```

**Step 2: Run existing tests**

Run: `go test ./supervisor/ -v -count=1`
Expected: PASS

**Step 3: Commit**

```
feat(supervisor): restore persisted remote config status on startup
```

---

### Task 8: Wire SaveRemoteConfigStatus callback (forward-compat)

Wire the `SaveRemoteConfigStatus` callback in the supervisor for forward-compatibility with future opamp-go versions.

**Files:**
- Modify: `supervisor/supervisor.go` — in `createOpAMPCallbacks` (around line 937)

**Step 1: Write the implementation**

Add to the callbacks struct in `createOpAMPCallbacks()`, before the closing brace:

```go
		SaveRemoteConfigStatus: func(ctx context.Context, status *protobufs.RemoteConfigStatus) {
			s.logger.Debug("SaveRemoteConfigStatus callback invoked",
				zap.String("status", status.GetStatus().String()),
			)
			if err := s.configManager.SaveRemoteConfigStatus(
				status.GetStatus(),
				status.GetErrorMessage(),
				status.GetLastRemoteConfigHash(),
			); err != nil {
				s.logger.Warn("Failed to persist remote config status from callback", zap.Error(err))
			}
		},
```

**Step 2: Run existing tests**

Run: `go test ./supervisor/ -v -count=1`
Expected: PASS

**Step 3: Commit**

```
feat(supervisor): wire SaveRemoteConfigStatus callback for forward-compat
```

---

### Task 9: End-to-end verification

Verify the full flow compiles and all tests pass.

**Step 1: Run all tests**

Run: `go test ./... -v -count=1`
Expected: All PASS

**Step 2: Run go vet**

Run: `go vet ./...`
Expected: No issues

**Step 3: Run go fix**

Run: `go fix ./...`
Expected: No changes needed
