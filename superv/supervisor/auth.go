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
	"net/http"

	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
	"go.uber.org/zap"
)

// initAuth initializes authentication by loading credentials or preparing enrollment.
// If enrollment is needed, this prepares the CSR which will be sent via OpAMP.
func (s *Supervisor) initAuth(ctx context.Context) error {
	if s.authManager.IsEnrolled() {
		s.logger.Debug("Loading existing credentials")
		if err := s.authManager.LoadCredentials(); err != nil {
			return fmt.Errorf("failed to load credentials: %w", err)
		}

		s.logger.Info("Credentials loaded",
			zap.String("cert_fingerprint", s.authManager.CertFingerprint()),
		)
		return nil
	}

	// Need to enroll - prepare the CSR
	if s.authCfg.EnrollmentEndpoint == "" {
		return fmt.Errorf("not enrolled and no enrollment URL configured")
	}
	if s.authCfg.EnrollmentToken == "" {
		return fmt.Errorf("not enrolled and no enrollment token configured")
	}

	s.logger.Info("Preparing enrollment")
	result, err := s.authManager.PrepareEnrollment(ctx, s.authCfg.EnrollmentEndpoint, s.authCfg.EnrollmentToken, s.instanceUID)
	if err != nil {
		return fmt.Errorf("enrollment preparation failed: %w", err)
	}

	// Store CSR to send via OpAMP after connection is established
	s.pendingCSR = result.CSRPEM

	s.logger.Info("Enrollment prepared, CSR ready for submission via OpAMP")

	return nil
}

func toHTTPHeaders(headers map[string]string) http.Header {
	h := make(http.Header)
	for k, v := range headers {
		h.Set(k, v)
	}
	return h
}

// buildAuthHeaders creates HTTP headers and an optional HeaderFunc for authentication.
// During enrollment, the enrollment JWT is set as a static Authorization header.
// After enrollment, a HeaderFunc is returned that generates a fresh JWT for each request,
// ensuring the token doesn't expire during long-running connections.
func (s *Supervisor) buildAuthHeaders(settings connection.Settings) (http.Header, func(http.Header) http.Header) {
	headers := toHTTPHeaders(settings.Headers)

	// If we're not enrolled yet, use the enrollment JWT as a static header.
	if !s.authManager.IsEnrolled() {
		if jwt := s.authManager.EnrollmentJWT(); jwt != "" {
			headers.Set("Authorization", "Bearer "+jwt)
		}
		return headers, nil
	}

	// When enrolled, generate a fresh JWT before each HTTP request so the
	// token never expires during long-running connections.
	headerFunc := func(h http.Header) http.Header {
		authHeader, err := s.authManager.GetAuthorizationHeader()
		if err != nil {
			s.logger.Error("Failed to generate JWT for OpAMP request", zap.Error(err))
			return h
		}
		h.Set("Authorization", authHeader)
		return h
	}

	return headers, headerFunc
}
