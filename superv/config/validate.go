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
	"strings"
	"time"
)

var (
	validSchemes       = []string{"ws", "wss", "http", "https"}
	validLogLevels     = []string{"debug", "info", "warn", "error"}
	validLogFormats    = []string{"json", "text"}
	validReloadMethods = []string{"auto", "signal", "restart"}
	validTransports    = []string{"websocket", "http", "auto"}
	minJWTLifetime     = 15 * time.Second
)

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	return errors.Join(
		c.Server.Validate(),
		c.Server.Auth.Validate(),
		c.Keys.Validate(),
		c.Agent.Validate(),
		c.Logging.Validate(),
		c.Telemetry.Logs.Validate(),
	)
}

func RenderErrors(err error) string {
	var sb strings.Builder

	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range joined.Unwrap() {
			sb.WriteString(fmt.Sprintf("  - %v\n", e))
		}
	} else {
		sb.WriteString(fmt.Sprintf("  - %v\n", err))
	}

	return sb.String()
}

// Validate checks ServerConfig for errors.
func (s ServerConfig) Validate() error {
	if err := s.Connection.RetryBackoff.Validate("server.connection.retry_backoff"); err != nil {
		return err
	}

	if s.Endpoint == "" {
		// An empty endpoint is okay for config validation, it can be set later via stored connection settings.
		return nil
	}

	u, err := url.Parse(s.Endpoint)
	if err != nil {
		return fmt.Errorf("server.endpoint: invalid URL: %w", err)
	}

	if !slices.Contains(validSchemes, u.Scheme) {
		return fmt.Errorf("server.endpoint: scheme must be one of %v, got %q", validSchemes, u.Scheme)
	}

	if !slices.Contains(validTransports, s.Transport) {
		return fmt.Errorf("server.transport: must be one of %v, got %q", validTransports, s.Transport)
	}

	return nil
}

// Validate checks BackoffConfig for errors. The prefix is used in error messages
// to identify which backoff config is invalid (e.g. "server.connection.retry_backoff").
func (b BackoffConfig) Validate(prefix string) error {
	if b.Initial <= 0 {
		return fmt.Errorf("%s.initial: must be positive, got %s", prefix, b.Initial)
	}
	if b.Max <= 0 {
		return fmt.Errorf("%s.max: must be positive, got %s", prefix, b.Max)
	}
	if b.Max < b.Initial {
		return fmt.Errorf("%s.max: must be >= initial (%s), got %s", prefix, b.Initial, b.Max)
	}
	if b.Multiplier < 1 {
		return fmt.Errorf("%s.multiplier: must be >= 1, got %g", prefix, b.Multiplier)
	}
	return nil
}

// Validate checks AuthConfig for errors.
func (k AuthConfig) Validate() error {
	if k.JWTLifetime < minJWTLifetime {
		return fmt.Errorf("server.auth.jwt_lifetime: JWT lifetime must be at least %s", minJWTLifetime)
	}
	// Zero means unset (DefaultConfig provides 0.75). Reject negative and >= 1.
	if k.RenewalFraction != 0 && (k.RenewalFraction <= 0 || k.RenewalFraction >= 1) {
		return fmt.Errorf("server.auth.renewal_fraction: must be between 0 (exclusive) and 1 (exclusive), got %g", k.RenewalFraction)
	}
	return nil
}

// Validate checks KeysConfig for errors.
func (k KeysConfig) Validate() error {
	if k.Dir == "" {
		return errors.New("keys.dir: is required")
	}
	if k.Encrypted && k.Passphrase.Env == "" && k.Passphrase.File == "" && len(k.Passphrase.Cmd) == 0 {
		return errors.New("keys.passphrase: source required when keys are encrypted")
	}
	return nil
}

// Validate checks AgentConfig for errors.
func (a AgentConfig) Validate() error {
	if a.Executable == "" {
		return errors.New("agent.executable: is required")
	}

	if !slices.Contains(validReloadMethods, a.Reload.Method) {
		return fmt.Errorf("agent.reload.method: must be one of %v, got %q", validReloadMethods, a.Reload.Method)
	}

	if a.Health.Endpoint != "" {
		u, err := url.Parse(a.Health.Endpoint)
		if err != nil {
			return fmt.Errorf("agent.health.endpoint: invalid URL %q: %w", a.Health.Endpoint, err)
		}
		if u.Host == "" {
			return fmt.Errorf("agent.health.endpoint: must be an absolute URL (e.g. http://localhost:13133/health), got %q", a.Health.Endpoint)
		}
	}

	return nil
}

// Validate checks TelemetryLogsConfig for errors.
func (t TelemetryLogsConfig) Validate() error {
	if !slices.Contains(validLogLevels, t.DefaultLevel) {
		return fmt.Errorf("telemetry.logs.default_level: must be one of %v, got %q", validLogLevels, t.DefaultLevel)
	}
	return nil
}

// Validate checks LoggingConfig for errors.
func (l LoggingConfig) Validate() error {
	if !slices.Contains(validLogLevels, l.Level) {
		return fmt.Errorf("logging.level must be one of %v, got %q", validLogLevels, l.Level)
	}

	if !slices.Contains(validLogFormats, l.Format) {
		return fmt.Errorf("logging.format must be one of %v, got %q", validLogFormats, l.Format)
	}

	return nil
}
