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
	"os"
	"runtime"
	"sync"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server/types"
	"go.uber.org/zap"

	"github.com/Graylog2/collector-sidecar/superv/internal/config"
	"github.com/Graylog2/collector-sidecar/superv/internal/keen"
	"github.com/Graylog2/collector-sidecar/superv/internal/opamp"
	"github.com/Graylog2/collector-sidecar/superv/internal/persistence"
	"github.com/Graylog2/collector-sidecar/superv/internal/version"
)

// Supervisor coordinates the management of an OpenTelemetry Collector.
type Supervisor struct {
	logger      *zap.Logger
	cfg         config.Config
	instanceUID string
	commander   *keen.Commander
	opampClient *opamp.Client
	opampServer *opamp.Server
	mu          sync.RWMutex
	running     bool
}

// New creates a new Supervisor instance.
func New(logger *zap.Logger, cfg config.Config) (*Supervisor, error) {
	// Load or create instance UID
	uid, err := persistence.LoadOrCreateInstanceUID(cfg.Persistence.Dir)
	if err != nil {
		return nil, err
	}

	return &Supervisor{
		logger:      logger,
		cfg:         cfg,
		instanceUID: uid,
	}, nil
}

// InstanceUID returns the supervisor's unique instance identifier.
func (s *Supervisor) InstanceUID() string {
	return s.instanceUID
}

// Start starts the supervisor and begins managing the collector.
func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	s.logger.Info("Starting supervisor",
		zap.String("instance_uid", s.instanceUID),
		zap.String("endpoint", s.cfg.Server.Endpoint),
	)

	// Create commander for agent process management
	cmd, err := keen.New(s.logger, s.cfg.Persistence.Dir, keen.Config{
		Executable:      s.cfg.Agent.Executable,
		Args:            s.cfg.Agent.Args,
		Env:             s.cfg.Agent.Env,
		PassthroughLogs: s.cfg.Agent.PassthroughLogs,
	})
	if err != nil {
		return err
	}
	s.commander = cmd

	// Create local OpAMP server for collector
	serverCallbacks := &opamp.ServerCallbacks{
		OnConnect: func(conn types.Connection) {
			s.logger.Info("Collector connected to local OpAMP server")
		},
		OnDisconnect: func(conn types.Connection) {
			s.logger.Info("Collector disconnected from local OpAMP server")
		},
	}

	opampServer, err := opamp.NewServer(s.logger, opamp.ServerConfig{
		ListenEndpoint: s.cfg.LocalOpAMP.Endpoint,
	}, serverCallbacks)
	if err != nil {
		return err
	}
	s.opampServer = opampServer

	// Start local OpAMP server
	if err := s.opampServer.Start(ctx); err != nil {
		return err
	}

	// Create OpAMP client for upstream server
	clientCallbacks := &opamp.Callbacks{
		OnConnect: func(ctx context.Context) {
			s.logger.Info("Connected to OpAMP server")
		},
		OnConnectFailed: func(ctx context.Context, err error) {
			s.logger.Error("Failed to connect to OpAMP server", zap.Error(err))
		},
		OnRemoteConfig: func(ctx context.Context, cfg *protobufs.AgentRemoteConfig) bool {
			s.logger.Info("Received remote configuration")
			// TODO: Apply configuration
			return true
		},
		OnOpampConnectionSettings: func(ctx context.Context, settings *protobufs.OpAMPConnectionSettings) error {
			s.logger.Info("Received connection settings update")
			// TODO: Handle token refresh
			return nil
		},
	}

	opampClient, err := opamp.NewClient(s.logger, opamp.ClientConfig{
		Endpoint:    s.cfg.Server.Endpoint,
		InstanceUID: s.instanceUID,
		Headers:     s.cfg.Server.ToHTTPHeaders(),
		Capabilities: opamp.Capabilities{
			ReportsStatus:                  true,
			AcceptsRemoteConfig:            true,
			ReportsEffectiveConfig:         true,
			ReportsHealth:                  true,
			AcceptsOpAMPConnectionSettings: true,
			AcceptsRestartCommand:          true,
		},
	}, clientCallbacks)
	if err != nil {
		s.opampServer.Stop(ctx)
		return err
	}
	s.opampClient = opampClient

	// Set initial agent description before starting
	if err := s.opampClient.SetAgentDescription(s.createAgentDescription()); err != nil {
		s.opampServer.Stop(ctx)
		return err
	}

	// Set initial health status before starting
	if err := s.opampClient.SetHealth(&protobufs.ComponentHealth{
		Healthy: true,
	}); err != nil {
		s.opampServer.Stop(ctx)
		return err
	}

	// Start OpAMP client
	if err := s.opampClient.Start(ctx); err != nil {
		s.opampServer.Stop(ctx)
		return err
	}

	// Start the collector agent
	if err := s.commander.Start(ctx); err != nil {
		s.opampClient.Stop(ctx)
		s.opampServer.Stop(ctx)
		return err
	}

	s.running = true
	return nil
}

// Stop stops the supervisor and the managed collector.
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("Stopping supervisor")

	// Stop commander (agent)
	if s.commander != nil {
		if err := s.commander.Stop(ctx); err != nil {
			s.logger.Error("Error stopping agent", zap.Error(err))
		}
	}

	// Stop OpAMP client
	if s.opampClient != nil {
		if err := s.opampClient.Stop(ctx); err != nil {
			s.logger.Error("Error stopping OpAMP client", zap.Error(err))
		}
	}

	// Stop OpAMP server
	if s.opampServer != nil {
		if err := s.opampServer.Stop(ctx); err != nil {
			s.logger.Error("Error stopping OpAMP server", zap.Error(err))
		}
	}

	s.running = false
	return nil
}

// IsRunning returns true if the supervisor is running.
func (s *Supervisor) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// createAgentDescription creates the initial agent description for OpAMP.
func (s *Supervisor) createAgentDescription() *protobufs.AgentDescription {
	hostname, _ := os.Hostname()

	return &protobufs.AgentDescription{
		IdentifyingAttributes: []*protobufs.KeyValue{
			{
				Key:   "service.name",
				Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: "opamp-supervisor"}},
			},
			{
				Key:   "service.instance.id",
				Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: s.instanceUID}},
			},
		},
		NonIdentifyingAttributes: []*protobufs.KeyValue{
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
		},
	}
}
