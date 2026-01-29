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

package main

import (
	"context"
	"crypto/sha256"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/google/uuid"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server"
	"github.com/open-telemetry/opamp-go/server/types"
)

const defaultConfigFile = "agent-config.yaml"

// OpAMPLogger implements the types.Logger interface
type OpAMPLogger struct {
	logger *log.Logger
}

func NewOpAMPLogger(logger *log.Logger) *OpAMPLogger {
	return &OpAMPLogger{logger: logger}
}

func (l *OpAMPLogger) Debugf(ctx context.Context, format string, v ...interface{}) {
	l.logger.Printf("[DEBUG] "+format, v...)
}

func (l *OpAMPLogger) Errorf(ctx context.Context, format string, v ...interface{}) {
	l.logger.Printf("[ERROR] "+format, v...)
}

type Agent struct {
	InstanceID string
	Status     *protobufs.AgentToServer
	Conn       types.Connection
	mutex      sync.Mutex
}

type AgentsMap struct {
	agents map[string]*Agent
	mutex  sync.RWMutex
}

func NewAgentsMap() *AgentsMap {
	return &AgentsMap{
		agents: make(map[string]*Agent),
	}
}

func (am *AgentsMap) GetOrCreate(instanceID string, conn types.Connection) *Agent {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	if agent, ok := am.agents[instanceID]; ok {
		agent.Conn = conn
		return agent
	}

	agent := &Agent{
		InstanceID: instanceID,
		Conn:       conn,
	}
	am.agents[instanceID] = agent
	return agent
}

func (am *AgentsMap) Remove(instanceID string) {
	am.mutex.Lock()
	defer am.mutex.Unlock()
	delete(am.agents, instanceID)
}

func (am *AgentsMap) Count() int {
	am.mutex.RLock()
	defer am.mutex.RUnlock()
	return len(am.agents)
}

type RemoteConfig struct {
	content   []byte
	hash      []byte
	configMap *protobufs.AgentConfigMap
	mutex     sync.RWMutex
}

func NewRemoteConfig() *RemoteConfig {
	return &RemoteConfig{}
}

func (rc *RemoteConfig) Load(filePath string) error {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	hash := sha256.Sum256(content)
	rc.content = content
	rc.hash = hash[:]

	rc.configMap = &protobufs.AgentConfigMap{
		ConfigMap: map[string]*protobufs.AgentConfigFile{
			"": {
				Body:        content,
				ContentType: "text/yaml",
			},
		},
	}

	return nil
}

func (rc *RemoteConfig) GetRemoteConfig() *protobufs.AgentRemoteConfig {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	if rc.configMap == nil {
		return nil
	}

	return &protobufs.AgentRemoteConfig{
		Config:     rc.configMap,
		ConfigHash: rc.hash,
	}
}

func (rc *RemoteConfig) HasConfig() bool {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()
	return rc.configMap != nil
}

type OpAMPServer struct {
	server       server.OpAMPServer
	agents       *AgentsMap
	remoteConfig *RemoteConfig
	logger       *log.Logger
}

func NewOpAMPServer() *OpAMPServer {
	return &OpAMPServer{
		agents:       NewAgentsMap(),
		remoteConfig: NewRemoteConfig(),
		logger:       log.New(os.Stdout, "[OpAMP Server] ", log.LstdFlags|log.Lmsgprefix),
	}
}

func (s *OpAMPServer) LoadConfig(configFile string) error {
	if err := s.remoteConfig.Load(configFile); err != nil {
		return err
	}
	s.logger.Printf("Loaded remote config from %s", configFile)
	return nil
}

func (s *OpAMPServer) OnConnecting(request *http.Request) types.ConnectionResponse {
	s.logger.Printf("Agent connecting from %s", request.RemoteAddr)
	if request.Header.Get("Authorization") != "" {
		s.logger.Printf("Agent connecting with Authorization: %s", request.Header.Get("Authorization"))
	}
	return types.ConnectionResponse{
		Accept: true,
	}
}

func (s *OpAMPServer) OnConnected(ctx context.Context, conn types.Connection) {
	s.logger.Printf("Agent connected: %s", conn.Connection().RemoteAddr())
}

func (s *OpAMPServer) OnMessage(ctx context.Context, conn types.Connection, msg *protobufs.AgentToServer) *protobufs.ServerToAgent {
	instanceUUID, err := uuid.FromBytes(msg.InstanceUid)
	instanceID := "unknown"
	if err == nil {
		instanceID = instanceUUID.String()
	}

	agent := s.agents.GetOrCreate(instanceID, conn)
	agent.mutex.Lock()
	agent.Status = msg
	agent.mutex.Unlock()

	s.logger.Printf("Message from agent %s", instanceID)

	if msg.AgentDescription != nil {
		s.logAgentDescription(msg.AgentDescription)
	}

	if msg.Health != nil {
		s.logAgentHealth(msg.Health)
	}

	if msg.EffectiveConfig != nil {
		s.logger.Printf("  Received effective config")
		configMap := msg.EffectiveConfig.GetConfigMap().ConfigMap
		s.logger.Println("\n" + string(configMap[""].Body))
	}

	if msg.RemoteConfigStatus != nil {
		s.logRemoteConfigStatus(msg.RemoteConfigStatus)
	}

	if msg.PackageStatuses != nil {
		s.logger.Printf("  Package statuses received: %d (error: %q)", len(msg.PackageStatuses.Packages), msg.PackageStatuses.ErrorMessage)
	}

	if msg.CustomCapabilities != nil {
		s.logger.Printf("  Custom capabilities received: %v", msg.CustomCapabilities.Capabilities)
	}

	response := &protobufs.ServerToAgent{
		InstanceUid:  msg.InstanceUid,
		Capabilities: s.getServerCapabilities(),
		ConnectionSettings: &protobufs.ConnectionSettingsOffers{
			OwnLogs: &protobufs.TelemetryConnectionSettings{
				DestinationEndpoint: "localhost:8080",
				Headers: &protobufs.Headers{
					Headers: []*protobufs.Header{
						{
							Key:   "X-OTLP-Access-Token",
							Value: "s3cr3t",
						},
					},
				},
			},
		},
	}

	// Request full agent status on first connect
	if msg.AgentDescription == nil {
		response.Flags = uint64(protobufs.ServerToAgentFlags_ServerToAgentFlags_ReportFullState)
	}

	// Send remote config if available and agent hasn't applied it yet
	if s.remoteConfig.HasConfig() {
		if s.shouldSendConfig(msg) {
			response.RemoteConfig = s.remoteConfig.GetRemoteConfig()
			s.logger.Printf("  Sending remote config to agent %s", instanceID)
		}
	}

	return response
}

func (s *OpAMPServer) shouldSendConfig(msg *protobufs.AgentToServer) bool {
	// Send config if agent reports no config status
	if msg.RemoteConfigStatus == nil {
		return true
	}

	// Send config if agent's config hash doesn't match ours
	remoteConfig := s.remoteConfig.GetRemoteConfig()
	if remoteConfig == nil {
		return false
	}

	// Compare hashes
	agentHash := msg.RemoteConfigStatus.LastRemoteConfigHash
	serverHash := remoteConfig.ConfigHash

	if len(agentHash) != len(serverHash) {
		return true
	}

	for i := range agentHash {
		if agentHash[i] != serverHash[i] {
			return true
		}
	}

	return false
}

func (s *OpAMPServer) OnConnectionClose(conn types.Connection) {
	s.logger.Printf("Agent disconnected: %s", conn.Connection().RemoteAddr())
}

func (s *OpAMPServer) logAgentDescription(desc *protobufs.AgentDescription) {
	s.logger.Printf("  Agent Description:")
	if desc.IdentifyingAttributes != nil {
		for _, attr := range desc.IdentifyingAttributes {
			s.logger.Printf("    [ID] %s: %v", attr.Key, getAttrValue(attr.Value))
		}
	}
	if desc.NonIdentifyingAttributes != nil {
		for _, attr := range desc.NonIdentifyingAttributes {
			s.logger.Printf("    [Attr] %s: %v", attr.Key, getAttrValue(attr.Value))
		}
	}
}

func (s *OpAMPServer) logAgentHealth(health *protobufs.ComponentHealth) {
	status := "unhealthy"
	if health.Healthy {
		status = "healthy"
	}
	s.logger.Printf("  Agent Health: %s", status)
	if health.LastError != "" {
		s.logger.Printf("    Last error: %s", health.LastError)
	}
	if health.StatusTimeUnixNano > 0 {
		s.logger.Printf("    Status time: %d", health.StatusTimeUnixNano)
	}
}

func (s *OpAMPServer) logRemoteConfigStatus(status *protobufs.RemoteConfigStatus) {
	statusStr := "unknown"
	switch status.Status {
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_UNSET:
		statusStr = "unset"
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED:
		statusStr = "applied"
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLYING:
		statusStr = "applying"
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED:
		statusStr = "failed"
	}
	s.logger.Printf("  Remote Config Status: %s", statusStr)
	if status.ErrorMessage != "" {
		s.logger.Printf("    Error: %s", status.ErrorMessage)
	}
}

func (s *OpAMPServer) getServerCapabilities() uint64 {
	return uint64(
		protobufs.ServerCapabilities_ServerCapabilities_AcceptsStatus |
			protobufs.ServerCapabilities_ServerCapabilities_OffersRemoteConfig |
			protobufs.ServerCapabilities_ServerCapabilities_AcceptsEffectiveConfig |
			protobufs.ServerCapabilities_ServerCapabilities_OffersPackages |
			protobufs.ServerCapabilities_ServerCapabilities_AcceptsPackagesStatus |
			protobufs.ServerCapabilities_ServerCapabilities_OffersConnectionSettings,
	)
}

func getAttrValue(val *protobufs.AnyValue) any {
	if val == nil {
		return nil
	}
	switch v := val.Value.(type) {
	case *protobufs.AnyValue_StringValue:
		return v.StringValue
	case *protobufs.AnyValue_IntValue:
		return v.IntValue
	case *protobufs.AnyValue_DoubleValue:
		return v.DoubleValue
	case *protobufs.AnyValue_BoolValue:
		return v.BoolValue
	case *protobufs.AnyValue_BytesValue:
		return v.BytesValue
	default:
		return val
	}
}

func (s *OpAMPServer) Start(listenAddr string) error {
	s.server = server.New(NewOpAMPLogger(s.logger))

	settings := server.StartSettings{
		Settings: server.Settings{
			Callbacks: types.Callbacks{
				OnConnecting: func(request *http.Request) types.ConnectionResponse {
					resp := s.OnConnecting(request)
					resp.ConnectionCallbacks = types.ConnectionCallbacks{
						OnConnected:       s.OnConnected,
						OnMessage:         s.OnMessage,
						OnConnectionClose: s.OnConnectionClose,
					}
					return resp
				},
			},
		},
		ListenEndpoint: listenAddr,
	}

	s.logger.Printf("Starting OpAMP server on %s", listenAddr)
	s.logger.Printf("WebSocket endpoint: ws://%s/v1/opamp", listenAddr)

	return s.server.Start(settings)
}

func (s *OpAMPServer) Stop(ctx context.Context) {
	if s.server != nil {
		s.logger.Printf("Stopping OpAMP server...")
		s.server.Stop(ctx)
	}
}

func main() {
	listenAddr := os.Getenv("OPAMP_LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = "0.0.0.0:9000"
	}

	configFile := os.Getenv("OPAMP_CONFIG_FILE")
	if configFile == "" {
		configFile = defaultConfigFile
	}

	opampServer := NewOpAMPServer()

	// Load remote config if file exists
	if _, err := os.Stat(configFile); err == nil {
		if err := opampServer.LoadConfig(configFile); err != nil {
			log.Printf("Warning: Failed to load config file %s: %v", configFile, err)
		}
	} else {
		log.Printf("No config file found at %s, remote config disabled", configFile)
	}

	if err := opampServer.Start(listenAddr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Server running. Press Ctrl+C to stop.")
	<-sigCh

	opampServer.Stop(context.Background())
	log.Printf("Server stopped.")
}
