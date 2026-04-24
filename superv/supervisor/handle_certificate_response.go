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

	"github.com/Graylog2/collector-sidecar/superv/ownlogs"
	"github.com/Graylog2/collector-sidecar/superv/version"
	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

// handleCertificateResponse handles certificate from enrollment or renewal response.
func (s *Supervisor) handleCertificateResponse(settings *protobufs.OpAMPConnectionSettings) (bool, error) {
	cert := settings.GetCertificate()
	if cert == nil {
		return false, nil
	}

	certPEM := cert.GetCert()
	if len(certPEM) == 0 {
		return false, nil
	}

	s.logger.Info("Received certificate from server")

	// Branch 1: enrollment (HasPendingEnrollment takes precedence — during enrollment
	// both HasPendingEnrollment() and pendingCSR != nil are true)
	if s.authManager.HasPendingEnrollment() {
		s.logger.Info("Completing enrollment with received certificate")
		if err := s.authManager.CompleteEnrollment(certPEM); err != nil {
			return false, fmt.Errorf("failed to complete enrollment: %w", err)
		}

		s.mu.Lock()
		s.pendingCSR = nil
		s.mu.Unlock()

		s.logger.Info("Enrollment completed successfully",
			zap.String("cert_fingerprint", s.authManager.CertFingerprint()),
		)
		return true, nil
	}

	// Branch 2: renewal
	s.mu.RLock()
	hasPendingCSR := s.pendingCSR != nil
	s.mu.RUnlock()

	if hasPendingCSR {
		s.logger.Info("Completing certificate renewal")
		// CompleteRenewal takes auth.Manager mutex internally.
		// Must NOT hold s.mu here to avoid deadlock.
		if err := s.authManager.CompleteRenewal(certPEM); err != nil {
			return false, fmt.Errorf("failed to complete renewal: %w", err)
		}

		s.mu.Lock()
		s.pendingCSR = nil
		s.nextRenewalRetry = time.Time{}
		s.renewalBackoff = 0
		s.mu.Unlock()

		s.logger.Info("Certificate renewal completed successfully",
			zap.String("cert_fingerprint", s.authManager.CertFingerprint()),
		)

		// Best-effort post-renewal actions dispatched to the work queue so they
		// don't block the synchronous OnOpampConnectionSettings callback.
		// The collector restart can wait on process shutdown timeouts, and the
		// own-logs reload must serialize with handleOwnLogs.
		if !s.enqueueWork(context.Background(), func(wCtx context.Context) {
			if s.commander != nil {
				if err := s.commander.Restart(wCtx); err != nil {
					if s.isShutdownCancellation(err) {
						s.logger.Debug("Supervisor shutdown interrupted collector restart after certificate renewal")
					} else {
						s.logger.Error("Failed to restart collector after certificate renewal", zap.Error(err))
					}
				}
			}
			if s.ownLogsManager != nil {
				if err := s.reloadOwnLogsCert(wCtx); err != nil {
					if s.isShutdownCancellation(err) {
						s.logger.Debug("Supervisor shutdown interrupted own-logs certificate reload")
					} else {
						s.logger.Warn("Failed to reload own-logs certificate", zap.Error(err))
					}
				}
			}
		}) {
			if s.isStopping() {
				s.logger.Debug("Skipping post-renewal actions during shutdown")
			} else {
				s.logger.Warn("Failed to enqueue post-renewal actions")
			}
		}

		// Return false: renewal does not require an OpAMP reconnect. The JWT
		// HeaderFunc picks up the new cert thumbprint automatically on the
		// next request. Only enrollment returns true (to switch from the
		// enrollment token to JWT auth).
		return false, nil
	}

	// Branch 3: no pending request
	s.logger.Debug("No pending enrollment or renewal, ignoring certificate")
	return false, nil
}

// reloadOwnLogsCert reloads the own-logs OTLP exporter's client certificate
// after a certificate renewal.
func (s *Supervisor) reloadOwnLogsCert(ctx context.Context) error {
	if s.ownLogsManager == nil || s.currentOwnLogs == nil || s.currentOwnLogs.TLSConfig == nil {
		return nil
	}

	certPath := s.authManager.GetSigningCertPath()
	keyPath := s.authManager.GetSigningKeyPath()

	if err := s.currentOwnLogs.LoadClientCert(certPath, keyPath); err != nil {
		return fmt.Errorf("load client cert: %w", err)
	}

	res := ownlogs.BuildResource(ServiceName, version.Version(), s.instanceUID)
	return s.ownLogsManager.Apply(ctx, *s.currentOwnLogs, res)
}
