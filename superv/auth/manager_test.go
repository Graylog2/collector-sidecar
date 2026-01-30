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
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/Graylog2/collector-sidecar/superv/persistence"
)

func TestManager_IsEnrolled(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	logger := zaptest.NewLogger(t)
	m := NewManager(logger, ManagerConfig{KeysDir: keysDir})

	// Initially not enrolled
	require.False(t, m.IsEnrolled())

	// Create keys and cert
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = persistence.SaveSigningKey(keysDir, priv)
	require.False(t, m.IsEnrolled()) // Still missing cert

	cert := createManagerTestCert(t, priv.Public().(ed25519.PublicKey))
	_ = persistence.SaveCertificate(keysDir, cert)
	require.True(t, m.IsEnrolled())
}

func TestManager_LoadCredentials(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	// Create credentials
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = persistence.SaveSigningKey(keysDir, priv)

	cert := createManagerTestCert(t, priv.Public().(ed25519.PublicKey))
	_ = persistence.SaveCertificate(keysDir, cert)

	// Load them
	logger := zaptest.NewLogger(t)
	m := NewManager(logger, ManagerConfig{KeysDir: keysDir})

	err := m.LoadCredentials()
	require.NoError(t, err)
	require.NotNil(t, m.Certificate())
	require.Equal(t, CertificateFingerprint(cert), m.CertFingerprint())
}

func TestManager_LoadCredentials_NotEnrolled(t *testing.T) {
	dir := t.TempDir()

	logger := zaptest.NewLogger(t)
	m := NewManager(logger, ManagerConfig{KeysDir: dir})

	err := m.LoadCredentials()
	require.Error(t, err)
}

func TestManager_GenerateJWT(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	// Create credentials
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = persistence.SaveSigningKey(keysDir, priv)

	cert := createManagerTestCert(t, priv.Public().(ed25519.PublicKey))
	_ = persistence.SaveCertificate(keysDir, cert)

	logger := zaptest.NewLogger(t)
	m := NewManager(logger, ManagerConfig{
		KeysDir:     keysDir,
		JWTLifetime: 5 * time.Minute,
	})

	err := m.LoadCredentials()
	require.NoError(t, err)

	jwtToken, err := m.GenerateJWT("opamp.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, jwtToken)

	// Verify the JWT
	err = VerifySupervisorJWT(jwtToken, cert)
	require.NoError(t, err)

	// Parse and check claims
	certFP, claims, err := ParseSupervisorJWT(jwtToken)
	require.NoError(t, err)
	require.Equal(t, "test-instance-uid", claims.Subject)
	require.Contains(t, claims.Audience, "opamp.example.com")
	require.Equal(t, CertificateFingerprint(cert), certFP)
}

func TestManager_GenerateJWT_NotLoaded(t *testing.T) {
	dir := t.TempDir()

	logger := zaptest.NewLogger(t)
	m := NewManager(logger, ManagerConfig{KeysDir: dir})

	_, err := m.GenerateJWT("example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "credentials not loaded")
}

func TestManager_GetAuthorizationHeader(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	// Create credentials
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = persistence.SaveSigningKey(keysDir, priv)

	cert := createManagerTestCert(t, priv.Public().(ed25519.PublicKey))
	_ = persistence.SaveCertificate(keysDir, cert)

	logger := zaptest.NewLogger(t)
	m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
	_ = m.LoadCredentials()

	header, err := m.GetAuthorizationHeader("example.com")
	require.NoError(t, err)
	require.True(t, len(header) > 7)
	require.Equal(t, "Bearer ", header[:7])
}

func TestManager_PrepareAndCompleteEnrollment(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	// Create server keys for JWKS and enrollment JWT
	serverPub, serverPriv, _ := ed25519.GenerateKey(rand.Reader)

	// Create a mock server that only serves JWKS (CSR is now via OpAMP)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/jwks.json":
			jwks := map[string]any{
				"keys": []map[string]any{
					{
						"kty": "OKP",
						"crv": "Ed25519",
						"kid": "server-key-1",
						"x":   base64.RawURLEncoding.EncodeToString(serverPub),
						"use": "sig",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jwks)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create enrollment JWT using jwt/v5
	enrollmentJWT := createTestEnrollmentJWT(t, serverPriv, "server-key-1", &EnrollmentClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		TenantID:     "test-tenant",
		KeyAlgorithm: "Ed25519",
	})

	enrollmentURL := server.URL + "/opamp/enroll/" + enrollmentJWT

	logger := zaptest.NewLogger(t)
	m := NewManager(logger, ManagerConfig{
		KeysDir:    keysDir,
		HTTPClient: server.Client(),
	})

	// Phase 1: Prepare enrollment (validates JWT, generates keys, creates CSR)
	result, err := m.PrepareEnrollment(context.Background(), enrollmentURL, "test-instance")
	require.NoError(t, err)
	require.NotEmpty(t, result.CSRPEM)
	require.Equal(t, "test-tenant", result.TenantID)

	// Verify pending state
	require.True(t, m.HasPendingEnrollment())
	require.False(t, m.IsEnrolled())

	// Parse CSR to verify it's valid
	block, _ := pem.Decode(result.CSRPEM)
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE REQUEST", block.Type)

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)
	require.Equal(t, "test-instance", csr.Subject.CommonName)

	// Simulate server signing the CSR and returning a certificate
	cert := signCSRForTest(t, csr)
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})

	// Phase 2: Complete enrollment with certificate from server
	err = m.CompleteEnrollment(certPEM)
	require.NoError(t, err)

	// Verify enrollment completed
	require.False(t, m.HasPendingEnrollment())
	require.True(t, m.IsEnrolled())
	require.NotNil(t, m.Certificate())
	require.NotEmpty(t, m.CertFingerprint())

	// Should be able to generate JWT now
	jwtToken, err := m.GenerateJWT("example.com")
	require.NoError(t, err)
	require.NotEmpty(t, jwtToken)
}

func TestManager_PrepareEnrollment_InvalidURL(t *testing.T) {
	dir := t.TempDir()

	logger := zaptest.NewLogger(t)
	m := NewManager(logger, ManagerConfig{KeysDir: dir})

	_, err := m.PrepareEnrollment(context.Background(), "not-a-url", "test-instance")
	require.Error(t, err)
	require.Contains(t, err.Error(), "enrollment URL")
}

func TestManager_CompleteEnrollment_NoPending(t *testing.T) {
	dir := t.TempDir()

	logger := zaptest.NewLogger(t)
	m := NewManager(logger, ManagerConfig{KeysDir: dir})

	// Try to complete without preparing first
	err := m.CompleteEnrollment([]byte("some-cert"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "no pending enrollment")
}

// signCSRForTest signs a CSR and returns a certificate (for testing).
func signCSRForTest(t *testing.T, csr *x509.CertificateRequest) *x509.Certificate {
	t.Helper()

	// Generate CA key for signing
	_, caPriv, _ := ed25519.GenerateKey(rand.Reader)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      csr.Subject,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caTemplate, csr.PublicKey, caPriv)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	return cert
}

// createManagerTestCert creates a self-signed certificate for testing.
func createManagerTestCert(t *testing.T, pub ed25519.PublicKey) *x509.Certificate {
	t.Helper()

	// Use a deterministic private key for self-signing
	seed := make([]byte, ed25519.SeedSize)
	priv := ed25519.NewKeyFromSeed(seed)

	return createManagerTestCertWithKey(t, pub, priv)
}

func createManagerTestCertWithKey(t *testing.T, pub ed25519.PublicKey, signingKey ed25519.PrivateKey) *x509.Certificate {
	t.Helper()

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-instance-uid",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, pub, signingKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	return cert
}

func createTestEnrollmentJWT(t *testing.T, priv ed25519.PrivateKey, kid string, claims *EnrollmentClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = kid

	tokenString, err := token.SignedString(priv)
	require.NoError(t, err)

	return tokenString
}

func mustReadAll(t *testing.T, r interface{ Read([]byte) (int, error) }) []byte {
	t.Helper()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	return buf[:n]
}
