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

package components

import (
	"cmp"
	"context"
	"crypto/sha256"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/open-telemetry/opamp-go/protobufs"
	"google.golang.org/protobuf/proto"
)

// Components holds the available components from a collector.
// This matches the YAML output format of the collector's "components" command.
type Components struct {
	BuildInfo  BuildInfo   `yaml:"buildinfo"`
	Receivers  []Component `yaml:"receivers"`
	Processors []Component `yaml:"processors"`
	Exporters  []Component `yaml:"exporters"`
	Extensions []Component `yaml:"extensions"`
	Connectors []Component `yaml:"connectors"`
	Providers  []Provider  `yaml:"providers"`
}

// BuildInfo contains collector build information.
type BuildInfo struct {
	Command     string `yaml:"command"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
}

// Component represents a collector component with its metadata.
type Component struct {
	Name      string    `yaml:"name"`
	Module    string    `yaml:"module"`
	Stability Stability `yaml:"stability"`
}

// Stability represents the stability levels for different signal types.
type Stability struct {
	Logs      string `yaml:"logs,omitempty"`
	Metrics   string `yaml:"metrics,omitempty"`
	Traces    string `yaml:"traces,omitempty"`
	Extension string `yaml:"extension,omitempty"`
}

// Provider represents a configuration provider.
type Provider struct {
	Scheme string `yaml:"scheme"`
	Module string `yaml:"module"`
}

// DiscoverConfig holds configuration for component discovery.
type DiscoverConfig struct {
	Executable string
	Timeout    time.Duration
}

// Discover runs the collector's components command and parses the output.
func Discover(ctx context.Context, cfg DiscoverConfig) (*Components, error) {
	cfg.Timeout = cmp.Or(cfg.Timeout, 10*time.Second)

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, cfg.Executable, "components")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run components command: %w", err)
	}

	return ParseComponentsOutput(output)
}

// ParseComponentsOutput parses the YAML output from the collector's components command.
func ParseComponentsOutput(output []byte) (*Components, error) {
	var components Components
	if err := yaml.Unmarshal(output, &components); err != nil {
		return nil, fmt.Errorf("parse components output: %w", err)
	}
	return &components, nil
}

// ToProto converts Components to the OpAMP protobuf format.
func (c *Components) ToProto() *protobufs.AvailableComponents {
	if c == nil {
		return nil
	}

	result := &protobufs.AvailableComponents{
		Components: make(map[string]*protobufs.ComponentDetails),
	}

	// Add receivers
	for _, comp := range c.Receivers {
		key := "receiver/" + comp.Name
		result.Components[key] = componentToDetails(comp)
	}

	// Add processors
	for _, comp := range c.Processors {
		key := "processor/" + comp.Name
		result.Components[key] = componentToDetails(comp)
	}

	// Add exporters
	for _, comp := range c.Exporters {
		key := "exporter/" + comp.Name
		result.Components[key] = componentToDetails(comp)
	}

	// Add extensions
	for _, comp := range c.Extensions {
		key := "extension/" + comp.Name
		result.Components[key] = componentToDetails(comp)
	}

	// Add connectors
	for _, comp := range c.Connectors {
		key := "connector/" + comp.Name
		result.Components[key] = componentToDetails(comp)
	}

	// Compute hash of components
	result.Hash = computeComponentsHash(result.Components)

	return result
}

// computeComponentsHash computes a deterministic hash of the components map.
func computeComponentsHash(components map[string]*protobufs.ComponentDetails) []byte {
	if len(components) == 0 {
		// Return a hash of empty content for consistency
		h := sha256.Sum256(nil)
		return h[:]
	}

	// Sort keys for deterministic ordering
	keys := make([]string, 0, len(components))
	for k := range components {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Hash each component in sorted order
	h := sha256.New()
	for _, key := range keys {
		h.Write([]byte(key))
		if details := components[key]; details != nil {
			if data, err := proto.Marshal(details); err == nil {
				h.Write(data)
			}
		}
	}

	return h.Sum(nil)
}

// componentToDetails converts a Component to protobufs.ComponentDetails.
func componentToDetails(comp Component) *protobufs.ComponentDetails {
	var metadata []*protobufs.KeyValue

	if comp.Module != "" {
		pkg, version := splitModule(comp.Module)
		if pkg != "" {
			metadata = append(metadata, &protobufs.KeyValue{
				Key:   "component.package",
				Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: pkg}},
			})
		}
		if version != "" {
			metadata = append(metadata, &protobufs.KeyValue{
				Key:   "component.version",
				Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: version}},
			})
		}
	}

	return &protobufs.ComponentDetails{
		Metadata: metadata,
	}
}

// splitModule splits a module string like "go.opentelemetry.io/collector/receiver/otlpreceiver v0.144.0"
// into package and version parts.
func splitModule(module string) (pkg, version string) {
	// Find the last space which separates package from version
	idx := strings.LastIndex(module, " ")
	if idx == -1 {
		return module, ""
	}
	return module[:idx], module[idx+1:]
}

// IsEmpty returns true if no components are available.
func (c *Components) IsEmpty() bool {
	if c == nil {
		return true
	}
	return len(c.Receivers) == 0 && len(c.Processors) == 0 &&
		len(c.Exporters) == 0 && len(c.Extensions) == 0 &&
		len(c.Connectors) == 0
}

// Count returns the total number of available components.
func (c *Components) Count() int {
	if c == nil {
		return 0
	}
	return len(c.Receivers) + len(c.Processors) + len(c.Exporters) +
		len(c.Extensions) + len(c.Connectors)
}

// ReceiverNames returns just the names of receivers.
func (c *Components) ReceiverNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, len(c.Receivers))
	for i, r := range c.Receivers {
		names[i] = r.Name
	}
	return names
}

// ProcessorNames returns just the names of processors.
func (c *Components) ProcessorNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, len(c.Processors))
	for i, p := range c.Processors {
		names[i] = p.Name
	}
	return names
}

// ExporterNames returns just the names of exporters.
func (c *Components) ExporterNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, len(c.Exporters))
	for i, e := range c.Exporters {
		names[i] = e.Name
	}
	return names
}

// ExtensionNames returns just the names of extensions.
func (c *Components) ExtensionNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, len(c.Extensions))
	for i, e := range c.Extensions {
		names[i] = e.Name
	}
	return names
}

// ConnectorNames returns just the names of connectors.
func (c *Components) ConnectorNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, len(c.Connectors))
	for i, conn := range c.Connectors {
		names[i] = conn.Name
	}
	return names
}
