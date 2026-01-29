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
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// EnrollmentClaims represents claims from an enrollment JWT.
type EnrollmentClaims struct {
	jwt.RegisteredClaims
	TenantID     string            `json:"tenant_id"`
	KeyAlgorithm string            `json:"key_algorithm"`
	AgentLabels  map[string]string `json:"agent_labels"`
}

// IsExpired returns true if the enrollment claims have expired.
func (c *EnrollmentClaims) IsExpired() bool {
	if c.ExpiresAt == nil {
		return false
	}
	return time.Now().After(c.ExpiresAt.Time)
}

// ParseEnrollmentURL extracts the hostname and JWT from an enrollment URL.
// URL format: https://server.example.com/opamp/enroll/<JWT>
func ParseEnrollmentURL(enrollmentURL string) (hostname string, jwtToken string, err error) {
	u, err := url.Parse(enrollmentURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid enrollment URL: %w", err)
	}

	if u.Scheme != "https" {
		return "", "", errors.New("enrollment URL must use HTTPS")
	}

	// Extract JWT from path (last segment after /enroll/)
	parts := strings.Split(u.Path, "/enroll/")
	if len(parts) != 2 || parts[1] == "" {
		return "", "", errors.New("enrollment URL must contain /enroll/<JWT>")
	}

	return u.Host, parts[1], nil
}

// ServerBaseURL extracts the base URL (scheme + host) from an enrollment URL.
func ServerBaseURL(enrollmentURL string) (string, error) {
	u, err := url.Parse(enrollmentURL)
	if err != nil {
		return "", fmt.Errorf("invalid enrollment URL: %w", err)
	}

	return fmt.Sprintf("%s://%s", u.Scheme, u.Host), nil
}

// ValidateEnrollmentJWT validates an enrollment JWT against the JWKS.
func ValidateEnrollmentJWT(tokenString string, keys []JWK) (*EnrollmentClaims, error) {
	claims := &EnrollmentClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unsupported signing method: %v", token.Header["alg"])
		}

		// Get key ID from header
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("missing kid in JWT header")
		}

		// Find the key
		key, err := GetKeyByID(keys, kid)
		if err != nil {
			return nil, fmt.Errorf("key not found: %w", err)
		}

		return key.PublicKey, nil
	})

	if err != nil {
		// Check for specific error types
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errors.New("JWT has expired")
		}
		if errors.Is(err, jwt.ErrSignatureInvalid) {
			return nil, errors.New("invalid JWT signature")
		}
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("invalid JWT")
	}

	return claims, nil
}
