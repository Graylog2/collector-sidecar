# otelzap own_logs Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Export supervisor logs to an OTLP endpoint configured dynamically via OpAMP's `ConnectionSettingsOffers.own_logs`.

**Architecture:** A `swappableCore` wrapping `zapcore.Core` is tee'd with the existing stderr core. When the OpAMP server sends `own_logs` settings, an `otelzap.Core` backed by a `BatchProcessor` + OTLP exporter is built and atomically swapped in. Settings are persisted to disk so export survives restarts.

**Tech Stack:** `go.opentelemetry.io/contrib/bridges/otelzap`, `go.opentelemetry.io/otel/sdk/log`, `go.opentelemetry.io/otel/exporters/otlp/otlplog/{otlploghttp,otlploggrpc}`

**Design doc:** `docs/plans/2026-03-03-otelzap-own-logs-design.md`

---

### Task 1: Add OTel dependencies to superv/go.mod

**Files:**
- Modify: `superv/go.mod`

**Step 1: Add dependencies**

```bash
cd superv && go get \
  go.opentelemetry.io/otel/sdk/log@latest \
  go.opentelemetry.io/otel/sdk@latest \
  go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp@latest \
  go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc@latest \
  go.opentelemetry.io/contrib/bridges/otelzap@latest
```

**Step 2: Tidy**

```bash
cd superv && go mod tidy
```

**Step 3: Verify build**

```bash
cd superv && go build ./...
```

Expected: compiles with no errors.

**Step 4: Commit**

```bash
git add superv/go.mod superv/go.sum
git commit -m "Add OTel SDK log and otelzap dependencies"
```

---

### Task 2: Create the swappableCore

**Files:**
- Create: `superv/ownlogs/swappable_core.go`
- Create: `superv/ownlogs/swappable_core_test.go`

**Step 1: Write the failing tests**

Test file `superv/ownlogs/swappable_core_test.go`:

```go
package ownlogs

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestSwappableCore_NilInner_DisablesAllLevels(t *testing.T) {
	sc := newSwappableCore()
	for _, lvl := range []zapcore.Level{
		zapcore.DebugLevel, zapcore.InfoLevel,
		zapcore.WarnLevel, zapcore.ErrorLevel,
	} {
		assert.False(t, sc.Enabled(lvl), "level %s should be disabled when inner is nil", lvl)
	}
}

func TestSwappableCore_WithInner_EnablesMatchingLevels(t *testing.T) {
	inner, _ := observer.New(zapcore.WarnLevel)
	sc := newSwappableCore()
	sc.swap(inner)

	assert.False(t, sc.Enabled(zapcore.InfoLevel))
	assert.True(t, sc.Enabled(zapcore.WarnLevel))
	assert.True(t, sc.Enabled(zapcore.ErrorLevel))
}

func TestSwappableCore_Write_DelegatesToInner(t *testing.T) {
	inner, logs := observer.New(zapcore.InfoLevel)
	sc := newSwappableCore()
	sc.swap(inner)

	logger := zap.New(sc)
	logger.Info("hello", zap.String("key", "val"))

	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	assert.Equal(t, "hello", entry.Message)
	assert.Equal(t, "val", entry.ContextMap()["key"])
}

func TestSwappableCore_Write_NilInner_NoOp(t *testing.T) {
	sc := newSwappableCore()
	logger := zap.New(sc)
	// Must not panic
	logger.Info("ignored")
}

func TestSwappableCore_With_SeesSwap(t *testing.T) {
	inner1, logs1 := observer.New(zapcore.InfoLevel)
	sc := newSwappableCore()
	sc.swap(inner1)

	// Create a derived logger with With() fields
	logger := zap.New(sc).With(zap.String("component", "test"))
	logger.Info("before swap")
	require.Equal(t, 1, logs1.Len())
	assert.Equal(t, "test", logs1.All()[0].ContextMap()["component"])

	// Swap to a new inner core
	inner2, logs2 := observer.New(zapcore.InfoLevel)
	sc.swap(inner2)

	logger.Info("after swap")
	// Old core should not receive the new message
	assert.Equal(t, 1, logs1.Len())
	// New core should receive it with the With() fields
	require.Equal(t, 1, logs2.Len())
	assert.Equal(t, "after swap", logs2.All()[0].Message)
	assert.Equal(t, "test", logs2.All()[0].ContextMap()["component"])
}

func TestSwappableCore_Swap_NilClearsInner(t *testing.T) {
	inner, logs := observer.New(zapcore.InfoLevel)
	sc := newSwappableCore()
	sc.swap(inner)

	logger := zap.New(sc)
	logger.Info("visible")
	require.Equal(t, 1, logs.Len())

	sc.swap(nil)
	logger.Info("invisible")
	assert.Equal(t, 1, logs.Len()) // no new entries
}

func TestSwappableCore_ConcurrentAccess(t *testing.T) {
	sc := newSwappableCore()
	logger := zap.New(sc).With(zap.String("worker", "x"))

	var done atomic.Bool
	go func() {
		for !done.Load() {
			inner, _ := observer.New(zapcore.InfoLevel)
			sc.swap(inner)
		}
	}()

	for range 1000 {
		logger.Info("msg")
	}
	done.Store(true)
}
```

**Step 2: Run tests to verify they fail**

```bash
cd superv && go test ./ownlogs/ -v -count=1
```

Expected: compilation error — package `ownlogs` does not exist.

**Step 3: Implement swappableCore**

File `superv/ownlogs/swappable_core.go`:

```go
package ownlogs

import (
	"sync/atomic"

	"go.uber.org/zap/zapcore"
)

// swappableCore is a zapcore.Core that atomically delegates to a replaceable
// inner core. When the inner is nil, the core acts as a no-op (Enabled returns
// false for all levels, Write is a no-op).
//
// With() returns a derivative that shares the same atomic pointer, so all
// derived loggers see swaps. Stored With-fields are re-applied on each Write
// call to ensure the current inner core receives them.
type swappableCore struct {
	inner  *atomic.Pointer[zapcore.Core]
	fields []zapcore.Field
}

var _ zapcore.Core = (*swappableCore)(nil)

func newSwappableCore() *swappableCore {
	return &swappableCore{
		inner: &atomic.Pointer[zapcore.Core]{},
	}
}

// swap atomically replaces the inner core. Pass nil to disable.
func (s *swappableCore) swap(core zapcore.Core) {
	if core == nil {
		s.inner.Store(nil)
	} else {
		s.inner.Store(&core)
	}
}

func (s *swappableCore) loadInner() zapcore.Core {
	p := s.inner.Load()
	if p == nil {
		return nil
	}
	return *p
}

func (s *swappableCore) Enabled(level zapcore.Level) bool {
	inner := s.loadInner()
	if inner == nil {
		return false
	}
	return inner.Enabled(level)
}

func (s *swappableCore) With(fields []zapcore.Field) zapcore.Core {
	combined := make([]zapcore.Field, 0, len(s.fields)+len(fields))
	combined = append(combined, s.fields...)
	combined = append(combined, fields...)
	return &swappableCore{
		inner:  s.inner,
		fields: combined,
	}
}

func (s *swappableCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	inner := s.loadInner()
	if inner == nil {
		return ce
	}
	if inner.Enabled(ent.Level) {
		return ce.AddCore(ent, s)
	}
	return ce
}

func (s *swappableCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	inner := s.loadInner()
	if inner == nil {
		return nil
	}
	if len(s.fields) > 0 {
		inner = inner.With(s.fields)
	}
	return inner.Write(ent, fields)
}

func (s *swappableCore) Sync() error {
	inner := s.loadInner()
	if inner == nil {
		return nil
	}
	return inner.Sync()
}
```

**Step 4: Run tests to verify they pass**

```bash
cd superv && go test ./ownlogs/ -v -count=1
```

Expected: all tests PASS.

**Step 5: Run race detector**

```bash
cd superv && go test ./ownlogs/ -race -count=1
```

Expected: PASS with no race conditions.

**Step 6: Commit**

```bash
git add superv/ownlogs/
git commit -m "Add swappableCore for hot-swappable otelzap integration"
```

---

### Task 3: Create the OwnLogs manager (exporter + provider lifecycle)

**Files:**
- Create: `superv/ownlogs/manager.go`
- Create: `superv/ownlogs/manager_test.go`

This is the public API: `Manager` wraps `swappableCore` and handles building
OTLP exporters, `LoggerProvider`, and `otelzap.Core` from
`TelemetryConnectionSettings`.

**Step 1: Write the failing tests**

File `superv/ownlogs/manager_test.go`:

```go
package ownlogs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestManager_Core_InitiallyDisabled(t *testing.T) {
	m := NewManager()
	core := m.Core()
	assert.False(t, core.Enabled(zapcore.InfoLevel))
}

func TestManager_Shutdown_WhenNeverApplied(t *testing.T) {
	m := NewManager()
	err := m.Shutdown(context.Background())
	require.NoError(t, err)
}
```

These test the basic lifecycle. Testing with a real OTLP exporter requires
an integration test (Task 6). For unit tests we test the swappable core
behavior (already covered in Task 2) and the manager's lifecycle.

**Step 2: Run tests to verify they fail**

```bash
cd superv && go test ./ownlogs/ -v -count=1 -run TestManager
```

Expected: compilation error — `NewManager` not defined.

**Step 3: Implement the Manager**

File `superv/ownlogs/manager.go`:

```go
package ownlogs

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"sync"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/credentials"
)

const instrumentationName = "github.com/Graylog2/collector-sidecar/superv"

// Settings holds the OTLP endpoint configuration derived from
// TelemetryConnectionSettings.
type Settings struct {
	Endpoint  string
	Headers   map[string]string
	TLSConfig *tls.Config
	Insecure  bool

	// Persisted TLS material for restart recovery. These are the raw PEM bytes
	// from TelemetryConnectionSettings so we can rebuild TLSConfig on restore.
	CertPEM       []byte
	KeyPEM        []byte
	CACertPEM     []byte
	TLSMinVersion string
	TLSMaxVersion string
	InsecureSkipVerify bool
}

// Manager manages the lifecycle of the OTel log exporter and provider.
// It exposes a zapcore.Core that can be tee'd with the stderr core.
type Manager struct {
	sc       *swappableCore
	mu       sync.Mutex // protects provider
	provider *sdklog.LoggerProvider
}

// NewManager creates a Manager with an initially disabled (nop) core.
func NewManager() *Manager {
	return &Manager{
		sc: newSwappableCore(),
	}
}

// Core returns the zapcore.Core to use in zap.NewTee alongside the stderr core.
func (m *Manager) Core() zapcore.Core {
	return m.sc
}

// Apply builds a new OTLP exporter and LoggerProvider from the given settings,
// swaps the otelzap core, and shuts down the previous provider.
func (m *Manager) Apply(ctx context.Context, settings Settings, res *resource.Resource) error {
	exporter, err := m.buildExporter(ctx, settings)
	if err != nil {
		return fmt.Errorf("build OTLP log exporter: %w", err)
	}

	opts := []sdklog.LoggerProviderOption{
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	}
	if res != nil {
		opts = append(opts, sdklog.WithResource(res))
	}
	newProvider := sdklog.NewLoggerProvider(opts...)

	newCore := otelzap.NewCore(instrumentationName,
		otelzap.WithLoggerProvider(newProvider),
	)

	// Swap core and provider atomically under the same lock to prevent
	// Apply/Disable interleaving from leaving a stale core pointing at
	// a shut-down provider.
	m.mu.Lock()
	oldProvider := m.provider
	m.provider = newProvider
	m.sc.swap(newCore)
	m.mu.Unlock()

	// Shut down old provider outside the lock to flush its batch buffer.
	if oldProvider != nil {
		_ = oldProvider.Shutdown(ctx)
	}

	return nil
}

// Disable disables OTLP log export and shuts down the current provider.
func (m *Manager) Disable(ctx context.Context) error {
	m.mu.Lock()
	oldProvider := m.provider
	m.provider = nil
	m.sc.swap(nil)
	m.mu.Unlock()

	if oldProvider != nil {
		return oldProvider.Shutdown(ctx)
	}
	return nil
}

// Shutdown flushes and shuts down the current provider. Call during graceful shutdown.
func (m *Manager) Shutdown(ctx context.Context) error {
	return m.Disable(ctx)
}

func (m *Manager) buildExporter(ctx context.Context, s Settings) (sdklog.Exporter, error) {
	if isGRPC(s.Endpoint) {
		return m.buildGRPCExporter(ctx, s)
	}
	return m.buildHTTPExporter(ctx, s)
}

func (m *Manager) buildHTTPExporter(ctx context.Context, s Settings) (sdklog.Exporter, error) {
	opts := []otlploghttp.Option{
		otlploghttp.WithEndpointURL(s.Endpoint),
	}
	if s.Insecure {
		opts = append(opts, otlploghttp.WithInsecure())
	}
	if s.TLSConfig != nil {
		opts = append(opts, otlploghttp.WithTLSClientConfig(s.TLSConfig))
	}
	if len(s.Headers) > 0 {
		opts = append(opts, otlploghttp.WithHeaders(s.Headers))
	}
	return otlploghttp.New(ctx, opts...)
}

func (m *Manager) buildGRPCExporter(ctx context.Context, s Settings) (sdklog.Exporter, error) {
	opts := []otlploggrpc.Option{
		otlploggrpc.WithEndpointURL(s.Endpoint),
	}
	if s.Insecure {
		opts = append(opts, otlploggrpc.WithInsecure())
	}
	if s.TLSConfig != nil {
		opts = append(opts, otlploggrpc.WithTLSCredentials(credentials.NewTLS(s.TLSConfig)))
	}
	if len(s.Headers) > 0 {
		opts = append(opts, otlploggrpc.WithHeaders(s.Headers))
	}
	return otlploggrpc.New(ctx, opts...)
}

// isGRPC detects whether the endpoint should use gRPC based on the URL.
// URLs with /v1/logs path use HTTP; port 4317 without a path uses gRPC.
func isGRPC(endpoint string) bool {
	if strings.Contains(endpoint, "/v1/logs") {
		return false
	}
	if strings.Contains(endpoint, ":4317") {
		return true
	}
	// Default to HTTP per the OpAMP spec.
	return false
}
```

**Step 4: Verify API compatibility**

```bash
cd superv && go build ./ownlogs/
```

**Step 5: Run tests**

```bash
cd superv && go test ./ownlogs/ -v -count=1
```

Expected: all tests PASS.

**Step 6: Commit**

```bash
git add superv/ownlogs/manager.go superv/ownlogs/manager_test.go
git commit -m "Add ownlogs.Manager for OTLP log export lifecycle"
```

---

### Task 4: Convert TelemetryConnectionSettings to ownlogs.Settings

**Files:**
- Create: `superv/ownlogs/convert.go`
- Create: `superv/ownlogs/convert_test.go`

This converts the OpAMP protobuf `TelemetryConnectionSettings` to the
`ownlogs.Settings` struct used by the Manager.

**Step 1: Write the failing tests**

File `superv/ownlogs/convert_test.go`:

```go
package ownlogs

import (
	"crypto/tls"
	"testing"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestCA and generateTestCert are test helpers that create valid
// self-signed PEM material. Implement using crypto/ecdsa, crypto/x509,
// and encoding/pem. See crypto/x509 CreateCertificate examples.

func TestConvertSettings_Endpoint(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
	}
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com:4318/v1/logs", s.Endpoint)
	assert.False(t, s.Insecure)
}

func TestConvertSettings_Headers(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Headers: &protobufs.Headers{
			Headers: []*protobufs.Header{
				{Key: "Authorization", Value: "Bearer token123"},
				{Key: "X-Custom", Value: "value"},
			},
		},
	}
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	assert.Equal(t, "Bearer token123", s.Headers["Authorization"])
	assert.Equal(t, "value", s.Headers["X-Custom"])
}

func TestConvertSettings_InsecureHTTP(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "http://example.com:4318/v1/logs",
	}
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	assert.True(t, s.Insecure)
}

func TestConvertSettings_TLSCertificate(t *testing.T) {
	// Generate a self-signed CA + client cert for testing.
	// Use crypto/ecdsa + crypto/x509 to create valid PEM material.
	// (Full test cert generation omitted for brevity — use a test helper
	// that generates a self-signed CA and client cert, returning PEM bytes.)
	caCertPEM, caKeyPEM := generateTestCA(t)
	clientCertPEM, clientKeyPEM := generateTestCert(t, caCertPEM, caKeyPEM)

	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Certificate: &protobufs.TLSCertificate{
			Cert:       clientCertPEM,
			PrivateKey: clientKeyPEM,
			CaCert:     caCertPEM,
		},
	}
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	require.NotNil(t, s.TLSConfig)
	assert.NotNil(t, s.TLSConfig.RootCAs)
	assert.Len(t, s.TLSConfig.Certificates, 1)
	// Raw PEM bytes are preserved for persistence
	assert.Equal(t, clientCertPEM, s.CertPEM)
	assert.Equal(t, clientKeyPEM, s.KeyPEM)
	assert.Equal(t, caCertPEM, s.CACertPEM)
}

func TestConvertSettings_TLSConnectionSettings(t *testing.T) {
	caCertPEM, _ := generateTestCA(t)

	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Tls: &protobufs.TLSConnectionSettings{
			CaPemContents:        string(caCertPEM),
			IncludeSystemCaCertsPool: true,
			InsecureSkipVerify:   true,
			MinVersion:           "1.3",
		},
	}
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	require.NotNil(t, s.TLSConfig)
	assert.True(t, s.TLSConfig.InsecureSkipVerify)
	assert.Equal(t, uint16(tls.VersionTLS13), s.TLSConfig.MinVersion)
	assert.NotNil(t, s.TLSConfig.RootCAs)
}

func TestConvertSettings_NilProto(t *testing.T) {
	_, err := ConvertSettings(nil)
	require.Error(t, err)
}

func TestConvertSettings_EmptyEndpoint(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{}
	_, err := ConvertSettings(proto)
	require.Error(t, err)
}
```

**Step 2: Run tests to verify they fail**

```bash
cd superv && go test ./ownlogs/ -v -count=1 -run TestConvert
```

Expected: compilation error — `ConvertSettings` not defined.

**Step 3: Implement ConvertSettings**

File `superv/ownlogs/convert.go`:

```go
package ownlogs

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"

	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
	"github.com/open-telemetry/opamp-go/protobufs"
)

// ConvertSettings converts OpAMP TelemetryConnectionSettings to ownlogs.Settings.
func ConvertSettings(proto *protobufs.TelemetryConnectionSettings) (Settings, error) {
	if proto == nil {
		return Settings{}, fmt.Errorf("nil TelemetryConnectionSettings")
	}
	if proto.DestinationEndpoint == "" {
		return Settings{}, fmt.Errorf("empty destination endpoint")
	}

	s := Settings{
		Endpoint: proto.DestinationEndpoint,
		Insecure: strings.HasPrefix(proto.DestinationEndpoint, "http://"),
	}

	// Convert headers
	if h := proto.GetHeaders(); h != nil && len(h.Headers) > 0 {
		s.Headers = make(map[string]string, len(h.Headers))
		for _, header := range h.Headers {
			s.Headers[header.Key] = header.Value
		}
	}

	// Build TLS config from Certificate and/or TLSConnectionSettings
	tlsCfg, err := buildTLSConfig(proto.GetCertificate(), proto.GetTls())
	if err != nil {
		return Settings{}, fmt.Errorf("build TLS config: %w", err)
	}
	if tlsCfg != nil {
		s.TLSConfig = tlsCfg
	}

	// Preserve raw PEM material for persistence
	if cert := proto.GetCertificate(); cert != nil {
		s.CertPEM = cert.GetCert()
		s.KeyPEM = cert.GetPrivateKey()
		s.CACertPEM = cert.GetCaCert()
	}
	if tlsSettings := proto.GetTls(); tlsSettings != nil {
		s.TLSMinVersion = tlsSettings.GetMinVersion()
		s.TLSMaxVersion = tlsSettings.GetMaxVersion()
		s.InsecureSkipVerify = tlsSettings.GetInsecureSkipVerify()
		// CaPemContents from TLS settings is also stored for persistence
		if len(s.CACertPEM) == 0 && tlsSettings.GetCaPemContents() != "" {
			s.CACertPEM = []byte(tlsSettings.GetCaPemContents())
		}
	}

	return s, nil
}

func buildTLSConfig(cert *protobufs.TLSCertificate, tlsSettings *protobufs.TLSConnectionSettings) (*tls.Config, error) {
	if cert == nil && tlsSettings == nil {
		return nil, nil
	}

	cfg := &tls.Config{}

	// Client certificate from TLSCertificate
	if cert != nil {
		if len(cert.GetCert()) > 0 && len(cert.GetPrivateKey()) > 0 {
			clientCert, err := tls.X509KeyPair(cert.GetCert(), cert.GetPrivateKey())
			if err != nil {
				return nil, fmt.Errorf("parse client certificate: %w", err)
			}
			cfg.Certificates = []tls.Certificate{clientCert}
		}

		// CA cert from TLSCertificate
		if len(cert.GetCaCert()) > 0 {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(cert.GetCaCert()) {
				return nil, fmt.Errorf("failed to parse CA certificate from TLSCertificate")
			}
			cfg.RootCAs = pool
		}
	}

	// TLSConnectionSettings overrides/supplements
	if tlsSettings != nil {
		if tlsSettings.GetInsecureSkipVerify() {
			cfg.InsecureSkipVerify = true
		}

		// CA from TLSConnectionSettings (supplements TLSCertificate CA)
		if caPEM := tlsSettings.GetCaPemContents(); caPEM != "" {
			if cfg.RootCAs == nil {
				if tlsSettings.GetIncludeSystemCaCertsPool() {
					var err error
					cfg.RootCAs, err = x509.SystemCertPool()
					if err != nil {
						cfg.RootCAs = x509.NewCertPool()
					}
				} else {
					cfg.RootCAs = x509.NewCertPool()
				}
			}
			if !cfg.RootCAs.AppendCertsFromPEM([]byte(caPEM)) {
				return nil, fmt.Errorf("failed to parse CA certificate from TLSConnectionSettings")
			}
		} else if tlsSettings.GetIncludeSystemCaCertsPool() && cfg.RootCAs == nil {
			var err error
			cfg.RootCAs, err = x509.SystemCertPool()
			if err != nil {
				return nil, fmt.Errorf("load system CA pool: %w", err)
			}
		}

		// Min/max TLS version
		if v := tlsSettings.GetMinVersion(); v != "" {
			parsed, err := parseTLSVersion(v)
			if err != nil {
				return nil, fmt.Errorf("parse TLS min version: %w", err)
			}
			cfg.MinVersion = parsed
		}
		if v := tlsSettings.GetMaxVersion(); v != "" {
			parsed, err := parseTLSVersion(v)
			if err != nil {
				return nil, fmt.Errorf("parse TLS max version: %w", err)
			}
			cfg.MaxVersion = parsed
		}
	}

	return cfg, nil
}

// parseTLSVersion reuses connection.TLSSettings.ToTLSVersion which accepts
// both "TLSv1.2" and "1.2" forms, trims whitespace, rejects TLS < 1.2,
// and returns an error for unsupported values.
func parseTLSVersion(v string) (uint16, error) {
	return connection.TLSSettings{}.ToTLSVersion(v)
}
```

**Step 4: Run tests**

```bash
cd superv && go test ./ownlogs/ -v -count=1 -run TestConvert
```

Expected: tests should mostly pass. The TLS certificate test will fail because
the test certs aren't valid X509. Use `crypto/x509` test helpers or skip
the cert parse validation in the test. Adjust test fixtures to use real
self-signed test certs or test the error path instead. Fix as needed.

**Step 5: Commit**

```bash
git add superv/ownlogs/convert.go superv/ownlogs/convert_test.go
git commit -m "Add TelemetryConnectionSettings to ownlogs.Settings converter"
```

---

### Task 5: Persist and restore own_logs settings

**Files:**
- Create: `superv/ownlogs/persistence.go`
- Create: `superv/ownlogs/persistence_test.go`

Follow the same pattern as `superv/supervisor/connection/settings.go` using
`persistence.StageYAMLFile` / `persistence.LoadYAMLFile`.

**Step 1: Write the failing tests**

File `superv/ownlogs/persistence_test.go`:

```go
package ownlogs

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistence_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	p := NewPersistence(dir)

	settings := Settings{
		Endpoint: "https://example.com:4318/v1/logs",
		Headers:  map[string]string{"Authorization": "Bearer tok"},
		Insecure: false,
	}

	err := p.Save(settings)
	require.NoError(t, err)

	loaded, exists, err := p.Load()
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, settings.Endpoint, loaded.Endpoint)
	assert.Equal(t, settings.Headers["Authorization"], loaded.Headers["Authorization"])
}

func TestPersistence_SaveAndLoad_WithTLS(t *testing.T) {
	dir := t.TempDir()
	p := NewPersistence(dir)

	caCertPEM, _ := generateTestCA(t)
	clientCertPEM, clientKeyPEM := generateTestCert(t, caCertPEM, nil)

	settings := Settings{
		Endpoint:      "https://example.com:4318/v1/logs",
		CertPEM:       clientCertPEM,
		KeyPEM:        clientKeyPEM,
		CACertPEM:     caCertPEM,
		TLSMinVersion: "1.3",
		InsecureSkipVerify: false,
	}

	err := p.Save(settings)
	require.NoError(t, err)

	loaded, exists, err := p.Load()
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, settings.CertPEM, loaded.CertPEM)
	assert.Equal(t, settings.KeyPEM, loaded.KeyPEM)
	assert.Equal(t, settings.CACertPEM, loaded.CACertPEM)
	assert.Equal(t, "1.3", loaded.TLSMinVersion)
	// TLSConfig should be rebuilt from persisted PEM material
	require.NotNil(t, loaded.TLSConfig)
	assert.NotNil(t, loaded.TLSConfig.RootCAs)
	assert.Len(t, loaded.TLSConfig.Certificates, 1)
}

func TestPersistence_Load_NoFile(t *testing.T) {
	dir := t.TempDir()
	p := NewPersistence(dir)

	_, exists, err := p.Load()
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestPersistence_FileLocation(t *testing.T) {
	dir := t.TempDir()
	p := NewPersistence(dir)

	err := p.Save(Settings{Endpoint: "https://example.com:4318/v1/logs"})
	require.NoError(t, err)

	// File should exist at expected path
	assert.FileExists(t, filepath.Join(dir, ownLogsFileName))
}
```

**Step 2: Run tests to verify they fail**

```bash
cd superv && go test ./ownlogs/ -v -count=1 -run TestPersistence
```

**Step 3: Implement persistence**

File `superv/ownlogs/persistence.go`:

```go
package ownlogs

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
)

// rebuildTLSConfigFromPEM reconstructs a *tls.Config from the raw PEM bytes
// stored in Settings. Returns nil if no TLS material is present.
func rebuildTLSConfigFromPEM(s Settings) (*tls.Config, error) {
	hasCert := len(s.CertPEM) > 0 && len(s.KeyPEM) > 0
	hasCA := len(s.CACertPEM) > 0
	hasTLSSettings := s.TLSMinVersion != "" || s.TLSMaxVersion != "" || s.InsecureSkipVerify

	if !hasCert && !hasCA && !hasTLSSettings {
		return nil, nil
	}

	cfg := &tls.Config{
		InsecureSkipVerify: s.InsecureSkipVerify,
	}

	if s.TLSMinVersion != "" {
		v, err := parseTLSVersion(s.TLSMinVersion)
		if err != nil {
			return nil, fmt.Errorf("parse TLS min version: %w", err)
		}
		cfg.MinVersion = v
	}
	if s.TLSMaxVersion != "" {
		v, err := parseTLSVersion(s.TLSMaxVersion)
		if err != nil {
			return nil, fmt.Errorf("parse TLS max version: %w", err)
		}
		cfg.MaxVersion = v
	}

	if hasCert {
		clientCert, err := tls.X509KeyPair(s.CertPEM, s.KeyPEM)
		if err != nil {
			return nil, fmt.Errorf("parse client certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{clientCert}
	}

	if hasCA {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(s.CACertPEM) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		cfg.RootCAs = pool
	}

	return cfg, nil
}

const ownLogsFileName = "own_logs.yaml"

// persistedSettings is the on-disk representation including TLS material
// so OTLP export survives restarts in mTLS/custom-CA deployments.
type persistedSettings struct {
	Endpoint           string            `koanf:"endpoint"`
	Headers            map[string]string `koanf:"headers,omitempty"`
	Insecure           bool              `koanf:"insecure,omitempty"`
	CertPEM            []byte            `koanf:"cert_pem,omitempty"`
	KeyPEM             []byte            `koanf:"key_pem,omitempty"`
	CACertPEM          []byte            `koanf:"ca_cert_pem,omitempty"`
	TLSMinVersion      string            `koanf:"tls_min_version,omitempty"`
	TLSMaxVersion      string            `koanf:"tls_max_version,omitempty"`
	InsecureSkipVerify bool              `koanf:"insecure_skip_verify,omitempty"`
}

// Persistence handles saving and loading own_logs settings to disk.
type Persistence struct {
	filePath string
}

// NewPersistence creates a Persistence that stores settings in dataDir.
func NewPersistence(dataDir string) *Persistence {
	return &Persistence{
		filePath: filepath.Join(dataDir, ownLogsFileName),
	}
}

// Save persists the settings to disk including TLS material.
func (p *Persistence) Save(s Settings) error {
	ps := persistedSettings{
		Endpoint:           s.Endpoint,
		Headers:            s.Headers,
		Insecure:           s.Insecure,
		CertPEM:            s.CertPEM,
		KeyPEM:             s.KeyPEM,
		CACertPEM:          s.CACertPEM,
		TLSMinVersion:      s.TLSMinVersion,
		TLSMaxVersion:      s.TLSMaxVersion,
		InsecureSkipVerify: s.InsecureSkipVerify,
	}
	return persistence.WriteYAMLFile(".", p.filePath, &ps)
}

// Load reads persisted settings from disk and rebuilds the TLS config.
// Returns (settings, exists, error).
func (p *Persistence) Load() (Settings, bool, error) {
	if _, err := os.Stat(p.filePath); errors.Is(err, os.ErrNotExist) {
		return Settings{}, false, nil
	}

	var ps persistedSettings
	if err := persistence.LoadYAMLFile(".", p.filePath, &ps); err != nil {
		return Settings{}, true, err
	}

	s := Settings{
		Endpoint:           ps.Endpoint,
		Headers:            ps.Headers,
		Insecure:           ps.Insecure,
		CertPEM:            ps.CertPEM,
		KeyPEM:             ps.KeyPEM,
		CACertPEM:          ps.CACertPEM,
		TLSMinVersion:      ps.TLSMinVersion,
		TLSMaxVersion:      ps.TLSMaxVersion,
		InsecureSkipVerify: ps.InsecureSkipVerify,
	}

	// Rebuild TLSConfig from persisted PEM material
	tlsCfg, err := rebuildTLSConfigFromPEM(s)
	if err != nil {
		return Settings{}, true, fmt.Errorf("rebuild TLS config: %w", err)
	}
	s.TLSConfig = tlsCfg

	return s, true, nil
}
```

**Step 4: Run tests**

```bash
cd superv && go test ./ownlogs/ -v -count=1 -run TestPersistence
```

Expected: PASS.

**Step 5: Commit**

```bash
git add superv/ownlogs/persistence.go superv/ownlogs/persistence_test.go
git commit -m "Add own_logs settings persistence"
```

---

### Task 6: Wire OpAMP callback — receive own_logs

**Files:**
- Modify: `superv/opamp/callbacks.go` — add `OnOwnLogs` field and wire it in `onMessage`
- Modify: `superv/opamp/client.go:580` — no change needed, `ReportsOwnLogs` already exists
- Modify: `superv/supervisor/supervisor.go` — enable `ReportsOwnLogs` capability, add callback, store `Manager`

**Step 1: Add `OnOwnLogs` to opamp.Callbacks**

In `superv/opamp/callbacks.go`, add the field to the `Callbacks` struct (after line 38):

```go
OnOwnLogs func(ctx context.Context, settings *protobufs.TelemetryConnectionSettings)
```

Wire it in `onMessage` (after the custom message handling at line 106):

```go
// Handle own logs connection settings
if msg.OwnLogsConnSettings != nil && c.OnOwnLogs != nil {
	c.OnOwnLogs(ctx, msg.OwnLogsConnSettings)
}
```

**Step 2: Enable ReportsOwnLogs capability**

In `superv/supervisor/supervisor.go` around line 580, add to the capabilities:

```go
ReportsOwnLogs: true,
```

**Step 3: Add Manager to Supervisor and wire callback**

In `superv/supervisor/supervisor.go`:

- Add `ownLogsManager *ownlogs.Manager` field to `Supervisor` struct
- In the constructor (`New` function), accept the manager or create it
- In `createOpAMPCallbacks`, add the `OnOwnLogs` callback:

```go
OnOwnLogs: func(ctx context.Context, settings *protobufs.TelemetryConnectionSettings) {
	s.logger.Info("Received own_logs connection settings",
		zap.String("endpoint", settings.GetDestinationEndpoint()),
	)

	converted, err := ownlogs.ConvertSettings(settings)
	if err != nil {
		s.logger.Error("Failed to convert own_logs settings", zap.Error(err))
		return
	}

	res := ownlogs.BuildResource("graylog-supervisor", version.Version(), s.instanceUID.String())

	if err := s.ownLogsManager.Apply(ctx, converted, res); err != nil {
		s.logger.Error("Failed to apply own_logs settings", zap.Error(err))
		return
	}

	// Persist for restart
	if err := s.ownLogsPersistence.Save(converted); err != nil {
		s.logger.Error("Failed to persist own_logs settings", zap.Error(err))
	}

	s.logger.Info("Own logs OTLP export enabled",
		zap.String("endpoint", converted.Endpoint),
	)
},
```

- Add a `BuildResource(serviceName, serviceVersion, instanceID string) *resource.Resource`
  function in the `ownlogs` package that builds a Resource with
  `service.name`, `service.version`, and `service.instance.id` (if non-empty).
  Use this from both the supervisor callback and the startup restore path.

- In `Stop()`, call `s.ownLogsManager.Shutdown(ctx)` before the final log
  statements (so the shutdown log itself still goes to stderr).

**Step 4: Verify build**

```bash
cd superv && go build ./...
```

**Step 5: Run all tests**

```bash
cd superv && go test ./... -count=1
```

Expected: PASS (existing tests should not break).

**Step 6: Commit**

```bash
git add superv/opamp/callbacks.go superv/supervisor/supervisor.go
git commit -m "Wire OpAMP own_logs callback to ownlogs.Manager"
```

---

### Task 7: Wire startup — tee cores and restore persisted settings

**Files:**
- Modify: `superv/cmd/supervisor/main.go` — create Manager, tee cores, restore

**Step 1: Update main.go**

In `initLogger` or after it in `main()`:

```go
// Create own logs manager for OTLP export
ownLogsManager := ownlogs.NewManager()

// Tee stderr core with the swappable OTLP core
stderrLogger := logger.Core()
teedCore := zapcore.NewTee(stderrLogger, ownLogsManager.Core())
logger = zap.New(teedCore, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

// Restore persisted own_logs settings
ownLogsPersist := ownlogs.NewPersistence(cfg.Persistence.Dir)
if settings, exists, err := ownLogsPersist.Load(); err != nil {
	logger.Warn("Failed to load persisted own_logs settings", zap.Error(err))
} else if exists {
	logger.Info("Restoring OTLP log export from persisted settings",
		zap.String("endpoint", settings.Endpoint),
	)
	// Build a basic resource with service.name and service.version.
	// service.instance.id is not yet available (assigned after OpAMP connects),
	// so we use what we have. The resource will be fully populated on the next
	// Apply() triggered by the OpAMP own_logs callback.
	res := ownlogs.BuildResource("graylog-supervisor", version.Version(), "")
	if err := ownLogsManager.Apply(context.Background(), settings, res); err != nil {
		logger.Warn("Failed to restore OTLP log export", zap.Error(err))
	}
}
```

Pass `ownLogsManager` and `ownLogsPersist` to `supervisor.New` (extend its
signature or use an options pattern).

Add `defer ownLogsManager.Shutdown(context.Background())` after the shutdown
sequence (after `sup.Stop`), so the batch buffer is flushed.

**Step 2: Verify build**

```bash
cd superv && go build ./cmd/supervisor/
```

**Step 3: Run all tests**

```bash
cd superv && go test ./... -count=1
```

**Step 4: Commit**

```bash
git add superv/cmd/supervisor/main.go superv/supervisor/supervisor.go
git commit -m "Wire ownlogs.Manager into supervisor startup and shutdown"
```

---

### Task 8: Format and tidy

**Step 1: Format**

```bash
make fmt
```

**Step 2: Fix**

```bash
cd superv && go fix ./...
```

**Step 3: Tidy**

```bash
cd superv && go mod tidy
```

**Step 4: Run full test suite**

```bash
cd superv && go test ./... -race -count=1
```

Expected: all PASS, no races.

**Step 5: Commit**

```bash
git add -A
git commit -m "Format, fix, and tidy after otelzap integration"
```

---

### Task 9: Manual integration test

**Step 1: Start a local OTLP receiver**

Use the debug exporter or a local collector:

```bash
# In a separate terminal, run a collector with debug exporter
# or use a tool like otel-tui
```

**Step 2: Run the supervisor with a test server**

Configure the OpAMP test server to send `ConnectionSettingsOffers` with
`own_logs` pointing to the local OTLP receiver. Verify:

- [ ] Supervisor logs appear at the OTLP receiver
- [ ] Log records have correct severity, message, and attributes
- [ ] Resource attributes include `service.name`, `service.version`, `service.instance.id`
- [ ] Stderr output continues working alongside OTLP export
- [ ] Sending new `own_logs` settings hot-swaps the exporter
- [ ] Stopping the supervisor flushes remaining logs
- [ ] Restarting the supervisor restores OTLP export from persisted settings
