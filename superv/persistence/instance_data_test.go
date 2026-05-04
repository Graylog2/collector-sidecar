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

	"github.com/Graylog2/collector-sidecar/superv/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndLoadInstanceData(t *testing.T) {
	dir := t.TempDir()
	data := &identity.InstanceData{
		InstanceUID: "test-instance-uid",
		CreatedAt:   time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
	}

	require.NoError(t, SaveInstanceData(dir, data))

	exists, err := InstanceDataExists(dir)
	require.NoError(t, err)
	assert.True(t, exists)

	info, err := os.Stat(filepath.Join(dir, instanceUIDFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o444), info.Mode().Perm())

	loaded, err := LoadInstanceData(dir)
	require.NoError(t, err)
	assert.Equal(t, data.InstanceUID, loaded.InstanceUID)
	assert.True(t, data.CreatedAt.Equal(loaded.CreatedAt))

	uid, err := LoadInstanceUID(dir)
	require.NoError(t, err)
	assert.Equal(t, data.InstanceUID, uid)
}

func TestLoadInstanceData_Missing(t *testing.T) {
	dir := t.TempDir()

	exists, err := InstanceDataExists(dir)
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = LoadInstanceData(dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)

	_, err = LoadInstanceUID(dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestLoadInstanceData_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, instanceUIDFile), []byte("created_at: ["), 0o444))

	_, err := LoadInstanceData(dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshaling instance data")
}

func TestSaveInstanceData_ParentPathIsFile(t *testing.T) {
	dir := t.TempDir()
	blockedPath := filepath.Join(dir, "not-a-dir")
	require.NoError(t, os.WriteFile(blockedPath, []byte("x"), 0o600))

	err := SaveInstanceData(blockedPath, identity.CreateInstanceData())

	require.Error(t, err)
}
