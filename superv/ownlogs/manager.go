// Copyright (C)  2026 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.
//
// SPDX-License-Identifier: SSPL-1.0

package ownlogs

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
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
	CertPEM                  []byte
	KeyPEM                   []byte
	CACertPEM                []byte
	TLSMinVersion            string
	TLSMaxVersion            string
	InsecureSkipVerify       bool
	IncludeSystemCACertsPool bool
	TLSCAPemContents         string // from TLSConnectionSettings.CaPemContents (separate from CACertPEM which comes from TLSCertificate.CaCert)
	TLSServerName            string // override from ?tls_server_name query param on DestinationEndpoint

	ProxyURL     string
	ProxyHeaders map[string]string
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
	endpoint := s.Endpoint
	// WithEndpointURL uses the path from the URL as-is and does not append
	// "/v1/logs". Ensure the OTLP log path is present.
	if !strings.HasSuffix(endpoint, "/v1/logs") {
		endpoint = strings.TrimRight(endpoint, "/") + "/v1/logs"
	}
	opts := []otlploghttp.Option{
		otlploghttp.WithEndpointURL(endpoint),
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
	// Only the proxy URL is applied here. ProxyHeaders (CONNECT headers) cannot be
	// wired without a custom http.Transport; gRPC also has no proxy support.
	if s.ProxyURL != "" {
		proxyURL, err := url.Parse(s.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("parse proxy URL: %w", err)
		}
		opts = append(opts, otlploghttp.WithProxy(http.ProxyURL(proxyURL)))
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
