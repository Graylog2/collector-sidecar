// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package persistence

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/google/uuid"
)

const instanceUIDFile = "instance_uid.yaml"

// InstanceData represents the persisted instance identity.
type InstanceData struct {
	InstanceUID string    `yaml:"instance_uid"`
	CreatedAt   time.Time `yaml:"created_at"`
}

// LoadOrCreateInstanceUID loads the instance UID from disk, or creates a new one
// if it doesn't exist. The file is created with read-only permissions (0444).
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

	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	// Marshal to YAML
	content, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}

	// Write file with read-only permissions
	if err := os.WriteFile(filePath, content, 0444); err != nil {
		return "", err
	}

	return data.InstanceUID, nil
}

// LoadInstanceData loads the instance data from disk.
func LoadInstanceData(dir string) (*InstanceData, error) {
	filePath := filepath.Join(dir, instanceUIDFile)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var data InstanceData
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	return &data, nil
}
