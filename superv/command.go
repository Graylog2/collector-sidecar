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
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/DeRuina/timberjack"
	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "supervisor",
		Short: "Start the supervisor",
		Long:  "Start the Collector supervisor process",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Explicitly check for supported platforms to avoid custom supervisor builds on unsupported platforms
			// to connect to the OpAMP server.
			switch runtime.GOOS {
			case "darwin", "linux", "windows":
				return nil
			default:
				return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
			}
		},
		RunE: runSupervisor,
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
			return config.Config{}, nil, fmt.Errorf("resolving config path: %w", err)
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
		return config.Config{}, nil, fmt.Errorf("loading config: %w", err)
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
			return config.Config{}, nil, fmt.Errorf("resolving dev data path: %w", err)
		}
		cfg.Persistence.Dir = filepath.Join(absPath, "supervisor")
		cfg.Agent.StorageDir = filepath.Join(absPath, "storage")
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

	var makeEncoderCfg func() zapcore.EncoderConfig
	if loggingCfg.Format == "json" {
		makeEncoderCfg = zap.NewProductionEncoderConfig
	} else {
		makeEncoderCfg = zap.NewDevelopmentEncoderConfig
	}

	stderrEncCfg := makeEncoderCfg()
	fileEncCfg := makeEncoderCfg()

	// color + JSON make no sense, silently ignore color in that case
	if loggingCfg.Color && loggingCfg.Format != "json" {
		enableConsoleColors()
		stderrEncCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		// no color in file encoder
	}

	var stderrEnc, fileEnc zapcore.Encoder
	if loggingCfg.Format == "json" {
		stderrEnc = zapcore.NewJSONEncoder(stderrEncCfg)
		fileEnc = zapcore.NewJSONEncoder(fileEncCfg)
	} else {
		stderrEnc = zapcore.NewConsoleEncoder(stderrEncCfg)
		fileEnc = zapcore.NewConsoleEncoder(fileEncCfg)
	}

	cores := []zapcore.Core{
		zapcore.NewCore(stderrEnc, zapcore.Lock(os.Stderr), zapLevel),
	}
	if loggingCfg.File != "" {
		if err := os.MkdirAll(filepath.Dir(loggingCfg.File), 0750); err != nil {
			return nil, fmt.Errorf("create log directory: %w", err)
		}
		rot := loggingCfg.FileRotation
		rotator := &timberjack.Logger{
			Filename:   loggingCfg.File,
			MaxSize:    cmp.Or(rot.MaxSize, 25),
			MaxBackups: cmp.Or(rot.MaxBackups, 5),
			MaxAge:     cmp.Or(rot.MaxAge, 30),
			LocalTime:  true,
		}
		cores = append(cores, zapcore.NewCore(fileEnc, zapcore.AddSync(rotator), zapLevel))
	}
	opts := []zap.Option{zap.AddCaller()}
	if debug {
		opts = append(opts, zap.AddStacktrace(zap.ErrorLevel))
	}
	return zap.New(zapcore.NewTee(cores...), opts...), nil
}

func runSupervisor(cmd *cobra.Command, _ []string) error {
	cfg, events, err := buildConfig(cmd)
	if err != nil {
		return fmt.Errorf("couldn't load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return Run(ctx, cfg, events)
}
