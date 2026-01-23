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
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// EnrollmentClaims represents claims from an enrollment JWT.
type EnrollmentClaims struct {
	Endpoint      string            `json:"endpoint"`
	TenantID      string            `json:"tenant_id"`
	CAFingerprint string            `json:"ca_fingerprint"`
	AgentLabels   map[string]string `json:"agent_labels"`
	ExpiresAt     time.Time         `json:"-"`
	Exp           int64             `json:"exp"`
}

// AgentClaims represents claims from an agent JWT.
type AgentClaims struct {
	Subject   string    `json:"sub"`
	TenantID  string    `json:"tenant_id"`
	Issuer    string    `json:"iss"`
	Audience  string    `json:"aud"`
	IssuedAt  time.Time `json:"-"`
	ExpiresAt time.Time `json:"-"`
	Iat       int64     `json:"iat"`
	Exp       int64     `json:"exp"`
}

// ParseEnrollmentJWT parses an enrollment JWT and extracts claims.
// Note: This does NOT verify the signature - that should be done separately
// based on the trust model (fingerprint or CA-verified).
func ParseEnrollmentJWT(token string) (*EnrollmentClaims, error) {
	claims, err := parseJWTClaims(token)
	if err != nil {
		return nil, err
	}

	var enrollmentClaims EnrollmentClaims
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(claimsJSON, &enrollmentClaims); err != nil {
		return nil, err
	}

	if enrollmentClaims.Exp > 0 {
		enrollmentClaims.ExpiresAt = time.Unix(enrollmentClaims.Exp, 0)
	}

	return &enrollmentClaims, nil
}

// ParseAgentJWT parses an agent JWT and extracts claims.
func ParseAgentJWT(token string) (*AgentClaims, error) {
	claims, err := parseJWTClaims(token)
	if err != nil {
		return nil, err
	}

	var agentClaims AgentClaims
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(claimsJSON, &agentClaims); err != nil {
		return nil, err
	}

	if agentClaims.Iat > 0 {
		agentClaims.IssuedAt = time.Unix(agentClaims.Iat, 0)
	}
	if agentClaims.Exp > 0 {
		agentClaims.ExpiresAt = time.Unix(agentClaims.Exp, 0)
	}

	return &agentClaims, nil
}

// parseJWTClaims extracts claims from a JWT without verifying the signature.
func parseJWTClaims(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid JWT format")
	}

	// Decode the payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("failed to decode JWT payload")
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errors.New("failed to parse JWT claims")
	}

	return claims, nil
}

// IsExpired returns true if the claims have expired.
func (c *EnrollmentClaims) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// IsExpired returns true if the claims have expired.
func (c *AgentClaims) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// IsExpiringSoon returns true if the claims will expire within the threshold.
func (c *AgentClaims) IsExpiringSoon(threshold time.Duration) bool {
	return time.Now().Add(threshold).After(c.ExpiresAt)
}

// ExtractAuthorizationHeader creates the Authorization header value for the token.
func ExtractAuthorizationHeader(token string) string {
	return "Bearer " + token
}
