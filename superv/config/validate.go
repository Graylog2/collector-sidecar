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

package config

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
)

var (
	validSchemes       = []string{"ws", "wss", "http", "https"}
	validLogLevels     = []string{"debug", "info", "warn", "error"}
	validLogFormats    = []string{"json", "text"}
	validReloadMethods = []string{"auto", "signal", "restart"}
	validTransports    = []string{"websocket", "http", "auto", ""}
)

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("server: %w", err)
	}

	if err := c.Keys.Validate(); err != nil {
		return fmt.Errorf("keys: %w", err)
	}

	if err := c.Agent.Validate(); err != nil {
		return fmt.Errorf("agent: %w", err)
	}

	if err := c.Logging.Validate(); err != nil {
		return fmt.Errorf("logging: %w", err)
	}

	return nil
}

// Validate checks ServerConfig for errors.
func (s ServerConfig) Validate() error {
	if s.Endpoint == "" {
		return errors.New("endpoint is required")
	}

	u, err := url.Parse(s.Endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	if !slices.Contains(validSchemes, u.Scheme) {
		return fmt.Errorf("endpoint scheme must be one of %v, got %q", validSchemes, u.Scheme)
	}

	if !slices.Contains(validTransports, s.Transport) {
		return fmt.Errorf("transport must be one of %v, got %q", validTransports, s.Transport)
	}

	return nil
}

// Validate checks KeysConfig for errors.
func (k KeysConfig) Validate() error {
	if k.Encrypted && k.Passphrase.Env == "" && k.Passphrase.File == "" && len(k.Passphrase.Cmd) == 0 {
		return errors.New("passphrase source required when keys are encrypted")
	}
	return nil
}

// Validate checks AgentConfig for errors.
func (a AgentConfig) Validate() error {
	if a.Executable == "" {
		return errors.New("executable is required")
	}

	if !slices.Contains(validReloadMethods, a.Reload.Method) {
		return fmt.Errorf("reload.method must be one of %v, got %q", validReloadMethods, a.Reload.Method)
	}

	return nil
}

// Validate checks LoggingConfig for errors.
func (l LoggingConfig) Validate() error {
	if !slices.Contains(validLogLevels, l.Level) {
		return fmt.Errorf("level must be one of %v, got %q", validLogLevels, l.Level)
	}

	if !slices.Contains(validLogFormats, l.Format) {
		return fmt.Errorf("format must be one of %v, got %q", validLogFormats, l.Format)
	}

	return nil
}
