// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package persistence

import (
	"os"
	"path/filepath"
	"time"

	"github.com/goccy/go-yaml"
)

const connectionFile = "connection.yaml"

// ConnectionState represents the persisted connection state.
type ConnectionState struct {
	Server       ServerState       `yaml:"server"`
	RemoteConfig RemoteConfigState `yaml:"remote_config"`
}

// ServerState represents the persisted server connection state.
type ServerState struct {
	Endpoint        string    `yaml:"endpoint"`
	LastConnected   time.Time `yaml:"last_connected"`
	LastSequenceNum uint64    `yaml:"last_sequence_num"`
}

// RemoteConfigState represents the persisted remote config state.
type RemoteConfigState struct {
	Hash       string    `yaml:"hash"`
	ReceivedAt time.Time `yaml:"received_at"`
	Status     string    `yaml:"status"`
	Error      string    `yaml:"error,omitempty"`
}

// SaveConnectionState saves the connection state to disk.
func SaveConnectionState(dir string, state *ConnectionState) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	content, err := yaml.Marshal(state)
	if err != nil {
		return err
	}

	filePath := filepath.Join(dir, connectionFile)
	return os.WriteFile(filePath, content, 0600)
}

// LoadConnectionState loads the connection state from disk.
func LoadConnectionState(dir string) (*ConnectionState, error) {
	filePath := filepath.Join(dir, connectionFile)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var state ConnectionState
	if err := yaml.Unmarshal(content, &state); err != nil {
		return nil, err
	}

	return &state, nil
}
