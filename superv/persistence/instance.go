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
	"time"

	"github.com/goccy/go-yaml"
	"github.com/google/uuid"
)

const instanceUIDFile = "identity.yaml"

// InstanceData represents the persisted instance identity.
type InstanceData struct {
	InstanceUID string    `yaml:"instance_uid"`
	CreatedAt   time.Time `yaml:"created_at"`
}

// LoadOrCreateInstanceUID loads the instance UID from disk, or creates a new one
// if it doesn't exist. The file is created with read-only permissions (0o444).
func LoadOrCreateInstanceUID(dir string) (string, error) {
	filePath := filepath.Join(dir, instanceUIDFile)

	// Try to load existing
	data, err := LoadInstanceData(dir)
	if err == nil {
		return data.InstanceUID, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	// Create new instance
	data = &InstanceData{
		InstanceUID: uuid.New().String(),
		CreatedAt:   time.Now().UTC(),
	}

	// Marshal to YAML
	content, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}

	// Write file with read-only permissions
	if err := WriteFile(filePath, content, 0o444); err != nil {
		return "", err
	}

	return data.InstanceUID, nil
}

// LoadInstanceData loads the instance data from disk.
func LoadInstanceData(dir string) (*InstanceData, error) {
	filePath := filepath.Join(dir, instanceUIDFile)

	// TODO: Remove legacy identity path handling!
	legacyFilePath := filepath.Join(dir, "instance_uid.yaml")
	if _, err := os.Stat(legacyFilePath); err == nil {
		if err := os.Rename(legacyFilePath, filePath); err != nil {
			return nil, fmt.Errorf("rename legacy file failed: %w", err)
		}
	}

	content, err := os.ReadFile(filePath) //nolint:gosec // Trusted path
	if err != nil {
		return nil, err
	}

	var data InstanceData
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	return &data, nil
}
