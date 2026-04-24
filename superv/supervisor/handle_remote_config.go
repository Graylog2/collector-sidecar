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
	"time"

	"github.com/Graylog2/collector-sidecar/superv/healthmonitor"
	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

// handleRemoteConfig processes the new remote config and restarts the collector if the config has changed.
func (s *Supervisor) handleRemoteConfig(ctx context.Context, cfg *protobufs.AgentRemoteConfig) bool {
	result, err := s.configManager.ApplyRemoteConfig(cfg)
	if err != nil {
		s.logger.Error("Failed to apply remote config", zap.Error(err))
		s.reportRemoteConfigStatus(ctx,
			protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED,
			err.Error(),
			cfg.GetConfigHash(),
		)
		return false
	}

	if !result.Changed {
		s.logger.Debug("Config unchanged, not restarting collector")
		return true
	}

	// Restart collector with new configcontext.Background(), .
	// Restart = Stop + Start, so if Start fails the collector is down.
	// On failure we roll back the config file and re-start the collector
	// with the previous config to avoid leaving it stopped.
	s.logger.Info("Config changed, restarting collector")
	if err := s.commander.Restart(ctx); err != nil {
		if s.isShutdownCancellation(err) {
			s.logger.Debug("Supervisor shutdown interrupted config apply during collector restart")
			return false
		}
		s.logger.Error("Failed to restart collector with new config", zap.Error(err))
		s.rollbackAndRecover(ctx, cfg.GetConfigHash(), err)
		return false
	}

	// Confirm the collector is healthy with the new config.
	// Commander.Start() returns immediately when crash recovery is
	// enabled (MaxRetries >= 1), so we must poll health to confirm
	// the process actually started successfully.
	if err := s.awaitCollectorHealthy(ctx, s.agentCfg.ConfigApplyTimeout); err != nil {
		if s.isShutdownCancellation(err) {
			s.logger.Debug("Supervisor shutdown interrupted config apply while awaiting collector health")
			return false
		}
		s.logger.Error("Collector unhealthy after restart", zap.Error(err))
		s.rollbackAndRecover(ctx, cfg.GetConfigHash(), err)
		return false
	}

	// Report effective config
	s.reportEffectiveConfig(ctx, result.EffectiveConfig)

	// Report success
	s.reportRemoteConfigStatus(ctx,
		protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
		"",
		cfg.GetConfigHash(),
	)

	return true
}

// awaitCollectorHealthy polls the health monitor until the collector reports
// healthy or the timeout expires. This is needed because Commander.Start()
// returns immediately when crash recovery is enabled (MaxRetries >= 1).
//
// If the health HTTP endpoint is never reachable (only connection errors, never
// an HTTP response) and the process is still running at timeout, we treat the
// collector as healthy. This avoids false rollbacks when the health endpoint is
// temporarily unavailable (e.g. slow bind, port conflict).
//
// However, if the endpoint IS reachable but returns non-2xx responses, that is a
// definitive unhealthy signal and the process-alive fallback does not apply.
func (s *Supervisor) awaitCollectorHealthy(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Wait for startup grace period before polling, giving the collector
	// time to bind its health endpoint after a restart. This mirrors the
	// same delay used by the background health monitor (StartPolling).
	if grace := s.agentCfg.Health.StartupGracePeriod; grace > 0 {
		s.logger.Debug("Waiting for startup grace period before health polling",
			zap.Duration("duration", grace))
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastStatus *healthmonitor.HealthStatus
	// Track whether we ever got an HTTP response (even an unhealthy one).
	// The process-alive fallback only applies when the endpoint was never reachable.
	endpointReached := false

	for {
		status, err := s.healthMonitor.CheckHealth(ctx)
		if err == nil && status.Healthy {
			return nil
		}

		if err == nil {
			// Got an HTTP response but non-2xx — definitive unhealthy signal.
			endpointReached = true
			lastStatus = status
		} else if s.commander.IsRunning() {
			// Connection error — endpoint not reachable (yet).
			s.logger.Debug("Health endpoint unreachable but process alive, waiting", zap.Error(err))
		}

		select {
		case <-ctx.Done():
			// If the endpoint was reachable but consistently unhealthy, report failure.
			if endpointReached {
				if lastStatus != nil {
					return fmt.Errorf("collector not healthy after %v: %s", timeout, lastStatus.ErrorMessage)
				}
				return fmt.Errorf("collector not healthy after %v", timeout)
			}
			// Endpoint was never reachable. Fall back to process-alive check.
			if s.commander.IsRunning() {
				s.logger.Warn("Health endpoint never became reachable, but process is running; treating as healthy")
				return nil
			}
			return fmt.Errorf("collector not healthy after %v: health endpoint unreachable and process not running", timeout)
		case <-ticker.C:
		}
	}
}

// rollbackAndRecover rolls back to the previous config and restarts the
// collector. Used when the new config fails (either restart error or health
// check failure). Reports FAILED status to the server.
func (s *Supervisor) rollbackAndRecover(ctx context.Context, configHash []byte, originalErr error) {
	if rbErr := s.configManager.RollbackConfig(); rbErr != nil {
		s.logger.Error("Failed to roll back config, skipping recovery restart", zap.Error(rbErr))
	} else {
		// Restart with rolled-back config. The collector may be stopped
		// (Restart = Stop + Start, failed Start leaves process down) or
		// running with a bad config (health check failed).
		if restartErr := s.commander.Restart(ctx); restartErr != nil {
			if s.isShutdownCancellation(restartErr) {
				s.logger.Debug("Supervisor shutdown interrupted rollback restart")
			} else {
				s.logger.Error("Failed to restart collector after rollback", zap.Error(restartErr))
			}
		}
	}

	s.reportRemoteConfigStatus(ctx,
		protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED,
		fmt.Sprintf("config apply failed: %v", originalErr),
		configHash,
	)
}

// reportRemoteConfigStatus reports config status to the OpAMP server and persists it to disk.
func (s *Supervisor) reportRemoteConfigStatus(_ context.Context, status protobufs.RemoteConfigStatuses, errMsg string, configHash []byte) {
	s.mu.RLock()
	client := s.opampClient
	s.mu.RUnlock()

	if client != nil {
		if err := client.SetRemoteConfigStatus(&protobufs.RemoteConfigStatus{
			Status:               status,
			ErrorMessage:         errMsg,
			LastRemoteConfigHash: configHash,
		}); err != nil {
			s.logger.Warn("Failed to report remote config status", zap.Error(err))
		}
	}

	if err := s.configManager.SaveRemoteConfigStatus(status, errMsg, configHash); err != nil {
		s.logger.Warn("Failed to persist remote config status", zap.Error(err))
	}
}

// reportEffectiveConfig reports the effective config to the OpAMP server.
func (s *Supervisor) reportEffectiveConfig(ctx context.Context, effectiveConfig []byte) {
	s.mu.RLock()
	client := s.opampClient
	s.mu.RUnlock()

	if client != nil {
		if err := client.SetEffectiveConfig(ctx, map[string]*protobufs.AgentConfigFile{
			"collector.yaml": {Body: effectiveConfig},
		}); err != nil {
			s.logger.Warn("Failed to report effective config", zap.Error(err))
		}
	}
}
