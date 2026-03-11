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

package superv

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/ownlogs"
	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"github.com/Graylog2/collector-sidecar/superv/supervisor"
	"github.com/Graylog2/collector-sidecar/superv/version"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "supervisor",
		Short: "Start the supervisor",
		Long:  "Start the Collector supervisor process",
		RunE:  runSupervisor,
	}

	cmd.Flags().StringP("config", "c", "", "Path to a supervisor configuration file")
	cmd.Flags().String("endpoint", "", "OpAMP server endpoint")
	cmd.Flags().String("enroll-endpoint", "", "Enrollment endpoint")
	cmd.Flags().String("enroll-token", "", "Enrollment token")
	cmd.Flags().String("data-dir", "", "Data directory")
	cmd.Flags().Bool("insecure", false, "Start in insecure mode (no TLS verification, etc.)")
	cmd.Flags().Bool("debug", false, "Enable debug logging")
	cmd.Flags().Bool("dev", false, "Enable development profile")
	_ = cmd.Flags().MarkHidden("dev") // Developer-only setting

	return cmd
}

func findConfigFile(paths []string) (string, error) {
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return "", fmt.Errorf("config exists but is not readable: %w", err)
			}
		} else {
			return path, nil
		}
	}
	return "", nil
}

func buildConfig(cmd *cobra.Command) (config.Config, []func(logger *zap.Logger), error) {
	var events []func(logger *zap.Logger)

	var configFile string
	if cmd.Flag("config").Changed {
		configFile, _ = cmd.Flags().GetString("config")

		events = append(events, func(logger *zap.Logger) {
			logger.Debug("Using config file path from command line flag", zap.String("config", configFile))
		})
	} else {
		file, err := findConfigFile(append(config.DefaultConfigPaths(), configFile))
		if err != nil {
			return config.Config{}, nil, fmt.Errorf("supervisor: %w", err)
		}
		configFile = file

	}

	if configFile != "" {
		absPath, err := filepath.Abs(configFile)
		if err != nil {
			return config.Config{}, nil, err
		}
		configFile = absPath

		events = append(events, func(logger *zap.Logger) {
			logger.Debug("Using config file", zap.String("config", configFile))
		})
	} else {
		events = append(events, func(logger *zap.Logger) {
			logger.Debug("Running without config file")
		})
	}

	cfg, err := config.Load(configFile)
	if err != nil {
		return config.Config{}, nil, err
	}

	if cmd.Flag("endpoint").Changed {
		endpoint, _ := cmd.Flags().GetString("endpoint")
		cfg.Server.Endpoint = endpoint

		events = append(events, func(logger *zap.Logger) {
			logger.Debug("Using server endpoint from command line flag", zap.String("endpoint", endpoint))
		})
	}
	if cmd.Flag("enroll-endpoint").Changed {
		enrollEndpoint, _ := cmd.Flags().GetString("enroll-endpoint")
		cfg.Server.Auth.EnrollmentEndpoint = enrollEndpoint

		events = append(events, func(logger *zap.Logger) {
			logger.Debug("Using enrollment endpoint from command line flag", zap.String("endpoint", enrollEndpoint))
		})
	}
	if cmd.Flag("enroll-token").Changed {
		enrollToken, _ := cmd.Flags().GetString("enroll-token")
		cfg.Server.Auth.EnrollmentToken = enrollToken

		events = append(events, func(logger *zap.Logger) {
			logger.Debug("Using enrollment token from command line flag")
		})
	}
	if cmd.Flag("data-dir").Changed {
		dataDir, _ := cmd.Flags().GetString("data-dir")
		cfg.Persistence.Dir = dataDir
		events = append(events, func(logger *zap.Logger) {
			logger.Debug("Using data directory from command line flag", zap.String("data-dir", dataDir))
		})
	}

	if isInsecure, _ := cmd.Flags().GetBool("insecure"); isInsecure {
		cfg.SetInsecure()
		events = append(events, func(logger *zap.Logger) {
			logger.Warn("Supervisor runs in insecure mode!")
		})
	}

	if cfg.Agent.Sidecar.Autodetect {
		// We take an existing Sidecar config in the same directory as the supervisor config as an indicator to start the
		// Sidecar extension (when auto-detection is enabled)
		sidecarConfigPath, err := findConfigFile([]string{
			"/etc/graylog/sidecar/sidecar.yml", // This was the default Sidecar path
			filepath.Join(filepath.Dir(configFile), "sidecar.yml"),
		})
		if err != nil {
			return config.Config{}, nil, fmt.Errorf("sidecar: %w", err)
		}
		if sidecarConfigPath != "" {
			cfg.Agent.Sidecar.Enabled = true

			events = append(events, func(logger *zap.Logger) {
				logger.Debug("Sidecar enabled via auto-detection", zap.String("config", sidecarConfigPath))
			})
		}
	}

	if isDev, _ := cmd.Flags().GetBool("dev"); isDev {
		absPath, err := filepath.Abs("./data")
		if err != nil {
			return config.Config{}, nil, err
		}
		cfg.Persistence.Dir = filepath.Join(absPath, "supervisor")
		cfg.Keys.Dir = filepath.Join(absPath, "keys")
		cfg.Packages.StorageDir = filepath.Join(absPath, "packages")
		cfg.Logging.Format = "text"
		cfg.Logging.Color = true

		events = append(events, func(logger *zap.Logger) {
			logger.Debug("DEV mode activated", zap.String("data-dir", cfg.Persistence.Dir),
				zap.String("logging-format", cfg.Logging.Format), zap.String("logging-color", "true"))
		})
	}
	if isDebug, _ := cmd.Flags().GetBool("debug"); isDebug {
		cfg.Debug = true
		cfg.Logging.Level = "debug"

		events = append(events, func(logger *zap.Logger) {
			logger.Debug("DEBUG mode activated", zap.String("logging-level", cfg.Logging.Level))
		})
	}

	// We usually expect the supervisor to be run as a subcommand of the collector.
	if cfg.Agent.Executable == "" {
		absPath, err := filepath.Abs(os.Args[0])
		if err != nil {
			return config.Config{}, nil, fmt.Errorf("failed to determine absolute path of supervisor executable: %w", err)
		}
		cfg.Agent.Executable = absPath

		events = append(events, func(logger *zap.Logger) {
			logger.Debug("Using supervisor binary as agent executable", zap.String("bin", cfg.Agent.Executable))
		})
	}

	if err := cfg.Validate(); err != nil {
		return config.Config{}, nil, fmt.Errorf("invalid configuration:\n%s", config.RenderErrors(err))
	}

	return cfg, events, nil
}

func initLogger(loggingCfg config.LoggingConfig, debug bool) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(loggingCfg.Level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	var cfg zap.Config
	if loggingCfg.Format == "json" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
	}
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)
	cfg.DisableStacktrace = !debug

	if loggingCfg.Color {
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	return cfg.Build()
}

func runSupervisor(cmd *cobra.Command, args []string) error {
	cfg, events, err := buildConfig(cmd)
	if err != nil {
		return fmt.Errorf("couldn't load config: %w", err)
	}

	logger, err := initLogger(cfg.Logging, cfg.Debug)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	// Create own logs manager for OTLP export
	ownLogsManager := ownlogs.NewManager(cfg.Telemetry.Logs)

	// Tee stderr core with the swappable OTLP core, preserving
	// all original logger options (development mode, caller, stacktrace threshold).
	logger = logger.WithOptions(zap.WrapCore(func(original zapcore.Core) zapcore.Core {
		return zapcore.NewTee(original, ownLogsManager.Core())
	}))

	for _, event := range events {
		event(logger)
	}

	// Load or create instance UID early so it's available for own_logs restore.
	instanceUID, err := persistence.LoadOrCreateInstanceUID(cfg.Persistence.Dir)
	if err != nil {
		return fmt.Errorf("failed to load instance UID: %w", err)
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
			settingsCopy := settings
			restoredOwnLogs = &settingsCopy
		}
	}

	sv, err := supervisor.New(logger.Named("supervisor"), cfg, instanceUID)
	if err != nil {
		return fmt.Errorf("failed to create supervisor: %w", err)
	}
	sv.SetOwnLogs(ownLogsManager, ownLogsPersist, restoredOwnLogs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)

	err = sv.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start supervisor: %w", err)
	}

	<-sigCtx.Done()
	stop()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()
	if err := sv.Stop(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown timeout: %w", err)
	}

	// Safety net: flush own logs in case supervisor stop was interrupted.
	_ = ownLogsManager.Shutdown(shutdownCtx)

	return nil
}
