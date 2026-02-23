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

	"github.com/Graylog2/collector-sidecar/superv/configmerge"
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

func TestApplyRemoteConfig_CreatesBackupFile(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config", "collector.yaml")

	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  filepath.Join(dir, "config"),
		OutputPath: outputPath,
	})

	// First apply — no backup expected (nothing to back up)
	cfg1 := createTestRemoteConfig("collector.yaml", []byte("receivers:\n  otlp:\n"), []byte("hash1"))
	result1, err := mgr.ApplyRemoteConfig(context.Background(), cfg1)
	require.NoError(t, err)
	require.True(t, result1.Changed)

	_, err = os.Stat(outputPath + ".prev")
	require.ErrorIs(t, err, os.ErrNotExist, "no backup on first apply")

	// Second apply — backup of first config expected
	cfg2 := createTestRemoteConfig("collector.yaml", []byte("receivers:\n  otlp:\n  filelog:\n"), []byte("hash2"))
	result2, err := mgr.ApplyRemoteConfig(context.Background(), cfg2)
	require.NoError(t, err)
	require.True(t, result2.Changed)

	bakContent, err := os.ReadFile(outputPath + ".prev")
	require.NoError(t, err)
	require.Contains(t, string(bakContent), "otlp")
	require.NotContains(t, string(bakContent), "filelog")
}

func TestApplyRemoteConfig_PreviousHashTracked(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config", "collector.yaml")

	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  filepath.Join(dir, "config"),
		OutputPath: outputPath,
	})

	// Apply first config
	cfg1 := createTestRemoteConfig("collector.yaml", []byte("receivers:\n  otlp:\n"), []byte("hash1"))
	_, err := mgr.ApplyRemoteConfig(context.Background(), cfg1)
	require.NoError(t, err)
	assert.Nil(t, mgr.previousHash, "previousHash should be nil after first apply")
	assert.Equal(t, []byte("hash1"), mgr.lastHash)

	// Apply second config
	cfg2 := createTestRemoteConfig("collector.yaml", []byte("receivers:\n  filelog:\n"), []byte("hash2"))
	_, err = mgr.ApplyRemoteConfig(context.Background(), cfg2)
	require.NoError(t, err)
	assert.Equal(t, []byte("hash1"), mgr.previousHash, "previousHash should be hash1 after second apply")
	assert.Equal(t, []byte("hash2"), mgr.lastHash)
}

func TestRollbackConfig_RestoresPreviousConfig(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config", "collector.yaml")

	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  filepath.Join(dir, "config"),
		OutputPath: outputPath,
	})

	// Apply two configs so we have a .prev
	cfg1 := createTestRemoteConfig("collector.yaml", []byte("version: 1\n"), []byte("hash1"))
	_, err := mgr.ApplyRemoteConfig(context.Background(), cfg1)
	require.NoError(t, err)

	cfg2 := createTestRemoteConfig("collector.yaml", []byte("version: 2\n"), []byte("hash2"))
	_, err = mgr.ApplyRemoteConfig(context.Background(), cfg2)
	require.NoError(t, err)

	// Rollback
	err = mgr.RollbackConfig()
	require.NoError(t, err)

	// Output should contain version 1 config
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	require.Contains(t, string(content), "version: 1")

	// .prev should be removed
	_, err = os.Stat(outputPath + ".prev")
	require.ErrorIs(t, err, os.ErrNotExist)

	// Hash should be reset so next apply with hash1 is not skipped
	require.Equal(t, []byte("hash1"), mgr.GetConfigHash())
}

func TestRollbackConfig_ErrorsWhenNoBackup(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config", "collector.yaml")

	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  filepath.Join(dir, "config"),
		OutputPath: outputPath,
	})

	err := mgr.RollbackConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no backup")
}

func TestSaveAndLoadRemoteConfigStatus(t *testing.T) {
	dir := t.TempDir()
	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  dir,
		OutputPath: filepath.Join(dir, "collector.yaml"),
	})

	hash := []byte("abc123")
	err := mgr.SaveRemoteConfigStatus(
		protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
		"",
		hash,
	)
	require.NoError(t, err)

	status, err := mgr.LoadRemoteConfigStatus()
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED, status.GetStatus())
	require.Equal(t, hash, status.GetLastRemoteConfigHash())
	require.Empty(t, status.GetErrorMessage())
}

func TestSaveAndLoadRemoteConfigStatus_Failed(t *testing.T) {
	dir := t.TempDir()
	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  dir,
		OutputPath: filepath.Join(dir, "collector.yaml"),
	})

	hash := []byte("abc123")
	err := mgr.SaveRemoteConfigStatus(
		protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED,
		"restart failed: exit code 1",
		hash,
	)
	require.NoError(t, err)

	status, err := mgr.LoadRemoteConfigStatus()
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED, status.GetStatus())
	require.Equal(t, "restart failed: exit code 1", status.GetErrorMessage())
	require.Equal(t, hash, status.GetLastRemoteConfigHash())
}

func TestLoadRemoteConfigStatus_NoFile(t *testing.T) {
	dir := t.TempDir()
	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  dir,
		OutputPath: filepath.Join(dir, "collector.yaml"),
	})

	status, err := mgr.LoadRemoteConfigStatus()
	require.NoError(t, err)
	require.Nil(t, status)
}

func TestOutputPath(t *testing.T) {
	mgr := New(zaptest.NewLogger(t), Config{
		OutputPath: "/var/lib/superv/config/collector.yaml",
	})
	require.Equal(t, "/var/lib/superv/config/collector.yaml", mgr.OutputPath())
}

func TestApplyRemoteConfig_InjectsHealthCheckExtension(t *testing.T) {
	dir := t.TempDir()
	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:  filepath.Join(dir, "config"),
		OutputPath: filepath.Join(dir, "config", "collector.yaml"),
		HealthCheck: configmerge.HealthCheckConfig{
			Endpoint: "localhost:13133",
			Path:     "/health",
		},
	})

	cfg := createTestRemoteConfig("collector.yaml", []byte("receivers:\n  otlp:\n"), []byte("hash1"))
	result, err := mgr.ApplyRemoteConfig(context.Background(), cfg)
	require.NoError(t, err)
	require.True(t, result.Changed)

	// Effective config should contain the health_check extension with correct settings
	require.Contains(t, string(result.EffectiveConfig), "health_check")
	require.Contains(t, string(result.EffectiveConfig), "localhost:13133")
	require.Contains(t, string(result.EffectiveConfig), "/health")
}

func TestSetLocalEndpoint(t *testing.T) {
	mgr := New(zaptest.NewLogger(t), Config{
		LocalEndpoint: "localhost:0",
	})

	mgr.SetLocalEndpoint("ws://127.0.0.1:54321/v1/opamp")
	assert.Equal(t, "ws://127.0.0.1:54321/v1/opamp", mgr.cfg.LocalEndpoint)
}

func TestEnsureBootstrapConfig_WritesWhenMissing(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "config", "collector.yaml")

	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:     filepath.Join(dir, "config"),
		OutputPath:    outputPath,
		LocalEndpoint: "ws://127.0.0.1:54321/v1/opamp",
		InstanceUID:   "test-uid-bootstrap",
		HealthCheck: configmerge.HealthCheckConfig{
			Endpoint: "localhost:13133",
			Path:     "/health",
		},
	})

	err := mgr.EnsureBootstrapConfig()
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "opamp")
	assert.Contains(t, s, "ws://127.0.0.1:54321/v1/opamp")
	assert.Contains(t, s, "test-uid-bootstrap")
	assert.Contains(t, s, "health_check")
	assert.Contains(t, s, "localhost:13133")

	// Bootstrap includes a nop pipeline so the collector accepts the config
	assert.Contains(t, s, "nop")
	assert.Contains(t, s, "logs/bootstrap")
}

func TestEnsureBootstrapConfig_ReinjectsExtensionsWhenExists(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	outputPath := filepath.Join(configDir, "collector.yaml")

	require.NoError(t, os.MkdirAll(configDir, 0o755))
	// Cached config with an old OpAMP endpoint and a real pipeline (simulates port change on restart)
	existing := []byte("extensions:\n  opamp:\n    server:\n      ws:\n        endpoint: ws://127.0.0.1:11111/v1/opamp\n    instance_uid: test-uid\nreceivers:\n  otlp:\n    protocols:\n      grpc: {}\nexporters:\n  debug: {}\nservice:\n  extensions:\n    - opamp\n  pipelines:\n    logs:\n      receivers:\n        - otlp\n      exporters:\n        - debug\n")
	require.NoError(t, os.WriteFile(outputPath, existing, 0o600))

	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:     configDir,
		OutputPath:    outputPath,
		LocalEndpoint: "ws://127.0.0.1:22222/v1/opamp",
		InstanceUID:   "test-uid",
		HealthCheck: configmerge.HealthCheckConfig{
			Endpoint: "localhost:13133",
		},
	})

	err := mgr.EnsureBootstrapConfig()
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	s := string(content)
	// Should have the NEW endpoint, not the old one
	assert.Contains(t, s, "ws://127.0.0.1:22222/v1/opamp")
	assert.NotContains(t, s, "ws://127.0.0.1:11111/v1/opamp")
	// Should also have health_check injected
	assert.Contains(t, s, "health_check")
	assert.Contains(t, s, "localhost:13133")
	// Cached config should NOT get a nop bootstrap pipeline
	assert.NotContains(t, s, "logs/bootstrap")
}

func TestEnsureBootstrapConfig_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	// Nested dir that doesn't exist yet
	outputPath := filepath.Join(dir, "deep", "nested", "collector.yaml")

	mgr := New(zaptest.NewLogger(t), Config{
		ConfigDir:     filepath.Join(dir, "deep", "nested"),
		OutputPath:    outputPath,
		LocalEndpoint: "ws://127.0.0.1:9999/v1/opamp",
		InstanceUID:   "test-uid-dir",
		HealthCheck: configmerge.HealthCheckConfig{
			Endpoint: "localhost:13133",
		},
	})

	err := mgr.EnsureBootstrapConfig()
	require.NoError(t, err)

	_, err = os.Stat(outputPath)
	require.NoError(t, err)
}
