# Design: Supervisor Log Export via otelzap and OpAMP own_logs

## Problem

The supervisor currently logs only to stderr. We want to export supervisor logs
to an OTLP endpoint, configured dynamically via OpAMP's
`ConnectionSettingsOffers.own_logs` field.

## Decisions

- **otelzap bridge** (Approach 1) — uses the existing `otelzap` bridge to
  convert Zap log entries to OTel log records, backed by a `BatchProcessor` and
  OTLP exporter.
- **Endpoint from OpAMP** — the OTLP endpoint, headers, and TLS settings come
  from `TelemetryConnectionSettings` delivered via the `OnConnectionSettings`
  callback. No local config field.
- **Both OTLP/HTTP and gRPC** — detect protocol from the URL (path contains
  `/v1/logs` → HTTP; otherwise gRPC).
- **Stderr-only until server offers** — no OTLP export until `own_logs`
  settings arrive (or are restored from disk).
- **Hot-swap exporter** — atomically replace the OTLP exporter when new
  settings arrive. Old provider drains its batch buffer and shuts down.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                    Zap Logger                        │
│  zap.New(zapcore.NewTee(stderrCore, swappableCore)) │
└──────────────┬──────────────────┬───────────────────┘
               │                  │
     ┌─────────▼──────┐  ┌───────▼────────────┐
     │  stderr core   │  │  swappableCore      │
     │  (always on)   │  │  (delegates to      │
     │                │  │   otelzap.Core or    │
     │                │  │   nop if no provider)│
     └────────────────┘  └───────┬─────────────┘
                                 │ when active
                         ┌───────▼─────────────┐
                         │  otelzap.Core        │
                         │  (LoggerProvider)    │
                         └───────┬─────────────┘
                                 │
                         ┌───────▼─────────────┐
                         │  BatchProcessor      │
                         │  (ring buffer, async)│
                         └───────┬─────────────┘
                                 │
                         ┌───────▼─────────────┐
                         │  OTLP Exporter       │
                         │  (HTTP or gRPC)      │
                         └─────────────────────┘
```

## swappableCore

A custom `zapcore.Core` that enables hot-swapping the underlying otelzap core.

```go
type swappableCore struct {
    inner  *atomic.Pointer[zapcore.Core] // shared across all With() derivatives
    fields []zapcore.Field               // accumulated With() fields
}
```

Key behaviors:

- **`Enabled(level)`** — loads inner; if nil, returns false (zero overhead when
  inactive — Zap short-circuits before allocating).
- **`With(fields)`** — returns a new `swappableCore` sharing the same `inner`
  pointer, with appended fields. This ensures all derived loggers see the swap.
- **`Check(entry, ce)`** — loads inner; if nil or disabled, returns ce
  unchanged. Otherwise adds itself to the `CheckedEntry`.
- **`Write(entry, fields)`** — loads current inner, applies stored `With()`
  fields, delegates write. Re-applying fields per write ensures derived loggers
  always use the current exporter.

## Non-blocking guarantee

The OTel SDK `BatchProcessor` uses a ring buffer (`queue` type). `OnEmit`
enqueues the record and returns immediately. When the queue is full, the oldest
records are silently dropped — the caller is never blocked. Export happens in a
separate goroutine. The only lock is a brief `sync.Mutex` for ring buffer
pointer updates (microseconds).

## OpAMP integration

1. **Enable capability** — set `ReportsOwnLogs: true` in the supervisor's
   capabilities struct (`supervisor.go`).

2. **Add `OnConnectionSettings` callback** — extract `settings.OwnLogs`
   (`*protobufs.TelemetryConnectionSettings`):
   - `destination_endpoint` — OTLP URL
   - `headers` — auth tokens, etc.
   - `certificate` / `tls` — TLS configuration

3. **Build exporter from settings** — detect protocol from URL:
   - Path contains `/v1/logs` → OTLP/HTTP exporter
   - Otherwise → gRPC exporter
   - Map headers and TLS settings to exporter options.

4. **Swap the provider** — build
   `sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)))`,
   create new `otelzap.Core`, swap into `swappableCore`, shut down old provider.

5. **Persist settings** — store `TelemetryConnectionSettings` to disk so OTLP
   export can be restored on restart without waiting for the server to re-offer.
   Same persistence pattern as `connectionSettingsManager` for OpAMP settings.

## Startup flow

1. `initLogger()` → stderr core (unchanged).
2. Create `swappableCore` with nil inner (nop, zero overhead).
3. Tee: `zap.New(zapcore.NewTee(stderrCore, swappableCore))`.
4. Load persisted `own_logs` settings from disk (if any).
   If found: build exporter + provider, swap into swappableCore.
   OTLP export is active immediately, before OpAMP connects.
5. `supervisor.New(logger, cfg)` → `Start(ctx)`.
6. OpAMP connects, server sends `ConnectionSettingsOffers.own_logs`.
   Build new exporter + provider, swap in (old provider drains and shuts down).
7. On shutdown: `provider.Shutdown(ctx)` flushes remaining batch.

## Resource attributes

Per the OpAMP spec, OTLP telemetry resources SHOULD include the agent's
identifying attributes. The `LoggerProvider` resource will include:

- `service.name` = `"graylog-supervisor"`
- `service.version` = supervisor version
- `service.instance.id` = agent instance UID
- Identifying attributes from the agent description

These are passed to `Apply()` alongside the `TelemetryConnectionSettings`.

## Package structure

New package: `superv/ownlogs/`

Public API:
- `NewSwappableCore() *SwappableCore` — creates the core with nil inner.
- `SwappableCore.Apply(settings *protobufs.TelemetryConnectionSettings, res resource.Resource) error` — builds exporter + provider, swaps in, shuts down old.
- `SwappableCore.Shutdown(ctx context.Context) error` — flushes and shuts down current provider.
- `SwappableCore.Core() zapcore.Core` — returns the `zapcore.Core` for use in `NewTee`.

## New dependencies

| Module | Purpose |
|--------|---------|
| `go.opentelemetry.io/otel/sdk/log` | `LoggerProvider`, `BatchProcessor` |
| `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp` | OTLP/HTTP log exporter |
| `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc` | OTLP/gRPC log exporter |
| `go.opentelemetry.io/contrib/bridges/otelzap` | Zap-to-OTel bridge core |
| `go.opentelemetry.io/otel/sdk` | `resource.Resource` |
