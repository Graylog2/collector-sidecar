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

// Package testserver provides a test OpAMP server for integration testing.
package testserver

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/open-telemetry/opamp-go/protobufs"
	"google.golang.org/protobuf/proto"
)

// Server is a test OpAMP server for integration testing.
type Server struct {
	// Server keys for signing enrollment JWTs
	ServerPublicKey  ed25519.PublicKey
	ServerPrivateKey ed25519.PrivateKey
	KeyID            string

	// CA keys for signing agent certificates
	CAPublicKey  ed25519.PublicKey
	CAPrivateKey ed25519.PrivateKey

	// HTTP server
	httpServer *httptest.Server

	// Connected agents
	mu     sync.RWMutex
	agents map[string]*AgentConnection

	// Enrolled agents (instance UID -> certificate) for JWT verification
	enrolledAgents map[string]*x509.Certificate

	// Callbacks
	OnAgentConnect    func(instanceUID string, conn *AgentConnection)
	OnAgentDisconnect func(instanceUID string)
	OnAgentMessage    func(instanceUID string, msg *protobufs.AgentToServer)
	OnCSRReceived     func(instanceUID string, csr *x509.CertificateRequest)

	// Configuration to send to agents
	remoteConfig *protobufs.AgentRemoteConfig

	// RequireAuth enables authentication verification
	RequireAuth bool

	upgrader websocket.Upgrader
}

// AgentConnection represents a connected agent.
type AgentConnection struct {
	InstanceUID string
	Conn        *websocket.Conn
	Certificate *x509.Certificate

	mu       sync.Mutex
	lastSeen time.Time
}

// New creates a new test OpAMP server.
func New() (*Server, error) {
	// Generate server signing keys (for enrollment JWTs)
	serverPub, serverPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate server keys: %w", err)
	}

	// Generate CA keys (for signing agent certificates)
	caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate CA keys: %w", err)
	}

	s := &Server{
		ServerPublicKey:  serverPub,
		ServerPrivateKey: serverPriv,
		KeyID:            "test-server-key-1",
		CAPublicKey:      caPub,
		CAPrivateKey:     caPriv,
		agents:           make(map[string]*AgentConnection),
		enrolledAgents:   make(map[string]*x509.Certificate),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		RequireAuth: true,
	}

	return s, nil
}

// Start starts the test server and returns its URL.
func (s *Server) Start() string {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/jwks.json", s.HandleJWKS)
	mux.HandleFunc("/v1/opamp", s.HandleOpAMP)

	s.httpServer = httptest.NewTLSServer(mux)
	return s.httpServer.URL
}

// Stop stops the test server.
func (s *Server) Stop() {
	if s.httpServer != nil {
		s.httpServer.Close()
	}
}

// Client returns an HTTP client configured to trust the test server's TLS cert.
func (s *Server) Client() *http.Client {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Client()
}

// URL returns the server's base URL.
func (s *Server) URL() string {
	if s.httpServer == nil {
		return ""
	}
	return s.httpServer.URL
}

// CreateEnrollmentJWT creates a signed enrollment JWT.
func (s *Server) CreateEnrollmentJWT(tenantID string, expiry time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"tenant_id":     tenantID,
		"key_algorithm": "Ed25519",
		"exp":           time.Now().Add(expiry).Unix(),
		"iat":           time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = s.KeyID

	return token.SignedString(s.ServerPrivateKey)
}

// CreateEnrollmentURL creates a full enrollment URL with embedded JWT.
func (s *Server) CreateEnrollmentURL(tenantID string, expiry time.Duration) (string, error) {
	jwt, err := s.CreateEnrollmentJWT(tenantID, expiry)
	if err != nil {
		return "", err
	}
	return s.URL() + "/opamp/enroll/" + jwt, nil
}

// SetRemoteConfig sets the configuration to send to agents.
func (s *Server) SetRemoteConfig(config *protobufs.AgentRemoteConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remoteConfig = config
}

// GetAgent returns the connection for a specific agent.
func (s *Server) GetAgent(instanceUID string) *AgentConnection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agents[instanceUID]
}

// AgentCount returns the number of connected agents.
func (s *Server) AgentCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.agents)
}

// SendToAgent sends a message to a specific agent.
func (s *Server) SendToAgent(instanceUID string, msg *protobufs.ServerToAgent) error {
	s.mu.RLock()
	agent := s.agents[instanceUID]
	s.mu.RUnlock()

	if agent == nil {
		return fmt.Errorf("agent %s not connected", instanceUID)
	}

	return agent.Send(msg)
}

// HandleJWKS serves the JWKS endpoint.
func (s *Server) HandleJWKS(w http.ResponseWriter, r *http.Request) {
	jwks := map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "OKP",
				"crv": "Ed25519",
				"kid": s.KeyID,
				"x":   base64.RawURLEncoding.EncodeToString(s.ServerPublicKey),
				"use": "sig",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jwks)
}

// AuthResult contains the result of authentication verification.
type AuthResult struct {
	// Authenticated is true if authentication succeeded
	Authenticated bool
	// IsEnrollment is true if this is an enrollment JWT (vs supervisor JWT)
	IsEnrollment bool
	// TenantID from the enrollment JWT (if IsEnrollment)
	TenantID string
	// InstanceUID from the supervisor JWT (if !IsEnrollment)
	InstanceUID string
	// Error message if authentication failed
	Error string
}

// checkAuth verifies the Authorization header and returns the auth result.
func (s *Server) checkAuth(r *http.Request) AuthResult {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return AuthResult{Error: "missing Authorization header"}
	}

	const bearerPrefix = "Bearer "
	if len(authHeader) < len(bearerPrefix) || authHeader[:len(bearerPrefix)] != bearerPrefix {
		return AuthResult{Error: "invalid Authorization header format"}
	}

	tokenString := authHeader[len(bearerPrefix):]

	// Try to validate as enrollment JWT first
	if result := s.validateEnrollmentJWT(tokenString); result.Authenticated {
		return result
	}

	// Try to validate as supervisor JWT
	if result := s.validateSupervisorJWT(tokenString); result.Authenticated {
		return result
	}

	return AuthResult{Error: "invalid token"}
}

// validateEnrollmentJWT validates an enrollment JWT signed by the server.
func (s *Server) validateEnrollmentJWT(tokenString string) AuthResult {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		// Verify key ID if present
		if kid, ok := token.Header["kid"].(string); ok && kid != s.KeyID {
			return nil, fmt.Errorf("unknown key ID: %s", kid)
		}
		return s.ServerPublicKey, nil
	})

	if err != nil || !token.Valid {
		return AuthResult{Error: "invalid enrollment JWT"}
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return AuthResult{Error: "invalid enrollment JWT claims"}
	}

	tenantID, _ := claims["tenant_id"].(string)

	return AuthResult{
		Authenticated: true,
		IsEnrollment:  true,
		TenantID:      tenantID,
	}
}

// validateSupervisorJWT validates a supervisor JWT signed by an enrolled agent.
func (s *Server) validateSupervisorJWT(tokenString string) AuthResult {
	// First parse without verification to get the instance UID
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return AuthResult{Error: "failed to parse supervisor JWT"}
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return AuthResult{Error: "invalid supervisor JWT claims"}
	}

	// Get instance UID from subject
	instanceUID, _ := claims["sub"].(string)
	if instanceUID == "" {
		return AuthResult{Error: "missing subject in supervisor JWT"}
	}

	// Look up the enrolled agent's certificate
	s.mu.RLock()
	cert := s.enrolledAgents[instanceUID]
	s.mu.RUnlock()

	if cert == nil {
		return AuthResult{Error: "unknown agent: " + instanceUID}
	}

	// Extract public key from certificate
	pubKey, ok := cert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return AuthResult{Error: "certificate does not contain Ed25519 public key"}
	}

	// Now verify the token with the agent's public key
	token, err = jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return pubKey, nil
	})

	if err != nil || !token.Valid {
		return AuthResult{Error: "supervisor JWT verification failed"}
	}

	return AuthResult{
		Authenticated: true,
		IsEnrollment:  false,
		InstanceUID:   instanceUID,
	}
}

// HandleOpAMP handles both WebSocket and HTTP OpAMP connections.
func (s *Server) HandleOpAMP(w http.ResponseWriter, r *http.Request) {
	// Check authentication if required
	if s.RequireAuth {
		authResult := s.checkAuth(r)
		if !authResult.Authenticated {
			http.Error(w, "Unauthorized: "+authResult.Error, http.StatusUnauthorized)
			return
		}
	}

	// Check if this is a WebSocket upgrade request
	if r.Header.Get("Upgrade") == "websocket" {
		conn, err := s.upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "failed to upgrade to websocket: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Handle the connection in a goroutine
		go s.handleAgentConnection(conn)
		return
	}

	// Handle HTTP POST for OpAMP polling
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.handleHTTPOpAMP(w, r)
}

// handleHTTPOpAMP handles HTTP-based OpAMP requests (polling mode).
func (s *Server) handleHTTPOpAMP(w http.ResponseWriter, r *http.Request) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse the message
	var msg protobufs.AgentToServer
	if err := proto.Unmarshal(body, &msg); err != nil {
		http.Error(w, "failed to parse message", http.StatusBadRequest)
		return
	}

	// Get or create agent connection for this request
	agent := s.getOrCreateAgent(&msg, nil)

	// Process message and create response
	response := s.processMessage(&msg, agent)

	// Marshal response
	respData, err := proto.Marshal(response)
	if err != nil {
		http.Error(w, "failed to marshal response", http.StatusInternalServerError)
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	w.Write(respData)
}

// handleAgentConnection handles an individual agent WebSocket connection.
func (s *Server) handleAgentConnection(conn *websocket.Conn) {
	defer conn.Close()

	var agent *AgentConnection

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			_ = messageType // avoid unused warning
			break
		}

		var msg protobufs.AgentToServer
		if err := proto.Unmarshal(message, &msg); err != nil {
			continue
		}

		// Get or create agent on first message
		if agent == nil {
			agent = s.getOrCreateAgent(&msg, conn)
		}

		// Process message and create response
		response := s.processMessage(&msg, agent)
		if err := agent.Send(response); err != nil {
			break
		}
	}

	// Cleanup
	if agent != nil && agent.InstanceUID != "" {
		s.mu.Lock()
		delete(s.agents, agent.InstanceUID)
		s.mu.Unlock()

		if s.OnAgentDisconnect != nil {
			s.OnAgentDisconnect(agent.InstanceUID)
		}
	}
}

// getOrCreateAgent returns an existing agent or creates a new one.
func (s *Server) getOrCreateAgent(msg *protobufs.AgentToServer, conn *websocket.Conn) *AgentConnection {
	var instanceUID string
	if msg.InstanceUid != nil {
		instanceUID = fmt.Sprintf("%x", msg.InstanceUid)
	}

	s.mu.Lock()
	if existing, ok := s.agents[instanceUID]; ok {
		s.mu.Unlock()
		return existing
	}

	agent := &AgentConnection{
		InstanceUID: instanceUID,
		Conn:        conn,
		lastSeen:    time.Now(),
	}

	if instanceUID != "" {
		s.agents[instanceUID] = agent
	}
	s.mu.Unlock()

	if instanceUID != "" && s.OnAgentConnect != nil {
		s.OnAgentConnect(instanceUID, agent)
	}

	return agent
}

// processMessage handles an incoming agent message and returns the response.
func (s *Server) processMessage(msg *protobufs.AgentToServer, agent *AgentConnection) *protobufs.ServerToAgent {
	agent.mu.Lock()
	agent.lastSeen = time.Now()
	agent.mu.Unlock()

	if s.OnAgentMessage != nil {
		s.OnAgentMessage(agent.InstanceUID, msg)
	}

	return s.createResponse(msg, agent)
}

// createResponse creates a response for an agent message.
func (s *Server) createResponse(msg *protobufs.AgentToServer, agent *AgentConnection) *protobufs.ServerToAgent {
	response := &protobufs.ServerToAgent{
		InstanceUid: msg.InstanceUid,
	}

	// Handle CSR request
	if csrRequest := msg.GetConnectionSettingsRequest(); csrRequest != nil {
		if opampRequest := csrRequest.GetOpamp(); opampRequest != nil {
			if certRequest := opampRequest.GetCertificateRequest(); certRequest != nil {
				certResponse := s.handleCSRRequest(certRequest.GetCsr(), agent)
				if certResponse != nil {
					response.ConnectionSettings = certResponse
				}
			}
		}
	}

	// Include remote config if we have one and agent reports capabilities
	s.mu.RLock()
	remoteConfig := s.remoteConfig
	s.mu.RUnlock()

	if remoteConfig != nil && msg.Capabilities&uint64(protobufs.AgentCapabilities_AgentCapabilities_AcceptsRemoteConfig) != 0 {
		response.RemoteConfig = remoteConfig
	}

	return response
}

// handleCSRRequest processes a CSR and returns connection settings with the certificate.
func (s *Server) handleCSRRequest(csrPEM []byte, agent *AgentConnection) *protobufs.ConnectionSettingsOffers {
	// Decode PEM
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil
	}

	// Parse CSR
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil
	}

	// Verify CSR signature
	if err := csr.CheckSignature(); err != nil {
		return nil
	}

	// Callback
	if s.OnCSRReceived != nil {
		s.OnCSRReceived(agent.InstanceUID, csr)
	}

	// Sign the CSR
	cert, err := s.signCSR(csr)
	if err != nil {
		return nil
	}

	// Store certificate in agent connection
	agent.mu.Lock()
	agent.Certificate = cert
	agent.mu.Unlock()

	// Store certificate for JWT verification (keyed by instance UID from CSR)
	instanceUID := csr.Subject.CommonName
	if instanceUID != "" {
		s.mu.Lock()
		s.enrolledAgents[instanceUID] = cert
		s.mu.Unlock()
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})

	return &protobufs.ConnectionSettingsOffers{
		Opamp: &protobufs.OpAMPConnectionSettings{
			Certificate: &protobufs.TLSCertificate{
				Cert: certPEM,
				// Note: private_key is NOT set - agent already has it
			},
		},
	}
}

// signCSR signs a CSR and returns a certificate.
func (s *Server) signCSR(csr *x509.CertificateRequest) (*x509.Certificate, error) {
	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   csr.Subject.CommonName,
			Organization: csr.Subject.Organization,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		Extensions:            csr.Extensions,
	}

	// Create CA certificate template for self-signing
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "Test CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caTemplate, csr.PublicKey, s.CAPrivateKey)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificate(certDER)
}

// Send sends a message to the agent.
func (a *AgentConnection) Send(msg *protobufs.ServerToAgent) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Conn.WriteMessage(websocket.BinaryMessage, data)
}

// LastSeen returns when the agent was last seen.
func (a *AgentConnection) LastSeen() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastSeen
}
