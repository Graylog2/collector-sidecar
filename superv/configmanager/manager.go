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
	ConfigDir      string   // Directory to store remote configs
	OutputPath     string   // Path to write final merged config
	LocalOverrides []string // Paths to local override files
	LocalEndpoint  string   // Local OpAMP server endpoint for injection
	InstanceUID    string   // Instance UID for injection
}

// ApplyResult contains the result of applying a remote config.
type ApplyResult struct {
	Changed         bool   // Whether the config changed
	EffectiveConfig []byte // The final merged config
	ConfigHash      []byte // Hash of the applied config
}

// Manager handles remote configuration from OpAMP server.
type Manager struct {
	logger   *zap.Logger
	cfg      Config
	lastHash []byte
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

	// Write result to OutputPath
	if err := persistence.WriteFile(m.cfg.OutputPath, mergedConfig); err != nil {
		return nil, fmt.Errorf("failed to write effective config: %w", err)
	}
	m.logger.Info("wrote effective config", zap.String("path", m.cfg.OutputPath))

	// Update lastHash on success
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

	for name, cfg := range configMap {
		if cfg == nil {
			continue
		}

		// Reject empty file names
		if name == "" {
			return fmt.Errorf("remote ConfigMap contains entry with empty name")
		}

		path := filepath.Join(remoteDir, name)
		if err := persistence.WriteFile(path, cfg.GetBody()); err != nil {
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

// GetConfigHash returns the hash of the last successfully applied config.
func (m *Manager) GetConfigHash() []byte {
	return m.lastHash
}
