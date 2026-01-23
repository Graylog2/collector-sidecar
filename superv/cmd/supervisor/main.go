// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/open-telemetry/opamp-supervisor/internal/config"
	"github.com/open-telemetry/opamp-supervisor/internal/supervisor"
	"github.com/open-telemetry/opamp-supervisor/internal/version"
)

func main() {
	var (
		configPath     string
		showVersion    bool
		bootstrapToken string
	)

	flag.StringVar(&configPath, "config", "", "Path to configuration file")
	flag.BoolVar(&showVersion, "version", false, "Show version and exit")
	flag.StringVar(&bootstrapToken, "bootstrap-token", "", "Enrollment JWT for zero-touch bootstrap")
	flag.Parse()

	if showVersion {
		fmt.Println(version.Version())
		os.Exit(0)
	}

	// Initialize logger
	logger, err := initLogger("info", "json")
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

	// Override with bootstrap token if provided
	if bootstrapToken != "" {
		cfg.Auth.EnrollmentToken = bootstrapToken
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration", zap.Error(err))
	}

	// Create supervisor
	sup, err := supervisor.New(logger, cfg)
	if err != nil {
		logger.Fatal("Failed to create supervisor", zap.Error(err))
	}

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
