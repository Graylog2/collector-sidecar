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
)

// WriteConfig writes content to the specified path atomically.
// It creates parent directories if they don't exist.
// The write is atomic: content is written to a temp file first, then renamed.
// File permissions are set to 0600.
//
// On Unix systems, this uses github.com/google/renameio/v2 for safe atomic writes.
// On Windows, this uses a custom implementation with temp file + rename.
func WriteConfig(path string, content []byte) error {
	// Create parent directories if they don't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return writeConfigAtomic(path, content, 0600)
}

// ReadConfig reads the content of a config file.
func ReadConfig(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// ConfigExists checks if a config file exists at the specified path.
func ConfigExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
