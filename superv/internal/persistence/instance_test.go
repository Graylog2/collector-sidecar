// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestLoadOrCreateInstanceUID_CreatesNew(t *testing.T) {
	dir := t.TempDir()
	uid, err := LoadOrCreateInstanceUID(dir)
	require.NoError(t, err)
	require.NotEmpty(t, uid)

	// Verify it's a valid UUID
	_, err = uuid.Parse(uid)
	require.NoError(t, err)

	// Verify file was created
	filePath := filepath.Join(dir, "instance_uid.yaml")
	_, err = os.Stat(filePath)
	require.NoError(t, err)
}

func TestLoadOrCreateInstanceUID_LoadsExisting(t *testing.T) {
	dir := t.TempDir()

	// Create first
	uid1, err := LoadOrCreateInstanceUID(dir)
	require.NoError(t, err)

	// Load again - should return same UID
	uid2, err := LoadOrCreateInstanceUID(dir)
	require.NoError(t, err)
	require.Equal(t, uid1, uid2)
}

func TestLoadOrCreateInstanceUID_FileIsReadOnly(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadOrCreateInstanceUID(dir)
	require.NoError(t, err)

	// Check file permissions are read-only
	filePath := filepath.Join(dir, "instance_uid.yaml")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0444), info.Mode().Perm())
}

func TestLoadOrCreateInstanceUID_PreservesCreatedAt(t *testing.T) {
	dir := t.TempDir()

	// Create instance
	_, err := LoadOrCreateInstanceUID(dir)
	require.NoError(t, err)

	// Read the file to get created_at
	data, err := LoadInstanceData(dir)
	require.NoError(t, err)
	originalCreatedAt := data.CreatedAt

	// Load again
	_, err = LoadOrCreateInstanceUID(dir)
	require.NoError(t, err)

	// Verify created_at is unchanged
	data2, err := LoadInstanceData(dir)
	require.NoError(t, err)
	require.Equal(t, originalCreatedAt, data2.CreatedAt)
}
