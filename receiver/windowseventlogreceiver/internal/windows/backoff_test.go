// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackoff_InitialDelay(t *testing.T) {
	b := newBackoff()
	require.Equal(t, 5*time.Second, b.next())
}

func TestBackoff_Doubles(t *testing.T) {
	b := newBackoff()
	require.Equal(t, 5*time.Second, b.next())
	require.Equal(t, 10*time.Second, b.next())
	require.Equal(t, 20*time.Second, b.next())
	require.Equal(t, 40*time.Second, b.next())
	require.Equal(t, 60*time.Second, b.next()) // capped
	require.Equal(t, 60*time.Second, b.next()) // stays capped
}

func TestBackoff_Reset(t *testing.T) {
	b := newBackoff()
	b.next()
	b.next()
	b.reset()
	require.Equal(t, 5*time.Second, b.next())
}

func TestIsRecoverableError(t *testing.T) {
	tests := []struct {
		name     string
		code     uint32
		expected bool
	}{
		{"ERROR_INVALID_HANDLE", 6, true},
		{"RPC_S_SERVER_UNAVAILABLE", 1722, true},
		{"RPC_S_CALL_CANCELLED", 1818, true},
		{"ERROR_EVT_QUERY_RESULT_STALE", 15011, true},
		{"ERROR_INVALID_PARAMETER", 87, true},
		{"ERROR_EVT_PUBLISHER_DISABLED", 15037, true},
		{"ERROR_EVT_CHANNEL_NOT_FOUND", 15007, false},
		{"ERROR_ACCESS_DENIED", 5, false},
		{"unknown error", 99999, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, isRecoverableError(tt.code))
		})
	}
}
