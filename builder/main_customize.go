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

// Add OpenTelemetry Collector customizations to this file.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Graylog2/collector-sidecar/superv"
	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/owntelemetry"
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

var ownLogsShutdown func(context.Context)

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

// makeCreateMeterProvider returns a CreateMeterProviderFunc that reads
// own-metrics config and builds a MeterProvider with OTLP export.
// If the config is missing or the allow-list is empty, returns noop.
// Errors are logged and result in noop (collector availability first).
func makeCreateMeterProvider(persistDir, certPath, keyPath string) telemetry.CreateMeterProviderFunc {
	// Read config from env vars set by the supervisor.
	metricsCfgJSON := os.Getenv("GLC_INTERNAL_METRICS_CONFIG")
	var batchCfg config.BatchConfig
	var exportedMetrics []string
	if metricsCfgJSON != "" {
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

func customizeCommand(params *otelcol.CollectorSettings, cmd *cobra.Command) {
	cmd.AddCommand(superv.GetCommand())
	if ownLogsShutdown != nil {
		// Best-effort flush: PersistentPostRun only fires when RunE succeeds.
		// Cobra skips all post-run hooks on error (command.go:1009), so on
		// error exits the batch processor's periodic export (~1s) is the only
		// flush mechanism. This is accepted — see the "Shutdown — Best-Effort
		// Flush" section in the design spec.
		existing := cmd.PersistentPostRun
		cmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
			if existing != nil {
				existing(cmd, args)
			}
			ownLogsShutdown(cmd.Context())
		}
	}
}
