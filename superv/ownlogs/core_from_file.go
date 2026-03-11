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
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.uber.org/zap/zapcore"
)

// NewCoreFromFile loads own-logs settings from persistenceDir/own-logs.yaml,
// builds an OTLP log exporter and otelzap core. Returns the core, a shutdown
// function, and any error. If the file doesn't exist, returns (nil, nil, nil).
//
// Callers must treat errors as non-fatal: a failure here must never prevent
// the collector from starting. Log the error and proceed without the OTLP tee.
func NewCoreFromFile(persistenceDir, clientCertPath, clientKeyPath string, res *resource.Resource) (zapcore.Core, func(context.Context), error) {
	filePath := filepath.Join(persistenceDir, ownLogsFileName)
	if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}

	var ps persistedSettings
	if err := persistence.LoadYAMLFile(".", filePath, &ps); err != nil {
		return nil, nil, fmt.Errorf("load %s: %w", ownLogsFileName, err)
	}

	s := Settings{
		Endpoint:                 ps.Endpoint,
		Headers:                  ps.Headers,
		Insecure:                 ps.Insecure,
		CertPEM:                  ps.CertPEM,
		KeyPEM:                   ps.KeyPEM,
		CACertPEM:                ps.CACertPEM,
		TLSMinVersion:            ps.TLSMinVersion,
		TLSMaxVersion:            ps.TLSMaxVersion,
		InsecureSkipVerify:       ps.InsecureSkipVerify,
		IncludeSystemCACertsPool: ps.IncludeSystemCACertsPool,
		TLSCAPemContents:         ps.TLSCAPemContents,
		TLSServerName:            ps.TLSServerName,
		ProxyURL:                 ps.ProxyURL,
		ProxyHeaders:             ps.ProxyHeaders,
		LogLevel:                 ps.LogLevel,
	}

	tlsCfg, err := rebuildTLSConfigFromPEM(s)
	if err != nil {
		return nil, nil, fmt.Errorf("rebuild TLS config: %w", err)
	}
	s.TLSConfig = tlsCfg

	if err := s.LoadClientCert(clientCertPath, clientKeyPath); err != nil {
		return nil, nil, fmt.Errorf("load client certificate: %w", err)
	}

	ctx := context.Background()
	exporter, err := buildExporter(ctx, s)
	if err != nil {
		return nil, nil, fmt.Errorf("build OTLP log exporter: %w", err)
	}

	opts := []sdklog.LoggerProviderOption{
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	}
	if res != nil {
		opts = append(opts, sdklog.WithResource(res))
	}
	provider := sdklog.NewLoggerProvider(opts...)

	core := otelzap.NewCore(instrumentationName,
		otelzap.WithLoggerProvider(provider),
	)

	// Apply log level filter if configured.
	var lvl zapcore.Level
	if s.LogLevel != "" {
		if err := lvl.UnmarshalText([]byte(s.LogLevel)); err != nil {
			lvl = zapcore.InfoLevel
		}
	}
	filteredCore, err := zapcore.NewIncreaseLevelCore(core, lvl)
	if err != nil {
		_ = provider.Shutdown(ctx)
		return nil, nil, fmt.Errorf("apply min level filter: %w", err)
	}

	shutdown := func(ctx context.Context) {
		_ = provider.Shutdown(ctx)
	}

	return filteredCore, shutdown, nil
}
