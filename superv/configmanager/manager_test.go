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

package configmanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func createTestRemoteConfig(configName string, configBody []byte, hash []byte) *protobufs.AgentRemoteConfig {
	return &protobufs.AgentRemoteConfig{
		Config: &protobufs.AgentConfigMap{
			ConfigMap: map[string]*protobufs.AgentConfigFile{
				configName: {
					Body:        configBody,
					ContentType: "text/yaml",
				},
			},
		},
		ConfigHash: hash,
	}
}

func TestConfigManager_ApplyRemoteConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "effective.yaml")

	cfg := Config{
		ConfigDir:     tmpDir,
		OutputPath:    outputPath,
		LocalEndpoint: "ws://localhost:4320/v1/opamp",
		InstanceUID:   "test-instance-123",
	}

	mgr := New(logger, cfg)

	// Create a simple collector config
	remoteConfig := []byte(`receivers:
  otlp:
    protocols:
      grpc:

exporters:
  debug:

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
`)

	remote := createTestRemoteConfig("collector.yaml", remoteConfig, []byte("hash1"))

	// Apply the config
	result, err := mgr.ApplyRemoteConfig(context.Background(), remote)
	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.NotEmpty(t, result.EffectiveConfig)
	assert.Equal(t, []byte("hash1"), result.ConfigHash)

	// Verify the file was written
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotEmpty(t, content)

	// Verify OpAMP extension was injected
	assert.Contains(t, string(content), "opamp")
	assert.Contains(t, string(content), "ws://localhost:4320/v1/opamp")
	assert.Contains(t, string(content), "test-instance-123")

	// Apply the same config again - should return Changed=false
	result2, err := mgr.ApplyRemoteConfig(context.Background(), remote)
	require.NoError(t, err)
	assert.False(t, result2.Changed)
	assert.Equal(t, []byte("hash1"), result2.ConfigHash)

	// Apply a new config with different hash
	remote2 := createTestRemoteConfig("collector.yaml", remoteConfig, []byte("hash2"))
	result3, err := mgr.ApplyRemoteConfig(context.Background(), remote2)
	require.NoError(t, err)
	assert.True(t, result3.Changed)
	assert.Equal(t, []byte("hash2"), result3.ConfigHash)
}

func TestConfigManager_ApplyRemoteConfig_WithLocalOverrides(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "effective.yaml")

	// Create a local override file
	overridePath := filepath.Join(tmpDir, "override.yaml")
	overrideContent := []byte(`exporters:
  otlphttp:
    endpoint: "http://localhost:4318"

service:
  pipelines:
    traces:
      exporters: [otlphttp]
`)
	err := os.WriteFile(overridePath, overrideContent, 0644)
	require.NoError(t, err)

	cfg := Config{
		ConfigDir:      tmpDir,
		OutputPath:     outputPath,
		LocalOverrides: []string{overridePath},
		LocalEndpoint:  "ws://localhost:4320/v1/opamp",
		InstanceUID:    "test-instance-456",
	}

	mgr := New(logger, cfg)

	// Create a base collector config
	remoteConfig := []byte(`receivers:
  otlp:
    protocols:
      grpc:

exporters:
  debug:

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
`)

	remote := createTestRemoteConfig("collector.yaml", remoteConfig, []byte("hash-override"))

	// Apply the config
	result, err := mgr.ApplyRemoteConfig(context.Background(), remote)
	require.NoError(t, err)
	assert.True(t, result.Changed)

	// Verify the file was written
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	// Verify the override was applied
	assert.Contains(t, string(content), "otlphttp")
	assert.Contains(t, string(content), "http://localhost:4318")

	// Verify OpAMP extension was still injected
	assert.Contains(t, string(content), "opamp")
	assert.Contains(t, string(content), "ws://localhost:4320/v1/opamp")
}

func TestConfigManager_ApplyRemoteConfig_MultipleOverrides(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "effective.yaml")

	// Create first override file
	override1Path := filepath.Join(tmpDir, "override1.yaml")
	override1Content := []byte(`exporters:
  otlphttp:
    endpoint: "http://override1:4318"
`)
	err := os.WriteFile(override1Path, override1Content, 0644)
	require.NoError(t, err)

	// Create second override file (should override the first)
	override2Path := filepath.Join(tmpDir, "override2.yaml")
	override2Content := []byte(`exporters:
  otlphttp:
    endpoint: "http://override2:4318"
`)
	err = os.WriteFile(override2Path, override2Content, 0644)
	require.NoError(t, err)

	cfg := Config{
		ConfigDir:      tmpDir,
		OutputPath:     outputPath,
		LocalOverrides: []string{override1Path, override2Path},
		LocalEndpoint:  "ws://localhost:4320/v1/opamp",
		InstanceUID:    "test-instance-789",
	}

	mgr := New(logger, cfg)

	remoteConfig := []byte(`receivers:
  otlp:
    protocols:
      grpc:

service:
  pipelines:
    traces:
      receivers: [otlp]
`)

	remote := createTestRemoteConfig("collector.yaml", remoteConfig, []byte("hash-multi"))

	result, err := mgr.ApplyRemoteConfig(context.Background(), remote)
	require.NoError(t, err)
	assert.True(t, result.Changed)

	// Verify the second override took precedence
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "http://override2:4318")
}

func TestConfigManager_ApplyRemoteConfig_MissingOverrideIgnored(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "effective.yaml")

	// Reference a non-existent override file
	cfg := Config{
		ConfigDir:      tmpDir,
		OutputPath:     outputPath,
		LocalOverrides: []string{filepath.Join(tmpDir, "nonexistent.yaml")},
		LocalEndpoint:  "ws://localhost:4320/v1/opamp",
		InstanceUID:    "test-instance-missing",
	}

	mgr := New(logger, cfg)

	remoteConfig := []byte(`receivers:
  otlp:
    protocols:
      grpc:

service:
  pipelines:
    traces:
      receivers: [otlp]
`)

	remote := createTestRemoteConfig("collector.yaml", remoteConfig, []byte("hash-missing"))

	// Should succeed even with missing override file
	result, err := mgr.ApplyRemoteConfig(context.Background(), remote)
	require.NoError(t, err)
	assert.True(t, result.Changed)
}

func TestConfigManager_ApplyRemoteConfig_EmptyKeyConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "effective.yaml")

	cfg := Config{
		ConfigDir:     tmpDir,
		OutputPath:    outputPath,
		LocalEndpoint: "ws://localhost:4320/v1/opamp",
		InstanceUID:   "test-instance-empty-key",
	}

	mgr := New(logger, cfg)

	remoteConfig := []byte(`receivers:
  otlp:

service:
  pipelines:
    traces:
      receivers: [otlp]
`)

	// Use empty string key - this should now fail as we require "collector.yaml"
	remote := createTestRemoteConfig("", remoteConfig, []byte("hash-empty-key"))

	result, err := mgr.ApplyRemoteConfig(context.Background(), remote)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "collector.yaml")
}

func TestConfigManager_ApplyRemoteConfig_NilRemoteConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tmpDir := t.TempDir()
	cfg := Config{
		ConfigDir:  tmpDir,
		OutputPath: filepath.Join(tmpDir, "effective.yaml"),
	}

	mgr := New(logger, cfg)

	// Apply nil config
	result, err := mgr.ApplyRemoteConfig(context.Background(), nil)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil")
}

func TestConfigManager_ApplyRemoteConfig_EmptyConfigMap(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tmpDir := t.TempDir()
	cfg := Config{
		ConfigDir:  tmpDir,
		OutputPath: filepath.Join(tmpDir, "effective.yaml"),
	}

	mgr := New(logger, cfg)

	// Apply config with empty ConfigMap
	remote := &protobufs.AgentRemoteConfig{
		Config: &protobufs.AgentConfigMap{
			ConfigMap: map[string]*protobufs.AgentConfigFile{},
		},
		ConfigHash: []byte("hash-empty"),
	}

	result, err := mgr.ApplyRemoteConfig(context.Background(), remote)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "collector.yaml")
}

func TestConfigManager_ApplyRemoteConfig_NoOpAMPInjection(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "effective.yaml")

	// Don't set LocalEndpoint and InstanceUID
	cfg := Config{
		ConfigDir:  tmpDir,
		OutputPath: outputPath,
	}

	mgr := New(logger, cfg)

	remoteConfig := []byte(`receivers:
  otlp:

service:
  pipelines:
    traces:
      receivers: [otlp]
`)

	remote := createTestRemoteConfig("collector.yaml", remoteConfig, []byte("hash-no-opamp"))

	result, err := mgr.ApplyRemoteConfig(context.Background(), remote)
	require.NoError(t, err)
	assert.True(t, result.Changed)

	// OpAMP extension should NOT be injected
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotContains(t, string(content), "opamp")
}

func TestConfigManager_GetEffectiveConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "effective.yaml")

	cfg := Config{
		ConfigDir:     tmpDir,
		OutputPath:    outputPath,
		LocalEndpoint: "ws://localhost:4320/v1/opamp",
		InstanceUID:   "test-instance-get",
	}

	mgr := New(logger, cfg)

	// Initially, GetEffectiveConfig should fail (file doesn't exist)
	_, err := mgr.GetEffectiveConfig()
	assert.Error(t, err)

	// Apply a config first
	remoteConfig := []byte(`receivers:
  otlp:

service:
  pipelines:
    traces:
      receivers: [otlp]
`)

	remote := createTestRemoteConfig("collector.yaml", remoteConfig, []byte("hash-get"))
	_, err = mgr.ApplyRemoteConfig(context.Background(), remote)
	require.NoError(t, err)

	// Now GetEffectiveConfig should succeed
	content, err := mgr.GetEffectiveConfig()
	require.NoError(t, err)
	assert.NotEmpty(t, content)
	assert.Contains(t, string(content), "otlp")
	assert.Contains(t, string(content), "opamp")
}

func TestConfigManager_GetEffectiveConfig_EmptyOutputPath(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := Config{
		OutputPath: "",
	}

	mgr := New(logger, cfg)

	_, err := mgr.GetEffectiveConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output path not configured")
}

func TestConfigManager_GetConfigHash(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tmpDir := t.TempDir()
	cfg := Config{
		ConfigDir:     tmpDir,
		OutputPath:    filepath.Join(tmpDir, "effective.yaml"),
		LocalEndpoint: "ws://localhost:4320/v1/opamp",
		InstanceUID:   "test-instance-hash",
	}

	mgr := New(logger, cfg)

	// Initially, GetConfigHash should return nil
	assert.Nil(t, mgr.GetConfigHash())

	// Apply a config
	remoteConfig := []byte(`receivers:
  otlp:
`)
	remote := createTestRemoteConfig("collector.yaml", remoteConfig, []byte("my-hash-123"))
	_, err := mgr.ApplyRemoteConfig(context.Background(), remote)
	require.NoError(t, err)

	// Now GetConfigHash should return the hash
	hash := mgr.GetConfigHash()
	assert.Equal(t, []byte("my-hash-123"), hash)
}

func TestConfigManager_StoresRemoteConfigs(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tmpDir := t.TempDir()
	cfg := Config{
		ConfigDir:     tmpDir,
		OutputPath:    filepath.Join(tmpDir, "effective.yaml"),
		LocalEndpoint: "ws://localhost:4320/v1/opamp",
		InstanceUID:   "test-instance-store",
	}

	mgr := New(logger, cfg)

	remoteConfig := []byte(`receivers:
  otlp:
`)

	remote := createTestRemoteConfig("collector.yaml", remoteConfig, []byte("hash-store"))
	_, err := mgr.ApplyRemoteConfig(context.Background(), remote)
	require.NoError(t, err)

	// Check that the remote config was stored
	storedPath := filepath.Join(tmpDir, "remote", "collector.yaml")
	content, err := os.ReadFile(storedPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "otlp")
}

func TestNew(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		ConfigDir:      "/tmp/test",
		OutputPath:     "/tmp/test/effective.yaml",
		LocalOverrides: []string{"/tmp/override.yaml"},
		LocalEndpoint:  "ws://localhost:4320/v1/opamp",
		InstanceUID:    "test-uid",
	}

	mgr := New(logger, cfg)

	assert.NotNil(t, mgr)
	assert.Equal(t, logger, mgr.logger)
	assert.Equal(t, cfg, mgr.cfg)
	assert.Nil(t, mgr.lastHash)
}
