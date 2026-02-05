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
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestParseEnrollmentURL(t *testing.T) {
	url := "https://opamp.example.com/opamp/enroll/eyJhbGciOiJFZERTQSJ9.eyJ0ZW5hbnRfaWQiOiJ0ZXN0In0.sig"

	hostname, jwtToken, err := ParseEnrollmentURL(url)
	require.NoError(t, err)
	require.Equal(t, "opamp.example.com", hostname)
	require.Equal(t, "eyJhbGciOiJFZERTQSJ9.eyJ0ZW5hbnRfaWQiOiJ0ZXN0In0.sig", jwtToken)
}

func TestParseEnrollmentURL_WithPort(t *testing.T) {
	url := "https://opamp.example.com:8443/opamp/enroll/token123"

	hostname, jwtToken, err := ParseEnrollmentURL(url)
	require.NoError(t, err)
	require.Equal(t, "opamp.example.com:8443", hostname)
	require.Equal(t, "token123", jwtToken)
}

func TestParseEnrollmentURL_InvalidFormat(t *testing.T) {
	_, _, err := ParseEnrollmentURL("not-a-url")
	require.Error(t, err)

	_, _, err = ParseEnrollmentURL("https://example.com/no-jwt-here")
	require.Error(t, err)

	_, _, err = ParseEnrollmentURL("https://example.com/enroll/")
	require.Error(t, err)
}

func TestServerBaseURL(t *testing.T) {
	url, err := ServerBaseURL("https://opamp.example.com/opamp/enroll/token")
	require.NoError(t, err)
	require.Equal(t, "https://opamp.example.com", url)

	url, err = ServerBaseURL("https://opamp.example.com:8443/opamp/enroll/token")
	require.NoError(t, err)
	require.Equal(t, "https://opamp.example.com:8443", url)
}

func TestValidateEnrollmentJWT(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	token := createSignedEnrollmentJWT(t, priv, "test-kid", &EnrollmentClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		TenantID:     "test-tenant",
		KeyAlgorithm: "Ed25519",
		AgentLabels:  map[string]string{"env": "test"},
	})

	keys := []JWK{{KeyID: "test-kid", PublicKey: pub}}

	validated, err := ValidateEnrollmentJWT(token, keys)
	require.NoError(t, err)
	require.Equal(t, "test-tenant", validated.TenantID)
	require.Equal(t, "Ed25519", validated.KeyAlgorithm)
	require.Equal(t, "test", validated.AgentLabels["env"])
}

func TestValidateEnrollmentJWT_Expired(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	token := createSignedEnrollmentJWT(t, priv, "test-kid", &EnrollmentClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)), // Expired
		},
		TenantID: "test",
	})

	keys := []JWK{{KeyID: "test-kid", PublicKey: pub}}

	_, err = ValidateEnrollmentJWT(token, keys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expired")
}

func TestValidateEnrollmentJWT_InvalidSignature(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	otherPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	token := createSignedEnrollmentJWT(t, priv, "test-kid", &EnrollmentClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		TenantID: "test",
	})

	keys := []JWK{{KeyID: "test-kid", PublicKey: otherPub}} // Different key

	_, err = ValidateEnrollmentJWT(token, keys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "signature")
}

func TestValidateEnrollmentJWT_KeyNotFound(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	token := createSignedEnrollmentJWT(t, priv, "unknown-kid", &EnrollmentClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		TenantID: "test",
	})

	keys := []JWK{} // No keys

	_, err = ValidateEnrollmentJWT(token, keys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestEnrollmentClaims_IsExpired(t *testing.T) {
	claims := &EnrollmentClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	require.True(t, claims.IsExpired())

	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
	require.False(t, claims.IsExpired())
}

// createSignedEnrollmentJWT creates a signed JWT for testing.
func createSignedEnrollmentJWT(t *testing.T, priv ed25519.PrivateKey, kid string, claims *EnrollmentClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = kid

	tokenString, err := token.SignedString(priv)
	require.NoError(t, err)

	return tokenString
}
