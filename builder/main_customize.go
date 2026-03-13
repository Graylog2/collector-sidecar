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
	"github.com/Graylog2/collector-sidecar/superv/ownlogs"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/otelcol"
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

	res := ownlogs.BuildResource("collector", params.BuildInfo.Version,
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
	// The OTel Collector's service layer attaches its own telemetry resource
	// (service.name, service.instance.id, etc.) as a zap field named "resource"
	// on component loggers. The otelzap bridge converts that field into an OTLP
	// log record attribute, which is redundant with the top-level OTLP resource
	// we set via BuildResource and confusing because the two carry different
	// values. Strip it before it reaches the bridge.
	params.LoggingOptions = append(params.LoggingOptions,
		zap.WrapCore(func(original zapcore.Core) zapcore.Core {
			return zapcore.NewTee(original, &ownlogs.FieldFilterCore{
				Core:       core,
				DropFields: []string{"resource"},
			})
		}),
	)
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
