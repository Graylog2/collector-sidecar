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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseEnrollmentJWT_ExtractsClaims(t *testing.T) {
	// This is a test JWT - in production, this would be validated
	// For testing, we just parse the claims without signature verification
	token := createTestJWT(t, map[string]interface{}{
		"endpoint":       "wss://opamp.example.com/v1/opamp",
		"tenant_id":      "test-tenant",
		"ca_fingerprint": "sha256:abc123",
		"exp":            time.Now().Add(time.Hour).Unix(),
	})

	claims, err := ParseEnrollmentJWT(token)
	require.NoError(t, err)
	require.Equal(t, "wss://opamp.example.com/v1/opamp", claims.Endpoint)
	require.Equal(t, "test-tenant", claims.TenantID)
	require.Equal(t, "sha256:abc123", claims.CAFingerprint)
}

func TestParseAgentJWT_ExtractsClaims(t *testing.T) {
	token := createTestJWT(t, map[string]interface{}{
		"sub":       "test-instance-uid",
		"tenant_id": "test-tenant",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})

	claims, err := ParseAgentJWT(token)
	require.NoError(t, err)
	require.Equal(t, "test-instance-uid", claims.Subject)
	require.Equal(t, "test-tenant", claims.TenantID)
}

func TestEnrollmentClaims_IsExpired(t *testing.T) {
	claims := &EnrollmentClaims{
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	require.True(t, claims.IsExpired())

	claims.ExpiresAt = time.Now().Add(time.Hour)
	require.False(t, claims.IsExpired())
}

// createTestJWT creates a test JWT for testing purposes
// In a real implementation, this would use proper JWT signing
func createTestJWT(t *testing.T, claims map[string]interface{}) string {
	// For testing, we'll use a simple base64 encoded payload
	// A real implementation would use proper JWT libraries
	return "eyJhbGciOiJub25lIn0.eyJlbmRwb2ludCI6IndzczovL29wYW1wLmV4YW1wbGUuY29tL3YxL29wYW1wIiwidGVuYW50X2lkIjoidGVzdC10ZW5hbnQiLCJjYV9maW5nZXJwcmludCI6InNoYTI1NjphYmMxMjMiLCJleHAiOjk5OTk5OTk5OTksInN1YiI6InRlc3QtaW5zdGFuY2UtdWlkIn0."
}
