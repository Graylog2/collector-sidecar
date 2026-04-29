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
	"bytes"
	"cmp"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"go.uber.org/zap"
	"golang.org/x/crypto/curve25519"

	"github.com/Graylog2/collector-sidecar/superv/persistence"
)

// Manager handles authentication including enrollment, key management, and JWT generation.
type Manager struct {
	logger      *zap.Logger
	keysDir     string
	httpClient  *http.Client
	jwtLifetime time.Duration

	mu sync.RWMutex

	// Cached credentials
	signingKey  ed25519.PrivateKey
	certificate *x509.Certificate

	// Enrollment state (before CSR is submitted)
	pendingSigningKey    ed25519.PrivateKey
	pendingEncryptionKey []byte
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
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: cfg.InsecureTLS}, //nolint:gosec // Intentionally configurable
			ExpectContinueTimeout: 1 * time.Second,
			// HTTP/2 PING-based liveness: send a PING after 30s of idle on a
			// connection; close the connection if no PONG returns within 10s.
			// Evicts half-dead HTTP/2 connections (upstream silently dropped,
			// no RST reaching the client) in ~40s instead of waiting for the
			// kernel's ~11-min TCP keepalive ladder. Any inbound frame resets
			// the SendPingTimeout so healthy traffic is unaffected.
			HTTP2: &http.HTTP2Config{
				SendPingTimeout: 30 * time.Second,
				PingTimeout:     10 * time.Second,
			},
		}}
	}

	jwtLifetime := cmp.Or(cfg.JWTLifetime, 5*time.Minute)

	return &Manager{
		logger:      logger,
		keysDir:     cfg.KeysDir,
		httpClient:  httpClient,
		jwtLifetime: jwtLifetime,
	}
}

// GetSigningKeyPath returns the path to the signing key file.
func (m *Manager) GetSigningKeyPath() string {
	return filepath.Join(m.keysDir, persistence.SigningKeyFile)
}

// GetSigningCertPath returns the path to the signing cert file.
func (m *Manager) GetSigningCertPath() string {
	return filepath.Join(m.keysDir, persistence.SigningCertFile)
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

	m.mu.Lock()
	m.signingKey = signingKey
	m.certificate = cert
	m.mu.Unlock()
	return nil
}

// EnrollmentResult contains the result of preparing enrollment.
type EnrollmentResult struct {
	// CSRPEM is the PEM-encoded CSR to send via OpAMP
	CSRPEM []byte
}

// PrepareEnrollment validates the enrollment JWT, generates keypairs, and creates a CSR.
// The CSR should be submitted via the OpAMP protocol using connection_settings_request.
// After receiving the certificate from the server, call CompleteEnrollment.
func (m *Manager) PrepareEnrollment(ctx context.Context, enrollmentEndpoint, enrollmentToken, instanceUID string) (*EnrollmentResult, error) {
	m.logger.Info("Preparing enrollment", zap.String("instance_uid", instanceUID), zap.String("endpoint", enrollmentEndpoint))

	baseURL, err := config.ServerBaseURL(enrollmentEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get server base URL: %w", err)
	}

	if enrollmentToken == "" {
		return nil, errors.New("enrollment token cannot be empty")
	}

	m.logger.Debug("Parsed server base URL", zap.String("url", baseURL))

	// Fetch JWKS
	m.logger.Debug("Fetching JWKS", zap.String("base-url", baseURL))
	jwks, err := FetchJWKS(ctx, m.httpClient, baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	// Validate enrollment JWT
	m.logger.Debug("Validating enrollment JWT")
	claims, err := ValidateEnrollmentJWT(enrollmentToken, jwks)
	if err != nil {
		return nil, fmt.Errorf("enrollment JWT validation failed: %w", err)
	}

	m.logger.Info("Enrollment JWT validated",
		zap.String("issuer", claims.Issuer),
		zap.String("id", claims.ID),
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
	csrDER, err := CreateCSR(signingPriv, instanceUID, encPub)
	if err != nil {
		return nil, fmt.Errorf("failed to create CSR: %w", err)
	}

	// Store pending credentials (will be saved when certificate is received).
	// HasPendingEnrollment and EnrollmentJWT read these fields under RLock
	// from other goroutines, so writes must be protected.
	m.mu.Lock()
	m.pendingSigningKey = signingPriv
	m.pendingEncryptionKey = encPriv
	m.pendingEnrollmentJWT = enrollmentToken
	m.mu.Unlock()

	// Return the CSR in PEM format for submission via OpAMP
	csrPEM := EncodeCSRToPEM(csrDER)

	m.logger.Info("Enrollment prepared, CSR ready for submission via OpAMP")

	return &EnrollmentResult{CSRPEM: csrPEM}, nil
}

// CompleteEnrollment stores the certificate received from the server and saves all credentials.
// The certPEM should be the certificate received in the OpAMP connection_settings response.
func (m *Manager) CompleteEnrollment(certPEM []byte) error {
	m.mu.RLock()
	pendingSigningKey := m.pendingSigningKey
	pendingEncryptionKey := m.pendingEncryptionKey
	m.mu.RUnlock()

	if pendingSigningKey == nil {
		return errors.New("no pending enrollment - call PrepareEnrollment first")
	}

	cert, err := parseCertificatePEM(certPEM)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Save credentials
	m.logger.Debug("Saving credentials")
	if err := persistence.SaveSigningKey(m.keysDir, pendingSigningKey); err != nil {
		return fmt.Errorf("failed to save signing key: %w", err)
	}

	if err := persistence.SaveEncryptionKey(m.keysDir, pendingEncryptionKey); err != nil {
		return fmt.Errorf("failed to save encryption key: %w", err)
	}

	if err := persistence.SaveCertificate(m.keysDir, cert); err != nil {
		return fmt.Errorf("failed to save certificate: %w", err)
	}

	// Update manager state and clear pending state atomically.
	m.mu.Lock()
	m.signingKey = pendingSigningKey
	m.certificate = cert
	m.pendingSigningKey = nil
	m.pendingEncryptionKey = nil
	m.pendingEnrollmentJWT = ""
	m.mu.Unlock()

	m.logger.Info("Enrollment completed successfully",
		zap.String("cert_fingerprint", CertificateHexFingerprint(cert)),
	)

	return nil
}

// HasPendingEnrollment returns true if PrepareEnrollment was called but CompleteEnrollment was not.
func (m *Manager) HasPendingEnrollment() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pendingSigningKey != nil
}

// GenerateJWT generates a new JWT for authenticating with the OpAMP server.
func (m *Manager) GenerateJWT() (string, error) {
	m.mu.RLock()
	signingKey := m.signingKey
	cert := m.certificate
	m.mu.RUnlock()

	if signingKey == nil || cert == nil {
		return "", errors.New("credentials not loaded")
	}

	// TODO: Should we really use the instance uid from the cert here or our stored instance UID?
	//       They should be the same but maybe we want to be explicit about it and check that they match?
	instanceUID := cert.Subject.CommonName
	return CreateSupervisorJWT(signingKey, cert, instanceUID, m.jwtLifetime)
}

// GetAuthorizationHeader returns the Authorization header value for OpAMP connections.
func (m *Manager) GetAuthorizationHeader() (string, error) {
	jwt, err := m.GenerateJWT()
	if err != nil {
		return "", err
	}
	return BearerToken(jwt), nil
}

// Certificate returns the loaded certificate.
func (m *Manager) Certificate() *x509.Certificate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.certificate
}

// CertFingerprint returns the fingerprint of the loaded certificate.
func (m *Manager) CertFingerprint() string {
	m.mu.RLock()
	cert := m.certificate
	m.mu.RUnlock()

	if cert == nil {
		return ""
	}
	return CertificateHexFingerprint(cert)
}

// EnrollmentJWT returns the pending enrollment JWT, or empty string if not enrolling.
func (m *Manager) EnrollmentJWT() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pendingEnrollmentJWT
}

// parseCertificatePEM decodes and parses a PEM-encoded X.509 certificate.
func parseCertificatePEM(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}
	return cert, nil
}

// CertificateNeedsRenewal returns true if the certificate has passed the renewal
// threshold. The threshold is computed as NotBefore + fraction * (NotAfter - NotBefore).
func (m *Manager) CertificateNeedsRenewal(renewalFraction float64) bool {
	return !m.CertificateRenewalTime(renewalFraction).IsZero() &&
		!time.Now().Before(m.CertificateRenewalTime(renewalFraction))
}

// CertificateRenewalTime returns the time at which the certificate should be
// renewed. Returns zero if no certificate is loaded.
func (m *Manager) CertificateRenewalTime(renewalFraction float64) time.Time {
	m.mu.RLock()
	cert := m.certificate
	m.mu.RUnlock()

	if cert == nil {
		return time.Time{}
	}

	lifetime := cert.NotAfter.Sub(cert.NotBefore)
	return cert.NotBefore.Add(time.Duration(float64(lifetime) * renewalFraction))
}

// CertificateExpired returns true if the certificate's NotAfter is in the past.
func (m *Manager) CertificateExpired() bool {
	m.mu.RLock()
	cert := m.certificate
	m.mu.RUnlock()

	if cert == nil {
		return false
	}

	return time.Now().After(cert.NotAfter)
}

// CompleteRenewal validates and persists a renewed certificate received from the server.
// The new cert must have the same public key as the current signing key and a later NotAfter.
func (m *Manager) CompleteRenewal(certPEM []byte) error {
	m.mu.RLock()
	signingKey := m.signingKey
	oldCert := m.certificate
	m.mu.RUnlock()

	if signingKey == nil || oldCert == nil {
		return errors.New("credentials not loaded")
	}

	newCert, err := parseCertificatePEM(certPEM)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Reject if the server issued a cert for a different key
	newPub, ok := newCert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return errors.New("renewed certificate does not contain an Ed25519 public key")
	}
	sigPub, ok := signingKey.Public().(ed25519.PublicKey)
	if !ok {
		return errors.New("signing key does not contain an Ed25519 public key")
	}
	if !bytes.Equal(newPub, sigPub) {
		return errors.New("public key mismatch: renewed certificate has a different public key")
	}

	// Reject if subject identity changed
	if newCert.Subject.CommonName != oldCert.Subject.CommonName {
		return fmt.Errorf("renewed certificate CommonName changed from %q to %q",
			oldCert.Subject.CommonName, newCert.Subject.CommonName)
	}

	// Reject if NotAfter is not extended
	if !newCert.NotAfter.After(oldCert.NotAfter) {
		return fmt.Errorf("renewed certificate NotAfter (%s) is not later than current (%s)",
			newCert.NotAfter.Format(time.RFC3339), oldCert.NotAfter.Format(time.RFC3339))
	}

	if err := persistence.SaveCertificate(m.keysDir, newCert); err != nil {
		return fmt.Errorf("failed to save renewed certificate: %w", err)
	}

	// Update cached cert so GenerateJWT uses the new thumbprint immediately.
	m.mu.Lock()
	m.certificate = newCert
	m.mu.Unlock()

	m.logger.Info("Certificate renewed",
		zap.String("cert_fingerprint", CertificateHexFingerprint(newCert)),
		zap.Time("old_expiration", oldCert.NotAfter),
		zap.Time("new_expiration", newCert.NotAfter),
	)

	return nil
}

// PrepareRenewal creates a CSR for certificate renewal using the existing keys.
// Unlike PrepareEnrollment, this does not generate new keypairs or validate tokens.
func (m *Manager) PrepareRenewal(instanceUID string) ([]byte, error) {
	m.mu.RLock()
	signingKey := m.signingKey
	cert := m.certificate
	m.mu.RUnlock()

	if signingKey == nil || cert == nil {
		return nil, errors.New("credentials not loaded")
	}

	// Load encryption private key from disk and derive public key
	encPriv, err := persistence.LoadEncryptionKey(m.keysDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load encryption key: %w", err)
	}

	encPub, err := curve25519.X25519(encPriv, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("failed to derive encryption public key: %w", err)
	}

	// Create CSR with existing signing key
	csrDER, err := CreateCSR(signingKey, instanceUID, encPub)
	if err != nil {
		return nil, fmt.Errorf("failed to create renewal CSR: %w", err)
	}

	m.logger.Info("Renewal CSR prepared", zap.String("instance_uid", instanceUID))
	return EncodeCSRToPEM(csrDER), nil
}
