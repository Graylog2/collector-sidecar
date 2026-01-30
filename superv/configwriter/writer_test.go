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

package configwriter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteConfig_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	content := []byte("key: value\n")

	err := WriteConfig(path, content)
	require.NoError(t, err)

	// Verify file exists
	require.True(t, ConfigExists(path))

	// Verify content
	read, err := ReadConfig(path)
	require.NoError(t, err)
	require.Equal(t, content, read)
}

func TestWriteConfig_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	content := []byte("key: value\n")

	err := WriteConfig(path, content)
	require.NoError(t, err)

	// Verify no temp files are left behind
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)

	// Should only have the target file, no .tmp files
	require.Len(t, entries, 1)
	require.Equal(t, "config.yaml", entries[0].Name())

	// Verify no files matching the temp pattern exist
	matches, err := filepath.Glob(filepath.Join(tmpDir, ".config-*.tmp"))
	require.NoError(t, err)
	require.Empty(t, matches)
}

func TestWriteConfig_CreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "deep", "config.yaml")
	content := []byte("key: value\n")

	err := WriteConfig(path, content)
	require.NoError(t, err)

	// Verify file exists
	require.True(t, ConfigExists(path))

	// Verify content
	read, err := ReadConfig(path)
	require.NoError(t, err)
	require.Equal(t, content, read)

	// Verify parent directories were created
	parentDir := filepath.Dir(path)
	info, err := os.Stat(parentDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestWriteConfig_PreservesPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	content := []byte("key: value\n")

	err := WriteConfig(path, content)
	require.NoError(t, err)

	// Verify file permissions are 0600
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestWriteConfig_OverwritesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	// Write initial content
	initial := []byte("initial: content\n")
	err := WriteConfig(path, initial)
	require.NoError(t, err)

	// Overwrite with new content
	updated := []byte("updated: content\n")
	err = WriteConfig(path, updated)
	require.NoError(t, err)

	// Verify content was updated
	read, err := ReadConfig(path)
	require.NoError(t, err)
	require.Equal(t, updated, read)
}

func TestReadConfig_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nonexistent.yaml")

	_, err := ReadConfig(path)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestConfigExists_True(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	// Create file
	err := os.WriteFile(path, []byte("content"), 0644)
	require.NoError(t, err)

	require.True(t, ConfigExists(path))
}

func TestConfigExists_False(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nonexistent.yaml")

	require.False(t, ConfigExists(path))
}

func TestWriteConfig_EmptyContent(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	content := []byte{}

	err := WriteConfig(path, content)
	require.NoError(t, err)

	// Verify file exists and is empty
	require.True(t, ConfigExists(path))
	read, err := ReadConfig(path)
	require.NoError(t, err)
	require.Empty(t, read)
}
