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
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const x5tHeader = "x5t#S256"
const cttHeader = "ctt"

// SupervisorClaims represents the claims in a supervisor-signed JWT.
type SupervisorClaims struct {
	jwt.RegisteredClaims
}

// IsExpired returns true if the supervisor claims have expired.
func (c *SupervisorClaims) IsExpired() bool {
	if c.ExpiresAt == nil {
		return false
	}
	return time.Now().After(c.ExpiresAt.Time)
}

// IsExpiringSoon returns true if the claims will expire within the threshold.
func (c *SupervisorClaims) IsExpiringSoon(threshold time.Duration) bool {
	if c.ExpiresAt == nil {
		return false
	}
	return time.Now().Add(threshold).After(c.ExpiresAt.Time)
}

// CreateSupervisorJWT creates a JWT signed by the supervisor's private key.
func CreateSupervisorJWT(
	privateKey ed25519.PrivateKey,
	cert *x509.Certificate,
	instanceUID string,
	audience string,
	lifetime time.Duration,
) (string, error) {
	now := time.Now()

	claims := SupervisorClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   instanceUID,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(lifetime)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)

	// Required by server to lookup correct certificate
	token.Header[x5tHeader] = CertificateB64URLFingerprint(cert)
	// Header required by server
	token.Header[cttHeader] = "agent"

	return token.SignedString(privateKey)
}

// ParseSupervisorJWT parses a supervisor-signed JWT without verifying the signature.
// Returns the certificate fingerprint from header and the claims.
func ParseSupervisorJWT(tokenString string) (certFingerprint string, claims *SupervisorClaims, err error) {
	claims = &SupervisorClaims{}

	// Parse without validation to extract claims and header
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, claims)
	if err != nil {
		return "", nil, err
	}

	// Extract certificate fingerprint from header
	if fp, ok := token.Header[x5tHeader].(string); ok {
		certFingerprint = fp
	}

	hexCertFingerprint, err := base64.URLEncoding.DecodeString(certFingerprint)
	if err != nil {
		return "", nil, fmt.Errorf("couldn't decode fingerprint from header: %w", err)
	}

	return hex.EncodeToString(hexCertFingerprint), claims, nil
}

// VerifySupervisorJWT verifies a supervisor-signed JWT against the certificate.
func VerifySupervisorJWT(tokenString string, cert *x509.Certificate) error {
	pubKey, ok := cert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return errors.New("certificate does not contain Ed25519 key")
	}

	claims := &SupervisorClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return pubKey, nil
	})

	if err != nil {
		return err
	}

	if !token.Valid {
		return errors.New("invalid token")
	}

	return nil
}

// BearerToken returns the token formatted for the Authorization header.
func BearerToken(token string) string {
	return "Bearer " + token
}

// CertificateHexFingerprint returns the SHA-256 fingerprint of a certificate as a hex string.
func CertificateHexFingerprint(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:])
}

// CertificateB64URLFingerprint returns the SHA-256 fingerprint of a certificate as a base64url-encoded string.
func CertificateB64URLFingerprint(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return base64.URLEncoding.EncodeToString(hash[:])
}
