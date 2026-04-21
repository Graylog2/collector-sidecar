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
	"fmt"
	"path/filepath"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/ownlogs"
	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"github.com/Graylog2/collector-sidecar/superv/supervisor"
	"github.com/Graylog2/collector-sidecar/superv/version"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Run starts the supervisor and blocks until ctx is cancelled.
// The caller controls the lifecycle: cancelling ctx triggers graceful shutdown.
func Run(ctx context.Context, cfg config.Config, events []func(*zap.Logger)) error {
	logger, err := initLogger(cfg.Logging, cfg.Debug)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	ownLogsManager := ownlogs.NewManager(cfg.Telemetry.Logs)

	logger = logger.WithOptions(zap.WrapCore(func(original zapcore.Core) zapcore.Core {
		return zapcore.NewTee(original, ownLogsManager.Core())
	}))

	for _, event := range events {
		event(logger)
	}

	instanceUID, err := persistence.LoadOrCreateInstanceUID(cfg.Persistence.Dir)
	if err != nil {
		return fmt.Errorf("failed to load instance UID: %w", err)
	}

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

	// Use a separate context for the supervisor's internal operations so that
	// cancelling ctx (the "stop" signal) does not immediately tear down in-flight
	// work — sv.Stop handles graceful draining.
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	if err := sv.Start(runCtx); err != nil {
		return fmt.Errorf("failed to start supervisor: %w", err)
	}

	<-ctx.Done()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()
	if err := sv.Stop(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown timeout: %w", err)
	}

	_ = ownLogsManager.Shutdown(shutdownCtx)

	return nil
}
