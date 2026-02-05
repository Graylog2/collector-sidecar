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
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestCreateSupervisorJWT(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	cert := createTestCert(t, pub)

	instanceUID := "01HQ3K5V7X2M4N8P9R0S1T2U3V"
	audience := "opamp.example.com"
	lifetime := 5 * time.Minute

	token, err := CreateSupervisorJWT(priv, cert, instanceUID, audience, lifetime)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// Parse and verify the token
	certFP, claims, err := ParseSupervisorJWT(token)
	require.NoError(t, err)

	require.Equal(t, CertificateHexFingerprint(cert), certFP)
	require.Equal(t, instanceUID, claims.Subject)
	require.Contains(t, claims.Audience, audience)
	require.WithinDuration(t, time.Now(), claims.IssuedAt.Time, time.Second)
	require.WithinDuration(t, time.Now().Add(lifetime), claims.ExpiresAt.Time, time.Second)

	t.Run("SetsHeaders", func(t *testing.T) {
		tk, _, err := jwt.NewParser().ParseUnverified(token, claims)
		require.Nil(t, err)

		require.Len(t, tk.Header["x5t#S256"], 44)
		require.Equal(t, "agent", tk.Header["ctt"])
	})
}

func TestVerifySupervisorJWT(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	cert := createTestCert(t, pub)

	token, err := CreateSupervisorJWT(priv, cert, "test-uid", "test-aud", 5*time.Minute)
	require.NoError(t, err)

	err = VerifySupervisorJWT(token, cert)
	require.NoError(t, err)
}

func TestVerifySupervisorJWT_WrongKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	otherPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Create cert with the signing key's public key
	cert := createTestCert(t, priv.Public().(ed25519.PublicKey))

	token, err := CreateSupervisorJWT(priv, cert, "test-uid", "test-aud", 5*time.Minute)
	require.NoError(t, err)

	// Try to verify with a cert containing different public key
	wrongCert := createTestCert(t, otherPub)
	err = VerifySupervisorJWT(token, wrongCert)
	require.Error(t, err)
}

func TestParseSupervisorJWT_InvalidFormat(t *testing.T) {
	_, _, err := ParseSupervisorJWT("not-a-jwt")
	require.Error(t, err)

	_, _, err = ParseSupervisorJWT("only.two")
	require.Error(t, err)
}

func TestSupervisorClaims_IsExpired(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cert := createTestCert(t, pub)

	// Create an already expired token
	token, err := CreateSupervisorJWT(priv, cert, "test", "aud", -time.Hour)
	require.NoError(t, err)

	_, claims, err := ParseSupervisorJWT(token)
	require.NoError(t, err)
	require.True(t, claims.IsExpired())

	// Create a valid token
	token2, err := CreateSupervisorJWT(priv, cert, "test", "aud", time.Hour)
	require.NoError(t, err)

	_, claims2, err := ParseSupervisorJWT(token2)
	require.NoError(t, err)
	require.False(t, claims2.IsExpired())
}

func TestSupervisorClaims_IsExpiringSoon(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cert := createTestCert(t, pub)

	// Create token expiring in 30 minutes
	token, err := CreateSupervisorJWT(priv, cert, "test", "aud", 30*time.Minute)
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
	seed := make([]byte, ed25519.SeedSize) // Deterministic for testing

	priv := ed25519.NewKeyFromSeed(seed)
	pub, ok := priv.Public().(ed25519.PublicKey)
	require.True(t, ok)

	// Create a certificate that always has the same fingerprint.
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-instance-uid",
		},
		NotBefore: time.UnixMilli(0),
		NotAfter:  time.UnixMilli(10),
		KeyUsage:  x509.KeyUsageDigitalSignature,
	}

	// Self-sign the certificate
	signPriv := ed25519.NewKeyFromSeed(seed)
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, pub, signPriv)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	t.Run("Hex", func(t *testing.T) {
		fp := CertificateHexFingerprint(cert)
		require.NotEmpty(t, fp)
		require.Equal(t, "02438c90d57d85802339a884786ffaf1f638e272b9ceebc6018ad474d45f22fa", fp)
	})

	t.Run("Base64URL", func(t *testing.T) {
		fp := CertificateB64URLFingerprint(cert)
		require.NotEmpty(t, fp)
		require.Equal(t, "AkOMkNV9hYAjOaiEeG_68fY44nK5zuvGAYrUdNRfIvo=", fp)
	})
}

// createTestCert creates a self-signed certificate for testing.
func createTestCert(t *testing.T, pub ed25519.PublicKey) *x509.Certificate {
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

	// Self-sign the certificate
	priv := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)) // Deterministic for testing
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	return cert
}
