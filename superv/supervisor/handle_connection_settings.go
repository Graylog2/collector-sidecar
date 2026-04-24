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
	"fmt"

	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

// pendingReconnect holds state between prepareConnectionSettings (phase 1)
// and applyConnectionSettings (phase 2).
type pendingReconnect struct {
	newSettings connection.Settings
	oldSettings connection.Settings
	stagedFile  persistence.StagedFile
}

// prepareConnectionSettings validates and stages new connection settings.
// Runs synchronously in the OnOpampConnectionSettings callback so the return
// value drives opamp-go's ConnectionSettingsStatus reporting.
func (s *Supervisor) prepareConnectionSettings(settings *protobufs.OpAMPConnectionSettings) (*pendingReconnect, error) {
	if settings == nil {
		return nil, nil
	}

	newlyEnrolled, err := s.handleCertificateResponse(settings)
	if err != nil {
		return nil, fmt.Errorf("enrollment certificate handling failed: %w", err)
	}

	newSettings, changed := s.connectionSettingsManager.SettingsChanged(settings)
	if !changed && !newlyEnrolled {
		return nil, nil
	}

	stagedFile, err := s.connectionSettingsManager.StageNext(newSettings)
	if err != nil {
		return nil, fmt.Errorf("failed to persist new settings: %w", err)
	}

	oldSettings := s.connectionSettingsManager.GetCurrent()
	return &pendingReconnect{
		newSettings: newSettings,
		oldSettings: oldSettings,
		stagedFile:  stagedFile,
	}, nil
}

// applyConnectionSettings reconnects the OpAMP client with new settings.
// Runs on the serialized worker goroutine. On failure, rolls back to old settings.
func (s *Supervisor) applyConnectionSettings(ctx context.Context, pending *pendingReconnect) {
	if err := s.reconnectClient(ctx, pending.newSettings); err != nil {
		if s.isShutdownCancellation(err) {
			s.logger.Debug("Supervisor shutdown interrupted connection settings apply")
			if cleanupErr := pending.stagedFile.Cleanup(); cleanupErr != nil {
				s.logger.Error("Failed to clean up staged settings file", zap.Error(cleanupErr))
			}
			return
		}
		s.logger.Error("Failed to connect with new settings, rolling back", zap.Error(err))
		if cleanupErr := pending.stagedFile.Cleanup(); cleanupErr != nil {
			s.logger.Error("Failed to clean up staged settings file", zap.Error(cleanupErr))
		}
		if rollbackErr := s.reconnectClient(ctx, pending.oldSettings); rollbackErr != nil {
			if s.isShutdownCancellation(rollbackErr) {
				s.logger.Debug("Supervisor shutdown interrupted connection settings rollback")
				return
			}
			s.logger.Error("Rollback also failed", zap.Error(rollbackErr))
		}
		return
	}

	if err := pending.stagedFile.Commit(); err != nil {
		s.logger.Error("Failed to commit staged settings file", zap.Error(err))

		// Reconnect to old settings since persisting the new settings failed.
		if reconnectErr := s.reconnectClient(ctx, pending.oldSettings); reconnectErr != nil {
			if s.isShutdownCancellation(reconnectErr) {
				s.logger.Debug("Supervisor shutdown interrupted connection settings rollback after persistence error")
				return
			}
			s.logger.Error("Rollback after persistence error failed", zap.Error(reconnectErr))

			// If rollback fails, try new settings again for runtime consistency.
			if recoverErr := s.reconnectClient(ctx, pending.newSettings); recoverErr != nil {
				if s.isShutdownCancellation(recoverErr) {
					s.logger.Debug("Supervisor shutdown interrupted connection settings recovery")
					return
				}
				s.logger.Error("Recovery with new settings also failed", zap.Error(recoverErr))
			} else {
				s.connectionSettingsManager.SetCurrent(pending.newSettings)
				if persistErr := s.connectionSettingsManager.Persist(pending.newSettings); persistErr != nil {
					s.logger.Error("Recovery succeeded but persisting new settings still failed", zap.Error(persistErr))
				}
			}
		}
		return
	}

	s.logger.Info("Connection settings applied successfully")
}
