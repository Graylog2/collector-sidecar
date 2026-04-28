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
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/internal/testpki"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestCreateSupervisorJWT(t *testing.T) {
	cert := createTestCert(t)

	instanceUID := "01HQ3K5V7X2M4N8P9R0S1T2U3V"
	lifetime := 5 * time.Minute

	token, err := CreateSupervisorJWT(cert.Key, cert.Cert, instanceUID, lifetime)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// Parse and verify the token
	certFP, claims, err := ParseSupervisorJWT(token)
	require.NoError(t, err)

	require.Equal(t, CertificateHexFingerprint(cert.Cert), certFP)
	require.Equal(t, instanceUID, claims.Subject)
	require.WithinDuration(t, time.Now(), claims.IssuedAt.Time, time.Second)
	require.WithinDuration(t, time.Now().Add(lifetime), claims.ExpiresAt.Time, time.Second)

	t.Run("SetsHeaders", func(t *testing.T) {
		tk, _, err := jwt.NewParser().ParseUnverified(token, claims)
		require.NoError(t, err)

		require.Len(t, tk.Header["x5t#S256"], 44)
		require.Equal(t, "agent", tk.Header["ctt"])
	})
}

func TestVerifySupervisorJWT(t *testing.T) {
	cert := createTestCert(t)

	token, err := CreateSupervisorJWT(cert.Key, cert.Cert, "test-uid", 5*time.Minute)
	require.NoError(t, err)

	err = VerifySupervisorJWT(token, cert.Cert)
	require.NoError(t, err)
}

func TestVerifySupervisorJWT_WrongKey(t *testing.T) {
	// Create cert with the signing key's public key
	cert := createTestCert(t)
	wrongCert := createTestCert(t)

	token, err := CreateSupervisorJWT(cert.Key, cert.Cert, "test-uid", 5*time.Minute)
	require.NoError(t, err)

	// Try to verify with a cert containing different public key
	err = VerifySupervisorJWT(token, wrongCert.Cert)
	require.Error(t, err)
}

func TestParseSupervisorJWT_InvalidFormat(t *testing.T) {
	_, _, err := ParseSupervisorJWT("not-a-jwt")
	require.Error(t, err)

	_, _, err = ParseSupervisorJWT("only.two")
	require.Error(t, err)
}

func TestSupervisorClaims_IsExpired(t *testing.T) {
	cert := createTestCert(t)

	// Create an already expired token
	token, err := CreateSupervisorJWT(cert.Key, cert.Cert, "test", -time.Hour)
	require.NoError(t, err)

	_, claims, err := ParseSupervisorJWT(token)
	require.NoError(t, err)
	require.True(t, claims.IsExpired())

	// Create a valid token
	token2, err := CreateSupervisorJWT(cert.Key, cert.Cert, "test", time.Hour)
	require.NoError(t, err)

	_, claims2, err := ParseSupervisorJWT(token2)
	require.NoError(t, err)
	require.False(t, claims2.IsExpired())
}

func TestSupervisorClaims_IsExpiringSoon(t *testing.T) {
	cert := createTestCert(t)

	// Create token expiring in 30 minutes
	token, err := CreateSupervisorJWT(cert.Key, cert.Cert, "test", 30*time.Minute)
	require.NoError(t, err)

	_, claims, err := ParseSupervisorJWT(token)
	require.NoError(t, err)

	// With 1 hour threshold, it should be expiring soon
	require.True(t, claims.IsExpiringSoon(1*time.Hour))

	// With 15 minute threshold, it should not be expiring soon
	require.False(t, claims.IsExpiringSoon(15*time.Minute))
}

func TestBearerToken(t *testing.T) {
	token := "eyJhbGciOiJFZERTQSJ9.eyJzdWIiOiJ0ZXN0In0.sig"
	require.Equal(t, "Bearer "+token, BearerToken(token))
}

func TestCertificateFingerprint(t *testing.T) {
	// Create a certificate that always has the same fingerprint.
	cert := testpki.GenerateTestCert(t,
		testpki.WithSubject("test-instance-uid"),
		testpki.WithSeed(make([]byte, ed25519.SeedSize)),
		testpki.WithNotBefore(time.UnixMilli(0)),
		testpki.WithNotAfter(time.UnixMilli(10)))

	t.Run("Hex", func(t *testing.T) {
		fp := CertificateHexFingerprint(cert.Cert)
		require.NotEmpty(t, fp)
		require.Equal(t, "9a5a9c5a7e8309bf2c8d2952ba34151691a8e529a5b8c4da5214df7f822edcbb", fp)
	})

	t.Run("Base64URL", func(t *testing.T) {
		fp := CertificateB64URLFingerprint(cert.Cert)
		require.NotEmpty(t, fp)
		require.Equal(t, "mlqcWn6DCb8sjSlSujQVFpGo5SmluMTaUhTff4Iu3Ls=", fp)
	})
}

// createTestCert creates a self-signed certificate for testing.
func createTestCert(t *testing.T) testpki.Cert {
	t.Helper()

	return testpki.GenerateTestCert(t, testpki.WithSubject("test-instance-uid"))
}
