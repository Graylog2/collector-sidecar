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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/ownlogs"
	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"github.com/Graylog2/collector-sidecar/superv/supervisor"
	"github.com/Graylog2/collector-sidecar/superv/version"
)

func main() {
	var (
		configPath         string
		showVersion        bool
		enrollmentEndpoint string
		enrollmentToken    string
		insecureTls        bool
		debug              bool
		jsonFormat         bool
	)

	flag.StringVar(&configPath, "config", "", "Path to configuration file")
	flag.BoolVar(&showVersion, "version", false, "Show version and exit")
	flag.BoolVar(&debug, "debug", false, "Enable debug mode")
	flag.BoolVar(&jsonFormat, "json", false, "Enable JSON log output")
	flag.StringVar(&enrollmentEndpoint, "enroll-endpoint", "", "Enrollment endpoint")
	flag.StringVar(&enrollmentToken, "enroll-token", "", "Enrollment token")
	flag.BoolVar(&insecureTls, "insecure-tls", false, "Use insecure TLS connection")
	flag.Parse()

	if showVersion {
		fmt.Println(version.Version())
		os.Exit(0)
	}

	// Initialize logger
	level := "info"
	format := "console"
	if debug {
		level = "debug"
	}
	if jsonFormat {
		format = "json"
	}
	logger, err := initLogger(level, format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Load configuration
	var cfg config.Config
	if configPath != "" {
		cfg, err = config.Load(configPath)
		if err != nil {
			logger.Fatal("Failed to load configuration", zap.Error(err))
		}
	} else {
		cfg = config.DefaultConfig()
	}

	cfg.Server.Auth.InsecureTLS = insecureTls

	// Override with enrollment URL if provided
	if enrollmentEndpoint != "" {
		cfg.Server.Auth.EnrollmentEndpoint = enrollmentEndpoint
	}
	if enrollmentToken != "" {
		cfg.Server.Auth.EnrollmentToken = enrollmentToken
	}

	if enrollmentEndpoint != "" {
		endpoint, err := config.DeriveEnrollmentEndpoint(enrollmentEndpoint)
		if err != nil {
			logger.Fatal("Failed to parse enrollment URL", zap.Error(err))
		}
		cfg.Server.Endpoint = endpoint
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration", zap.Error(err))
	}

	// Create own logs manager for OTLP export
	ownLogsManager := ownlogs.NewManager(cfg.Telemetry.Logs)

	// Tee stderr core with the swappable OTLP core, preserving
	// all original logger options (development mode, caller, stacktrace threshold).
	logger = logger.WithOptions(zap.WrapCore(func(original zapcore.Core) zapcore.Core {
		return zapcore.NewTee(original, ownLogsManager.Core())
	}))

	// Load or create instance UID early so it's available for own_logs restore.
	instanceUID, err := persistence.LoadOrCreateInstanceUID(cfg.Persistence.Dir)
	if err != nil {
		logger.Fatal("Failed to load instance UID", zap.Error(err))
	}

	// Restore persisted own_logs settings
	certPath := filepath.Join(cfg.Keys.Dir, persistence.SigningCertFile)
	keyPath := filepath.Join(cfg.Keys.Dir, persistence.SigningKeyFile)
	ownLogsPersist := ownlogs.NewPersistence(cfg.Persistence.Dir, certPath, keyPath)
	var restoredOwnLogs *ownlogs.Settings
	if settings, exists, loadErr := ownLogsPersist.Load(); loadErr != nil {
		logger.Warn("Failed to load persisted own_logs settings", zap.Error(loadErr))
	} else if exists {
		logger.Info("Restoring OTLP log export from persisted settings",
			zap.String("endpoint", settings.Endpoint),
		)
		res := ownlogs.BuildResource(supervisor.ServiceName, version.Version(), instanceUID)
		if applyErr := ownLogsManager.Apply(context.Background(), settings, res); applyErr != nil {
			logger.Warn("Failed to restore OTLP log export", zap.Error(applyErr))
		} else {
			restoredOwnLogs = new(settings)
		}
	}

	// Create supervisor
	sup, err := supervisor.New(logger, cfg, instanceUID)
	if err != nil {
		logger.Fatal("Failed to create supervisor", zap.Error(err))
	}
	sup.SetOwnLogs(ownLogsManager, ownLogsPersist, restoredOwnLogs)

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("Received signal, shutting down", zap.String("signal", sig.String()))
		cancel()
	}()

	// Start supervisor
	if err := sup.Start(ctx); err != nil {
		logger.Fatal("Failed to start supervisor", zap.Error(err))
	}

	// Wait for shutdown signal
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Agent.Shutdown.GracefulTimeout)
	defer shutdownCancel()

	if err := sup.Stop(shutdownCtx); err != nil {
		logger.Error("Error during shutdown", zap.Error(err))
	}

	// Flush own logs after supervisor stop (supervisor.Stop already calls ownLogsManager.Shutdown,
	// but this is a safety net in case the supervisor was never started or stop was interrupted).
	_ = ownLogsManager.Shutdown(shutdownCtx)

	logger.Info("Supervisor stopped")
}

func initLogger(level, format string) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	var cfg zap.Config
	if format == "json" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
	}
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)

	return cfg.Build()
}
