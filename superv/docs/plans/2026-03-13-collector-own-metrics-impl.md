# Collector Own-Metrics Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Export collector internal metrics to Graylog via OTLP, configured dynamically via OpAMP `own_metrics`.

**Architecture:** Wrap the OTel collector's telemetry factory in `customizeSettings` to inject a custom `CreateMeterProvider` that reads `own-metrics.yaml` and builds an OTLP periodic reader with allow-list filtering. Supervisor-side handles OpAMP callbacks, persistence, and collector restart. Rename `ownlogs` → `owntelemetry` package.

**Tech Stack:** Go, OTel SDK (`sdkmetric`), OTLP metric exporters (gRPC/HTTP), OTel Collector telemetry factory API

**Spec:** `superv/docs/plans/2026-03-13-collector-own-metrics-design.md`

---

## File Structure

### New files
- `superv/owntelemetry/meter_from_file.go` — `NewMeterProviderFromFile`, `buildMetricExporter`, allow-list views
- `superv/owntelemetry/meter_from_file_test.go` — tests for above

### Renamed files (package rename `ownlogs` → `owntelemetry`)
- `superv/ownlogs/*.go` → `superv/owntelemetry/*.go` (all files, just change package name + directory)
- Note: the spec proposes further file renames within the package (e.g., `manager.go` → `log_manager.go`). These are deferred to a follow-up to keep this plan focused on functionality.

### Modified files
- `superv/owntelemetry/manager.go` — generalize `isGRPC` to check `/v1/` prefix instead of `/v1/logs`
- `superv/owntelemetry/manager.go` — add `ExportInterval` field to `Settings`, update `Equal()`
- `superv/owntelemetry/persistence.go` — parameterize filename (remove hardcoded `ownLogsFileName`), add `ExportInterval` to `persistedSettings`
- `superv/owntelemetry/convert.go` — extract `?export_interval=` query param
- `superv/owntelemetry/resource.go` — accept `receiverType` parameter in `BuildResource`
- `superv/config/types.go` — add `TelemetryMetricsConfig`, extend `TelemetryConfig`, add defaults
- `superv/opamp/callbacks.go` — add `OnOwnMetrics` callback + `onMessage` dispatch
- `superv/supervisor/supervisor.go` — add `handleOwnMetrics`, `SetOwnMetrics`, own-metrics fields, capability
- `superv/cmd/supervisor/main.go` — create metrics persistence, load on startup, pass to supervisor
- `builder/main_customize.go` — wrap `params.Factories` to inject custom `CreateMeterProvider`

---

## Chunk 1: Package Rename and Shared Code Changes

### Task 1: Rename `ownlogs` → `owntelemetry`

**Files:**
- Rename: `superv/ownlogs/` → `superv/owntelemetry/` (all `.go` files)

- [ ] **Step 1: Rename directory and update package declarations**

```bash
cd /home/bernd/graylog/sidecar/superv
git mv ownlogs owntelemetry
```

Then update every `package ownlogs` → `package owntelemetry` in all `.go` files under `superv/owntelemetry/`.

- [ ] **Step 2: Update all imports across the codebase**

Search for `"github.com/Graylog2/collector-sidecar/superv/ownlogs"` and replace with `"github.com/Graylog2/collector-sidecar/superv/owntelemetry"` in:
- `superv/cmd/supervisor/main.go`
- `superv/supervisor/supervisor.go`
- `builder/main_customize.go`

- [ ] **Step 3: Verify build**

Run: `cd /home/bernd/graylog/sidecar/superv && go build ./...`
Expected: PASS

Run: `cd /home/bernd/graylog/sidecar/builder && go build ./...`
Expected: PASS

- [ ] **Step 4: Verify tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./owntelemetry/...`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add -A superv/ownlogs superv/owntelemetry superv/cmd/supervisor/main.go superv/supervisor/supervisor.go builder/main_customize.go
git commit -m "refactor: rename ownlogs package to owntelemetry

Prepares for adding own-metrics support alongside own-logs in the same
package."
```

---

### Task 2: Generalize `isGRPC` to be signal-agnostic

**Files:**
- Modify: `superv/owntelemetry/manager.go:299-308`
- Test: `superv/owntelemetry/manager_test.go`

- [ ] **Step 1: Write failing test for `/v1/metrics` path**

Add to `superv/owntelemetry/manager_test.go`:

```go
func TestIsGRPC_MetricsPath(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     bool
	}{
		{"HTTP metrics path", "https://example.com:4318/v1/metrics", false},
		{"HTTP logs path", "https://example.com:4318/v1/logs", false},
		{"gRPC port", "https://example.com:4317", true},
		{"HTTP default", "https://example.com:4318", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGRPC(tt.endpoint); got != tt.want {
				t.Errorf("isGRPC(%q) = %v, want %v", tt.endpoint, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./owntelemetry/ -run TestIsGRPC_MetricsPath -v`
Expected: PASS (actually `/v1/metrics` is not explicitly checked but falls through to port check, so the first case passes by accident on port 4318. But let's verify behavior is correct after the change.)

- [ ] **Step 3: Change `isGRPC` to check `/v1/` prefix**

In `superv/owntelemetry/manager.go`, replace lines 299-308:

```go
// isGRPC detects whether the endpoint should use gRPC based on the URL.
// URLs with a /v1/ path (e.g. /v1/logs, /v1/metrics) use HTTP;
// port 4317 without a path uses gRPC.
func isGRPC(endpoint string) bool {
	if strings.Contains(endpoint, "/v1/") {
		return false
	}
	if u, err := url.Parse(endpoint); err == nil && u.Port() == "4317" {
		return true
	}
	// Default to HTTP per the OpAMP spec.
	return false
}
```

- [ ] **Step 4: Run all tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./owntelemetry/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add superv/owntelemetry/manager.go superv/owntelemetry/manager_test.go
git commit -m "fix: generalize isGRPC to detect any /v1/ path prefix

Previously only checked for /v1/logs. Now works for /v1/metrics and any
future signal path."
```

---

### Task 3: Add `ExportInterval` to `Settings` and `Equal`

**Files:**
- Modify: `superv/owntelemetry/manager.go:45-90`

- [ ] **Step 1: Add `ExportInterval` field to `Settings`**

In `superv/owntelemetry/manager.go`, after the `LogLevel` field (line 68), add:

```go
	// ExportInterval overrides the configured default metrics export interval
	// when set via ?export_interval=<duration> on the DestinationEndpoint URL.
	ExportInterval time.Duration
```

Add `"time"` to the imports.

- [ ] **Step 2: Add `ExportInterval` to `Equal`**

In `superv/owntelemetry/manager.go`, change line 89 from:

```go
		s.LogLevel == other.LogLevel
```

to:

```go
		s.LogLevel == other.LogLevel &&
		s.ExportInterval == other.ExportInterval
```

- [ ] **Step 3: Run tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./owntelemetry/ -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add superv/owntelemetry/manager.go
git commit -m "feat: add ExportInterval field to owntelemetry.Settings

Supports OpAMP-driven export interval override via ?export_interval=
query param on the metrics endpoint URL."
```

---

### Task 4: Extract `?export_interval=` in `ConvertSettings`

**Files:**
- Modify: `superv/owntelemetry/convert.go:51-75`
- Test: `superv/owntelemetry/convert_test.go`

- [ ] **Step 1: Write failing test**

Add to `superv/owntelemetry/convert_test.go`:

```go
func TestConvertSettings_ExportInterval(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/metrics?export_interval=15s",
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Equal(t, 15*time.Second, s.ExportInterval)
	// Query param should be stripped from the endpoint
	assert.NotContains(t, s.Endpoint, "export_interval")
}
```

(Uses the existing `writeTestClientCert` helper from `convert_test.go`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./owntelemetry/ -run TestConvertSettings_ExportInterval -v`
Expected: FAIL — `ExportInterval` is zero

- [ ] **Step 3: Add `export_interval` extraction to `ConvertSettings`**

In `superv/owntelemetry/convert.go`, inside the query-param extraction block (after the `log_level` block, around line 69), add:

```go
		// ?export_interval=<duration> — metrics export interval override,
		// e.g. "10s", "1m". Takes precedence over the config.yaml default.
		if ei := q.Get("export_interval"); ei != "" {
			if d, parseErr := time.ParseDuration(ei); parseErr == nil {
				s.ExportInterval = d // will be set on s below
			}
			q.Del("export_interval")
			changed = true
		}
```

Note: the `ExportInterval` is stored in a local variable and then applied when building `s`. Move the local var declaration up next to `logLevel`:

```go
	var tlsServerName string
	var logLevel string
	var exportInterval time.Duration
```

Then in the block, store to `exportInterval` instead:

```go
		if ei := q.Get("export_interval"); ei != "" {
			if d, parseErr := time.ParseDuration(ei); parseErr == nil {
				exportInterval = d
			}
			q.Del("export_interval")
			changed = true
		}
```

And after `s.LogLevel = logLevel` (line 99), add:

```go
	s.ExportInterval = exportInterval
```

Add `"time"` to imports.

- [ ] **Step 4: Run test**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./owntelemetry/ -run TestConvertSettings_ExportInterval -v`
Expected: PASS

- [ ] **Step 5: Run all tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./owntelemetry/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add superv/owntelemetry/convert.go superv/owntelemetry/convert_test.go
git commit -m "feat: extract ?export_interval= query param in ConvertSettings

Allows the OpAMP server to override the metrics export interval
per-agent via the endpoint URL."
```

---

### Task 5: Parameterize `Persistence` filename

**Files:**
- Modify: `superv/owntelemetry/persistence.go:104,136-141`
- Test: `superv/owntelemetry/persistence_test.go`

- [ ] **Step 1: Add `ExportInterval` to `persistedSettings`**

In `superv/owntelemetry/persistence.go`, after the `LogLevel` field (line 123), add:

```go
	ExportInterval time.Duration `koanf:"export_interval,omitempty"`
```

Add `"time"` to the imports.

- [ ] **Step 2: Replace hardcoded filename with constructor parameter**

Change the `ownLogsFileName` constant (line 104) and `NewPersistence` (lines 136-142):

Remove the `const ownLogsFileName` line. Change `NewPersistence` to:

```go
// NewPersistence creates a Persistence that stores settings in the given file
// under dataDir. clientCertPath and clientKeyPath are the paths to the mTLS
// client certificate and key files that will be loaded when restoring settings.
func NewPersistence(dataDir, fileName, clientCertPath, clientKeyPath string) *Persistence {
	return &Persistence{
		filePath:       filepath.Join(dataDir, fileName),
		clientCertPath: clientCertPath,
		clientKeyPath:  clientKeyPath,
	}
}
```

- [ ] **Step 3: Add `ExportInterval` to Save and Load**

In `Save` (line 154), add after `LogLevel` assignment:

```go
		ExportInterval: s.ExportInterval,
```

In `Load` (line 177), add after `LogLevel` assignment (line 201):

```go
		ExportInterval: ps.ExportInterval,
```

- [ ] **Step 4: Update all callers to pass filename**

In `superv/cmd/supervisor/main.go` (line 132), change:

```go
ownLogsPersist := ownlogs.NewPersistence(cfg.Persistence.Dir, certPath, keyPath)
```

to:

```go
ownLogsPersist := owntelemetry.NewPersistence(cfg.Persistence.Dir, "own-logs.yaml", certPath, keyPath)
```

In `superv/owntelemetry/core_from_file.go` (line 37), change:

```go
p := NewPersistence(persistenceDir, clientCertPath, clientKeyPath)
```

to:

```go
p := NewPersistence(persistenceDir, "own-logs.yaml", clientCertPath, clientKeyPath)
```

- [ ] **Step 5: Update tests**

Search `persistence_test.go` and `core_from_file_test.go` for `NewPersistence` calls and add the `"own-logs.yaml"` filename parameter.

- [ ] **Step 6: Run all tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./owntelemetry/ -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add superv/owntelemetry/persistence.go superv/owntelemetry/core_from_file.go superv/owntelemetry/persistence_test.go superv/owntelemetry/core_from_file_test.go superv/cmd/supervisor/main.go
git commit -m "refactor: parameterize Persistence filename

NewPersistence now takes a fileName parameter instead of hardcoding
own-logs.yaml. Also adds ExportInterval to persisted settings."
```

---

### Task 6: Parameterize `BuildResource` with `receiverType`

**Files:**
- Modify: `superv/owntelemetry/resource.go:30-48`
- Modify: all callers of `BuildResource`

- [ ] **Step 1: Add `receiverType` parameter**

In `superv/owntelemetry/resource.go`, change `BuildResource` signature:

```go
// BuildResource creates an OTel resource with service identifying attributes
// for the supervisor's own telemetry.
func BuildResource(serviceName, serviceVersion, instanceID, receiverType string) *resource.Resource {
	attrs := []resource.Option{
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			// Required to let the server correctly process the record.
			attribute.String("collector.receiver.type", receiverType),
		),
	}
```

- [ ] **Step 2: Update all callers**

In `superv/cmd/supervisor/main.go` (line 140):

```go
res := owntelemetry.BuildResource(supervisor.ServiceName, version.Version(), instanceUID, "collector_log")
```

In `superv/supervisor/supervisor.go` (line 1317):

```go
res := owntelemetry.BuildResource(ServiceName, version.Version(), s.instanceUID, "collector_log")
```

In `builder/main_customize.go` (line 45):

```go
res := owntelemetry.BuildResource("collector", params.BuildInfo.Version,
    os.Getenv("GLC_INTERNAL_INSTANCE_UID"), "collector_log")
```

- [ ] **Step 3: Run tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./... 2>&1 | tail -20`
Expected: All PASS

Run: `cd /home/bernd/graylog/sidecar/builder && go build ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add superv/owntelemetry/resource.go superv/cmd/supervisor/main.go superv/supervisor/supervisor.go builder/main_customize.go
git commit -m "refactor: parameterize BuildResource with receiverType

Accepts collector.receiver.type as a parameter instead of hardcoding
collector_log. Prepares for collector_metric usage."
```

---

## Chunk 2: Configuration and Metrics Provider

### Task 7: Add `TelemetryMetricsConfig` to config

**Files:**
- Modify: `superv/config/types.go:190-215,299-309`

- [ ] **Step 1: Add `TelemetryMetricsConfig` type and extend `TelemetryConfig`**

In `superv/config/types.go`, after line 193:

```go
type TelemetryConfig struct {
	Logs    TelemetryLogsConfig    `koanf:"logs"`
	Metrics TelemetryMetricsConfig `koanf:"metrics"`
}

// TelemetryMetricsConfig configures own-metrics export via OTLP.
type TelemetryMetricsConfig struct {
	// Batch configures the periodic reader's export interval and timeout.
	// Only ExportInterval and ExportTimeout apply; MaxQueueSize and
	// ExportMaxBatchSize are ignored for metrics.
	Batch BatchConfig `koanf:"batch"`
	// ExportedMetrics is the allow-list of metric names to export.
	// If empty, no metrics are exported even if own-metrics.yaml exists.
	ExportedMetrics []string `koanf:"exported_metrics"`
}
```

- [ ] **Step 2: Add defaults in `DefaultConfig`**

In `superv/config/types.go`, extend the `Telemetry` block in `DefaultConfig()` (after line 308):

```go
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
			Metrics: TelemetryMetricsConfig{
				Batch: BatchConfig{
					ExportInterval: 10 * time.Second,
					ExportTimeout:  30 * time.Second,
				},
				ExportedMetrics: []string{},
			},
		},
```

- [ ] **Step 3: Run tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go build ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add superv/config/types.go
git commit -m "feat: add TelemetryMetricsConfig with ExportedMetrics allow-list

Adds configuration for own-metrics export: batch settings (10s default
interval per OpAMP spec) and exported_metrics allow-list."
```

---

### Task 8: Implement `NewMeterProviderFromFile`

**Files:**
- Create: `superv/owntelemetry/meter_from_file.go`
- Create: `superv/owntelemetry/meter_from_file_test.go`

- [ ] **Step 1: Write failing test**

Create `superv/owntelemetry/meter_from_file_test.go`:

```go
package owntelemetry

import (
	"testing"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/stretchr/testify/assert"
)

func TestNewMeterProviderFromFile_NoFile(t *testing.T) {
	provider, err := NewMeterProviderFromFile(
		t.TempDir(), "", "",
		nil,
		config.BatchConfig{ExportInterval: 10 * time.Second},
		[]string{"otelcol_exporter_sent_spans"},
	)
	assert.NoError(t, err)
	assert.Nil(t, provider)
}

func TestNewMeterProviderFromFile_EmptyAllowList(t *testing.T) {
	// Even with a valid config file, empty allow-list → nil provider
	dir := t.TempDir()
	p := NewPersistence(dir, "own-metrics.yaml", "", "")
	_ = p.Save(Settings{Endpoint: "http://localhost:4318"})

	provider, err := NewMeterProviderFromFile(
		dir, "", "",
		nil,
		config.BatchConfig{ExportInterval: 10 * time.Second},
		[]string{},
	)
	assert.NoError(t, err)
	assert.Nil(t, provider)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./owntelemetry/ -run TestNewMeterProviderFromFile -v`
Expected: FAIL — `NewMeterProviderFromFile` not defined

- [ ] **Step 3: Implement `NewMeterProviderFromFile`**

Create `superv/owntelemetry/meter_from_file.go`:

```go
package owntelemetry

import (
	"context"
	"fmt"
	"strings"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc/credentials"
)

// NewMeterProviderFromFile loads own-metrics settings from
// persistenceDir/own-metrics.yaml, builds an OTLP metric exporter and
// MeterProvider with allow-list filtering. Returns the provider and any error.
// If the file doesn't exist or exportedMetrics is empty, returns (nil, nil).
//
// Callers must treat errors as non-fatal: a failure here must never prevent
// the collector from starting.
func NewMeterProviderFromFile(
	persistenceDir, clientCertPath, clientKeyPath string,
	res *resource.Resource,
	batchCfg config.BatchConfig,
	exportedMetrics []string,
) (*sdkmetric.MeterProvider, error) {
	if len(exportedMetrics) == 0 {
		return nil, nil
	}

	p := NewPersistence(persistenceDir, "own-metrics.yaml", clientCertPath, clientKeyPath)
	s, exists, err := p.Load()
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	ctx := context.Background()
	exporter, err := buildMetricExporter(ctx, s)
	if err != nil {
		return nil, fmt.Errorf("build OTLP metric exporter: %w", err)
	}

	// Use OpAMP-provided export interval if set, otherwise use config default.
	interval := batchCfg.ExportInterval
	if s.ExportInterval > 0 {
		interval = s.ExportInterval
	}

	var readerOpts []sdkmetric.PeriodicReaderOption
	if interval > 0 {
		readerOpts = append(readerOpts, sdkmetric.WithInterval(interval))
	}
	if batchCfg.ExportTimeout > 0 {
		readerOpts = append(readerOpts, sdkmetric.WithTimeout(batchCfg.ExportTimeout))
	}

	reader := sdkmetric.NewPeriodicReader(exporter, readerOpts...)

	views := buildAllowListViews(exportedMetrics)

	opts := []sdkmetric.Option{
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(views...),
	}
	if res != nil {
		opts = append(opts, sdkmetric.WithResource(res))
	}

	return sdkmetric.NewMeterProvider(opts...), nil
}

// buildAllowListViews creates SDK views that only pass through metrics
// in the allow-list and drop everything else.
func buildAllowListViews(allowList []string) []sdkmetric.View {
	var views []sdkmetric.View

	// Pass-through views for each allowed metric (must come before drop-all)
	for _, name := range allowList {
		views = append(views, sdkmetric.NewView(
			sdkmetric.Instrument{Name: name},
			sdkmetric.Stream{},
		))
	}

	// Drop-all catch-all view
	views = append(views, sdkmetric.NewView(
		sdkmetric.Instrument{Name: "*"},
		sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}},
	))

	return views
}

func buildMetricExporter(ctx context.Context, s Settings) (sdkmetric.Exporter, error) {
	if isGRPC(s.Endpoint) {
		return buildGRPCMetricExporter(ctx, s)
	}
	return buildHTTPMetricExporter(ctx, s)
}

func buildHTTPMetricExporter(ctx context.Context, s Settings) (sdkmetric.Exporter, error) {
	endpoint := s.Endpoint
	if !strings.HasSuffix(endpoint, "/v1/metrics") {
		endpoint = strings.TrimRight(endpoint, "/") + "/v1/metrics"
	}
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpointURL(endpoint),
	}
	if s.Insecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}
	if s.TLSConfig != nil {
		opts = append(opts, otlpmetrichttp.WithTLSClientConfig(s.TLSConfig))
	}
	if len(s.Headers) > 0 {
		opts = append(opts, otlpmetrichttp.WithHeaders(s.Headers))
	}
	httpClient, err := newHTTPClient(s)
	if err != nil {
		return nil, err
	}
	if httpClient != nil {
		opts = append(opts, otlpmetrichttp.WithHTTPClient(httpClient))
	}
	return otlpmetrichttp.New(ctx, opts...)
}

func buildGRPCMetricExporter(ctx context.Context, s Settings) (sdkmetric.Exporter, error) {
	if s.ProxyURL != "" || len(s.ProxyHeaders) > 0 {
		return nil, fmt.Errorf("proxy settings are not supported for gRPC own_metrics endpoints")
	}

	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpointURL(s.Endpoint),
	}
	if s.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}
	if s.TLSConfig != nil {
		opts = append(opts, otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(s.TLSConfig)))
	}
	if len(s.Headers) > 0 {
		opts = append(opts, otlpmetricgrpc.WithHeaders(s.Headers))
	}
	return otlpmetricgrpc.New(ctx, opts...)
}
```

- [ ] **Step 4: Run tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./owntelemetry/ -run TestNewMeterProviderFromFile -v`
Expected: PASS

- [ ] **Step 5: Run all tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./owntelemetry/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add superv/owntelemetry/meter_from_file.go superv/owntelemetry/meter_from_file_test.go
git commit -m "feat: add NewMeterProviderFromFile with allow-list filtering

Builds an OTLP metric exporter and MeterProvider from own-metrics.yaml
with SDK views to only export metrics in the allow-list. Supports both
gRPC and HTTP transports."
```

---

### Task 9: Add metric exporter dependencies

**Files:**
- Modify: `superv/go.mod`, `superv/go.sum`

- [ ] **Step 1: Tidy modules**

```bash
cd /home/bernd/graylog/sidecar/superv && go mod tidy
```

- [ ] **Step 2: Verify build**

Run: `cd /home/bernd/graylog/sidecar/superv && go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add superv/go.mod superv/go.sum
git commit -m "deps: add OTLP metric exporter dependencies"
```

---

## Chunk 3: Collector-Side Integration

### Task 10: Wrap telemetry factory in `customizeSettings`

**Files:**
- Modify: `builder/main_customize.go`

- [ ] **Step 1: Add factory wrapping and `makeCreateMeterProvider`**

In `builder/main_customize.go`, add the factory wrapping inside `customizeSettings`, after the existing logging setup. Also add the `makeCreateMeterProvider` function.

The full updated `customizeSettings` function:

```go
func customizeSettings(params *otelcol.CollectorSettings) {
	// Disable caller information in logs to reduce log chatter and avoid exposing source code file names.
	params.LoggingOptions = append(params.LoggingOptions, zap.WithCaller(false))

	persistDir := os.Getenv("GLC_INTERNAL_PERSISTENCE_DIR")
	if persistDir == "" {
		return
	}

	certPath := os.Getenv("GLC_INTERNAL_TLS_CLIENT_CERT_PATH")
	keyPath := os.Getenv("GLC_INTERNAL_TLS_CLIENT_KEY_PATH")

	res := owntelemetry.BuildResource("collector", params.BuildInfo.Version,
		os.Getenv("GLC_INTERNAL_INSTANCE_UID"), "collector_log")

	core, shutdown, err := owntelemetry.NewCoreFromFile(
		persistDir, certPath, keyPath, res,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: own-logs setup failed, continuing without OTLP log export: %v\n", err)
		return
	}
	if core != nil {
		ownLogsShutdown = shutdown
		params.LoggingOptions = append(params.LoggingOptions,
			zap.WrapCore(func(original zapcore.Core) zapcore.Core {
				return zapcore.NewTee(original, &owntelemetry.FieldFilterCore{
					Core:       core,
					DropFields: []string{"resource"},
				})
			}),
		)
	}

	// Wrap Factories to inject custom meter provider for own-metrics.
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
			telemetry.WithCreateMeterProvider(
				makeCreateMeterProvider(persistDir, certPath, keyPath),
			),
		)
		return f, nil
	}
}
```

Add the `makeCreateMeterProvider` function:

```go
// makeCreateMeterProvider returns a CreateMeterProviderFunc that reads
// own-metrics.yaml and builds a MeterProvider with OTLP export.
// If own-metrics.yaml doesn't exist or the allow-list is empty, returns noop.
// Errors are logged and result in noop (collector availability first).
func makeCreateMeterProvider(persistDir, certPath, keyPath string) telemetry.CreateMeterProviderFunc {
	// Read config from env vars set by the supervisor.
	metricsCfgJSON := os.Getenv("GLC_INTERNAL_METRICS_CONFIG")
	var batchCfg config.BatchConfig
	var exportedMetrics []string
	if metricsCfgJSON != "" {
		// Parse JSON-encoded metrics config from env var.
		var mcfg struct {
			Batch           config.BatchConfig `json:"batch"`
			ExportedMetrics []string           `json:"exported_metrics"`
		}
		if err := json.Unmarshal([]byte(metricsCfgJSON), &mcfg); err == nil {
			batchCfg = mcfg.Batch
			exportedMetrics = mcfg.ExportedMetrics
		}
	}

	return func(
		ctx context.Context,
		set telemetry.MeterSettings,
		cfg component.Config,
	) (telemetry.MeterProvider, error) {
		if len(exportedMetrics) == 0 {
			return noopMeterProvider{}, nil
		}

		// Build resource from set.Resource (includes user-configured attrs)
		// and append the Graylog-specific collector.receiver.type attribute.
		attrs := pcommonAttrsToOTelAttrs(set.Resource)
		attrs = append(attrs, attribute.String("collector.receiver.type", "collector_metric"))
		res := sdkresource.NewWithAttributes("", attrs...)

		provider, err := owntelemetry.NewMeterProviderFromFile(
			persistDir, certPath, keyPath,
			res, batchCfg, exportedMetrics,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: own-metrics setup failed, continuing without OTLP metric export: %v\n", err)
			return noopMeterProvider{}, nil
		}
		if provider == nil {
			return noopMeterProvider{}, nil
		}
		return provider, nil
	}
}

// pcommonAttrsToOTelAttrs converts pcommon.Resource attributes to OTel SDK attributes.
func pcommonAttrsToOTelAttrs(res *pcommon.Resource) []attribute.KeyValue {
	var result []attribute.KeyValue
	if res != nil {
		attrs := res.Attributes()
		attrs.Range(func(k string, v pcommon.Value) bool {
			result = append(result, attribute.String(k, v.AsString()))
			return true
		})
	}
	return result
}

// noopMeterProvider implements telemetry.MeterProvider with no-ops.
// telemetry.MeterProvider = metric.MeterProvider + Shutdown(context.Context) error.
type noopMeterProvider struct {
	noopmetric.MeterProvider
}

func (noopMeterProvider) Shutdown(context.Context) error {
	return nil
}
```

Add the required imports:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Graylog2/collector-sidecar/superv/config"
	owntelemetry "github.com/Graylog2/collector-sidecar/superv/owntelemetry"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/service/telemetry"
	"go.opentelemetry.io/otel/attribute"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)
```

**Note:** No `PersistentPostRun` shutdown hook is needed for metrics (unlike logs). The collector's service layer calls `Shutdown` on the `MeterProvider` returned by `CreateMeterProvider` during graceful shutdown, which flushes the periodic reader automatically.

- [ ] **Step 2: Verify build**

Run: `cd /home/bernd/graylog/sidecar/builder && go build ./...`
Expected: PASS

- [ ] **Step 3: Tidy builder module**

```bash
cd /home/bernd/graylog/sidecar/builder && go mod tidy
```

- [ ] **Step 4: Commit**

```bash
git add builder/main_customize.go builder/go.mod builder/go.sum
git commit -m "feat: wrap telemetry factory to inject custom CreateMeterProvider

Reads own-metrics.yaml at collector startup and builds an OTLP metric
exporter with allow-list filtering. Falls back to noop if unconfigured
or on error (collector availability first)."
```

---

### Task 11: Pass metrics config from supervisor to collector

The supervisor needs to pass `TelemetryMetricsConfig` to the collector process. The collector reads it via the `GLC_INTERNAL_METRICS_CONFIG` env var (JSON-encoded).

**Files:**
- Modify: `superv/supervisor/supervisor.go:261-269` (`buildCollectorEnv()` method)

- [ ] **Step 1: Add `GLC_INTERNAL_METRICS_CONFIG` env var**

In `superv/supervisor/supervisor.go`, in the `buildCollectorEnv()` method (lines 261-269) where the other `GLC_INTERNAL_*` env vars are set, add the JSON-encoded metrics config after the existing env var assignments (before `maps.Copy`):

Add `"encoding/json"` to the imports if not already present.

```go
// JSON-encode metrics config for the collector's own-metrics setup.
if mcfgJSON, err := json.Marshal(struct {
	Batch           config.BatchConfig `json:"batch"`
	ExportedMetrics []string           `json:"exported_metrics"`
}{
	Batch:           s.cfg.Telemetry.Metrics.Batch,
	ExportedMetrics: s.cfg.Telemetry.Metrics.ExportedMetrics,
}); err == nil {
	env["GLC_INTERNAL_METRICS_CONFIG"] = string(mcfgJSON)
}
```

Note: `buildCollectorEnv()` returns `map[string]string`, so use map assignment syntax (not slice append).

- [ ] **Step 2: Verify build**

Run: `cd /home/bernd/graylog/sidecar/superv && go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add superv/supervisor/supervisor.go
git commit -m "feat: pass metrics config to collector via GLC_INTERNAL_METRICS_CONFIG env var"
```

---

## Chunk 4: Supervisor-Side OpAMP Integration

### Task 12: Add `OnOwnMetrics` callback and `onMessage` dispatch

**Files:**
- Modify: `superv/opamp/callbacks.go:28-40,109-112`

- [ ] **Step 1: Add `OnOwnMetrics` to `Callbacks` struct**

In `superv/opamp/callbacks.go`, after `OnOwnLogs` (line 37), add:

```go
	OnOwnMetrics func(ctx context.Context, settings *protobufs.TelemetryConnectionSettings)
```

- [ ] **Step 2: Add `onMessage` dispatch for `OwnMetricsConnSettings`**

In `superv/opamp/callbacks.go`, after the own-logs block (line 112), add:

```go
	// Handle own metrics connection settings
	if msg.OwnMetricsConnSettings != nil && c.OnOwnMetrics != nil {
		c.OnOwnMetrics(ctx, msg.OwnMetricsConnSettings)
	}
```

- [ ] **Step 3: Run tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./opamp/ -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add superv/opamp/callbacks.go
git commit -m "feat: add OnOwnMetrics callback and onMessage dispatch

Dispatches OwnMetricsConnSettings from OpAMP messages to the
OnOwnMetrics callback handler."
```

---

### Task 13: Add own-metrics fields and handler to supervisor

**Files:**
- Modify: `superv/supervisor/supervisor.go:100-104,523-530,635-650,1214-1219,1260-1344`

- [ ] **Step 1: Add own-metrics fields**

In `superv/supervisor/supervisor.go`, after the own-logs fields (lines 101-103), add:

```go
	// ownMetricsPersistence handles persisting own-metrics settings.
	ownMetricsPersistence *owntelemetry.Persistence
	currentOwnMetrics     *owntelemetry.Settings
```

- [ ] **Step 2: Add `SetOwnMetrics` method**

After `SetOwnLogs` (lines 523-530), add:

```go
// SetOwnMetrics configures the own-metrics persistence for OTLP metric export.
// Must be called before Start.
func (s *Supervisor) SetOwnMetrics(persistence *owntelemetry.Persistence, current *owntelemetry.Settings) {
	s.ownMetricsPersistence = persistence
	if current != nil {
		settingsCopy := *current
		s.currentOwnMetrics = &settingsCopy
	}
}
```

- [ ] **Step 3: Enable `ReportsOwnMetrics` capability**

In `superv/supervisor/supervisor.go` (around line 647), after `ReportsOwnLogs`:

```go
			ReportsOwnLogs:                  s.ownLogsManager != nil,
			ReportsOwnMetrics:               s.ownMetricsPersistence != nil,
```

- [ ] **Step 4: Add `OnOwnMetrics` callback registration**

After the `OnOwnLogs` callback registration (lines 1214-1220), add:

```go
		OnOwnMetrics: func(ctx context.Context, settings *protobufs.TelemetryConnectionSettings) {
			if !s.enqueueWork(ctx, func(wCtx context.Context) {
				s.handleOwnMetrics(wCtx, settings)
			}) {
				s.logger.Warn("Failed to enqueue own_metrics apply")
			}
		},
```

- [ ] **Step 5: Add `handleOwnMetrics` method**

After `handleOwnLogs` (line 1344), add:

```go
// handleOwnMetrics processes own_metrics connection settings from the OpAMP server.
// It persists settings and restarts the collector so it picks up the new
// own-metrics.yaml at startup.
func (s *Supervisor) handleOwnMetrics(ctx context.Context, settings *protobufs.TelemetryConnectionSettings) {
	if s.ownMetricsPersistence == nil {
		s.logger.Warn("Received own_metrics settings but own metrics persistence is not configured")
		return
	}

	// Empty endpoint signals "stop sending own metrics".
	if settings.GetDestinationEndpoint() == "" {
		shouldDisable := s.currentOwnMetrics != nil
		if !shouldDisable {
			_, exists, err := s.ownMetricsPersistence.Load()
			if err != nil {
				s.logger.Error("Failed to load persisted own_metrics settings during disable", zap.Error(err))
				return
			}
			shouldDisable = exists
		}
		if !shouldDisable {
			s.logger.Debug("Own metrics already disabled, skipping apply")
			return
		}
		s.logger.Info("Received own_metrics with empty endpoint, disabling OTLP metric export")
		if err := s.ownMetricsPersistence.Delete(); err != nil {
			s.logger.Error("Failed to delete persisted own_metrics settings, skipping collector restart", zap.Error(err))
			return
		}
		s.currentOwnMetrics = nil
		s.restartCollector(ctx)
		return
	}

	s.logger.Info("Received own_metrics connection settings",
		zap.String("endpoint", settings.GetDestinationEndpoint()),
	)

	converted, err := owntelemetry.ConvertSettings(settings,
		s.authManager.GetSigningCertPath(),
		s.authManager.GetSigningKeyPath(),
	)
	if err != nil {
		s.logger.Error("Failed to convert own_metrics settings", zap.Error(err))
		return
	}

	if s.currentOwnMetrics != nil && s.currentOwnMetrics.Equal(converted) {
		s.logger.Debug("Own metrics settings unchanged, skipping apply")
		return
	}

	if err := s.ownMetricsPersistence.Save(converted); err != nil {
		s.logger.Error("Failed to persist own_metrics settings, skipping collector restart", zap.Error(err))
		return
	}
	settingsCopy := converted
	s.currentOwnMetrics = &settingsCopy

	s.logger.Info("Own metrics OTLP export enabled",
		zap.String("endpoint", converted.Endpoint),
	)
	s.restartCollector(ctx)
}
```

- [ ] **Step 6: Update `restartCollector` log message**

The `restartCollector` method (line 1350) has hardcoded "own_logs" in its doc comment and log messages. Now that it's shared between own-logs and own-metrics, update it to be signal-agnostic:

```go
// restartCollector restarts the collector process as a best-effort follow-up
// to own telemetry changes (own_logs or own_metrics). Failures are logged but
// not returned because telemetry updates are handled asynchronously and have
// no status/error response path back to the OpAMP server.
func (s *Supervisor) restartCollector(ctx context.Context) {
	s.logger.Info("Restarting collector to apply own telemetry changes")
	if err := s.commander.Restart(ctx); err != nil {
		s.logger.Error("Failed to restart collector after own telemetry change", zap.Error(err))
	}
}
```

- [ ] **Step 7: Run tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go build ./...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add superv/supervisor/supervisor.go
git commit -m "feat: add handleOwnMetrics and SetOwnMetrics to supervisor

Handles OpAMP own_metrics connection settings: persists to
own-metrics.yaml and restarts the collector. No supervisor-side
metrics manager needed yet."
```

---

### Task 14: Initialize own-metrics persistence in supervisor main

**Files:**
- Modify: `superv/cmd/supervisor/main.go:129-154`

- [ ] **Step 1: Create metrics persistence and load on startup**

In `superv/cmd/supervisor/main.go`, after the own-logs persistence setup (around line 147), add:

```go
	// Restore persisted own_metrics settings
	ownMetricsPersist := owntelemetry.NewPersistence(cfg.Persistence.Dir, "own-metrics.yaml", certPath, keyPath)
	var restoredOwnMetrics *owntelemetry.Settings
	if settings, exists, loadErr := ownMetricsPersist.Load(); loadErr != nil {
		logger.Warn("Failed to load persisted own_metrics settings", zap.Error(loadErr))
	} else if exists {
		logger.Info("Found persisted own_metrics settings",
			zap.String("endpoint", settings.Endpoint),
		)
		settingsCopy := settings
		restoredOwnMetrics = &settingsCopy
	}
```

After `sup.SetOwnLogs(...)` (line 154), add:

```go
	sup.SetOwnMetrics(ownMetricsPersist, restoredOwnMetrics)
```

- [ ] **Step 2: Verify build**

Run: `cd /home/bernd/graylog/sidecar/superv && go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add superv/cmd/supervisor/main.go
git commit -m "feat: initialize own-metrics persistence in supervisor startup

Loads persisted own-metrics.yaml on startup and passes to supervisor
for OpAMP-driven reconfiguration."
```

---

## Chunk 5: Verification

### Task 15: End-to-end build and test verification

- [ ] **Step 1: Run all superv tests**

Run: `cd /home/bernd/graylog/sidecar/superv && go test ./... 2>&1 | tail -30`
Expected: All PASS

- [ ] **Step 2: Build builder**

Run: `cd /home/bernd/graylog/sidecar/builder && go build ./...`
Expected: PASS

- [ ] **Step 3: Run `make fmt`**

Run: `cd /home/bernd/graylog/sidecar && make fmt`
Expected: PASS

- [ ] **Step 4: Run `go fix`**

Run: `cd /home/bernd/graylog/sidecar/superv && go fix ./...`
Run: `cd /home/bernd/graylog/sidecar/builder && go fix ./...`
Expected: no changes

- [ ] **Step 5: Final review**

Review all changes with `git diff --stat` to ensure no unintended files were modified.
