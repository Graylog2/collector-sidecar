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

package owntelemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc/credentials"
)

// LogsConfig holds the supervisor-side logs configuration passed to the
// collector process via the GLC_INTERNAL_LOGS_CONFIG env var (JSON-encoded).
type LogsConfig struct {
	Batch config.BatchConfig `json:"batch"`
}

// MetricsConfig holds the supervisor-side metrics configuration passed to the
// collector process via the GLC_INTERNAL_METRICS_CONFIG env var (JSON-encoded).
type MetricsConfig struct {
	Batch           config.BatchConfig `json:"batch"`
	ExportedMetrics []string           `json:"exported_metrics"`
}

// ParseLogsConfigEnv reads and JSON-decodes the GLC_INTERNAL_LOGS_CONFIG
// environment variable. Returns a zero LogsConfig if the var is unset or
// malformed.
func ParseLogsConfigEnv() LogsConfig {
	return parseConfigEnv[LogsConfig]("GLC_INTERNAL_LOGS_CONFIG")
}

// ParseMetricsConfigEnv reads and JSON-decodes the GLC_INTERNAL_METRICS_CONFIG
// environment variable. Returns a zero MetricsConfig if the var is unset or
// malformed.
func ParseMetricsConfigEnv() MetricsConfig {
	return parseConfigEnv[MetricsConfig]("GLC_INTERNAL_METRICS_CONFIG")
}

func parseConfigEnv[T any](envVar string) T {
	var zero T
	raw := os.Getenv(envVar)
	if raw == "" {
		return zero
	}
	var cfg T
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: failed to parse %s: %v\n", envVar, err)
		return zero
	}
	return cfg
}

// NoopMeterProvider implements a no-op meter provider with a Shutdown method.
// It satisfies the collector's telemetry.MeterProvider interface
// (metric.MeterProvider + Shutdown).
type NoopMeterProvider struct{ noop.MeterProvider }

// Shutdown is a no-op.
func (NoopMeterProvider) Shutdown(context.Context) error { return nil }

// NewMeterProviderFromFile loads own-metrics settings from
// persistenceDir/own-metrics.yaml, builds an OTLP metric exporter and
// MeterProvider with allow-list filtering. Returns the provider and any error.
// If the file doesn't exist or exportedMetrics is empty, returns a
// NoopMeterProvider (not nil).
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
		return nil, nil // no settings file → caller should use noop
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

	opts := []sdkmetric.Option{
		sdkmetric.WithReader(reader),
	}

	// "*" means export all metrics (no views needed).
	// Otherwise, build allow-list views to filter.
	if !slices.Contains(exportedMetrics, "*") {
		views := buildAllowListViews(exportedMetrics)
		opts = append(opts, sdkmetric.WithView(views...))
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
