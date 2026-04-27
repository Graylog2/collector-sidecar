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
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/internal/testpki"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/Graylog2/collector-sidecar/superv/persistence"
)

func TestManager_GetSigningKeyPath(t *testing.T) {
	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: "/tmp/test-keys"})

	got := m.GetSigningKeyPath()
	require.Equal(t, filepath.Join("/tmp/test-keys", persistence.SigningKeyFile), got)
}

func TestManager_GetSigningCertPath(t *testing.T) {
	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: "/tmp/test-keys"})

	got := m.GetSigningCertPath()
	require.Equal(t, filepath.Join("/tmp/test-keys", persistence.SigningCertFile), got)
}

func TestManager_IsEnrolled(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: keysDir})

	// Initially not enrolled
	require.False(t, m.IsEnrolled())

	cert := testpki.GenerateTestCert(t)

	require.NoError(t, persistence.SaveSigningKey(keysDir, cert.Key))

	require.False(t, m.IsEnrolled()) // Still missing cert

	require.NoError(t, persistence.SaveCertificate(keysDir, cert.Cert))

	require.True(t, m.IsEnrolled())
}

func TestManager_LoadCredentials(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	cert := testpki.GenerateTestCert(t)
	require.NoError(t, persistence.SaveSigningKey(keysDir, cert.Key))
	require.NoError(t, persistence.SaveCertificate(keysDir, cert.Cert))

	// Load them
	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: keysDir})

	err := m.LoadCredentials()
	require.NoError(t, err)
	require.NotNil(t, m.Certificate())
	require.Equal(t, CertificateHexFingerprint(cert.Cert), m.CertFingerprint())
}

func TestManager_LoadCredentials_NotEnrolled(t *testing.T) {
	dir := t.TempDir()

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: dir})

	err := m.LoadCredentials()
	require.Error(t, err)
}

func TestManager_GenerateJWT(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	cert := testpki.GenerateTestCert(t, testpki.WithSubject("test-instance-uid"))
	require.NoError(t, persistence.SaveSigningKey(keysDir, cert.Key))
	require.NoError(t, persistence.SaveCertificate(keysDir, cert.Cert))

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{
		KeysDir:     keysDir,
		JWTLifetime: 5 * time.Minute,
	})

	err := m.LoadCredentials()
	require.NoError(t, err)

	jwtToken, err := m.GenerateJWT()
	require.NoError(t, err)
	require.NotEmpty(t, jwtToken)

	// Verify the JWT
	err = VerifySupervisorJWT(jwtToken, cert.Cert)
	require.NoError(t, err)

	// Parse and check claims
	certFP, claims, err := ParseSupervisorJWT(jwtToken)
	require.NoError(t, err)
	require.Equal(t, "test-instance-uid", claims.Subject)
	require.Equal(t, CertificateHexFingerprint(cert.Cert), certFP)
}

func TestManager_GenerateJWT_NotLoaded(t *testing.T) {
	dir := t.TempDir()

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: dir})

	_, err := m.GenerateJWT()
	require.Error(t, err)
	require.ErrorContains(t, err, "credentials not loaded")
}

func TestManager_GetAuthorizationHeader(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	cert := testpki.GenerateTestCert(t)
	require.NoError(t, persistence.SaveSigningKey(keysDir, cert.Key))
	require.NoError(t, persistence.SaveCertificate(keysDir, cert.Cert))

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: keysDir})
	_ = m.LoadCredentials()

	header, err := m.GetAuthorizationHeader()
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
			_ = json.NewEncoder(w).Encode(jwks)

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
	})

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{
		KeysDir:    keysDir,
		HTTPClient: server.Client(),
	})

	// Phase 1: Prepare enrollment (validates JWT, generates keys, creates CSR)
	result, err := m.PrepareEnrollment(context.Background(), server.URL, enrollmentJWT, "test-instance")
	require.NoError(t, err)
	require.NotEmpty(t, result.CSRPEM)

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
	cert := testpki.GenerateTestCert(t, testpki.WithCSR(csr))

	// Phase 2: Complete enrollment with certificate from server
	err = m.CompleteEnrollment(cert.CertPEM)
	require.NoError(t, err)

	// Verify enrollment completed
	require.False(t, m.HasPendingEnrollment())
	require.True(t, m.IsEnrolled())
	require.NotNil(t, m.Certificate())
	require.NotEmpty(t, m.CertFingerprint())

	// Should be able to generate JWT now
	jwtToken, err := m.GenerateJWT()
	require.NoError(t, err)
	require.NotEmpty(t, jwtToken)
}

func TestManager_PrepareEnrollment_InvalidEndpoint(t *testing.T) {
	dir := t.TempDir()

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: dir})

	_, err := m.PrepareEnrollment(context.Background(), "", "ey", "test-instance")
	require.Error(t, err)
	require.ErrorContains(t, err, "enrollment URL")
}

func TestManager_PrepareEnrollment_InvalidToken(t *testing.T) {
	dir := t.TempDir()

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: dir})

	_, err := m.PrepareEnrollment(context.Background(), "https://example.com", "", "test-instance")
	require.Error(t, err)
	require.ErrorContains(t, err, "enrollment token")
}

func TestManager_CompleteEnrollment_NoPending(t *testing.T) {
	dir := t.TempDir()

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: dir})

	// Try to complete without preparing first
	err := m.CompleteEnrollment([]byte("some-cert"))
	require.Error(t, err)
	require.ErrorContains(t, err, "no pending enrollment")
}

func TestManager_CertificateNeedsRenewal(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("no cert loaded returns false", func(t *testing.T) {
		m := NewManager(logger, ManagerConfig{})
		require.False(t, m.CertificateNeedsRenewal(0.75))
	})

	t.Run("cert valid now to now+24h fraction 0.75 returns false", func(t *testing.T) {
		now := time.Now()
		cert := testpki.GenerateTestCert(t,
			testpki.WithNotBefore(now),
			testpki.WithNotAfter(now.Add(24*time.Hour)))
		m := NewManager(logger, ManagerConfig{})
		m.mu.Lock()
		m.certificate = cert.Cert
		m.mu.Unlock()
		// threshold is now + 18h, which is in the future
		require.False(t, m.CertificateNeedsRenewal(0.75))
	})

	t.Run("cert valid from 24h ago to 1h from now fraction 0.75 returns true", func(t *testing.T) {
		now := time.Now()
		cert := testpki.GenerateTestCert(t,
			testpki.WithNotBefore(now.Add(-24*time.Hour)),
			testpki.WithNotAfter(now.Add(time.Hour)))
		m := NewManager(logger, ManagerConfig{})
		m.mu.Lock()
		m.certificate = cert.Cert
		m.mu.Unlock()
		// lifetime = 25h, threshold = -24h + 0.75*25h = -24h + 18.75h = -5.25h (in the past)
		require.True(t, m.CertificateNeedsRenewal(0.75))
	})

	t.Run("expired cert returns true", func(t *testing.T) {
		now := time.Now()
		cert := testpki.GenerateTestCert(t,
			testpki.WithNotBefore(now.Add(-48*time.Hour)),
			testpki.WithNotAfter(now.Add(-1*time.Hour)))
		m := NewManager(logger, ManagerConfig{})
		m.mu.Lock()
		m.certificate = cert.Cert
		m.mu.Unlock()
		require.True(t, m.CertificateNeedsRenewal(0.75))
	})
}

func TestManager_CertificateExpired(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("no cert loaded returns false", func(t *testing.T) {
		m := NewManager(logger, ManagerConfig{})
		require.False(t, m.CertificateExpired())
	})

	t.Run("cert not expired returns false", func(t *testing.T) {
		now := time.Now()
		cert := testpki.GenerateTestCert(t,
			testpki.WithNotBefore(now),
			testpki.WithNotAfter(now.Add(24*time.Hour)))
		m := NewManager(logger, ManagerConfig{})
		m.mu.Lock()
		m.certificate = cert.Cert
		m.mu.Unlock()
		require.False(t, m.CertificateExpired())
	})

	t.Run("cert expired returns true", func(t *testing.T) {
		now := time.Now()
		cert := testpki.GenerateTestCert(t,
			testpki.WithNotBefore(now.Add(-48*time.Hour)),
			testpki.WithNotAfter(now.Add(-1*time.Hour)))
		m := NewManager(logger, ManagerConfig{})
		m.mu.Lock()
		m.certificate = cert.Cert
		m.mu.Unlock()
		require.True(t, m.CertificateExpired())
	})
}

func TestManager_PrepareRenewal(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	// Generate signing keypair and save
	_, signingPriv, err := GenerateSigningKeypair()
	require.NoError(t, err)
	err = persistence.SaveSigningKey(keysDir, signingPriv)
	require.NoError(t, err)

	// Generate encryption keypair and save private key
	encPubExpected, encPriv, err := GenerateEncryptionKeypair()
	require.NoError(t, err)
	err = persistence.SaveEncryptionKey(keysDir, encPriv)
	require.NoError(t, err)

	// Create cert with Organization
	cert := testpki.GenerateTestCert(t)
	err = persistence.SaveCertificate(keysDir, cert.Cert)
	require.NoError(t, err)

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: keysDir})
	err = m.LoadCredentials()
	require.NoError(t, err)

	csrPEM, err := m.PrepareRenewal("test-instance-uid")
	require.NoError(t, err)
	require.NotEmpty(t, csrPEM)

	// Parse the returned PEM CSR
	block, _ := pem.Decode(csrPEM)
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE REQUEST", block.Type)

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)

	// Verify CN and Organization
	require.Equal(t, "test-instance-uid", csr.Subject.CommonName)

	// Verify public key matches signing key
	require.Equal(t, signingPriv.Public(), csr.PublicKey)

	// Verify encryption public key extension is present with correct value
	var foundEncPub []byte
	for _, ext := range csr.Extensions {
		if ext.Id.Equal(OIDEncryptionPublicKey) {
			foundEncPub = ext.Value
			break
		}
	}
	require.NotNil(t, foundEncPub, "encryption public key extension not found")
	require.Equal(t, encPubExpected, foundEncPub)
}

func TestManager_PrepareRenewal_NotLoaded(t *testing.T) {
	dir := t.TempDir()

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: dir})

	_, err := m.PrepareRenewal("test-instance-uid")
	require.Error(t, err)
	require.ErrorContains(t, err, "credentials not loaded")
}

func TestManager_CompleteRenewal(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	_, priv, err := GenerateSigningKeypair()
	require.NoError(t, err)
	err = persistence.SaveSigningKey(keysDir, priv)
	require.NoError(t, err)

	now := time.Now()
	oldCert := testpki.GenerateTestCert(t,
		testpki.WithPrivateKey(priv),
		testpki.WithNotBefore(now.Add(-24*time.Hour)),
		testpki.WithNotAfter(now.Add(time.Hour)))
	err = persistence.SaveCertificate(keysDir, oldCert.Cert)
	require.NoError(t, err)

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: keysDir})
	err = m.LoadCredentials()
	require.NoError(t, err)

	newCert := testpki.GenerateTestCert(t,
		testpki.WithPrivateKey(priv),
		testpki.WithNotBefore(now),
		testpki.WithNotAfter(now.Add(24*time.Hour)))

	err = m.CompleteRenewal(newCert.CertPEM)
	require.NoError(t, err)

	// In-memory certificate updated
	require.Equal(t, newCert.Cert.NotAfter.Unix(), m.Certificate().NotAfter.Unix())

	// Persisted certificate updated
	persisted, err := persistence.LoadCertificate(keysDir)
	require.NoError(t, err)
	require.Equal(t, newCert.Cert.NotAfter.Unix(), persisted.NotAfter.Unix())
}

func TestManager_CompleteRenewal_WrongKey(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	_, priv, err := GenerateSigningKeypair()
	require.NoError(t, err)
	err = persistence.SaveSigningKey(keysDir, priv)
	require.NoError(t, err)

	now := time.Now()
	oldCert := testpki.GenerateTestCert(t,
		testpki.WithPrivateKey(priv),
		testpki.WithNotBefore(now.Add(-24*time.Hour)),
		testpki.WithNotAfter(now.Add(1*time.Hour)))
	err = persistence.SaveCertificate(keysDir, oldCert.Cert)
	require.NoError(t, err)

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: keysDir})
	err = m.LoadCredentials()
	require.NoError(t, err)

	// Create cert with a different key
	_, differentPriv, err := GenerateSigningKeypair()
	require.NoError(t, err)
	newCert := testpki.GenerateTestCert(t,
		testpki.WithPrivateKey(differentPriv),
		testpki.WithNotBefore(now),
		testpki.WithNotAfter(now.Add(48*time.Hour)))

	err = m.CompleteRenewal(newCert.CertPEM)
	require.Error(t, err)
	require.ErrorContains(t, err, "public key mismatch")

	// Old cert still loaded
	require.Equal(t, oldCert.Cert.NotAfter.Unix(), m.Certificate().NotAfter.Unix())
}

func TestManager_CompleteRenewal_NotAfterNotExtended(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	_, priv, err := GenerateSigningKeypair()
	require.NoError(t, err)
	err = persistence.SaveSigningKey(keysDir, priv)
	require.NoError(t, err)

	now := time.Now()
	oldCert := testpki.GenerateTestCert(t,
		testpki.WithPrivateKey(priv),
		testpki.WithNotBefore(now.Add(-24*time.Hour)),
		testpki.WithNotAfter(now.Add(time.Hour)))
	err = persistence.SaveCertificate(keysDir, oldCert.Cert)
	require.NoError(t, err)

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: keysDir})
	err = m.LoadCredentials()
	require.NoError(t, err)

	// New cert with same NotAfter as old cert
	newCert := testpki.GenerateTestCert(t,
		testpki.WithPrivateKey(priv),
		testpki.WithNotBefore(now),
		testpki.WithNotAfter(oldCert.Cert.NotAfter))

	err = m.CompleteRenewal(newCert.CertPEM)
	require.Error(t, err)
	require.ErrorContains(t, err, "NotAfter")

	// Old cert still loaded
	require.Equal(t, oldCert.Cert.NotAfter.Unix(), m.Certificate().NotAfter.Unix())
}

func TestManager_CompleteRenewal_InvalidPEM(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	_, priv, err := GenerateSigningKeypair()
	require.NoError(t, err)
	err = persistence.SaveSigningKey(keysDir, priv)
	require.NoError(t, err)

	now := time.Now()
	oldCert := testpki.GenerateTestCert(t,
		testpki.WithPrivateKey(priv),
		testpki.WithNotBefore(now.Add(-24*time.Hour)),
		testpki.WithNotAfter(now.Add(time.Hour)))
	err = persistence.SaveCertificate(keysDir, oldCert.Cert)
	require.NoError(t, err)

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: keysDir})
	err = m.LoadCredentials()
	require.NoError(t, err)

	err = m.CompleteRenewal([]byte("not-a-cert"))
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid certificate PEM")
}

func TestManager_CompleteRenewal_ChangedCommonName(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	require.NoError(t, persistence.SaveSigningKey(keysDir, priv))

	now := time.Now()
	oldCert := testpki.GenerateTestCert(t,
		testpki.WithPrivateKey(priv),
		testpki.WithNotBefore(now),
		testpki.WithNotAfter(now.Add(time.Hour)))
	require.NoError(t, persistence.SaveCertificate(keysDir, oldCert.Cert))

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: keysDir})
	require.NoError(t, m.LoadCredentials())

	newCert := testpki.GenerateTestCert(t,
		testpki.WithSubject("different-uid"),
		testpki.WithPrivateKey(priv),
		testpki.WithNotBefore(now),
		testpki.WithNotAfter(now.Add(time.Hour)))

	err = m.CompleteRenewal(newCert.CertPEM)
	require.Error(t, err)
	require.ErrorContains(t, err, "CommonName changed")
}

func TestManager_CompleteRenewal_NotLoaded(t *testing.T) {
	dir := t.TempDir()

	m := NewManager(zaptest.NewLogger(t), ManagerConfig{KeysDir: dir})

	err := m.CompleteRenewal([]byte("anything"))
	require.Error(t, err)
	require.ErrorContains(t, err, "credentials not loaded")
}

func createTestEnrollmentJWT(t *testing.T, priv ed25519.PrivateKey, kid string, claims *EnrollmentClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = kid

	tokenString, err := token.SignedString(priv)
	require.NoError(t, err)

	return tokenString
}
