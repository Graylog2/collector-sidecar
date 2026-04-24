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

package configmerge

import (
	"fmt"
	"slices"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
)

// HealthCheckConfig holds configuration for the health check extension injection.
type HealthCheckConfig struct {
	Endpoint string // e.g., "localhost:13133"
	Path     string // e.g., "/health" (optional, defaults to collector's default)
}

// InjectHealthCheckExtension injects the health_check extension configuration into a collector config.
// It adds extensions.health_check with the endpoint configuration, and ensures
// the health_check extension is listed in service.extensions.
func InjectHealthCheckExtension(config []byte, cfg HealthCheckConfig) ([]byte, error) {
	healthCheckConfig := map[string]any{
		"endpoint": cfg.Endpoint,
	}
	if cfg.Path != "" {
		healthCheckConfig["path"] = cfg.Path
	}

	result, err := InjectSettings(config, map[string]any{
		"extensions::health_check": healthCheckConfig,
	})
	if err != nil {
		return nil, err
	}

	return InjectServiceExtension(result, "health_check")
}

// InjectOpAMPExtension injects the OpAMP extension configuration into a collector config.
// It adds extensions.opamp with server.ws.endpoint and instance_uid, and ensures
// the opamp extension is listed in service.extensions.
func InjectOpAMPExtension(config []byte, endpoint string, instanceUID string) ([]byte, error) {
	// First inject the opamp extension configuration
	opampConfig := map[string]any{
		"server": map[string]any{
			"ws": map[string]any{
				"endpoint": endpoint,
			},
		},
		"instance_uid": instanceUID,
	}

	result, err := InjectSettings(config, map[string]any{
		"extensions::opamp": opampConfig,
	})
	if err != nil {
		return nil, err
	}

	// Then ensure opamp is in service.extensions
	return InjectServiceExtension(result, "opamp")
}

// InjectServiceExtension ensures the specified extension is listed in service.extensions.
// If the extension is already present, it is not duplicated.
func InjectServiceExtension(config []byte, extensionName string) ([]byte, error) {
	k := koanf.New("::")

	// Load existing config
	if len(config) > 0 {
		if err := k.Load(rawbytes.Provider(config), yaml.Parser()); err != nil {
			return nil, err
		}
	}

	// Get existing service.extensions
	existingExtensions := k.Strings("service::extensions")

	// Add extension if not already present
	if !slices.Contains(existingExtensions, extensionName) {
		existingExtensions = append(existingExtensions, extensionName)
	}

	// Set the updated extensions list
	if err := k.Set("service::extensions", existingExtensions); err != nil {
		return nil, fmt.Errorf("couldn't set key service:extensions: %w", err)
	}

	return k.Marshal(yaml.Parser())
}

// InjectDisableTelemetryMetrics injects configuration to disable the telemetry metrics.
// This disables the default Prometheus HTTP endpoint on localhost:8888.
func InjectDisableTelemetryMetrics(config []byte) ([]byte, error) {
	// See: https://opentelemetry.io/docs/collector/internal-telemetry/#configure-internal-metrics
	result, err := InjectSettings(config, map[string]any{
		"service::telemetry::metrics::level": "none",
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
