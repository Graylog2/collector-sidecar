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

func TestSaveAndLoadConnectionState(t *testing.T) {
	dir := t.TempDir()

	state := &ConnectionState{
		Server: ServerState{
			Endpoint:        "wss://opamp.example.com/v1/opamp",
			LastConnected:   time.Now().UTC().Truncate(time.Second),
			LastSequenceNum: 42,
		},
		RemoteConfig: RemoteConfigState{
			Hash:       "sha256:abc123",
			ReceivedAt: time.Now().UTC().Truncate(time.Second),
			Status:     "APPLIED",
		},
	}

	err := SaveConnectionState(dir, state)
	require.NoError(t, err)

	loaded, err := LoadConnectionState(dir)
	require.NoError(t, err)
	require.Equal(t, state.Server.Endpoint, loaded.Server.Endpoint)
	require.Equal(t, state.Server.LastSequenceNum, loaded.Server.LastSequenceNum)
	require.Equal(t, state.RemoteConfig.Hash, loaded.RemoteConfig.Hash)
	require.Equal(t, state.RemoteConfig.Status, loaded.RemoteConfig.Status)
}

func TestLoadConnectionState_NotExists(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadConnectionState(dir)
	require.Error(t, err)
}

func TestConnectionState_FilePermissions(t *testing.T) {
	dir := t.TempDir()

	state := &ConnectionState{
		Server: ServerState{
			Endpoint: "wss://opamp.example.com/v1/opamp",
		},
	}

	err := SaveConnectionState(dir, state)
	require.NoError(t, err)

	filePath := filepath.Join(dir, "connection.yaml")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}
