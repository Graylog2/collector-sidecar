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

	"github.com/Graylog2/collector-sidecar/superv/configwriter"
)

const opampSettingsFile = "opamp_settings.yaml"

// OpAMPSettings holds connection settings received from the OpAMP server.
type OpAMPSettings struct {
	Endpoint          string            `yaml:"endpoint,omitempty"`
	Headers           map[string]string `yaml:"headers,omitempty"`
	CACertPEM         string            `yaml:"ca_cert_pem,omitempty"`
	ClientCertPEM     string            `yaml:"client_cert_pem,omitempty"`
	ClientKeyPEM      string            `yaml:"client_key_pem,omitempty"`
	ProxyURL          string            `yaml:"proxy_url,omitempty"`
	HeartbeatInterval time.Duration     `yaml:"heartbeat_interval,omitempty"`
	UpdatedAt         time.Time         `yaml:"updated_at"`
}

// SaveOpAMPSettings persists OpAMP connection settings to disk.
// File is written atomically with 0600 permissions as it may contain private keys.
func SaveOpAMPSettings(dir string, settings *OpAMPSettings) error {
	if settings == nil {
		return errors.New("settings cannot be nil")
	}

	data, err := yaml.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal opamp settings: %w", err)
	}

	path := filepath.Join(dir, opampSettingsFile)
	if err := configwriter.WriteConfig(path, data); err != nil {
		return fmt.Errorf("write opamp settings: %w", err)
	}

	return nil
}

// LoadOpAMPSettings loads OpAMP connection settings from disk.
// Returns nil, nil if the file does not exist.
func LoadOpAMPSettings(dir string) (*OpAMPSettings, error) {
	path := filepath.Join(dir, opampSettingsFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read opamp settings: %w", err)
	}

	var settings OpAMPSettings
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("unmarshal opamp settings: %w", err)
	}

	return &settings, nil
}

// DeleteOpAMPSettings removes the persisted settings file.
func DeleteOpAMPSettings(dir string) error {
	path := filepath.Join(dir, opampSettingsFile)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete opamp settings: %w", err)
	}
	return nil
}
