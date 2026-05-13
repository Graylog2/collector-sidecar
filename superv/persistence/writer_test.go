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

package persistence

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteFileCreatesParentDirsAndWritesContent(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "deep", "nested", "settings.txt")
	content := []byte("atomic write content")

	err := WriteFile(filePath, content, 0o600)
	require.NoError(t, err)

	actual, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, content, actual)

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestStageFileCommitWritesContent(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "stage", "connection.yaml")
	content := []byte("endpoint: localhost:4317\n")

	stage, err := StageFile(filePath, content)
	require.NoError(t, err)

	_, err = os.Stat(filePath)
	require.ErrorIs(t, err, os.ErrNotExist)

	err = stage.Commit()
	require.NoError(t, err)

	actual, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, content, actual)
}

func TestStageFileCleanupDiscardPendingWrite(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "stage", "rollback.yaml")

	stage, err := StageFile(filePath, []byte("new-value: true\n"))
	require.NoError(t, err)

	err = stage.Cleanup()
	require.NoError(t, err)

	_, err = os.Stat(filePath)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestStageFileCommitRunsCallback(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "stage", "callback.yaml")

	stage, err := StageFile(filePath, []byte("ok: true\n"))
	require.NoError(t, err)

	called := false
	stage.SetCommitCallback(func() error {
		called = true
		return nil
	})

	err = stage.Commit()
	require.NoError(t, err)
	require.True(t, called)
}

func TestStageFileCommitReturnsCallbackErrorAfterPersisting(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "stage", "callback-error.yaml")
	content := []byte("value: persisted\n")

	stage, err := StageFile(filePath, content)
	require.NoError(t, err)

	callbackErr := errors.New("callback failed")
	stage.SetCommitCallback(func() error {
		return callbackErr
	})

	err = stage.Commit()
	require.Error(t, err)
	require.ErrorIs(t, err, callbackErr)

	actual, readErr := os.ReadFile(filePath)
	require.NoError(t, readErr)
	require.Equal(t, content, actual)
}

func TestWriteFileReturnsErrorWhenParentPathIsAFile(t *testing.T) {
	dir := t.TempDir()
	blockedPath := filepath.Join(dir, "blocked")
	err := os.WriteFile(blockedPath, []byte("not a directory"), 0o600)
	require.NoError(t, err)

	err = WriteFile(filepath.Join(blockedPath, "target.txt"), []byte("content"), 0o600)
	require.Error(t, err)
	require.Contains(t, err.Error(), "creating parent directories")
}

func TestStageFileReturnsErrorWhenParentPathIsAFile(t *testing.T) {
	dir := t.TempDir()
	blockedPath := filepath.Join(dir, "blocked")
	err := os.WriteFile(blockedPath, []byte("not a directory"), 0o600)
	require.NoError(t, err)

	_, err = StageFile(filepath.Join(blockedPath, "target.txt"), []byte("content"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "creating parent directories")
}
