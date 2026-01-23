// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

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
	mu          sync.Mutex
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
	s.opampServer = server.New(newLoggerFromZap(s.logger))

	settings := server.StartSettings{
		Settings: server.Settings{
			Callbacks: types.Callbacks{
				OnConnecting: s.onConnecting,
			},
		},
		ListenEndpoint: s.cfg.ListenEndpoint,
	}

	return s.opampServer.Start(settings)
}

// Stop stops the local OpAMP server.
func (s *Server) Stop(ctx context.Context) error {
	if s.opampServer == nil {
		return nil
	}
	return s.opampServer.Stop(ctx)
}

// Addr returns the server's listen address.
func (s *Server) Addr() string {
	if s.opampServer == nil {
		return ""
	}
	addr := s.opampServer.Addr()
	if addr == nil {
		return ""
	}
	return addr.String()
}

// SendToAgent sends a message to a connected agent.
func (s *Server) SendToAgent(ctx context.Context, conn types.Connection, msg *protobufs.ServerToAgent) error {
	return conn.Send(ctx, msg)
}

// Broadcast sends a message to all connected agents.
func (s *Server) Broadcast(ctx context.Context, msg *protobufs.ServerToAgent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for conn := range s.connections {
		if err := conn.Send(ctx, msg); err != nil {
			s.logger.Error("Failed to send to agent", zap.Error(err))
		}
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
