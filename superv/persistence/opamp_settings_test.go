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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpAMPSettings_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	settings := &OpAMPSettings{
		Endpoint:          "wss://server.example.com:4320/v1/opamp",
		Headers:           map[string]string{"X-Custom": "value"},
		CACertPEM:         "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
		ClientCertPEM:     "-----BEGIN CERTIFICATE-----\nclient\n-----END CERTIFICATE-----",
		ClientKeyPEM:      "-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----",
		ProxyURL:          "http://proxy.example.com:8080",
		HeartbeatInterval: 45 * time.Second,
		UpdatedAt:         time.Now().UTC().Truncate(time.Second),
	}

	err := SaveOpAMPSettings(dir, settings)
	require.NoError(t, err)

	loaded, err := LoadOpAMPSettings(dir)
	require.NoError(t, err)
	require.Equal(t, settings.Endpoint, loaded.Endpoint)
	require.Equal(t, settings.Headers, loaded.Headers)
	require.Equal(t, settings.CACertPEM, loaded.CACertPEM)
	require.Equal(t, settings.ClientCertPEM, loaded.ClientCertPEM)
	require.Equal(t, settings.ClientKeyPEM, loaded.ClientKeyPEM)
	require.Equal(t, settings.ProxyURL, loaded.ProxyURL)
	require.Equal(t, settings.HeartbeatInterval, loaded.HeartbeatInterval)
}

func TestOpAMPSettings_LoadNonExistent(t *testing.T) {
	dir := t.TempDir()

	settings, err := LoadOpAMPSettings(dir)
	require.NoError(t, err)
	require.Nil(t, settings, "should return nil for non-existent settings")
}

func TestOpAMPSettings_FilePermissions(t *testing.T) {
	dir := t.TempDir()

	settings := &OpAMPSettings{
		Endpoint:     "wss://server.example.com:4320/v1/opamp",
		ClientKeyPEM: "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----",
	}

	err := SaveOpAMPSettings(dir, settings)
	require.NoError(t, err)

	// Verify file has restricted permissions (contains private key)
	info, err := os.Stat(filepath.Join(dir, opampSettingsFile))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm(),
		"settings file should have 0600 permissions")
}
