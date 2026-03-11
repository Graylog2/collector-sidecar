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
	"errors"
	"net/http"
	"sync"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server"
	"github.com/open-telemetry/opamp-go/server/types"
	"go.uber.org/zap"
)

// ServerConfig holds configuration for the local OpAMP server.
type ServerConfig struct {
	ListenEndpoint string
}

// Validate validates the server configuration.
func (c ServerConfig) Validate() error {
	if c.ListenEndpoint == "" {
		return errors.New("listen endpoint is required")
	}
	return nil
}

// ServerCallbacks handles OpAMP server callbacks.
type ServerCallbacks struct {
	OnConnect        func(conn types.Connection)
	OnDisconnect     func(conn types.Connection)
	OnMessage        func(conn types.Connection, msg *protobufs.AgentToServer) *protobufs.ServerToAgent
	OnConnectingFunc func(request *http.Request) (accept bool, rejectStatusCode int)
}

// Server wraps the opamp-go server for local collector communication.
type Server struct {
	logger      *zap.Logger
	cfg         ServerConfig
	callbacks   *ServerCallbacks
	opampServer server.OpAMPServer
	mu          sync.RWMutex
	connections map[types.Connection]struct{}
}

// NewServer creates a new local OpAMP server.
func NewServer(logger *zap.Logger, cfg ServerConfig, callbacks *ServerCallbacks) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Server{
		logger:      logger,
		cfg:         cfg,
		callbacks:   callbacks,
		connections: make(map[types.Connection]struct{}),
	}, nil
}

// Start starts the local OpAMP server.
func (s *Server) Start(_ context.Context) error {
	srv := server.New(newLoggerFromZap(s.logger))

	settings := server.StartSettings{
		Settings: server.Settings{
			Callbacks: types.Callbacks{
				OnConnecting: s.onConnecting,
			},
		},
		ListenEndpoint: s.cfg.ListenEndpoint,
	}

	if err := srv.Start(settings); err != nil {
		return err
	}

	s.mu.Lock()
	s.opampServer = srv
	s.mu.Unlock()

	return nil
}

// Stop stops the local OpAMP server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.RLock()
	srv := s.opampServer
	s.mu.RUnlock()

	if srv == nil {
		return nil
	}
	return srv.Stop(ctx)
}

// Addr returns the server's listen address.
func (s *Server) Addr() string {
	s.mu.RLock()
	srv := s.opampServer
	s.mu.RUnlock()

	if srv == nil {
		return ""
	}
	addr := srv.Addr()
	if addr == nil {
		return ""
	}
	return addr.String()
}

// Broadcast sends a message to all connected agents.
func (s *Server) Broadcast(ctx context.Context, msg *protobufs.ServerToAgent) {
	// Snapshot connections under lock, then send outside lock
	s.mu.RLock()
	conns := make([]types.Connection, 0, len(s.connections))
	for conn := range s.connections {
		conns = append(conns, conn)
	}
	s.mu.RUnlock()

	// Send to each connection outside the lock
	for _, conn := range conns {
		if err := conn.Send(ctx, msg); err != nil {
			s.logger.Warn("Failed to send broadcast message", zap.Error(err))
		}
	}
}

// DisconnectAll cleanly closes all active agent connections. Closing the
// underlying network connection from the server side causes the read loop
// to exit without logging an error (as opposed to the agent process dying
// with an unclean WebSocket close). Safe to call on a nil receiver.
func (s *Server) DisconnectAll() {
	if s == nil {
		return
	}
	s.mu.Lock()
	conns := make([]types.Connection, 0, len(s.connections))
	for conn := range s.connections {
		conns = append(conns, conn)
	}
	s.mu.Unlock()

	for _, conn := range conns {
		_ = conn.Disconnect()
	}
}

// Server callback implementations

func (s *Server) onConnecting(request *http.Request) types.ConnectionResponse {
	// Check if custom onConnecting function is provided
	if s.callbacks != nil && s.callbacks.OnConnectingFunc != nil {
		accept, rejectStatusCode := s.callbacks.OnConnectingFunc(request)
		if !accept {
			return types.ConnectionResponse{
				Accept:         false,
				HTTPStatusCode: rejectStatusCode,
			}
		}
	}

	return types.ConnectionResponse{
		Accept: true,
		ConnectionCallbacks: types.ConnectionCallbacks{
			OnConnected:       s.onConnected,
			OnMessage:         s.onMessage,
			OnConnectionClose: s.onConnectionClose,
		},
	}
}

func (s *Server) onConnected(_ context.Context, conn types.Connection) {
	s.mu.Lock()
	s.connections[conn] = struct{}{}
	s.mu.Unlock()

	s.logger.Debug("Agent connected")
	if s.callbacks != nil && s.callbacks.OnConnect != nil {
		s.callbacks.OnConnect(conn)
	}
}

func (s *Server) onMessage(_ context.Context, conn types.Connection, msg *protobufs.AgentToServer) *protobufs.ServerToAgent {
	if s.callbacks != nil && s.callbacks.OnMessage != nil {
		return s.callbacks.OnMessage(conn, msg)
	}
	// Return empty response if no callback is provided
	return &protobufs.ServerToAgent{}
}

func (s *Server) onConnectionClose(conn types.Connection) {
	s.mu.Lock()
	delete(s.connections, conn)
	s.mu.Unlock()

	s.logger.Debug("Agent disconnected")
	if s.callbacks != nil && s.callbacks.OnDisconnect != nil {
		s.callbacks.OnDisconnect(conn)
	}
}
