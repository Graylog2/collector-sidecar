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
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"

	"github.com/Graylog2/collector-sidecar/superv/configmerge"
	"github.com/Graylog2/collector-sidecar/superv/persistence"
)

// Config holds the configuration for the config manager.
type Config struct {
	ConfigDir      string                        // Directory to store remote configs
	OutputPath     string                        // Path to write final merged config
	LocalOverrides []string                      // Paths to local override files
	LocalEndpoint  string                        // Local OpAMP server endpoint for injection
	InstanceUID    string                        // Instance UID for injection
	HealthCheck    configmerge.HealthCheckConfig // Health check extension injection settings
}

// ApplyResult contains the result of applying a remote config.
type ApplyResult struct {
	Changed         bool   // Whether the config changed
	EffectiveConfig []byte // The final merged config
	ConfigHash      []byte // Hash of the applied config
}

// Manager handles remote configuration from OpAMP server.
type Manager struct {
	logger       *zap.Logger
	cfg          Config
	lastHash     []byte
	previousHash []byte
}

// New creates a new config manager.
func New(logger *zap.Logger, cfg Config) *Manager {
	return &Manager{
		logger: logger,
		cfg:    cfg,
	}
}

// ApplyRemoteConfig applies the remote configuration from the OpAMP server.
// It extracts collector.yaml from the remote ConfigMap, merges with local overrides,
// injects OpAMP extension, and writes the result to OutputPath.
func (m *Manager) ApplyRemoteConfig(ctx context.Context, remote *protobufs.AgentRemoteConfig) (*ApplyResult, error) {
	if remote == nil {
		return nil, fmt.Errorf("remote config is nil")
	}

	// Check if config hash is unchanged
	if bytes.Equal(m.lastHash, remote.GetConfigHash()) && len(m.lastHash) > 0 {
		m.logger.Debug("config hash unchanged, skipping apply")
		return &ApplyResult{
			Changed:    false,
			ConfigHash: m.lastHash,
		}, nil
	}

	// Extract collector.yaml from remote ConfigMap
	configMap := remote.GetConfig()
	if configMap == nil {
		return nil, fmt.Errorf("remote config has no ConfigMap")
	}

	var remoteConfig []byte
	configMapEntries := configMap.GetConfigMap()
	if configMapEntries == nil {
		return nil, fmt.Errorf("remote ConfigMap is nil")
	}

	// Strictly require collector.yaml entry
	cfg, ok := configMapEntries["collector.yaml"]
	if !ok {
		return nil, fmt.Errorf("remote ConfigMap missing required 'collector.yaml' entry")
	}
	if cfg == nil {
		return nil, fmt.Errorf("remote ConfigMap 'collector.yaml' entry is nil")
	}
	remoteConfig = cfg.GetBody()
	if len(remoteConfig) == 0 {
		return nil, fmt.Errorf("remote ConfigMap 'collector.yaml' entry is empty")
	}

	// Store raw remote configs to ConfigDir/remote/ for debugging
	if err := m.storeRemoteConfigs(configMapEntries); err != nil {
		m.logger.Warn("failed to store remote configs for debugging", zap.Error(err))
		// Continue processing, this is not critical
	}

	// Start with the remote config as the base
	mergedConfig := remoteConfig

	// Merge with local overrides
	for _, overridePath := range m.cfg.LocalOverrides {
		overrideContent, err := os.ReadFile(overridePath)
		if err != nil {
			m.logger.Warn("failed to read local override file, skipping",
				zap.String("path", overridePath),
				zap.Error(err))
			continue
		}

		mergedConfig, err = configmerge.MergeConfigs(mergedConfig, overrideContent)
		if err != nil {
			return nil, fmt.Errorf("failed to merge with override %s: %w", overridePath, err)
		}
		m.logger.Debug("merged local override", zap.String("path", overridePath))
	}

	// Inject OpAMP extension
	if m.cfg.LocalEndpoint != "" && m.cfg.InstanceUID != "" {
		var err error
		mergedConfig, err = configmerge.InjectOpAMPExtension(mergedConfig, m.cfg.LocalEndpoint, m.cfg.InstanceUID)
		if err != nil {
			return nil, fmt.Errorf("failed to inject OpAMP extension: %w", err)
		}
		m.logger.Debug("injected OpAMP extension",
			zap.String("endpoint", m.cfg.LocalEndpoint),
			zap.String("instanceUID", m.cfg.InstanceUID))
	}

	// Inject health_check extension to guarantee it stays reachable.
	// This runs after merge so remote config cannot override the endpoint.
	if m.cfg.HealthCheck.Endpoint != "" {
		var err error
		mergedConfig, err = configmerge.InjectHealthCheckExtension(mergedConfig, m.cfg.HealthCheck)
		if err != nil {
			return nil, fmt.Errorf("failed to inject health_check extension: %w", err)
		}
		m.logger.Debug("injected health_check extension",
			zap.String("endpoint", m.cfg.HealthCheck.Endpoint))
	}

	// Back up current config for rollback (skip if no existing config)
	if existing, err := os.ReadFile(m.cfg.OutputPath); err == nil {
		if err := persistence.WriteFile(m.cfg.OutputPath+".prev", existing, 0o600); err != nil {
			return nil, fmt.Errorf("failed to back up current config: %w", err)
		}
		m.logger.Debug("backed up current config", zap.String("path", m.cfg.OutputPath+".prev"))
	}

	// Write result to OutputPath
	if err := persistence.WriteFile(m.cfg.OutputPath, mergedConfig, 0o600); err != nil {
		return nil, fmt.Errorf("failed to write effective config: %w", err)
	}
	m.logger.Info("wrote effective config", zap.String("path", m.cfg.OutputPath))

	// Update lastHash on success
	m.previousHash = m.lastHash
	m.lastHash = remote.GetConfigHash()

	return &ApplyResult{
		Changed:         true,
		EffectiveConfig: mergedConfig,
		ConfigHash:      m.lastHash,
	}, nil
}

// storeRemoteConfigs stores all remote configs to ConfigDir/remote/ for debugging.
func (m *Manager) storeRemoteConfigs(configMap map[string]*protobufs.AgentConfigFile) error {
	if m.cfg.ConfigDir == "" {
		return nil
	}

	remoteDir := filepath.Join(m.cfg.ConfigDir, "remote")

	seen := make(map[string]string) // basename -> original name

	for name, cfg := range configMap {
		if cfg == nil {
			continue
		}

		// Use only the base name — paths in filenames are not supported.
		base := filepath.Base(name)
		if base == "" || base == "." || base == ".." {
			m.logger.Warn("Skipping remote config with invalid name", zap.String("name", name))
			continue
		}

		if prev, ok := seen[base]; ok {
			m.logger.Warn("Remote config basename collision, later entry overwrites earlier",
				zap.String("basename", base),
				zap.String("previous", prev),
				zap.String("current", name))
		}
		seen[base] = name

		path := filepath.Join(remoteDir, base)
		if err := persistence.WriteFile(path, cfg.GetBody(), 0o600); err != nil {
			return fmt.Errorf("failed to store remote config %s: %w", name, err)
		}
	}

	return nil
}

// GetEffectiveConfig reads and returns the current effective config from OutputPath.
func (m *Manager) GetEffectiveConfig() ([]byte, error) {
	if m.cfg.OutputPath == "" {
		return nil, fmt.Errorf("output path not configured")
	}

	content, err := os.ReadFile(m.cfg.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read effective config: %w", err)
	}

	return content, nil
}

// RollbackConfig restores the previous config from the backup file.
// It resets lastHash to previousHash so the next remote config with
// the old hash is not skipped by the deduplication check.
func (m *Manager) RollbackConfig() error {
	bakPath := m.cfg.OutputPath + ".prev"

	bakContent, err := os.ReadFile(bakPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no backup config to roll back to")
		}
		return fmt.Errorf("failed to read backup config: %w", err)
	}

	if err := persistence.WriteFile(m.cfg.OutputPath, bakContent, 0o600); err != nil {
		return fmt.Errorf("failed to write rolled-back config: %w", err)
	}

	if err := os.Remove(bakPath); err != nil {
		m.logger.Warn("failed to remove backup file after rollback", zap.Error(err))
	}

	m.lastHash = m.previousHash
	m.previousHash = nil

	m.logger.Info("rolled back to previous config", zap.String("path", m.cfg.OutputPath))
	return nil
}

// GetConfigHash returns the hash of the last successfully applied config.
func (m *Manager) GetConfigHash() []byte {
	return m.lastHash
}

// OutputPath returns the path where the effective config is written.
func (m *Manager) OutputPath() string {
	return m.cfg.OutputPath
}

// SetLocalEndpoint updates the local OpAMP endpoint used for extension injection.
// Call this after the local OpAMP server starts, to replace the static config
// value (which may be "localhost:0") with the actual bound address.
func (m *Manager) SetLocalEndpoint(endpoint string) {
	m.cfg.LocalEndpoint = endpoint
}

// EnsureBootstrapConfig ensures a valid collector config exists at OutputPath
// before the collector starts.
//
// If no config file exists (first run), it writes a minimal bootstrap config
// containing only the opamp and health_check extensions.
//
// If a config file already exists (cached from a previous run), it re-injects
// the opamp and health_check extensions to update the local OpAMP endpoint,
// which may have changed if the local server binds to an ephemeral port.
func (m *Manager) EnsureBootstrapConfig() error {
	// Ensure the config directory exists.
	dir := filepath.Dir(m.cfg.OutputPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}

	// Load existing config or start from empty.
	config, err := os.ReadFile(m.cfg.OutputPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to read existing config %s: %w", m.cfg.OutputPath, err)
	}

	bootstrap := len(config) == 0
	if bootstrap {
		if errors.Is(err, os.ErrNotExist) {
			m.logger.Info("no existing config found, writing bootstrap config",
				zap.String("path", m.cfg.OutputPath))
		} else {
			m.logger.Info("existing config is empty, writing bootstrap config",
				zap.String("path", m.cfg.OutputPath))
		}
	} else {
		m.logger.Info("re-injecting extensions into cached config",
			zap.String("path", m.cfg.OutputPath))
	}

	// Inject OpAMP extension (updates endpoint on every start).
	if m.cfg.LocalEndpoint != "" && m.cfg.InstanceUID != "" {
		config, err = configmerge.InjectOpAMPExtension(config, m.cfg.LocalEndpoint, m.cfg.InstanceUID)
		if err != nil {
			return fmt.Errorf("failed to inject OpAMP extension: %w", err)
		}
	}

	// Inject health_check extension.
	if m.cfg.HealthCheck.Endpoint != "" {
		config, err = configmerge.InjectHealthCheckExtension(config, m.cfg.HealthCheck)
		if err != nil {
			return fmt.Errorf("failed to inject health_check extension: %w", err)
		}
	}

	// The collector requires at least one pipeline. If the config has none
	// (fresh bootstrap or a cached config without pipelines), inject a
	// minimal nop pipeline so the collector can start.
	if !configmerge.HasPipelines(config) {
		config, err = configmerge.InjectSettings(config, map[string]any{
			"receivers::nop":  nil,
			"exporters::nop":  nil,
			"service::pipelines::logs/bootstrap": map[string]any{
				"receivers": []string{"nop"},
				"exporters": []string{"nop"},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to inject bootstrap pipeline: %w", err)
		}
	}

	if err := persistence.WriteFile(m.cfg.OutputPath, config, 0o600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	if bootstrap {
		m.logger.Info("wrote bootstrap config", zap.String("path", m.cfg.OutputPath))
	} else {
		m.logger.Info("updated cached config with current extensions", zap.String("path", m.cfg.OutputPath))
	}
	return nil
}

// remoteConfigStatusYAML is the on-disk YAML representation of RemoteConfigStatus.
type remoteConfigStatusYAML struct {
	Status         string `koanf:"status"`
	ErrorMessage   string `koanf:"error_message"`
	LastConfigHash string `koanf:"last_config_hash"` // base64-encoded
}

const remoteConfigStatusFile = "remote-config-status.yaml"

// SaveRemoteConfigStatus persists the remote config status to disk as YAML.
func (m *Manager) SaveRemoteConfigStatus(status protobufs.RemoteConfigStatuses, errorMessage string, configHash []byte) error {
	data := remoteConfigStatusYAML{
		Status:         status.String(),
		ErrorMessage:   errorMessage,
		LastConfigHash: base64.StdEncoding.EncodeToString(configHash),
	}

	path := filepath.Join(m.cfg.ConfigDir, remoteConfigStatusFile)
	return persistence.WriteYAMLFile(".", path, data)
}

// LoadRemoteConfigStatus loads the persisted remote config status from disk.
// Returns nil, nil if the file does not exist (fresh install).
func (m *Manager) LoadRemoteConfigStatus() (*protobufs.RemoteConfigStatus, error) {
	path := filepath.Join(m.cfg.ConfigDir, remoteConfigStatusFile)

	var data remoteConfigStatusYAML
	if err := persistence.LoadYAMLFile(".", path, &data); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load remote config status: %w", err)
	}

	hash, err := base64.StdEncoding.DecodeString(data.LastConfigHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode config hash: %w", err)
	}

	statusEnum, ok := protobufs.RemoteConfigStatuses_value[data.Status]
	if !ok {
		return nil, fmt.Errorf("unknown remote config status: %s", data.Status)
	}

	return &protobufs.RemoteConfigStatus{
		Status:               protobufs.RemoteConfigStatuses(statusEnum),
		ErrorMessage:         data.ErrorMessage,
		LastRemoteConfigHash: hash,
	}, nil
}
