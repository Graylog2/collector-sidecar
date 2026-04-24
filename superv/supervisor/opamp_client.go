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

package supervisor

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/components"
	"github.com/Graylog2/collector-sidecar/superv/opamp"
	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
	"github.com/Graylog2/collector-sidecar/superv/version"
	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

// createAndStartClient creates a new OpAMP client with current config, sets it up, and starts it.
// This is used when reconnecting with new or restored connection settings.
func (s *Supervisor) createAndStartClient(ctx context.Context, settings connection.Settings) (*opamp.Client, error) {
	headers, headerFunc := s.buildAuthHeaders(settings)

	minVersion, maxVersion, err := settings.TLS.ToTLSMinMaxVersion()
	if err != nil {
		return nil, fmt.Errorf("invalid TLS settings: %w", err)
	}

	client, err := opamp.NewClient(s.logger, opamp.ClientConfig{
		Endpoint:             settings.Endpoint,
		InstanceUID:          s.instanceUID,
		Headers:              headers,
		HeaderFunc:           headerFunc,
		HeartbeatInterval:    settings.HeartbeatInterval,
		MaxHeartbeatInterval: s.maxHeartbeatInterval,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: settings.TLS.Insecure,
			MinVersion:         minVersion,
			MaxVersion:         maxVersion,
		},
		Capabilities: opamp.Capabilities{
			AcceptsRemoteConfig:            true,
			ReportsEffectiveConfig:         true,
			ReportsRemoteConfig:            true,
			ReportsHealth:                  true,
			AcceptsOpAMPConnectionSettings: true,
			// ReportsConnectionSettingsStatus is disabled because opamp-go schedules APPLIED immediately after the callback
			// returns, but the actual reconnect happens asynchronously on the worker. The sender's in-flight status POST races
			// with client.Stop(), producing a spurious "context canceled" error. Enable once opamp-go exposes a public
			// SetConnectionSettingsStatus API so we can report the outcome after the async reconnect completes.
			ReportsConnectionSettingsStatus: false,
			AcceptsRestartCommand:           true,
			ReportsOwnLogs:                  s.ownLogsManager != nil,
			ReportsHeartbeat:                true,
			ReportsAvailableComponents:      true,
		},
	}, s.createOpAMPCallbacks())
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	// Discover and set available components first, so the collector version
	// is available for the agent description below.
	if err := s.setClientAvailableComponents(ctx, client); err != nil {
		return nil, fmt.Errorf("set available components: %w", err)
	}

	if err := client.SetAgentDescription(s.createAgentDescription()); err != nil {
		return nil, fmt.Errorf("set agent description: %w", err)
	}

	if err := client.SetHealth(s.initialComponentHealth()); err != nil {
		return nil, fmt.Errorf("set health: %w", err)
	}

	// Restore persisted remote config status so the server knows our
	// last config state after a supervisor restart.
	if status, err := s.configManager.LoadRemoteConfigStatus(); err != nil {
		s.logger.Warn("Failed to load persisted remote config status, starting with UNSET", zap.Error(err))
	} else if status != nil {
		client.SetInitialRemoteConfigStatus(status)
	}

	// Read pendingCSR under RLock — handleEnrollmentCertificate (phase 1, on opamp-go
	// goroutine) clears it under Lock concurrently with the worker calling this method.
	s.mu.RLock()
	csr := s.pendingCSR
	s.mu.RUnlock()

	if len(csr) > 0 {
		s.logger.Info("Sending CSR for enrollment via OpAMP")
		if err := client.RequestConnectionSettings(csr); err != nil {
			return nil, fmt.Errorf("request connection settings: %w", err)
		}
	}

	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("start client: %w", err)
	}

	return client, nil
}

// createOpAMPCallbacks creates the OpAMP client callbacks.
// This is extracted to avoid duplication when recreating the client.
func (s *Supervisor) createOpAMPCallbacks() *opamp.Callbacks {
	return &opamp.Callbacks{
		OnConnect: func(_ context.Context) {
			s.logger.Debug("Connected to OpAMP server", zap.String("endpoint", s.connectionSettingsManager.GetCurrent().Endpoint))
		},
		OnConnectFailed: func(_ context.Context, err error) {
			s.logger.Error("Failed to connect to OpAMP server", zap.Error(err))
		},
		OnError: func(_ context.Context, err *protobufs.ServerErrorResponse) {
			s.logger.Error("OpAMP server error", zap.Error(fmt.Errorf("%s: %s", err.GetType(), err.GetErrorMessage())))
		},
		OnRemoteConfig: func(clientCtx context.Context, cfg *protobufs.AgentRemoteConfig) {
			s.logger.Info("Received remote configuration")

			if !s.enqueueWork(clientCtx, func(ctx context.Context) { s.handleRemoteConfig(ctx, cfg) }) {
				s.logger.Error("Couldn't enqueue remote config update")
				s.reportRemoteConfigStatus(clientCtx,
					protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED,
					"couldn't enqueue remote config update",
					cfg.GetConfigHash(),
				)
			}
		},
		OnOpampConnectionSettings: func(clientCtx context.Context, settings *protobufs.OpAMPConnectionSettings) error {
			s.logger.Info("Received connection settings update")

			// Phase 1: validate and prepare (synchronous, returns status to opamp-go)
			pending, err := s.prepareConnectionSettings(settings)
			if err != nil {
				return err
			}
			if pending == nil {
				s.logger.Debug("No connection settings changes requiring reconnection")
				return nil // no reconnection needed
			}

			// Phase 2: reconnect (async on worker, can't block callback)
			if !s.enqueueWork(clientCtx, func(wCtx context.Context) {
				s.applyConnectionSettings(wCtx, pending)
			}) {
				// Enqueue failed (context cancelled during shutdown or opamp-go timeout).
				// Commit staged file so new settings are persisted for next restart.
				// The OpAMP spec says the server SHOULD NOT re-send unchanged settings.
				s.logger.Warn("Failed to enqueue connection settings apply (context cancelled), persisting for next restart")
				if commitErr := pending.stagedFile.Commit(); commitErr != nil {
					s.logger.Error("Failed to commit staged settings file", zap.Error(commitErr))
					if cleanupErr := pending.stagedFile.Cleanup(); cleanupErr != nil {
						s.logger.Error("Failed to clean up staged settings file", zap.Error(cleanupErr))
					}
					return fmt.Errorf("failed to persist settings for deferred apply: %w", commitErr)
				}

				// Restore old settings as in-memory baseline since we're not applying
				// the new settings at runtime. They're persisted on disk for next restart.
				s.connectionSettingsManager.SetCurrent(pending.oldSettings)
			}

			// TODO: Returning nil here reports APPLIED to the server, but reconnect has not
			// completed yet. This is optimistic — see "Status Reporting Limitations" in the
			// design doc. Once opamp-go exposes a public SetConnectionSettingsStatus API,
			// applyConnectionSettings should send late FAILED/APPLIED after async reconnect.
			return nil
		},
		OnPackagesAvailable: func(_ context.Context, packages *protobufs.PackagesAvailable) bool {
			// TODO: Implement package handling - opamp-go/client/types.PackagesSyncer
			s.logger.Warn("TODO: Received packages available", zap.String("packages", fmt.Sprintf("%v", packages.GetPackages())))
			return false
		},
		OnCommand: func(_ context.Context, command *protobufs.ServerToAgentCommand) error {
			s.logger.Warn("TODO: Received command", zap.String("type", command.GetType().String()))
			return nil
		},
		OnCustomMessage: func(clientCtx context.Context, customMessage *protobufs.CustomMessage) {
			// Forward custom messages to the local OpAMP server (collector)
			s.forwardCustomMessage(clientCtx, customMessage)
		},
		OnOwnLogs: func(clientCtx context.Context, settings *protobufs.TelemetryConnectionSettings) {
			if !s.enqueueWork(clientCtx, func(ctx context.Context) {
				s.handleOwnLogs(ctx, settings)
			}) {
				if s.isStopping() {
					s.logger.Debug("Skipping own_logs apply during shutdown")
					return
				}
				s.logger.Warn("Failed to enqueue own_logs apply")
			}
		},
	}
}

// createAgentDescription creates the initial agent description for OpAMP.
func (s *Supervisor) createAgentDescription() *protobufs.AgentDescription {
	hostname, _ := os.Hostname()

	return &protobufs.AgentDescription{
		IdentifyingAttributes: []*protobufs.KeyValue{
			{
				Key:   "service.name",
				Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: ServiceName}},
			},
			{
				Key:   "service.instance.id",
				Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: s.instanceUID}},
			},
		},
		NonIdentifyingAttributes: s.nonIdentifyingAttributes(hostname),
	}
}

// nonIdentifyingAttributes builds the list of non-identifying attributes for the agent description.
func (s *Supervisor) nonIdentifyingAttributes(hostname string) []*protobufs.KeyValue {
	attrs := []*protobufs.KeyValue{
		{
			Key:   "service.version",
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: version.Version()}},
		},
		{
			Key:   "host.name",
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: hostname}},
		},
		{
			Key:   "os.type",
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: runtime.GOOS}},
		},
		{
			Key:   "host.arch",
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: runtime.GOARCH}},
		},
	}

	if s.collectorVersion != "" {
		attrs = append(attrs, &protobufs.KeyValue{
			Key:   "collector.version",
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: s.collectorVersion}},
		})
	}

	return attrs
}

// setClientAvailableComponents discovers and sets available components on the given client.
// It also stores the discovered collector version for use in agent descriptions.
func (s *Supervisor) setClientAvailableComponents(ctx context.Context, client *opamp.Client) error {
	availableComponents, collectorVersion := s.discoverComponents(ctx)
	if availableComponents == nil {
		// Use empty components if discovery fails (still need valid hash for opamp-go)
		availableComponents = (&components.Components{}).ToProto()
	}
	s.collectorVersion = collectorVersion
	return client.SetAvailableComponents(availableComponents)
}

// discoverComponents discovers available components from the collector.
// This is a best-effort operation - failures are logged but don't prevent startup.
// Returns the protobuf representation and the collector's build version (if available).
func (s *Supervisor) discoverComponents(ctx context.Context) (*protobufs.AvailableComponents, string) {
	cfg := components.DiscoverConfig{
		Executable: s.agentCfg.Executable,
		Timeout:    10 * time.Second,
	}

	discovered, err := components.Discover(ctx, cfg)
	if err != nil {
		s.logger.Warn("Failed to discover components", zap.Error(err))
		return nil, ""
	}

	s.logger.Info("Discovered available components",
		zap.Int("receivers", len(discovered.Receivers)),
		zap.Int("processors", len(discovered.Processors)),
		zap.Int("exporters", len(discovered.Exporters)),
		zap.Int("extensions", len(discovered.Extensions)),
	)

	return discovered.ToProto(), discovered.BuildInfo.Version
}

// initialComponentHealth returns the latest known collector health to seed a new
// OpAMP client. During first-time enrollment, health polling may observe failures
// before enrollment completes; seeding reconnects from monitor state prevents that
// latest status from being lost due to health deduplication.
func (s *Supervisor) initialComponentHealth() *protobufs.ComponentHealth {
	if s.healthMonitor == nil {
		return &protobufs.ComponentHealth{Healthy: true}
	}

	status := s.healthMonitor.LastStatus()
	if status == nil {
		return &protobufs.ComponentHealth{Healthy: true}
	}

	return status.ToComponentHealth(nil)
}

func (s *Supervisor) reconnectClient(ctx context.Context, settings connection.Settings) error {
	s.mu.Lock()
	client := s.opampClient
	s.opampClient = nil // Nil immediately so concurrent readers see nil, not stopped client.
	s.mu.Unlock()

	// If client is nil, skip the stop step. This happens during rollback when a previous
	// reconnect failed after nilling s.opampClient but before assigning a new client.
	if client != nil {
		s.logger.Info("Stopping OpAMP client for connection settings update")
		if err := client.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop client: %w", err)
		}
	}

	s.logger.Info("Starting OpAMP client with new connection settings",
		zap.String("endpoint", settings.Endpoint))
	newClient, err := s.createClientFunc(ctx, settings)
	if err != nil {
		return fmt.Errorf("apply connection settings: %w", err)
	}

	// Check s.ctx under mu before publishing newClient. Supervisor.Stop() cancels s.ctx while
	// holding the same mu, so this check-and-assign is atomic with respect to supervisor shutdown.
	// We use s.ctx instead of s.isRunning because s.isRunning is only set at the end of Start(),
	// but reconnects can happen during that startup window.
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		_ = newClient.Stop(ctx)
		return fmt.Errorf("supervisor stopped during reconnect")
	}
	s.opampClient = newClient
	s.mu.Unlock()

	return nil
}
