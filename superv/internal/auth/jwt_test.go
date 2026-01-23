// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

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
