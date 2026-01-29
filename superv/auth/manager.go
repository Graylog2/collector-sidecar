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

package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/Graylog2/collector-sidecar/superv/persistence"
)

// Manager handles authentication including enrollment, key management, and JWT generation.
type Manager struct {
	logger      *zap.Logger
	keysDir     string
	httpClient  *http.Client
	jwtLifetime time.Duration

	// Cached credentials
	signingKey  ed25519.PrivateKey
	certificate *x509.Certificate
	serverHost  string

	// Enrollment state (before CSR is submitted)
	pendingSigningKey    ed25519.PrivateKey
	pendingEncryptionKey []byte
	pendingTenantID      string
	pendingEnrollmentJWT string
}

// ManagerConfig holds configuration for the auth manager.
type ManagerConfig struct {
	KeysDir     string
	JWTLifetime time.Duration
	HTTPClient  *http.Client
	InsecureTLS bool
}

// NewManager creates a new authentication manager.
func NewManager(logger *zap.Logger, cfg ManagerConfig) *Manager {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: cfg.InsecureTLS},
			ExpectContinueTimeout: 1 * time.Second,
		}}
	}

	jwtLifetime := cfg.JWTLifetime
	if jwtLifetime == 0 {
		jwtLifetime = 5 * time.Minute
	}

	return &Manager{
		logger:      logger,
		keysDir:     cfg.KeysDir,
		httpClient:  httpClient,
		jwtLifetime: jwtLifetime,
	}
}

// IsEnrolled returns true if the supervisor has valid credentials.
func (m *Manager) IsEnrolled() bool {
	return persistence.SigningKeyExists(m.keysDir) &&
		persistence.CertificateExists(m.keysDir)
}

// LoadCredentials loads existing credentials from disk.
func (m *Manager) LoadCredentials() error {
	signingKey, err := persistence.LoadSigningKey(m.keysDir)
	if err != nil {
		return fmt.Errorf("failed to load signing key: %w", err)
	}

	cert, err := persistence.LoadCertificate(m.keysDir)
	if err != nil {
		return fmt.Errorf("failed to load certificate: %w", err)
	}

	m.signingKey = signingKey
	m.certificate = cert
	return nil
}

// EnrollmentResult contains the result of preparing enrollment.
type EnrollmentResult struct {
	// CSRPEM is the PEM-encoded CSR to send via OpAMP
	CSRPEM []byte
	// TenantID from the enrollment JWT
	TenantID string
}

// PrepareEnrollment validates the enrollment JWT, generates keypairs, and creates a CSR.
// The CSR should be submitted via the OpAMP protocol using connection_settings_request.
// After receiving the certificate from the server, call CompleteEnrollment.
func (m *Manager) PrepareEnrollment(ctx context.Context, enrollmentURL, instanceUID string) (*EnrollmentResult, error) {
	m.logger.Info("Preparing enrollment", zap.String("instance_uid", instanceUID))

	// Parse enrollment URL
	hostname, enrollmentJWT, err := ParseEnrollmentURL(enrollmentURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse enrollment URL: %w", err)
	}

	baseURL, err := ServerBaseURL(enrollmentURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get server base URL: %w", err)
	}

	m.serverHost = hostname
	m.logger.Debug("Parsed enrollment URL", zap.String("host", hostname))

	// Fetch JWKS
	m.logger.Debug("Fetching JWKS")
	jwks, err := FetchJWKS(m.httpClient, baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	// Validate enrollment JWT
	m.logger.Debug("Validating enrollment JWT")
	claims, err := ValidateEnrollmentJWT(enrollmentJWT, jwks)
	if err != nil {
		return nil, fmt.Errorf("enrollment JWT validation failed: %w", err)
	}

	m.logger.Info("Enrollment JWT validated",
		zap.String("tenant_id", claims.TenantID),
		zap.String("key_algorithm", claims.KeyAlgorithm),
	)

	// Generate signing keypair
	m.logger.Debug("Generating signing keypair")
	_, signingPriv, err := GenerateSigningKeypair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate signing keypair: %w", err)
	}

	// Generate encryption keypair
	m.logger.Debug("Generating encryption keypair")
	encPub, encPriv, err := GenerateEncryptionKeypair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate encryption keypair: %w", err)
	}

	// Create CSR
	m.logger.Debug("Creating CSR")
	var csrDER []byte
	if claims.TenantID != "" {
		csrDER, err = CreateCSRWithTenant(signingPriv, instanceUID, claims.TenantID, encPub)
	} else {
		csrDER, err = CreateCSR(signingPriv, instanceUID, encPub)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create CSR: %w", err)
	}

	// Store pending credentials (will be saved when certificate is received)
	m.pendingSigningKey = signingPriv
	m.pendingEncryptionKey = encPriv
	m.pendingTenantID = claims.TenantID
	m.pendingEnrollmentJWT = enrollmentJWT

	// Return the CSR in PEM format for submission via OpAMP
	csrPEM := EncodeCSRToPEM(csrDER)

	m.logger.Info("Enrollment prepared, CSR ready for submission via OpAMP")

	return &EnrollmentResult{
		CSRPEM:   csrPEM,
		TenantID: claims.TenantID,
	}, nil
}

// CompleteEnrollment stores the certificate received from the server and saves all credentials.
// The certPEM should be the certificate received in the OpAMP connection_settings response.
func (m *Manager) CompleteEnrollment(certPEM []byte) error {
	if m.pendingSigningKey == nil {
		return errors.New("no pending enrollment - call PrepareEnrollment first")
	}

	// Parse certificate
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return errors.New("invalid certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Save credentials
	m.logger.Debug("Saving credentials")
	if err := persistence.SaveSigningKey(m.keysDir, m.pendingSigningKey); err != nil {
		return fmt.Errorf("failed to save signing key: %w", err)
	}

	if err := persistence.SaveEncryptionKey(m.keysDir, m.pendingEncryptionKey); err != nil {
		return fmt.Errorf("failed to save encryption key: %w", err)
	}

	if err := persistence.SaveCertificate(m.keysDir, cert); err != nil {
		return fmt.Errorf("failed to save certificate: %w", err)
	}

	// Update manager state
	m.signingKey = m.pendingSigningKey
	m.certificate = cert

	// Clear pending state
	m.pendingSigningKey = nil
	m.pendingEncryptionKey = nil
	m.pendingTenantID = ""
	m.pendingEnrollmentJWT = ""

	m.logger.Info("Enrollment completed successfully",
		zap.String("cert_fingerprint", CertificateFingerprint(cert)),
	)

	return nil
}

// HasPendingEnrollment returns true if PrepareEnrollment was called but CompleteEnrollment was not.
func (m *Manager) HasPendingEnrollment() bool {
	return m.pendingSigningKey != nil
}

// GenerateJWT generates a new JWT for authenticating with the OpAMP server.
func (m *Manager) GenerateJWT(audience string) (string, error) {
	if m.signingKey == nil || m.certificate == nil {
		return "", errors.New("credentials not loaded")
	}

	instanceUID := m.certificate.Subject.CommonName
	return CreateSupervisorJWT(m.signingKey, m.certificate, instanceUID, audience, m.jwtLifetime)
}

// GetAuthorizationHeader returns the Authorization header value for OpAMP connections.
func (m *Manager) GetAuthorizationHeader(audience string) (string, error) {
	jwt, err := m.GenerateJWT(audience)
	if err != nil {
		return "", err
	}
	return BearerToken(jwt), nil
}

// Certificate returns the loaded certificate.
func (m *Manager) Certificate() *x509.Certificate {
	return m.certificate
}

// CertFingerprint returns the fingerprint of the loaded certificate.
func (m *Manager) CertFingerprint() string {
	if m.certificate == nil {
		return ""
	}
	return CertificateFingerprint(m.certificate)
}

// ServerHost returns the server hostname from enrollment.
func (m *Manager) ServerHost() string {
	return m.serverHost
}

// SetServerHost sets the server host for JWT audience.
func (m *Manager) SetServerHost(host string) {
	m.serverHost = host
}

// EnrollmentJWT returns the pending enrollment JWT, or empty string if not enrolling.
func (m *Manager) EnrollmentJWT() string {
	return m.pendingEnrollmentJWT
}
