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
	"fmt"
	"os"
	"path/filepath"

	"github.com/Graylog2/collector-sidecar/superv/identity"
	"github.com/goccy/go-yaml"
)

const instanceUIDFile = "identity.yaml"

// SaveInstanceData saves the instance data to disk.
func SaveInstanceData(dir string, data *identity.InstanceData) error {
	filePath := filepath.Join(dir, instanceUIDFile)

	// Marshal to YAML
	content, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling instance data: %w", err)
	}

	// Write file with read-only permissions
	if err := WriteFile(filePath, content, 0o444); err != nil {
		return err
	}

	return nil
}

// LoadInstanceData loads the instance data from disk.
func LoadInstanceData(dir string) (*identity.InstanceData, error) {
	filePath := filepath.Join(dir, instanceUIDFile)

	content, err := os.ReadFile(filePath) //nolint:gosec // Trusted path
	if err != nil {
		return nil, fmt.Errorf("reading instance file: %w", err)
	}

	var data identity.InstanceData
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("unmarshaling instance data: %w", err)
	}

	return &data, nil
}

// LoadInstanceUID loads the instance data from disk and returns the instance UID.
func LoadInstanceUID(dir string) (string, error) {
	data, err := LoadInstanceData(dir)
	if err != nil {
		return "", err
	}
	return data.InstanceUID, nil
}

// InstanceDataExists returns true if the instance data exists on disk.
func InstanceDataExists(dir string) (bool, error) {
	if _, err := os.Stat(filepath.Join(dir, instanceUIDFile)); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("couldn't stat instance data: %w", err)
	}
	return true, nil
}
