# Collector Reload & Remote Config Status Reporting

**Date:** 2026-02-20
**Status:** Accepted

## Context

The supervisor receives remote configurations from the OpAMP server and merges them with local overrides, but two critical pieces are missing:

1. The collector process is never restarted after a config change (TODO at supervisor.go:858).
2. No `RemoteConfigStatus` (APPLIED/FAILED) is reported back to the server.

## Decisions

### Reload method: Always full restart

The OTel Collector supports both SIGHUP (config reload) and full restart. We always use `commander.Restart(ctx)` because:

- Some config changes (e.g. receiver port changes) require a full restart.
- Simpler logic — no need to detect which changes are SIGHUP-safe.
- Brief data collection gap during restart is acceptable.

### Synchronous restart in OnRemoteConfig callback

The restart happens inline in the `OnRemoteConfig` callback rather than via the async worker queue.

**Restart outcome verification:** `Commander.Start()` returns immediately when crash recovery is enabled (`MaxRetries >= 1`) — the actual process start happens in a background goroutine. To avoid reporting APPLIED prematurely, we poll `healthMonitor.CheckHealth()` with a configurable timeout (`agent.config_apply_timeout`, default 5s) after `Restart()` returns. Only once the collector is confirmed healthy (or the timeout expires) do we report APPLIED or FAILED.

**Health endpoint guarantee:** The supervisor injects the `health_check` extension into the effective config (same as the existing `opamp` extension injection). Injection happens after merge, so it cannot be overridden by remote config. If health HTTP check still fails with connection refused (e.g., port conflict), `awaitCollectorHealthy` falls back to checking the process is still alive for the remainder of the timeout. This avoids false rollbacks while still catching genuine startup failures.

### Config rollback on restart failure

If the collector restart fails, the config manager rolls back to the previous config file. This gives the strongest safety guarantee.

- The previous config is backed up to `OutputPath + ".bak"` on disk before writing the new config.
- Single backup file, not versioned history — only one rollback level needed.
- On-disk backup survives supervisor crashes mid-cycle.
- If no backup exists (first remote config ever), rollback is skipped and FAILED is reported.

### Config manager owns rollback

The config manager (not the supervisor) manages backup and rollback state. It already owns config file writes and hash tracking.

**New methods on `configmanager.Manager`:**
- `RollbackConfig()` — copies `.bak` back to `OutputPath`, resets `lastHash`, removes `.bak`

**Changes to `ApplyRemoteConfig`:**
- Before writing new config, copies existing `OutputPath` to `OutputPath + ".bak"`

### Remote config status reporting

After config application and restart, the supervisor reports status to the server via `client.SetRemoteConfigStatus()`.

- `APPLIED` with `ConfigHash` on success.
- `FAILED` with `ErrorMessage` and `ConfigHash` on failure (both apply errors and restart errors).

### Status persistence in YAML

The remote config status is persisted to `ConfigDir/remote_config_status.yaml` using `github.com/goccy/go-yaml`, consistent with other project serialization.

**New methods on `configmanager.Manager`:**
- `SaveRemoteConfigStatus(status, errorMessage, configHash)` — writes YAML to disk
- `LoadRemoteConfigStatus()` — reads YAML, returns `*protobufs.RemoteConfigStatus` or nil

On startup, the supervisor loads persisted status and passes it to `StartSettings.RemoteConfigStatus`. If the file doesn't exist or is corrupt, startup continues with UNSET status.

The `SaveRemoteConfigStatus` callback in `opamp/callbacks.go` is also wired up defensively for forward-compatibility with future opamp-go versions (v0.23.0 never calls it).

## Complete Flow

### Startup

```
supervisor.Start()
  ├─ configManager.LoadRemoteConfigStatus() → *protobufs.RemoteConfigStatus (or nil)
  ├─ create OpAMP client with StartSettings.RemoteConfigStatus
  └─ commander.Start()
```

### OnRemoteConfig

```
OnRemoteConfig(cfg)
  │
  ├─ configManager.ApplyRemoteConfig(cfg)
  │   ├─ backs up current config to .bak
  │   ├─ merges, injects, writes new config
  │   └─ returns ApplyResult{Changed, EffectiveConfig, ConfigHash}
  │
  ├─ error from ApplyRemoteConfig:
  │   ├─ client.SetRemoteConfigStatus(FAILED, err, configHash)
  │   ├─ configManager.SaveRemoteConfigStatus(FAILED, err, configHash)
  │   └─ return false
  │
  ├─ result.Changed == false:
  │   └─ return true (no-op)
  │
  ├─ commander.Restart(ctx)
  │   ├─ error (Restart = Stop + Start, so collector may be stopped):
  │   │   ├─ configManager.RollbackConfig()
  │   │   ├─ commander.Start(ctx) to bring collector back with old config
  │   │   ├─ client.SetRemoteConfigStatus(FAILED, err, configHash)
  │   │   ├─ configManager.SaveRemoteConfigStatus(FAILED, err, configHash)
  │   │   └─ return false
  │   └─ success: continue
  │
  ├─ healthMonitor.CheckHealth() with timeout
  │   ├─ healthy:
  │   │   ├─ client.SetEffectiveConfig(result.EffectiveConfig)
  │   │   ├─ client.SetRemoteConfigStatus(APPLIED, configHash)
  │   │   ├─ configManager.SaveRemoteConfigStatus(APPLIED, configHash)
  │   │   └─ return true
  │   └─ unhealthy/timeout:
  │       ├─ configManager.RollbackConfig()
  │       ├─ commander.Restart(ctx) to recover with old config
  │       ├─ client.SetRemoteConfigStatus(FAILED, err, configHash)
  │       ├─ configManager.SaveRemoteConfigStatus(FAILED, err, configHash)
  │       └─ return false
```

## Files Changed

| File | Changes |
|------|---------|
| `configmanager/manager.go` | Add `.bak` backup before write, `RollbackConfig()`, `SaveRemoteConfigStatus()`, `LoadRemoteConfigStatus()`, `OutputPath()` accessor, inject `health_check` extension, add `HealthEndpoint` config field |
| `supervisor/supervisor.go` | Enable `ReportsRemoteConfig` capability, fix config path to use `configManager.OutputPath()`, pass `HealthEndpoint` to config manager, load status on startup, full OnRemoteConfig flow with restart + health confirmation + rollback + recovery + status reporting, `awaitCollectorHealthy()` with configurable timeout (`agent.config_apply_timeout`) and process-alive fallback |
| `opamp/client.go` | Add `remoteConfigStatus` field and `SetInitialRemoteConfigStatus()` for startup restore |
| `opamp/callbacks.go` | Wire `SaveRemoteConfigStatus` callback (forward-compat) |
