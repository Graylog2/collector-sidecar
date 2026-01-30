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

//go:build windows

package configwriter

import (
	"io/fs"
	"os"
	"path/filepath"
)

// writeConfigAtomic writes content to path atomically on Windows.
// Windows doesn't support atomic rename over existing files, so we use
// a temp file + rename approach with explicit cleanup.
func writeConfigAtomic(path string, content []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)

	// Create temp file in the same directory for atomic rename
	tempFile, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()

	// Clean up temp file on any error
	defer func() {
		if tempPath != "" {
			os.Remove(tempPath)
		}
	}()

	// Write content to temp file
	if _, err := tempFile.Write(content); err != nil {
		tempFile.Close()
		return err
	}

	// Set permissions before closing
	if err := tempFile.Chmod(perm); err != nil {
		tempFile.Close()
		return err
	}

	// Close the file before rename
	if err := tempFile.Close(); err != nil {
		return err
	}

	// On Windows, we need to remove the target file first if it exists
	// because os.Rename doesn't support atomic overwrite on Windows
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return err
		}
	}

	// Rename temp file to target
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}

	// Rename succeeded, don't remove in defer
	tempPath = ""
	return nil
}
