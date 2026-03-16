# Design: Collector Internal Metrics Export via Own-Metrics

## Problem

The OTel collector produces internal metrics about its operations (spans
processed, queue sizes, exporter errors, etc.). We want to export these to the
Graylog server via OTLP, configured dynamically via OpAMP's
`ConnectionSettingsOffers.own_metrics` field — the same pattern used for
own-logs.

The standard `service.telemetry.metrics` YAML configuration doesn't meet our
needs (missing server name setting, no OpAMP-driven reconfiguration). We need a
programmatic hook.

## Decisions

- **Replace the telemetry factory's meter provider** — wrap `params.Factories`
  in `customizeSettings` to inject a custom `CreateMeterProvider` that reads
  `own-metrics.yaml` and builds an OTLP periodic reader. All other factory
  methods (logger, resource, tracer) delegate to the original
  `otelconftelemetry` implementation.
- **Restart-based reconfiguration** — same as own-logs. When OpAMP pushes new
  metrics settings, the supervisor persists `own-metrics.yaml` and restarts the
  collector. No runtime hot-swap of the meter provider.
- **Separate persistence file** — `own-metrics.yaml`, independent from
  `own-logs.yaml`. Mirrors the OpAMP model where each signal has its own
  `TelemetryConnectionSettings`.
- **Allow-list filtering via `exported_metrics`** — only metrics listed in
  `exported_metrics` config are exported. If the list is empty, no metrics are
  exported even if `own-metrics.yaml` exists. This replaces the upstream
  views/level handling entirely. The default `config.yaml` ships with a curated
  list of useful metrics. This is a deliberate choice: the upstream
  `DefaultViews` and `configtelemetry.Level` mechanism controls cardinality via
  aggregation views, but we want explicit control over which metrics reach the
  Graylog server.
- **Noop when unconfigured** — if `own-metrics.yaml` doesn't exist or
  `exported_metrics` is empty, return a noop meter provider.
- **Use `set.Resource` from MeterSettings** — the `CreateMeterProvider` function
  receives the resource produced by `CreateResource` through
  `telemetry.MeterSettings.Resource`. We use that as the base (which includes
  user-configured telemetry resource attributes) and append the Graylog-specific
  `collector.receiver.type=collector_metric` attribute. We do NOT use a separate
  `BuildResource` call for metrics.
- **Rename `ownlogs` → `owntelemetry`** — merge logs and metrics own-telemetry
  code into a single package. Shared connection/persistence infrastructure, with
  signal-specific managers.
- **Both gRPC and OTLP/HTTP** — same as own-logs, both transports are supported.
  This is a project-specific extension beyond the OpAMP spec, which describes
  `destination_endpoint` as an OTLP/HTTP URL. We support gRPC because the
  existing own-logs infrastructure already handles both, and some deployments
  prefer gRPC.
- **10s default export interval** — per the OpAMP spec recommendation for own
  metrics reporting. Configurable via supervisor config and overridable via
  OpAMP `?export_interval=` query param.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                  customizeSettings()                      │
│  Wraps params.Factories to replace Telemetry factory      │
└──────────────────────┬───────────────────────────────────┘
                       │
          ┌────────────▼────────────────┐
          │  Custom telemetry.Factory    │
          │                             │
          │  CreateDefaultConfig  → orig│
          │  CreateResource      → orig│
          │  CreateLogger        → orig│
          │  CreateTracerProvider → orig│
          │  CreateMeterProvider → OURS │
          └────────────┬────────────────┘
                       │
          ┌────────────▼─────────────────────┐
          │  ourCreateMeterProvider()         │
          │                                  │
          │  1. Read own-metrics.yaml         │
          │  2. If absent or no metrics → noop│
          │  3. Build OTLP exporter          │
          │  4. PeriodicReader (10s default)  │
          │  5. Views: drop-all + pass-list  │
          │  6. sdkmetric.MeterProvider      │
          └────────────┬─────────────────────┘
                       │
          ┌────────────▼────────────────┐
          │  PeriodicReader              │
          │  (interval from config/     │
          │   OpAMP override)           │
          └────────────┬────────────────┘
                       │
          ┌────────────▼────────────────┐
          │  OTLP Exporter              │
          │  (HTTP or gRPC)             │
          └─────────────────────────────┘
```

## Package rename: `ownlogs` → `owntelemetry`

The `superv/ownlogs` package is renamed to `superv/owntelemetry`. All code
stays in one package to minimize import overhead.

### File structure

```
superv/owntelemetry/
├── settings.go            # Settings, Equal(), LoadClientCert()
├── exporter.go            # buildExporter(), isGRPC()
├── resource.go            # BuildResource()
├── persistence.go         # Persistence (parameterized by filename)
├── convert.go             # ConvertSettings() from OpAMP proto
├── log_manager.go         # Manager (log manager)
├── log_core_from_file.go  # NewCoreFromFile()
├── swappable_core.go      # SwappableCore
├── field_filter_core.go   # FieldFilterCore
├── meter_from_file.go     # NewMeterProviderFromFile()
└── *_test.go
```

### Shared code

The following are shared between logs and metrics:

- **`Settings`** — endpoint, headers, TLS config, proxy, raw PEM material.
  Log-specific: `LogLevel`. Metric-specific: `ExportInterval` (parsed from
  `?export_interval=` query param, overrides config default).
- **`buildExporter`** — currently builds `sdklog.Exporter`. Since the OTLP
  client options differ between `otlploghttp` and `otlpmetrichttp` (different
  option types), we'll have a separate `buildMetricExporter` that follows the
  same pattern but uses `otlpmetricgrpc`/`otlpmetrichttp`.
- **`isGRPC`** — shared, but must be generalized. Currently checks for
  `/v1/logs` to detect HTTP. Must change to check for any `/v1/` path prefix
  (e.g., `/v1/logs`, `/v1/metrics`) so it works for both signals.
- **`Persistence`** — parameterized by filename. Constructor takes the filename
  (`own-logs.yaml` or `own-metrics.yaml`). Same YAML format, same TLS rebuild
  logic.
- **`ConvertSettings`** — shared. The OpAMP `TelemetryConnectionSettings` proto
  is the same shape for both signals. Extracts signal-specific query params:
  `?log_level=` for logs, `?export_interval=` for metrics.
- **`BuildResource`** — shared. Takes a `receiverType` parameter: `"collector_log"`
  for logs, `"collector_metric"` for metrics. This controls the
  `collector.receiver.type` resource attribute that the Graylog server uses to
  route incoming data. Note: for the collector-side metrics integration,
  `BuildResource` is only used for the `collector.receiver.type` attribute
  appended to `set.Resource` — see "Use `set.Resource`" decision above.
- **`Settings.Equal`** — shared. The `LogLevel` and `ExportInterval` fields are
  signal-specific but harmless for cross-signal comparison: `ConvertSettings`
  leaves them empty for the other signal, so they won't trigger false
  change-detection.

## Allow-list filtering

The `exported_metrics` config field controls which collector internal metrics
are exported. Implementation uses OTel SDK views:

1. If `exported_metrics` is empty → return noop provider (no metrics exported)
2. If non-empty → register views:
   - A catch-all view with drop aggregation (drops everything by default)
   - One pass-through view per metric name in the list

This gives explicit control over cardinality and cost. The default `config.yaml`
ships with a curated list of useful collector metrics.

## `NewMeterProviderFromFile`

Parallel to `NewCoreFromFile`. Called from `customizeSettings`.

```go
func NewMeterProviderFromFile(
    persistDir string,
    clientCertPath string,
    clientKeyPath string,
    res *sdkresource.Resource,
    batchCfg config.BatchConfig,
    exportedMetrics []string,
) (*sdkmetric.MeterProvider, error)
```

Returns `(nil, nil)` when `own-metrics.yaml` doesn't exist — caller returns
noop. Unlike `NewCoreFromFile` (which returns a separate shutdown func), no
shutdown func is needed: the collector's service layer calls `Shutdown` on the
returned `MeterProvider` directly during graceful shutdown.

The `res` parameter is the merged resource: the caller (`CreateMeterProvider`
wrapper) converts `set.Resource` from pcommon to SDK resource attributes and
appends `collector.receiver.type=collector_metric`. This merged resource is
passed in because `sdkmetric.WithResource` is only available at construction
time — there is no post-construction setter on `MeterProvider`.

Steps:

1. Load `own-metrics.yaml` from `persistDir` via `Persistence.Load()`
2. If file doesn't exist, return `(nil, nil)` — caller returns noop
3. If `exportedMetrics` is empty, return `(nil, nil)` — caller returns noop
4. Rebuild TLS config from persisted PEM material
5. Load mTLS client cert from `clientCertPath`/`clientKeyPath`
6. Build OTLP metric exporter (gRPC or HTTP based on `isGRPC`)
7. Create `sdkmetric.PeriodicReader` with interval/timeout from `batchCfg`,
   overridden by `Settings.ExportInterval` if set via OpAMP
8. Build views: drop-all + pass-through for each `exportedMetrics` entry
9. Create `sdkmetric.NewMeterProvider` with reader, views, and resource
10. Return provider

## `buildMetricExporter`

Follows the same pattern as the log exporter builder but uses metric-specific
OTLP exporter packages:

- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp`

Both are already indirect dependencies in `builder/go.mod`.

The HTTP variant must use the `/v1/metrics` path suffix (not `/v1/logs`).

## Error handling — collector availability first

If `NewMeterProviderFromFile` returns an error (bad TLS config, malformed YAML,
etc.), `CreateMeterProvider` must log a warning and return a noop `MeterProvider`
— never return the error. Returning an error from `CreateMeterProvider` would
prevent the collector from starting, violating the "collector availability first"
principle established by the own-logs pattern.

## Collector-side integration (`builder/main_customize.go`)

```go
func customizeSettings(params *otelcol.CollectorSettings) {
    // ... existing logging setup ...

    // Wrap Factories to inject custom meter provider
    origFactories := params.Factories
    params.Factories = func() (otelcol.Factories, error) {
        f, err := origFactories()
        if err != nil {
            return f, err
        }
        orig := f.Telemetry
        f.Telemetry = telemetry.NewFactory(
            orig.CreateDefaultConfig,
            telemetry.WithCreateResource(orig.CreateResource),
            telemetry.WithCreateLogger(orig.CreateLogger),
            telemetry.WithCreateTracerProvider(orig.CreateTracerProvider),
            telemetry.WithCreateMeterProvider(makeCreateMeterProvider(persistDir, certPath, keyPath)),
        )
        return f, nil
    }
}
```

`makeCreateMeterProvider` returns a `CreateMeterProviderFunc` closure that:

1. Converts `set.Resource` (pcommon) to SDK resource attributes
2. Appends `collector.receiver.type=collector_metric` attribute
3. Calls `NewMeterProviderFromFile` with the merged resource
4. If nil (unconfigured), returns a noop `MeterProvider`
5. If error, logs warning, returns noop `MeterProvider`
6. Returns the provider

### Shutdown

The collector's service layer calls `Shutdown` on the `MeterProvider` returned
by `CreateMeterProvider` during graceful shutdown. The `sdkmetric.MeterProvider`
flushes the periodic reader on shutdown. No additional `PersistentPostRun` hook
is needed (unlike logs, which needed one because the zap core tee is outside the
service layer's shutdown path).

## Supervisor-side changes

### OpAMP callbacks (`opamp/callbacks.go`)

Add `OnOwnMetrics` callback:

```go
OnOwnMetrics func(ctx context.Context, settings *protobufs.TelemetryConnectionSettings)
```

The `onMessage` handler in `opamp/callbacks.go` needs a new branch to dispatch
`msg.OwnMetricsConnSettings` to the `OnOwnMetrics` callback.

### Capabilities (`opamp/client.go`)

Set `ReportsOwnMetrics: true` (field already exists, currently `false`).

### Handler (`supervisor/supervisor.go`)

Add `handleOwnMetrics` following the `handleOwnLogs` pattern:

1. If empty endpoint → delete `own-metrics.yaml`, restart collector
2. Convert via `owntelemetry.ConvertSettings`
3. Skip if unchanged (`Settings.Equal`)
4. Save to `own-metrics.yaml` via `Persistence`
5. Restart collector

### Startup (`cmd/supervisor/main.go`)

- Create `owntelemetry.NewPersistence("own-metrics.yaml", ...)` instance
- Load persisted settings on startup
- Pass to supervisor via `SetOwnMetrics(persistence, restoredSettings)`

Note: no `MetricsManager` on the supervisor side — the supervisor doesn't
produce its own metrics yet. It only persists config and restarts the collector.

## Configuration (`config/types.go`)

```go
type TelemetryConfig struct {
    Logs    TelemetryLogsConfig    `koanf:"logs"`
    Metrics TelemetryMetricsConfig `koanf:"metrics"`
}

type TelemetryMetricsConfig struct {
    Batch           BatchConfig `koanf:"batch"`
    ExportedMetrics []string    `koanf:"exported_metrics"`
}
```

The `BatchConfig` type is already defined (used by `TelemetryLogsConfig`). Only
`ExportInterval` and `ExportTimeout` apply to the periodic reader — the
`MaxQueueSize` and `ExportMaxBatchSize` fields are log-specific (batch
processor) and are ignored for metrics.

Default config values:
- `ExportInterval`: 10s (per OpAMP spec recommendation)
- `ExportedMetrics`: curated list of useful collector metrics (TBD)

The `ExportInterval` can be overridden per-agent via OpAMP using the
`?export_interval=` query param on the metrics endpoint URL. If present in the
OpAMP settings, it takes precedence over the config.yaml default.

## Dependencies

The following are already indirect dependencies in `builder/go.mod` and need to
become direct:

- `go.opentelemetry.io/otel/sdk/metric`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp`

The `superv/go.mod` needs the same metric exporter dependencies added.

## Known risks

- **`telemetry.Factory` interface is experimental** — the OTel Collector marks
  it as such and warns it may change. Our factory wrapping delegates all methods
  except `CreateMeterProvider` to the original. If the interface gains new
  methods, `telemetry.NewFactory` would use default implementations instead of
  delegating. Mitigation: review on OTel Collector upgrades.

## Not in scope

- **Supervisor-side metrics manager** — future work. The supervisor doesn't
  produce its own metrics yet.
- **Hot-swap meter provider at runtime** — restart-based, consistent with logs.
