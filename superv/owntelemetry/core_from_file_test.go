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

package owntelemetry

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestNewCoreFromFile_NoFile(t *testing.T) {
	dir := t.TempDir()
	res := BuildResource("collector", "1.0.0", "test-instance", "collector_log")

	core, shutdown, err := NewCoreFromFile(dir, "", "", res)

	require.NoError(t, err)
	assert.Nil(t, core)
	assert.Nil(t, shutdown)
}

func TestNewCoreFromFile_ValidFile(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeTestClientCert(t)

	// Write a valid own-logs.yaml via Persistence
	p := NewPersistence(dir, "own-logs.yaml", certPath, keyPath)
	err := p.Save(Settings{
		Endpoint: "https://example.com:4318/v1/logs",
		Headers:  map[string]string{"Authorization": "Bearer tok"},
	})
	require.NoError(t, err)

	res := BuildResource("collector", "1.0.0", "test-instance", "collector_log")

	core, shutdown, err := NewCoreFromFile(dir, certPath, keyPath, res)

	require.NoError(t, err)
	require.NotNil(t, core)
	require.NotNil(t, shutdown)

	// Core should be enabled at info level
	assert.True(t, core.Enabled(zapcore.InfoLevel))

	// Shutdown should not panic
	shutdown(context.Background())
}

func TestNewCoreFromFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	// Write invalid YAML to own-logs.yaml
	err := os.WriteFile(filepath.Join(dir, "own-logs.yaml"), []byte(":::invalid yaml"), 0o644)
	require.NoError(t, err)

	res := BuildResource("collector", "1.0.0", "test-instance", "collector_log")

	core, shutdown, err := NewCoreFromFile(dir, "", "", res)

	require.Error(t, err)
	assert.Nil(t, core)
	assert.Nil(t, shutdown)
}

func TestNewCoreFromFile_ShutdownIdempotent(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeTestClientCert(t)

	p := NewPersistence(dir, "own-logs.yaml", certPath, keyPath)
	err := p.Save(Settings{
		Endpoint: "https://example.com:4318/v1/logs",
	})
	require.NoError(t, err)

	res := BuildResource("collector", "1.0.0", "test-instance", "collector_log")
	_, shutdown, err := NewCoreFromFile(dir, certPath, keyPath, res)
	require.NoError(t, err)
	require.NotNil(t, shutdown)

	// Calling shutdown twice should not panic
	shutdown(context.Background())
	shutdown(context.Background())
}
