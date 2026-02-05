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
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

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
		return errors.New("instance_uid is required")
	}
	// Validate instance UID format
	if _, err := parseInstanceUID(c.InstanceUID); err != nil {
		return fmt.Errorf("invalid instance_uid: %w", err)
	}
	return nil
}

// Capabilities represents the supervisor's OpAMP capabilities.
// Note: ReportsStatus is always enabled per the OpAMP specification and is not
// included here as a configurable option.
type Capabilities struct {
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
	ReportsHeartbeat               bool
	ReportsAvailableComponents     bool
}

// ToProto converts capabilities to protobuf format.
// ReportsStatus is always set as required by the OpAMP specification.
func (c Capabilities) ToProto() protobufs.AgentCapabilities {
	// ReportsStatus is mandatory per OpAMP spec - always set it
	caps := protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus

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
	if c.ReportsHeartbeat {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsHeartbeat
	}
	if c.ReportsAvailableComponents {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsAvailableComponents
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
	logger                    *zap.Logger
	cfg                       ClientConfig
	callbacks                 *Callbacks
	opampClient               client.OpAMPClient
	initialDescription        *protobufs.AgentDescription
	initialHealth             *protobufs.ComponentHealth
	initialComponents         *protobufs.AvailableComponents
	initialConnectionSettings *protobufs.ConnectionSettingsRequest
	effectiveConfig           *protobufs.EffectiveConfig

	mu                sync.RWMutex
	heartbeatInterval time.Duration
}

// NewClient creates a new OpAMP client wrapper.
func NewClient(logger *zap.Logger, cfg ClientConfig, callbacks *Callbacks) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Client{
		logger:            logger,
		cfg:               cfg,
		callbacks:         callbacks,
		heartbeatInterval: 30 * time.Second, // Default per OpAMP spec
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
	// Error is not expected here since Validate() is called in NewClient,
	// but we handle it for safety.
	instanceUID, err := parseInstanceUID(c.cfg.InstanceUID)
	if err != nil {
		return fmt.Errorf("invalid instance UID: %w", err)
	}

	// Set GetEffectiveConfig to return our stored config
	// This allows SetEffectiveConfig to work both before and after Start()
	c.callbacks.GetEffectiveConfig = func(ctx context.Context) (*protobufs.EffectiveConfig, error) {
		return c.effectiveConfig, nil
	}

	settings := types.StartSettings{
		OpAMPServerURL: c.cfg.Endpoint,
		InstanceUid:    instanceUID,
		Callbacks:      c.callbacks.ToTypesCallbacks(),
		Header:         c.cfg.Headers,
	}

	// opamp-go will fail if TLSConfig is set but the URL is not HTTPS/WSS
	if strings.HasPrefix(c.cfg.Endpoint, "wss") || strings.HasPrefix(c.cfg.Endpoint, "https") {
		settings.TLSConfig = c.cfg.TLSConfig
	}

	// Apply initial health before starting (required by opamp-go)
	health := c.initialHealth
	if health == nil {
		health = &protobufs.ComponentHealth{Healthy: true}
	}
	if err := opampClient.SetHealth(health); err != nil {
		return fmt.Errorf("SetHealth: %w", err)
	}

	// Apply initial agent description before starting (required by opamp-go)
	if c.initialDescription != nil {
		if err := opampClient.SetAgentDescription(c.initialDescription); err != nil {
			return fmt.Errorf("SetAgentDescription: %w", err)
		}
	}

	// Apply initial available components BEFORE setting capabilities
	// (opamp-go validates that components exist when ReportsAvailableComponents capability is set)
	if c.initialComponents != nil {
		if err := opampClient.SetAvailableComponents(c.initialComponents); err != nil {
			return fmt.Errorf("SetAvailableComponents: %w", err)
		}
	}

	// We set an initial connection settings request in the enrollment phase.
	if c.initialConnectionSettings != nil {
		if err := opampClient.RequestConnectionSettings(c.initialConnectionSettings); err != nil {
			return fmt.Errorf("RequestConnectionSettings: %w", err)
		}
	}

	// Set capabilities (must be after setting components if ReportsAvailableComponents is enabled)
	caps := c.cfg.Capabilities.ToProto()
	if err := opampClient.SetCapabilities(&caps); err != nil {
		return fmt.Errorf("SetCapabilities: %w", err)
	}

	if err := opampClient.Start(ctx, settings); err != nil {
		return err
	}

	c.opampClient = opampClient

	// Trigger effective config update if we have an initial config
	if c.effectiveConfig != nil {
		if err := c.opampClient.UpdateEffectiveConfig(ctx); err != nil {
			c.logger.Warn("failed to send initial effective config", zap.Error(err))
		}
	}

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

// SetAvailableComponents updates the available components reported to the server.
// Can be called before Start() to set the initial components.
func (c *Client) SetAvailableComponents(components *protobufs.AvailableComponents) error {
	if components == nil {
		return fmt.Errorf("components cannot be nil")
	}
	if len(components.Hash) == 0 {
		return fmt.Errorf("components hash cannot be empty")
	}
	if c.opampClient == nil {
		// Store for use when Start() is called
		c.initialComponents = components
		return nil
	}
	return c.opampClient.SetAvailableComponents(components)
}

// SetEffectiveConfig updates the effective configuration reported to the server.
// Can be called before Start() to set the initial effective config.
func (c *Client) SetEffectiveConfig(ctx context.Context, config map[string]*protobufs.AgentConfigFile) error {
	c.effectiveConfig = &protobufs.EffectiveConfig{
		ConfigMap: &protobufs.AgentConfigMap{
			ConfigMap: config,
		},
	}

	if c.opampClient == nil {
		// Store for use when Start() is called
		return nil
	}

	// Trigger the OpAMP client to fetch and send the effective config via callback
	return c.opampClient.UpdateEffectiveConfig(ctx)
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
		c.initialConnectionSettings = request
		return nil
	}
	return c.opampClient.RequestConnectionSettings(request)
}

// HeartbeatInterval returns the current heartbeat interval.
func (c *Client) HeartbeatInterval() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.heartbeatInterval
}

// SetHeartbeatInterval updates the heartbeat interval.
// This takes effect on the next client restart.
func (c *Client) SetHeartbeatInterval(interval time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.heartbeatInterval = interval
}

// SetConnectionSettingsStatus reports the status of applying connection settings.
// Note: The current opamp-go library does not expose SetConnectionSettingsStatus on the
// public client interface. This method is a placeholder that will automatically work
// if/when opamp-go adds this capability. For now, status is logged for debugging.
func (c *Client) SetConnectionSettingsStatus(status *protobufs.ConnectionSettingsStatus) error {
	if c.opampClient == nil {
		// Client not started, just log
		c.logger.Debug("Connection settings status set before client started",
			zap.String("status", status.GetStatus().String()),
			zap.String("error", status.GetErrorMessage()),
		)
		return nil
	}

	// Check if opamp-go client supports SetConnectionSettingsStatus.
	// Currently it does not, but this will work automatically if added in the future.
	if setter, ok := c.opampClient.(interface {
		SetConnectionSettingsStatus(*protobufs.ConnectionSettingsStatus) error
	}); ok {
		return setter.SetConnectionSettingsStatus(status)
	}

	// Log the status for debugging since we can't report it to the server yet
	c.logger.Debug("Connection settings status (opamp-go does not support reporting)",
		zap.String("status", status.GetStatus().String()),
		zap.String("error", status.GetErrorMessage()),
	)
	return nil
}

// parseInstanceUID parses a string as a UUID and returns a 16-byte InstanceUid.
// Returns an error if the input is not a valid UUID.
func parseInstanceUID(s string) (types.InstanceUid, error) {
	if s == "" {
		return types.InstanceUid{}, errors.New("instance_uid cannot be empty")
	}

	parsed, err := uuid.Parse(s)
	if err != nil {
		return types.InstanceUid{}, fmt.Errorf("instance_uid must be a valid UUID: %w", err)
	}

	var uid types.InstanceUid
	copy(uid[:], parsed[:])
	return uid, nil
}
