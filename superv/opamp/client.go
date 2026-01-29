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

package opamp

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/open-telemetry/opamp-go/client"
	"github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

// ClientConfig holds configuration for the OpAMP client.
type ClientConfig struct {
	Endpoint     string
	InstanceUID  string
	Headers      http.Header
	TLSConfig    *tls.Config
	Capabilities Capabilities
}

// Validate validates the client configuration.
func (c ClientConfig) Validate() error {
	if c.Endpoint == "" {
		return errors.New("endpoint is required")
	}
	if c.InstanceUID == "" {
		return errors.New("instance UID is required")
	}
	return nil
}

// Capabilities represents the supervisor's OpAMP capabilities.
type Capabilities struct {
	ReportsStatus                  bool
	AcceptsRemoteConfig            bool
	ReportsEffectiveConfig         bool
	AcceptsPackages                bool
	ReportsPackageStatuses         bool
	ReportsOwnTraces               bool
	ReportsOwnMetrics              bool
	ReportsOwnLogs                 bool
	AcceptsOpAMPConnectionSettings bool
	AcceptsRestartCommand          bool
	ReportsHealth                  bool
	ReportsRemoteConfig            bool
}

// ToProto converts capabilities to protobuf format.
func (c Capabilities) ToProto() protobufs.AgentCapabilities {
	caps := protobufs.AgentCapabilities(0)

	if c.ReportsStatus {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus
	}
	if c.AcceptsRemoteConfig {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_AcceptsRemoteConfig
	}
	if c.ReportsEffectiveConfig {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsEffectiveConfig
	}
	if c.AcceptsPackages {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_AcceptsPackages
	}
	if c.ReportsPackageStatuses {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsPackageStatuses
	}
	if c.ReportsOwnTraces {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsOwnTraces
	}
	if c.ReportsOwnMetrics {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsOwnMetrics
	}
	if c.ReportsOwnLogs {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsOwnLogs
	}
	if c.AcceptsOpAMPConnectionSettings {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_AcceptsOpAMPConnectionSettings
	}
	if c.AcceptsRestartCommand {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_AcceptsRestartCommand
	}
	if c.ReportsHealth {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsHealth
	}
	if c.ReportsRemoteConfig {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsRemoteConfig
	}

	return caps
}

// opampLogger adapts zap.Logger to the types.Logger interface required by opamp-go.
type opampLogger struct {
	l *zap.SugaredLogger
}

// Debugf implements types.Logger.
func (o *opampLogger) Debugf(_ context.Context, format string, v ...any) {
	o.l.Debugf(format, v...)
}

// Errorf implements types.Logger.
func (o *opampLogger) Errorf(_ context.Context, format string, v ...any) {
	o.l.Errorf(format, v...)
}

// newLoggerFromZap creates a types.Logger from a zap.Logger.
func newLoggerFromZap(l *zap.Logger) types.Logger {
	return &opampLogger{
		l: l.Sugar().Named("opamp").WithOptions(zap.AddCallerSkip(1)),
	}
}

// Ensure opampLogger implements types.Logger
var _ types.Logger = (*opampLogger)(nil)

// Client wraps the opamp-go client with supervisor-specific functionality.
type Client struct {
	logger             *zap.Logger
	cfg                ClientConfig
	callbacks          *Callbacks
	opampClient        client.OpAMPClient
	initialDescription *protobufs.AgentDescription
	initialHealth      *protobufs.ComponentHealth
}

// NewClient creates a new OpAMP client wrapper.
func NewClient(logger *zap.Logger, cfg ClientConfig, callbacks *Callbacks) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Client{
		logger:    logger,
		cfg:       cfg,
		callbacks: callbacks,
	}, nil
}

// Start starts the OpAMP client connection.
func (c *Client) Start(ctx context.Context) error {
	u, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return err
	}

	var opampClient client.OpAMPClient
	opampLog := newLoggerFromZap(c.logger)

	if strings.HasPrefix(u.Scheme, "ws") {
		opampClient = client.NewWebSocket(opampLog)
	} else {
		opampClient = client.NewHTTP(opampLog)
	}

	// Convert string InstanceUID to types.InstanceUid ([16]byte)
	instanceUID := parseInstanceUID(c.cfg.InstanceUID)

	settings := types.StartSettings{
		OpAMPServerURL: c.cfg.Endpoint,
		InstanceUid:    instanceUID,
		Callbacks:      c.callbacks.ToTypesCallbacks(),
		Header:         c.cfg.Headers,
		TLSConfig:      c.cfg.TLSConfig,
	}

	// Apply initial health before starting (required by opamp-go)
	health := c.initialHealth
	if health == nil {
		health = &protobufs.ComponentHealth{Healthy: true}
	}
	if err := opampClient.SetHealth(health); err != nil {
		return err
	}

	// Set capabilities before starting
	caps := c.cfg.Capabilities.ToProto()
	if err := opampClient.SetCapabilities(&caps); err != nil {
		return err
	}

	// Apply initial agent description before starting (required by opamp-go)
	if c.initialDescription != nil {
		if err := opampClient.SetAgentDescription(c.initialDescription); err != nil {
			return err
		}
	}

	if err := opampClient.Start(ctx, settings); err != nil {
		return err
	}

	c.opampClient = opampClient
	return nil
}

// Stop stops the OpAMP client connection.
func (c *Client) Stop(ctx context.Context) error {
	if c.opampClient == nil {
		return nil
	}
	return c.opampClient.Stop(ctx)
}

// SetAgentDescription updates the agent description.
// Can be called before Start() to set the initial description.
func (c *Client) SetAgentDescription(desc *protobufs.AgentDescription) error {
	if c.opampClient == nil {
		// Store for use when Start() is called
		c.initialDescription = desc
		return nil
	}
	return c.opampClient.SetAgentDescription(desc)
}

// SetHealth updates the agent health status.
// Can be called before Start() to set the initial health.
func (c *Client) SetHealth(health *protobufs.ComponentHealth) error {
	if c.opampClient == nil {
		// Store for use when Start() is called
		c.initialHealth = health
		return nil
	}
	return c.opampClient.SetHealth(health)
}

// UpdateEffectiveConfig updates the effective configuration.
func (c *Client) UpdateEffectiveConfig(ctx context.Context) error {
	if c.opampClient == nil {
		return errors.New("client not started")
	}
	return c.opampClient.UpdateEffectiveConfig(ctx)
}

// SetRemoteConfigStatus sets the remote config status.
func (c *Client) SetRemoteConfigStatus(status *protobufs.RemoteConfigStatus) error {
	if c.opampClient == nil {
		return errors.New("client not started")
	}
	return c.opampClient.SetRemoteConfigStatus(status)
}

// RequestConnectionSettings sends a connection settings request to the server.
// This is used for certificate enrollment - the csrPEM should be a PEM-encoded CSR.
func (c *Client) RequestConnectionSettings(csrPEM []byte) error {
	request := &protobufs.ConnectionSettingsRequest{
		Opamp: &protobufs.OpAMPConnectionSettingsRequest{
			CertificateRequest: &protobufs.CertificateRequest{
				Csr: csrPEM,
			},
		},
	}

	if c.opampClient == nil {
		return errors.New("client not started")
	}
	return c.opampClient.RequestConnectionSettings(request)
}

// parseInstanceUID converts a string UID to types.InstanceUid.
// Expects a valid UUID string (e.g., "550e8400-e29b-41d4-a716-446655440000").
// Falls back to copying the raw bytes if parsing fails.
func parseInstanceUID(uid string) types.InstanceUid {
	var instanceUID types.InstanceUid
	parsed, err := uuid.Parse(uid)
	if err == nil {
		copy(instanceUID[:], parsed[:])
	} else {
		// Fallback for non-UUID strings
		copy(instanceUID[:], []byte(uid))
	}
	return instanceUID
}
