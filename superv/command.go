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
	"github.com/Graylog2/collector-sidecar/superv/supervisor"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "supervisor",
		Short:             "Start the supervisor",
		Long:              "Start the Collector supervisor process",
		ValidArgs:         nil,
		ValidArgsFunction: nil,
		Args:              nil,
		ArgAliases:        nil,
		RunE:              runSupervisor,
	}

	cmd.Flags().StringP("config", "c", "", "Path to a supervisor configuration file")
	cmd.Flags().String("endpoint", "", "OpAMP server endpoint")
	cmd.Flags().String("enroll", "", "Enroll collector with enrollment token")
	cmd.Flags().String("data-dir", "", "Data directory")
	cmd.Flags().Bool("insecure", false, "Start in insecure mode (no TLS verification, etc.)")
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

func buildConfig(cmd *cobra.Command) (config.Config, error) {
	var configFile string
	if cmd.Flag("config").Changed {
		configFile, _ = cmd.Flags().GetString("config")
	} else {
		file, err := findConfigFile(append(config.DefaultConfigPaths(), configFile))
		if err != nil {
			return config.Config{}, fmt.Errorf("supervisor: %w", err)
		}
		configFile = file
	}

	if configFile != "" {
		absPath, err := filepath.Abs(configFile)
		if err != nil {
			return config.Config{}, err
		}
		configFile = absPath
	}

	cfg, err := config.Load(configFile)
	if err != nil {
		return config.Config{}, err
	}

	if cmd.Flag("endpoint").Changed {
		endpoint, _ := cmd.Flags().GetString("endpoint")
		cfg.Server.Endpoint = endpoint
	}
	if cmd.Flag("enroll").Changed {
		enroll, _ := cmd.Flags().GetString("enroll")
		cfg.Auth.EnrollmentURL = enroll
	}
	if cmd.Flag("data-dir").Changed {
		dataDir, _ := cmd.Flags().GetString("data-dir")
		cfg.Persistence.Dir = dataDir
	}

	if isInsecure, _ := cmd.Flags().GetBool("insecure"); isInsecure {
		cfg.SetInsecure()
	}

	if cfg.Agent.Sidecar.Autodetect {
		// We take an existing Sidecar config in the same directory as the supervisor config as an indicator to start the
		// Sidecar extension (when auto-detection is enabled)
		sidecarConfigPath, err := findConfigFile([]string{
			"/etc/graylog/sidecar/sidecar.yml", // This was the default Sidecar path
			filepath.Join(filepath.Dir(configFile), "sidecar.yml"),
		})
		if err != nil {
			return config.Config{}, fmt.Errorf("sidecar: %w", err)
		}
		cfg.Agent.Sidecar.Enabled = sidecarConfigPath != ""
	}

	if isDev, _ := cmd.Flags().GetBool("dev"); isDev {
		absPath, err := filepath.Abs("./data/supervisor")
		if err != nil {
			return config.Config{}, err
		}
		cfg.Persistence.Dir = absPath
	}

	return cfg, nil
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

func runSupervisor(cmd *cobra.Command, args []string) error {
	cfg, err := buildConfig(cmd)
	if err != nil {
		return fmt.Errorf("couldn't load config: %w", err)
	}

	logger, err := initLogger(cfg.Logging.Level, cfg.Logging.Format)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	if cfg.IsInsecure() {
		logger.Warn("Supervisor runs in insecure mode!")
	}

	sv, err := supervisor.New(logger.Named("supervisor"), cfg)
	if err != nil {
		return fmt.Errorf("failed to create supervisor: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)

	err = sv.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start supervisor: %w", err)
	}

	select {
	case <-sigCtx.Done():
		stop()
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()
	if err := sv.Stop(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown timeout: %w", err)
	}

	return nil
}
