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

package winrenameio

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- WriteFile tests ---

func TestWriteFile_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	data := []byte("hello world")

	err := WriteFile(path, data, 0o644)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestWriteFile_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")

	err := WriteFile(path, []byte("original"), 0o644)
	require.NoError(t, err)

	err = WriteFile(path, []byte("replaced"), 0o644)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("replaced"), got)
}

func TestWriteFile_EmptyData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")

	err := WriteFile(path, []byte{}, 0o644)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestWriteFile_LargeData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.bin")

	data := make([]byte, 1<<20) // 1 MiB
	for i := range data {
		data[i] = byte(i % 251)
	}

	err := WriteFile(path, data, 0o600)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestWriteFile_SetsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not fully support Unix file permissions")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "perms.txt")

	err := WriteFile(path, []byte("secret"), 0o600)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestWriteFile_NoTempFileLeftBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.txt")

	err := WriteFile(path, []byte("data"), 0o644)
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	// Only the target file should remain.
	require.Len(t, entries, 1)
	assert.Equal(t, "clean.txt", entries[0].Name())
}

func TestWriteFile_NonexistentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no", "such", "dir", "file.txt")

	err := WriteFile(path, []byte("data"), 0o644)
	require.Error(t, err)
}

// --- ReplaceFile tests ---

func TestReplaceFile_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	require.NoError(t, os.WriteFile(dst, []byte("old"), 0o644))
	require.NoError(t, os.WriteFile(src, []byte("new"), 0o644))

	err := ReplaceFile(src, dst)
	require.NoError(t, err)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, []byte("new"), got)

	// Source should no longer exist (it was renamed).
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err))
}

func TestReplaceFile_DestinationDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))

	err := ReplaceFile(src, dst)
	require.NoError(t, err)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), got)
}

func TestReplaceFile_SourceDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nonexistent.txt")
	dst := filepath.Join(dir, "dst.txt")

	err := ReplaceFile(src, dst)
	require.Error(t, err)
}

// --- NewPendingFile + CloseAtomicallyReplace tests ---

func TestPendingFile_CloseAtomicallyReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")

	pf, err := NewPendingFile(path)
	require.NoError(t, err)

	_, err = pf.Write([]byte("pending data"))
	require.NoError(t, err)

	err = pf.CloseAtomicallyReplace()
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("pending data"), got)
}

func TestPendingFile_CloseAtomicallyReplace_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")

	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))

	pf, err := NewPendingFile(path)
	require.NoError(t, err)

	_, err = pf.Write([]byte("new"))
	require.NoError(t, err)

	err = pf.CloseAtomicallyReplace()
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("new"), got)
}

func TestPendingFile_CloseAtomicallyReplace_NoTempFileLeftBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")

	pf, err := NewPendingFile(path)
	require.NoError(t, err)

	_, err = pf.Write([]byte("data"))
	require.NoError(t, err)

	err = pf.CloseAtomicallyReplace()
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "target.txt", entries[0].Name())
}

func TestPendingFile_CloseAtomicallyReplace_DoubleCloseReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")

	pf, err := NewPendingFile(path)
	require.NoError(t, err)

	err = pf.CloseAtomicallyReplace()
	require.NoError(t, err)

	err = pf.CloseAtomicallyReplace()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already closed")
}

// --- NewPendingFile + Cleanup tests ---

func TestPendingFile_Cleanup_RemovesTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")

	pf, err := NewPendingFile(path)
	require.NoError(t, err)

	_, err = pf.Write([]byte("discard me"))
	require.NoError(t, err)

	err = pf.Cleanup()
	require.NoError(t, err)

	// Target should not have been created.
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))

	// Temp file should be gone too.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestPendingFile_Cleanup_NoopAfterReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")

	pf, err := NewPendingFile(path)
	require.NoError(t, err)

	_, err = pf.Write([]byte("keep me"))
	require.NoError(t, err)

	err = pf.CloseAtomicallyReplace()
	require.NoError(t, err)

	// Cleanup after successful replace should be a no-op.
	err = pf.Cleanup()
	require.NoError(t, err)

	// Target file should still exist.
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("keep me"), got)
}

// --- WithReplaceOnClose option ---

func TestPendingFile_WithReplaceOnClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")

	pf, err := NewPendingFile(path, WithReplaceOnClose())
	require.NoError(t, err)

	_, err = pf.Write([]byte("auto-replace"))
	require.NoError(t, err)

	// Close should trigger atomic replace.
	err = pf.Close()
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("auto-replace"), got)
}

func TestPendingFile_WithoutReplaceOnClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")

	pf, err := NewPendingFile(path)
	require.NoError(t, err)

	_, err = pf.Write([]byte("no replace"))
	require.NoError(t, err)

	// Close without WithReplaceOnClose should NOT create the target
	// and should clean up the temp file.
	err = pf.Close()
	require.NoError(t, err)

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))

	// No temp files should be left behind.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// --- WithStaticPermissions option ---

func TestPendingFile_WithStaticPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not fully support Unix file permissions")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")

	pf, err := NewPendingFile(path, WithStaticPermissions(0o640), WithReplaceOnClose())
	require.NoError(t, err)

	_, err = pf.Write([]byte("permissioned"))
	require.NoError(t, err)

	err = pf.Close()
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}

func TestPendingFile_DefaultPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not fully support Unix file permissions")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")

	pf, err := NewPendingFile(path, WithReplaceOnClose())
	require.NoError(t, err)

	_, err = pf.Write([]byte("default perms"))
	require.NoError(t, err)

	err = pf.Close()
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// --- Mirrors renameio usage pattern from writer_unix.go ---

func TestPendingFile_RenameioUsagePattern(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("receivers:\n  otlp:\n    protocols:\n      grpc:\n")

	// This mirrors the exact usage pattern from writer_unix.go:
	//   file, err := renameio.NewPendingFile(path, renameio.WithStaticPermissions(perm), renameio.WithReplaceOnClose())
	//   file.Write(data)
	//   file.Sync()
	//   ... on commit: file.Close()
	//   ... on abort:  file.Cleanup()

	pf, err := NewPendingFile(path, WithStaticPermissions(0o600), WithReplaceOnClose())
	require.NoError(t, err)

	_, err = pf.Write(data)
	require.NoError(t, err)

	err = pf.Sync()
	require.NoError(t, err)

	// Commit
	err = pf.Close()
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestPendingFile_RenameioUsagePattern_Abort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	pf, err := NewPendingFile(path, WithStaticPermissions(0o600), WithReplaceOnClose())
	require.NoError(t, err)

	_, err = pf.Write([]byte("aborted content"))
	require.NoError(t, err)

	err = pf.Sync()
	require.NoError(t, err)

	// Abort instead of commit
	err = pf.Cleanup()
	require.NoError(t, err)

	// Target should not exist.
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))

	// No temp files left.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
