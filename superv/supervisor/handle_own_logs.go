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

package supervisor

import (
	"context"

	"github.com/Graylog2/collector-sidecar/superv/ownlogs"
	"github.com/Graylog2/collector-sidecar/superv/version"
	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

// handleOwnLogs processes own_logs connection settings from the OpAMP server.
// It applies settings to the supervisor's own logger, persists them, and restarts
// the collector so it picks up the new own-logs.yaml at startup.
func (s *Supervisor) handleOwnLogs(ctx context.Context, settings *protobufs.TelemetryConnectionSettings) {
	if s.ownLogsManager == nil {
		s.logger.Warn("Received own_logs settings but own logs manager is not configured")
		return
	}

	restartCollector := func(ctx context.Context) {
		s.logger.Info("Restarting collector to apply own_logs changes")
		if err := s.commander.Restart(ctx); err != nil {
			if s.isShutdownCancellation(err) {
				s.logger.Debug("Supervisor shutdown interrupted collector restart for own_logs change")
				return
			}
			s.logger.Error("Failed to restart collector after own_logs change", zap.Error(err))
		}
	}

	// Empty endpoint signals "stop sending own logs".
	if settings.GetDestinationEndpoint() == "" {
		shouldDisable := s.currentOwnLogs != nil
		if !shouldDisable && s.ownLogsPersistence != nil {
			_, exists, err := s.ownLogsPersistence.Load()
			if err != nil {
				s.logger.Error("Failed to load persisted own_logs settings during disable", zap.Error(err))
				return
			}
			shouldDisable = exists
		}
		if !shouldDisable {
			s.logger.Debug("Own logs already disabled, skipping apply")
			return
		}
		s.logger.Info("Received own_logs with empty endpoint, disabling OTLP log export")
		if err := s.ownLogsManager.Disable(ctx); err != nil {
			s.logger.Error("Failed to disable own_logs export", zap.Error(err))
		}
		if s.ownLogsPersistence != nil {
			if err := s.ownLogsPersistence.Delete(); err != nil {
				s.logger.Error("Failed to delete persisted own_logs settings, skipping collector restart", zap.Error(err))
				return
			}
		}
		s.currentOwnLogs = nil
		// TODO: If own_logs and a config change arrive close together, the collector
		// may be restarted twice. This is harmless but wasteful. Consider coalescing
		// restarts in the future.
		restartCollector(ctx)
		return
	}

	s.logger.Info("Received own_logs connection settings",
		zap.String("endpoint", settings.GetDestinationEndpoint()),
	)

	converted, err := ownlogs.ConvertSettings(settings,
		s.authManager.GetSigningCertPath(),
		s.authManager.GetSigningKeyPath(),
	)
	if err != nil {
		s.logger.Error("Failed to convert own_logs settings", zap.Error(err))
		return
	}

	if s.currentOwnLogs != nil && s.currentOwnLogs.Equal(converted) {
		s.logger.Debug("Own logs settings unchanged, skipping apply")
		return
	}

	res := ownlogs.BuildResource(ServiceName, version.Version(), s.instanceUID)

	if err := s.ownLogsManager.Apply(ctx, converted, res); err != nil {
		s.logger.Error("Failed to apply own_logs settings", zap.Error(err))
		return
	}

	// Persist for restart. Only restart the collector if persistence succeeds,
	// otherwise the collector would read stale or missing settings.
	if s.ownLogsPersistence != nil {
		if err := s.ownLogsPersistence.Save(converted); err != nil {
			s.logger.Error("Failed to persist own_logs settings, skipping collector restart", zap.Error(err))
			return
		}
	}
	s.currentOwnLogs = new(converted)

	s.logger.Info("Own logs OTLP export enabled",
		zap.String("endpoint", converted.Endpoint),
	)
	// Restart collector so it picks up the new own-logs.yaml at startup.
	// Unlike remote config updates, this does not change collector.yaml, and
	// the collector treats own-logs startup errors as non-fatal. There is
	// therefore no config rollback path here; a restart failure is just the
	// ordinary stop/start failure mode and is logged below.
	restartCollector(ctx)
}
