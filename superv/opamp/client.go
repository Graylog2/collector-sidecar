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
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/open-telemetry/opamp-go/client"
	"github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

// ClientConfig holds configuration for the OpAMP client.
type ClientConfig struct {
	Endpoint             string
	InstanceUID          string
	Headers              http.Header
	HeaderFunc           func(http.Header) http.Header
	TLSConfig            *tls.Config
	Capabilities         Capabilities
	HeartbeatInterval    time.Duration // 0 uses opamp-go default (30s)
	MaxHeartbeatInterval time.Duration // 0 means no cap
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
	AcceptsRemoteConfig             bool
	ReportsEffectiveConfig          bool
	AcceptsPackages                 bool
	ReportsPackageStatuses          bool
	ReportsOwnTraces                bool
	ReportsOwnMetrics               bool
	ReportsOwnLogs                  bool
	AcceptsOpAMPConnectionSettings  bool
	ReportsConnectionSettingsStatus bool
	AcceptsRestartCommand           bool
	ReportsHealth                   bool
	ReportsRemoteConfig             bool
	ReportsHeartbeat                bool
	ReportsAvailableComponents      bool
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
	if c.ReportsConnectionSettingsStatus {
		caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsConnectionSettingsStatus
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

// Ensure opampLogger implements types.Logger.
var _ types.Logger = (*opampLogger)(nil)

// Client wraps the opamp-go client with supervisor-specific functionality.
type Client struct {
	logger             *zap.Logger
	cfg                ClientConfig
	callbacks          *Callbacks
	opampClient        client.OpAMPClient
	effectiveConfig    atomic.Pointer[protobufs.EffectiveConfig]
	remoteConfigStatus *protobufs.RemoteConfigStatus
	started            atomic.Bool
}

// NewClient creates a new OpAMP client wrapper.
// The underlying opamp-go client is created immediately, so Set* methods
// (SetAgentDescription, SetHealth, SetAvailableComponents) can be called
// before Start() — opamp-go stores this state and includes it in the first message.
func NewClient(logger *zap.Logger, cfg ClientConfig, callbacks *Callbacks) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}

	if callbacks == nil {
		callbacks = &Callbacks{}
	}

	opampLog := newLoggerFromZap(logger)
	var opampClient client.OpAMPClient
	if strings.HasPrefix(u.Scheme, "ws") {
		opampClient = client.NewWebSocket(opampLog)
	} else {
		opampClient = client.NewHTTP(opampLog)
	}

	c := &Client{
		logger:      logger,
		cfg:         cfg,
		callbacks:   callbacks,
		opampClient: opampClient,
	}

	// Wire GetEffectiveConfig callback to return the stored effective config.
	// This overrides any caller-provided callback because the wrapper owns
	// effective config management via SetEffectiveConfig.
	callbacks.GetEffectiveConfig = func(_ context.Context) (*protobufs.EffectiveConfig, error) {
		return c.effectiveConfig.Load(), nil
	}

	// Set default health so opamp-go has valid state before Start().
	// Callers typically override this via SetHealth() before Start().
	if err := opampClient.SetHealth(&protobufs.ComponentHealth{Healthy: true}); err != nil {
		return nil, fmt.Errorf("set default health: %w", err)
	}

	return c, nil
}

// Start starts the OpAMP client connection.
// All state set via Set* methods before Start() is already stored in the
// opamp-go client and will be included in the first message.
func (c *Client) Start(ctx context.Context) error {
	instanceUID, err := parseInstanceUID(c.cfg.InstanceUID)
	if err != nil {
		return fmt.Errorf("invalid instance UID: %w", err)
	}

	settings := types.StartSettings{
		OpAMPServerURL:     c.cfg.Endpoint,
		InstanceUid:        instanceUID,
		Callbacks:          c.callbacks.ToTypesCallbacks(),
		Header:             c.cfg.Headers,
		HeaderFunc:         c.cfg.HeaderFunc,
		RemoteConfigStatus: c.remoteConfigStatus,
	}

	// Pass heartbeat interval to opamp-go if configured. When zero (default),
	// opamp-go uses its own default (30s). The interval is sourced from
	// persisted connection.Settings, so it survives reconnects correctly.
	//
	// Cap with MaxHeartbeatInterval: opamp-go's HTTP client uses this as
	// the polling interval, so an excessively large value (e.g. server
	// sending days/years) would effectively stop the client from polling
	// and prevent recovery from transient errors.
	if c.cfg.HeartbeatInterval > 0 {
		interval := c.cfg.HeartbeatInterval
		if c.cfg.MaxHeartbeatInterval > 0 {
			interval = min(interval, c.cfg.MaxHeartbeatInterval)
		}
		settings.HeartbeatInterval = &interval
	}

	// opamp-go will fail if TLSConfig is set but the URL is not HTTPS/WSS
	if strings.HasPrefix(c.cfg.Endpoint, "wss") || strings.HasPrefix(c.cfg.Endpoint, "https") {
		settings.TLSConfig = c.cfg.TLSConfig
	}

	// Set capabilities (must be after setting components if ReportsAvailableComponents is enabled)
	if err := c.opampClient.SetCapabilities(new(c.cfg.Capabilities.ToProto())); err != nil {
		return fmt.Errorf("SetCapabilities: %w", err)
	}

	if err := c.opampClient.Start(ctx, settings); err != nil {
		return fmt.Errorf("opamp client start: %w", err)
	}

	c.started.Store(true)

	// Trigger effective config update if we have an initial config
	if c.effectiveConfig.Load() != nil {
		if err := c.opampClient.UpdateEffectiveConfig(ctx); err != nil {
			c.logger.Warn("Failed to send initial effective config", zap.Error(err))
		}
	}

	return nil
}

// Stop stops the OpAMP client connection.
func (c *Client) Stop(ctx context.Context) error {
	if !c.started.Load() {
		return nil
	}
	err := c.opampClient.Stop(ctx)
	if err != nil {
		return fmt.Errorf("stopping OpAMP client: %w", err)
	}
	c.started.Store(false)
	return nil
}

// SetAgentDescription updates the agent description.
// Can be called before or after Start() — opamp-go stores state internally.
func (c *Client) SetAgentDescription(desc *protobufs.AgentDescription) error {
	if err := c.opampClient.SetAgentDescription(desc); err != nil {
		return fmt.Errorf("setting agent description: %w", err)
	}
	return nil
}

// SetHealth updates the agent health status.
// Can be called before or after Start() — opamp-go stores state internally.
func (c *Client) SetHealth(health *protobufs.ComponentHealth) error {
	if err := c.opampClient.SetHealth(health); err != nil {
		return fmt.Errorf("setting health: %w", err)
	}
	return nil
}

// SetAvailableComponents updates the available components reported to the server.
// Can be called before or after Start() — opamp-go stores state internally.
func (c *Client) SetAvailableComponents(components *protobufs.AvailableComponents) error {
	if components == nil {
		return fmt.Errorf("components cannot be nil")
	}
	if len(components.GetHash()) == 0 {
		return fmt.Errorf("components hash cannot be empty")
	}
	if err := c.opampClient.SetAvailableComponents(components); err != nil {
		return fmt.Errorf("setting available components: %w", err)
	}
	return nil
}

// SetEffectiveConfig updates the effective configuration reported to the server.
// Can be called before Start() to set the initial effective config.
func (c *Client) SetEffectiveConfig(ctx context.Context, config map[string]*protobufs.AgentConfigFile) error {
	c.effectiveConfig.Store(&protobufs.EffectiveConfig{
		ConfigMap: &protobufs.AgentConfigMap{
			ConfigMap: config,
		},
	})

	if !c.started.Load() {
		return nil
	}

	// Trigger the OpAMP client to fetch and send the effective config via callback
	if err := c.opampClient.UpdateEffectiveConfig(ctx); err != nil {
		return fmt.Errorf("updating effective config: %w", err)
	}
	return nil
}

// UpdateEffectiveConfig updates the effective configuration.
func (c *Client) UpdateEffectiveConfig(ctx context.Context) error {
	if !c.started.Load() {
		return errors.New("client not started")
	}
	if err := c.opampClient.UpdateEffectiveConfig(ctx); err != nil {
		return fmt.Errorf("updating effective config: %w", err)
	}
	return nil
}

// SetRemoteConfigStatus sets the remote config status.
func (c *Client) SetRemoteConfigStatus(status *protobufs.RemoteConfigStatus) error {
	if !c.started.Load() {
		return errors.New("client not started")
	}
	if err := c.opampClient.SetRemoteConfigStatus(status); err != nil {
		return fmt.Errorf("setting remote config status: %w", err)
	}
	return nil
}

// SetInitialRemoteConfigStatus sets the remote config status to include in
// StartSettings. Must be called before Start(). This restores persisted status
// so the server knows our last config state after a supervisor restart.
func (c *Client) SetInitialRemoteConfigStatus(status *protobufs.RemoteConfigStatus) {
	c.remoteConfigStatus = status
}

// RequestConnectionSettings sends a connection settings request to the server.
// This is used for certificate enrollment — the csrPEM should be a PEM-encoded CSR.
// Can be called before or after Start() — opamp-go stores state internally.
func (c *Client) RequestConnectionSettings(csrPEM []byte) error {
	if err := c.opampClient.RequestConnectionSettings(&protobufs.ConnectionSettingsRequest{
		Opamp: &protobufs.OpAMPConnectionSettingsRequest{
			CertificateRequest: &protobufs.CertificateRequest{
				Csr: csrPEM,
			},
		},
	}); err != nil {
		return fmt.Errorf("requesting connection settings: %w", err)
	}
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
