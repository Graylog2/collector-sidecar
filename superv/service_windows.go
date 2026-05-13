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

//go:build windows

package superv

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"go.uber.org/zap"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

// The source name needs to match the Name parameter in util:EventSource in graylog-collector.wxs.
// TODO: Branding
const eventLogSourceName = "graylog-collector"

const (
	serviceStarting uint32 = iota + 1
	serviceStarted
	serviceStartError
	serviceStopping
	serviceStopped
	serviceStopError
	serviceError
	serviceEventLogError
)

// NewSvcHandler returns a Windows service handler for the supervisor.
func NewSvcHandler() svc.Handler {
	return &supervisorService{}
}

type supervisorService struct{}

func (s *supervisorService) Execute(_ []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}

	elog, err := eventlog.Open(eventLogSourceName)
	if err != nil {
		openErr := err
		// Fall back to the generic Application source so we can still record the problem with the app-specific event log.
		elog, err = eventlog.Open("")
		if err != nil {
			return true, 1
		}
		defer elog.Close()
		_ = elog.Error(serviceEventLogError, fmt.Sprintf("Unable to open event log source %q: %v", eventLogSourceName, openErr))
		return true, 1
	}
	defer elog.Close()

	_ = elog.Info(serviceStarting, "Starting Collector service")

	cfg, events, err := buildServiceConfig()
	if err != nil {
		_ = elog.Error(serviceStartError, "Invalid configuration: "+err.Error())
		return true, 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, cfg, events)
	}()

	changes <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}
	_ = elog.Info(serviceStarted, "Collector service started")

	// Adding a few seconds to give the shutdown some more slack.
	shutdownTimeoutMs := (cfg.Shutdown.GracefulTimeout + 5*time.Second).Milliseconds()
	shutdownTimeoutMs = min(max(shutdownTimeoutMs, 1), int64(math.MaxUint32))
	waitHint := uint32(shutdownTimeoutMs)

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				_ = elog.Info(serviceStopping, "Stopping Collector service")
				// Ideally we would also send periodic heartbeats with a monotonically increasing CheckPoint for
				// full spec compliance.
				changes <- svc.Status{State: svc.StopPending, WaitHint: waitHint}
				cancel()
				if err := <-errCh; err != nil {
					_ = elog.Error(serviceStopError, "Shutdown error: "+err.Error())
					return true, 1
				}
				_ = elog.Info(serviceStopped, "Collector service stopped")
				return false, 0
			}
		case err := <-errCh:
			if err != nil {
				_ = elog.Error(serviceError, "Service error: "+err.Error())
				return true, 1
			}
			return false, 0
		}
	}
}

// buildServiceConfig builds the supervisor config without cobra flags.
// Environment variables (GLC_*) and the default config file are the only
// configuration sources when running as a Windows service.
func buildServiceConfig() (config.Config, []func(*zap.Logger), error) {
	var events []func(logger *zap.Logger)

	configFile, err := findConfigFile(config.DefaultConfigPaths())
	if err != nil {
		return config.Config{}, nil, fmt.Errorf("supervisor: %w", err)
	}

	if configFile != "" {
		absPath, absErr := filepath.Abs(configFile)
		if absErr != nil {
			return config.Config{}, nil, absErr
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

	if cfg.Agent.Sidecar.Autodetect && configFile != "" {
		sidecarConfigPath, sidecarErr := findConfigFile([]string{
			filepath.Join(filepath.Dir(configFile), "sidecar.yml"),
		})
		if sidecarErr != nil {
			return config.Config{}, nil, fmt.Errorf("sidecar: %w", sidecarErr)
		}
		if sidecarConfigPath != "" {
			cfg.Agent.Sidecar.Enabled = true
			events = append(events, func(logger *zap.Logger) {
				logger.Debug("Sidecar enabled via auto-detection", zap.String("config", sidecarConfigPath))
			})
		}
	}

	if cfg.Agent.Executable == "" {
		absPath, absErr := filepath.Abs(os.Args[0])
		if absErr != nil {
			return config.Config{}, nil, fmt.Errorf("failed to determine absolute path of supervisor executable: %w", absErr)
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
