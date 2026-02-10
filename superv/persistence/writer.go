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
	"fmt"
	"os"
	"path/filepath"
)

// StagedFile represents a file that has been staged for atomic writing.
type StagedFile interface {
	// Commit finalizes the staged file, making it visible at the target path.
	Commit() error
	// Cleanup removes any temporary files associated with the staged file.
	Cleanup() error
	// SetCommitCallback sets a callback function to be called after a successful commit.
	SetCommitCallback(commitCallback func() error)
}

// WriteFile writes content to the specified path atomically.
// It creates parent directories if they don't exist.
// Content is written to a temp file, fsynced, and renamed over the target.
// File permissions are set to 0600.
//
// On Unix, this uses github.com/google/renameio/v2.
// On Windows, this uses the winrenameio package (MoveFileExW with MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH).
func WriteFile(path string, content []byte) error {
	if err := ensurePath(path, 0700); err != nil {
		return err
	}

	return writeFileAtomic(path, content, 0600)
}

func StageFile(path string, content []byte) (StagedFile, error) {
	if err := ensurePath(path, 0700); err != nil {
		return nil, err
	}

	staged, err := newStagedFile(path, content, 0600)
	if err != nil {
		return nil, err
	}

	return staged, nil
}

func ensurePath(path string, perm os.FileMode) error {
	// Create parent directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(path), perm); err != nil {
		return fmt.Errorf("creating parent directories: %w", err)
	}
	return nil
}
