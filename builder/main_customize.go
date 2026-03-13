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
	"fmt"
	"os"

	"github.com/Graylog2/collector-sidecar/superv"
	"github.com/Graylog2/collector-sidecar/superv/owntelemetry"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/service/telemetry"
	"go.opentelemetry.io/otel/attribute"
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

	logsCfg := owntelemetry.ParseLogsConfigEnv()
	core, shutdown, err := owntelemetry.NewCoreFromFile(
		persistDir, certPath, keyPath, res, logsCfg.Batch,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: own-logs setup failed, continuing without OTLP log export: %v\n", err)
		return
	}
	if core != nil {
		ownLogsShutdown = shutdown
		// The OTel Collector's service layer attaches its own telemetry resource
		// (service.name, service.instance.id, etc.) as a zap field named "resource"
		// on component loggers. The otelzap bridge converts that field into an OTLP
		// log record attribute, which is redundant with the top-level OTLP resource
		// we set via BuildResource and confusing because the two carry different
		// values. Strip it before it reaches the bridge.
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
	mcfg := owntelemetry.ParseMetricsConfigEnv()
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
				makeCreateMeterProvider(persistDir, certPath, keyPath, mcfg),
			),
		)
		return f, nil
	}
}

// makeCreateMeterProvider returns a CreateMeterProviderFunc that reads
// own-metrics.yaml and builds a MeterProvider with OTLP export.
// If own-metrics.yaml doesn't exist or the allow-list is empty, returns noop.
// Errors are logged and result in noop (collector availability first).
func makeCreateMeterProvider(persistDir, certPath, keyPath string, mcfg owntelemetry.MetricsConfig) telemetry.CreateMeterProviderFunc {
	return func(
		ctx context.Context,
		set telemetry.MeterSettings,
		cfg component.Config,
	) (telemetry.MeterProvider, error) {
		if len(mcfg.ExportedMetrics) == 0 {
			return owntelemetry.NoopMeterProvider{}, nil
		}

		// Build resource from set.Resource (includes user-configured attrs)
		// and append the Graylog-specific collector.receiver.type attribute.
		attrs := pcommonAttrsToSDKAttrs(set.Resource)
		attrs = append(attrs, attribute.String("collector.receiver.type", "collector_metric"))
		res := sdkresource.NewWithAttributes("", attrs...)

		provider, err := owntelemetry.NewMeterProviderFromFile(
			persistDir, certPath, keyPath,
			res, mcfg.Batch, mcfg.ExportedMetrics,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: own-metrics setup failed, continuing without OTLP metric export: %v\n", err)
			return owntelemetry.NoopMeterProvider{}, nil
		}
		if provider == nil {
			return owntelemetry.NoopMeterProvider{}, nil
		}
		return provider, nil
	}
}

// pcommonAttrsToSDKAttrs converts pcommon.Resource attributes to OTel SDK
// key-value pairs. This conversion lives here because pcommon is a collector
// dependency that the superv module does not import.
func pcommonAttrsToSDKAttrs(res *pcommon.Resource) []attribute.KeyValue {
	var result []attribute.KeyValue
	if res != nil {
		res.Attributes().Range(func(k string, v pcommon.Value) bool {
			result = append(result, attribute.String(k, v.AsString()))
			return true
		})
	}
	return result
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
