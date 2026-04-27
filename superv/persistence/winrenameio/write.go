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

// Package winrenameio provides atomic file replacement on Windows.
//
// The API mirrors [github.com/google/renameio/v2] so that callers can switch
// between the two packages using only a build-tag-gated import. Use renameio
// on Unix and winrenameio on Windows.
//
// # Problem
//
// On Unix systems, os.Rename atomically replaces the destination file because
// rename(2) is specified by POSIX to atomically replace the target. This makes
// the common "write temp file, fsync, rename" pattern safe: readers always see
// either the old or the new file contents, never a partially written file.
//
// On Windows, the situation is more nuanced. Go's os.Rename calls MoveFileExW
// with MOVEFILE_REPLACE_EXISTING, which does overwrite existing files. However:
//
//   - Microsoft does not formally guarantee atomicity for MoveFileEx. In
//     practice, on NTFS with same-volume moves, MoveFileEx resolves to a
//     metadata-level rename that is atomic. But this is an implementation
//     detail, not a contract.
//   - Go's os.Rename does NOT pass the MOVEFILE_WRITE_THROUGH flag, which means
//     the rename operation may not be flushed to disk before the call returns.
//     A power loss at the wrong moment could leave neither the old nor the new
//     file on disk.
//   - The github.com/google/renameio/v2 package avoids Windows entirely,
//     falling back to plain os.WriteFile in its "maybe" sub-package.
//
// # Solution
//
// This package calls MoveFileExW directly via golang.org/x/sys/windows with
// both MOVEFILE_REPLACE_EXISTING and MOVEFILE_WRITE_THROUGH flags:
//
//   - MOVEFILE_REPLACE_EXISTING: if the destination file already exists, it is
//     replaced. Without this flag, MoveFileEx fails when the destination exists.
//   - MOVEFILE_WRITE_THROUGH: the function does not return until the file has
//     been actually moved on disk. This improves durability but does not
//     strictly guarantee it end-to-end (storage drivers, disk firmware caches,
//     etc. may still defer the write).
//
// This is the same approach used by github.com/natefinch/atomic.
//
// # Limitations
//
//   - Only same-volume moves are supported. Without MOVEFILE_COPY_ALLOWED,
//     cross-volume moves fail with an error. Callers should create temporary
//     files in the same directory as the target to ensure same-volume operation.
//   - While MoveFileEx with MOVEFILE_REPLACE_EXISTING is atomic in practice on
//     NTFS, Microsoft does not formally document this guarantee. For config file
//     writes (where the realistic failure mode is a power loss during rename)
//     this is the best available approach on Windows.
//   - Antivirus software, search indexers, or other processes holding file
//     handles can cause transient ERROR_ACCESS_DENIED or
//     ERROR_SHARING_VIOLATION errors. [ReplaceFile] retries these with
//     exponential backoff for up to 2 seconds, matching the strategy used
//     by Go's own cmd/internal/robustio package.
//
// # References
//
//   - MoveFileExW: https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw
//   - golang/go#22397: https://github.com/golang/go/issues/22397
//   - google/renameio#1: https://github.com/google/renameio/issues/1
//   - natefinch/atomic: https://github.com/natefinch/atomic
package winrenameio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile mirrors [github.com/google/renameio/v2.WriteFile], atomically
// creating or replacing the file at filename with data.
//
// It writes to a temporary file in the same directory, fsyncs, sets
// permissions, and then atomically replaces the target via [ReplaceFile].
func WriteFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)

	f, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(filename)+"-*")
	if err != nil {
		return fmt.Errorf("winrenameio: creating temp file: %w", err)
	}
	tmpName := f.Name()
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
		_ = os.Remove(tmpName)
	}()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("winrenameio: writing temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("winrenameio: syncing temp file: %w", err)
	}
	if err := f.Chmod(perm); err != nil {
		return fmt.Errorf("winrenameio: setting permissions: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("winrenameio: closing temp file: %w", err)
	}
	closed = true

	if err := ReplaceFile(tmpName, filename); err != nil {
		return fmt.Errorf("winrenameio: replacing file: %w", err)
	}
	return nil
}

// PendingFile is a temporary file waiting to atomically replace a destination
// path. It mirrors [github.com/google/renameio/v2.PendingFile].
//
// Callers write to the embedded [*os.File], then call [PendingFile.Close] to
// atomically replace the target, or [PendingFile.Cleanup] to discard.
type PendingFile struct {
	*os.File

	path    string
	perm    os.FileMode
	done    bool
	replace bool
}

// NewPendingFile mirrors [github.com/google/renameio/v2.NewPendingFile].
//
// It creates a temporary file in the same directory as path. Callers should
// write content to the returned [PendingFile], then call Close to atomically
// replace the target.
//
// Supported options: [WithStaticPermissions], [WithReplaceOnClose].
func NewPendingFile(path string, opts ...Option) (*PendingFile, error) {
	dir := filepath.Dir(path)

	f, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return nil, fmt.Errorf("winrenameio: creating temp file: %w", err)
	}

	pf := &PendingFile{
		File: f,
		path: path,
		perm: 0o600,
	}
	for _, opt := range opts {
		opt.apply(pf)
	}
	return pf, nil
}

// CloseAtomicallyReplace syncs the temporary file, sets permissions, and
// atomically replaces the destination via [ReplaceFile].
//
// After a successful call, [PendingFile.Cleanup] is a no-op.
func (pf *PendingFile) CloseAtomicallyReplace() error {
	if pf.done {
		return fmt.Errorf("winrenameio: already closed")
	}
	pf.done = true

	tmpName := pf.Name()
	closed := false
	success := false
	defer func() {
		if success {
			return
		}
		if !closed {
			_ = pf.File.Close()
		}
		_ = os.Remove(tmpName)
	}()

	if err := pf.Sync(); err != nil {
		return fmt.Errorf("winrenameio: syncing temp file: %w", err)
	}
	if err := pf.Chmod(pf.perm); err != nil {
		return fmt.Errorf("winrenameio: setting permissions: %w", err)
	}
	if err := pf.File.Close(); err != nil {
		return fmt.Errorf("winrenameio: closing temp file: %w", err)
	}
	closed = true

	if err := ReplaceFile(tmpName, pf.path); err != nil {
		return fmt.Errorf("winrenameio: replacing file: %w", err)
	}
	success = true
	return nil
}

// Close closes the PendingFile. If the file was created with
// [WithReplaceOnClose], Close calls [PendingFile.CloseAtomicallyReplace].
// Otherwise it closes the underlying file and removes the temp file without
// replacing the target.
func (pf *PendingFile) Close() error {
	if pf.replace {
		return pf.CloseAtomicallyReplace()
	}
	pf.done = true
	closeErr := pf.File.Close()
	removeErr := os.Remove(pf.Name())
	return errors.Join(closeErr, removeErr)
}

// Cleanup removes the temporary file. It is a no-op if the PendingFile has
// already been closed or committed via [PendingFile.CloseAtomicallyReplace].
func (pf *PendingFile) Cleanup() error {
	if pf.done {
		return nil
	}
	pf.done = true
	_ = pf.File.Close()
	if err := os.Remove(pf.Name()); err != nil {
		return fmt.Errorf("winrenameio: removing temp file: %w", err)
	}
	return nil
}

// Option configures [NewPendingFile].
type Option interface {
	apply(pf *PendingFile)
}

type staticPermissions struct {
	perm os.FileMode
}

func (o staticPermissions) apply(pf *PendingFile) {
	pf.perm = o.perm
}

// WithStaticPermissions sets the file permissions for the target file,
// ignoring the umask. This mirrors
// [github.com/google/renameio/v2.WithStaticPermissions].
func WithStaticPermissions(perm os.FileMode) Option {
	return staticPermissions{perm: perm}
}

type replaceOnClose struct{}

func (replaceOnClose) apply(pf *PendingFile) {
	pf.replace = true
}

// WithReplaceOnClose causes [PendingFile.Close] to call
// [PendingFile.CloseAtomicallyReplace], making Close atomic by default. This
// mirrors [github.com/google/renameio/v2.WithReplaceOnClose].
func WithReplaceOnClose() Option {
	return replaceOnClose{}
}
