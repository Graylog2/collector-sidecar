# Collector Own-Logs via Zap Core Tee

**Date:** 2026-03-11
**Status:** Proposed
**Replaces:** Relay approach from `add/own-telemetry-collector-relay` branch

## Problem

The supervisor already exports its own logs via OTLP (the `superv/ownlogs` package), but the
collector process's logs are not captured. The relay approach (intercepting collector logs via a
local OTLP receiver in the supervisor) proved too complex and fragile.

## Solution

Hook into the collector's zap logger at startup via `otelcol.CollectorSettings.LoggingOptions`
to tee log entries to an OTLP exporter. Reuse the supervisor's `own-logs.yaml` persistence file
as the configuration source. The supervisor restarts the collector when own_logs settings change,
so the collector only needs to read the file once at startup.

## Design Principle: Collector Availability First

Own-logs export is strictly best-effort. The collector must always start and continue running,
even if the own-logs configuration is missing, malformed, or the OTLP exporter fails to
connect. Errors in `NewCoreFromFile()` or during export are logged to stderr and silently
ignored — the collector proceeds without the OTLP tee. The batch processor's non-blocking
ring buffer (drops on overflow, never blocks the caller) ensures that a slow or unreachable
OTLP endpoint cannot stall the collector's logging pipeline.

## Design

### Data Flow

```
OpAMP server
    │
    ▼ own_logs settings
Supervisor (OnOwnLogs callback)
    │
    ├─► Apply to supervisor's own logger (existing)
    ├─► Persist to own-logs.yaml (existing)
    └─► Restart collector (new)
            │
            ▼
Collector startup (customizeSettings)
    │
    ├─► Read GLC_INTERNAL_PERSISTENCE_DIR env var
    ├─► Load own-logs.yaml via ownlogs.NewCoreFromFile()
    └─► Tee zap core: zapcore.NewTee(original, otlpCore)
            │
            ▼
Collector logs ──► stderr (existing)
               ──► OTLP exporter (new)
```

### Component 1: `superv/ownlogs` — `NewCoreFromFile()`

New exported function:

```go
// NewCoreFromFile loads own-logs settings from persistenceDir/own-logs.yaml,
// builds an OTLP log exporter and otelzap core. Returns the core, a shutdown
// function, and any error. If the file doesn't exist, returns (nil, nil, nil).
//
// Callers must treat errors as non-fatal: a failure here must never prevent
// the collector from starting. Log the error and proceed without the OTLP tee.
func NewCoreFromFile(persistenceDir, clientCertPath, clientKeyPath string, res *resource.Resource) (zapcore.Core, func(context.Context), error)
```

Internally:
1. Join `persistenceDir` + `ownLogsFileName` ("own-logs.yaml")
2. Load `persistedSettings` from YAML (reuse existing load + `rebuildTLSConfigFromPEM`)
3. Build OTLP exporter (HTTP or gRPC based on endpoint scheme)
4. Create `LoggerProvider` with `BatchProcessor` (default batch config)
5. Create `otelzap.NewCore` wrapping the provider
6. Return core + shutdown func (calls `LoggerProvider.Shutdown`)

The exporter-building logic currently lives in `Manager.buildExporter()` and will be extracted
so both `Manager.Apply()` and `NewCoreFromFile()` can share it.

**Note:** `ProxyHeaders` on HTTP exporters are not currently supported by `Manager.buildExporter()`
(documented limitation in `manager.go`). This limitation carries over to `NewCoreFromFile()` and
is explicitly out of scope for this design.

### Component 2: `builder/main_customize.go` — Collector-side Wiring

```go
var ownLogsShutdown func(context.Context)

func customizeSettings(params *otelcol.CollectorSettings) {
    params.LoggingOptions = append(params.LoggingOptions, zap.WithCaller(false))

    persistDir := os.Getenv("GLC_INTERNAL_PERSISTENCE_DIR")
    if persistDir == "" {
        return
    }

    res := ownlogs.BuildResource("collector", version.Version(),
        os.Getenv("GLC_INTERNAL_INSTANCE_UID"))

    core, shutdown, err := ownlogs.NewCoreFromFile(
        persistDir,
        os.Getenv("GLC_INTERNAL_TLS_CLIENT_CERT_PATH"),
        os.Getenv("GLC_INTERNAL_TLS_CLIENT_KEY_PATH"),
        res,
    )
    if err != nil {
        // Collector availability first: log warning, continue without own-logs.
        // A broken own-logs config must never prevent the collector from starting.
        fmt.Fprintf(os.Stderr, "WARNING: own-logs setup failed, continuing without OTLP log export: %v\n", err)
        return
    }
    if core == nil {
        return // no own-logs.yaml, skip
    }

    ownLogsShutdown = shutdown
    params.LoggingOptions = append(params.LoggingOptions,
        zap.WrapCore(func(original zapcore.Core) zapcore.Core {
            return zapcore.NewTee(original, core)
        }),
    )
}

func customizeCommand(params *otelcol.CollectorSettings, cmd *cobra.Command) {
    cmd.AddCommand(superv.GetCommand())
    if ownLogsShutdown != nil {
        cmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
            ownLogsShutdown(cmd.Context())
        }
    }
}
```

### Component 3: Supervisor-side Changes

**`buildCollectorEnv()`** — add persistence dir to collector environment:

```go
"GLC_INTERNAL_PERSISTENCE_DIR": s.persistenceDir,
```

**`OnOwnLogs` callback** — restart collector after settings change:

- After persisting settings (enable path): call `s.commander.Restart(ctx)`
- After deleting settings (disable path): call `s.commander.Restart(ctx)`

```go
// TODO: If own_logs and a config change arrive close together, the collector
// may be restarted twice. This is harmless but wasteful. Consider coalescing
// restarts in the future.
```

**Persistence-gated restart:** The collector is only restarted if the persistence operation
(Save or Delete) succeeds. If persistence fails, the restart is skipped to avoid the
collector reading stale or missing settings while the supervisor has already applied
different settings in-memory. The supervisor logs the persistence error.

### Resource Attributes

The collector's own-logs use the same resource attribute pattern as the supervisor:

| Attribute | Value |
|---|---|
| `service.name` | `"collector"` |
| `service.version` | Build version |
| `service.instance.id` | Instance UID (from `GLC_INTERNAL_INSTANCE_UID`) |
| `collector.receiver.type` | `"collector_log"` |

### Shutdown — Best-Effort Flush

Own-logs export follows the same availability-first principle as the rest of this design:
flushing is best-effort, and no shutdown mechanism is allowed to delay or complicate the
collector's exit.

**Why deterministic flush is not achievable:** The collector's shutdown path
(`service.Shutdown`) calls `loggerShutdownFunc.Shutdown()` for its built-in telemetry logger
but does not call `logger.Sync()` on the zap logger. There is no public hook to register
additional cleanup in the collector's shutdown sequence. Workarounds (signal handlers, custom
extensions) add complexity and risk interfering with the collector's own lifecycle.

**What we do instead:**

- **`PersistentPostRun` → `Shutdown()`:** The cobra hook calls `LoggerProvider.Shutdown()`,
  which flushes the batch processor and releases exporter resources. This fires on the clean
  success path only — cobra skips `PersistentPostRun` when `RunE` returns an error.

- **Batch processor periodic export:** The `BatchProcessor` exports every ~1s (configurable
  via `ExportInterval`). During the collector's graceful shutdown (SIGTERM → context
  cancellation → service teardown), there are typically several seconds of activity during
  which the batch processor fires at least one more export cycle.

**Accepted data loss window:** On error exits or abrupt termination (SIGKILL, OOM kill), up
to one batch interval (~1s) of buffered logs may be lost. This is acceptable because:

1. Own-logs export is a diagnostic side-channel, not a data pipeline.
2. The supervisor sends SIGTERM first, giving the batch processor time to export during the
   graceful shutdown period (up to 10s) before falling back to SIGKILL.
3. The most important logs (startup errors, config problems) are emitted early and exported
   in prior batch cycles, well before shutdown.

## Alternatives Considered

### Relay approach (rejected)
A local OTLP receiver in the supervisor that the collector sends logs to via
`service.telemetry.logs`. Too complex: requires managing a second OTLP pipeline, local
endpoint coordination, and adds latency.

### `zap.Hooks` (rejected)
Simpler hook-based capture via `zap.Hooks()`. Only receives `zapcore.Entry` (level, message,
logger name) — does not include structured fields, losing valuable context from collector logs.

### File watching / polling (rejected)
Having the collector watch `own-logs.yaml` for changes to hot-swap the exporter without
restarts. Adds complexity; restarting the collector is simpler and sufficient since settings
changes are infrequent.

## Scope

- Collector own-logs export via OTLP (this design)
- Supervisor own-logs export via OTLP (already implemented in `superv/ownlogs`)
- Collector restart on own_logs settings change (new)

Out of scope:
- Metrics or traces export from the collector
- Hot-swap of collector own-logs settings without restart
- ProxyHeaders support on HTTP OTLP exporters (existing limitation in `Manager.buildExporter()`)

## Test Plan

### `superv/ownlogs` — `NewCoreFromFile()`
- **No file:** returns `(nil, nil, nil)` when `own-logs.yaml` does not exist
- **Valid file:** returns a non-nil core and shutdown func; core implements `Enabled`, `Write`
- **Invalid file:** returns error (malformed YAML, bad TLS material)
- **Shutdown:** calling shutdown func does not panic, is idempotent

### `builder/main_customize.go`
- **No env var:** `customizeSettings` with empty `GLC_INTERNAL_PERSISTENCE_DIR` does not modify
  `LoggingOptions` beyond `WithCaller(false)`
- **With env var + file:** `LoggingOptions` includes `WrapCore`; `PersistentPostRun` is set
- **With env var, no file:** no `WrapCore` added, no `PersistentPostRun` set
- **Broken file:** `customizeSettings` logs warning and proceeds without `WrapCore` — collector
  starts normally (availability first)

### `superv/supervisor` — OnOwnLogs + restart
- **Enable flow:** `OnOwnLogs` with valid settings persists file and calls `commander.Restart`
- **Disable flow:** `OnOwnLogs` with empty endpoint deletes file and calls `commander.Restart`
- **`buildCollectorEnv`:** includes `GLC_INTERNAL_PERSISTENCE_DIR`
