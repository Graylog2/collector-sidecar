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
	"time"

	"go.uber.org/zap"
)

// checkCertificateRenewal checks if the certificate needs renewal and initiates
// or retries the renewal process. It returns the duration until the next check
// should occur, allowing the caller to use a resettable timer instead of a
// fixed-interval ticker.
func (s *Supervisor) checkCertificateRenewal() time.Duration {
	s.logger.Debug("Checking certificate renewal")

	fallback := s.authCfg.RenewalInterval

	if !s.authManager.IsEnrolled() {
		return fallback
	}

	if s.authManager.CertificateExpired() {
		s.logger.Error("Certificate expired, renewal pending")
	}

	if s.authManager.HasPendingEnrollment() {
		return fallback // enrollment in progress, not our concern
	}

	s.mu.RLock()
	hasPendingCSR := s.pendingCSR != nil
	nextRetry := s.nextRenewalRetry
	s.mu.RUnlock()

	if !hasPendingCSR {
		fraction := s.authCfg.RenewalFraction
		if fraction == 0 {
			fraction = 0.75 // default if unset
		}
		if s.authManager.CertificateNeedsRenewal(fraction) {
			s.requestCertificateRenewal()
			return fallback
		}
		// Schedule next check for when the cert actually needs renewal,
		// but no later than the fallback interval so we still periodically
		// verify cert state for long-lived certificates.
		renewalTime := s.authManager.CertificateRenewalTime(fraction)
		if renewalTime.IsZero() {
			return fallback
		}
		delay := time.Until(renewalTime)
		if delay <= 0 {
			return fallback
		}
		return min(delay, fallback)
	}

	// Renewal pending — check retry/response timeout
	if time.Now().After(nextRetry) {
		s.requestCertificateRenewal()
		return fallback
	}

	// Wait until the next retry is due.
	return time.Until(nextRetry)
}

// requestCertificateRenewal generates a renewal CSR and sends it via OpAMP.
func (s *Supervisor) requestCertificateRenewal() {
	s.logger.Debug("Requesting certificate renewal")

	csrPEM, err := s.authManager.PrepareRenewal(s.instanceUID)
	if err != nil {
		s.logger.Error("Failed to prepare renewal CSR", zap.Error(err))
		s.advanceRenewalBackoff()
		return
	}

	s.mu.RLock()
	client := s.opampClient
	s.mu.RUnlock()

	if client == nil {
		s.logger.Warn("OpAMP client not available for certificate renewal")
		s.advanceRenewalBackoff()
		return
	}

	s.mu.Lock()
	s.pendingCSR = csrPEM
	s.mu.Unlock()

	if err := client.RequestConnectionSettings(csrPEM); err != nil {
		s.logger.Warn("Failed to send certificate renewal request", zap.Error(err))
		s.advanceRenewalBackoff()
		return
	}

	// Request sent successfully — set response timeout
	s.mu.Lock()
	s.nextRenewalRetry = time.Now().Add(renewalResponseTimeout)
	s.mu.Unlock()

	s.logger.Info("Certificate renewal requested, awaiting response")
}

// advanceRenewalBackoff advances the exponential backoff for renewal retries.
func (s *Supervisor) advanceRenewalBackoff() {
	s.mu.Lock()
	if s.renewalBackoff == 0 {
		s.renewalBackoff = s.renewalBackoffCfg.Initial
	} else {
		s.renewalBackoff = time.Duration(float64(s.renewalBackoff) * s.renewalBackoffCfg.Multiplier)
	}
	if s.renewalBackoff > s.renewalBackoffCfg.Max {
		s.renewalBackoff = s.renewalBackoffCfg.Max
	}
	s.nextRenewalRetry = time.Now().Add(s.renewalBackoff)
	s.mu.Unlock()
}
