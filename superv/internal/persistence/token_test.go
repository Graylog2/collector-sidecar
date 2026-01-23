// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package persistence

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSaveAndLoadAgentToken(t *testing.T) {
	dir := t.TempDir()
	authDir := filepath.Join(dir, "auth")

	token := &AgentToken{
		Token:      "test-jwt-token",
		ReceivedAt: time.Now().UTC().Truncate(time.Second),
		ExpiresAt:  time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second),
	}

	err := SaveAgentToken(authDir, token)
	require.NoError(t, err)

	loaded, err := LoadAgentToken(authDir)
	require.NoError(t, err)
	require.Equal(t, token.Token, loaded.Token)
	require.Equal(t, token.ReceivedAt, loaded.ReceivedAt)
	require.Equal(t, token.ExpiresAt, loaded.ExpiresAt)
}

func TestLoadAgentToken_NotExists(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadAgentToken(dir)
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestSaveAgentToken_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	authDir := filepath.Join(dir, "auth")

	token := &AgentToken{
		Token:      "test-jwt-token",
		ReceivedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(24 * time.Hour),
	}

	err := SaveAgentToken(authDir, token)
	require.NoError(t, err)

	filePath := filepath.Join(authDir, "agent_token.yaml")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestAgentToken_IsExpired(t *testing.T) {
	token := &AgentToken{
		Token:      "test",
		ReceivedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(-1 * time.Hour), // Already expired
	}
	require.True(t, token.IsExpired())

	token.ExpiresAt = time.Now().UTC().Add(1 * time.Hour)
	require.False(t, token.IsExpired())
}

func TestAgentToken_IsExpiringSoon(t *testing.T) {
	token := &AgentToken{
		Token:      "test",
		ReceivedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(30 * time.Minute), // Expires in 30 min
	}

	// With 1 hour threshold, it should be expiring soon
	require.True(t, token.IsExpiringSoon(1*time.Hour))

	// With 15 minute threshold, it should not be expiring soon
	require.False(t, token.IsExpiringSoon(15*time.Minute))
}

func TestDeleteAgentToken(t *testing.T) {
	dir := t.TempDir()
	authDir := filepath.Join(dir, "auth")

	token := &AgentToken{
		Token:      "test-token",
		ReceivedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(24 * time.Hour),
	}

	// Save token
	err := SaveAgentToken(authDir, token)
	require.NoError(t, err)

	// Delete token
	err = DeleteAgentToken(authDir)
	require.NoError(t, err)

	// Verify file is gone
	_, err = LoadAgentToken(authDir)
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestDeleteAgentToken_NotExists(t *testing.T) {
	dir := t.TempDir()

	// Delete non-existent token should not error
	err := DeleteAgentToken(dir)
	require.NoError(t, err)
}
