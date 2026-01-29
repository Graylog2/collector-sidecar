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

//go:build integration

package supervisor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/protobuf/proto"

	"github.com/Graylog2/collector-sidecar/superv/auth"
	"github.com/Graylog2/collector-sidecar/superv/internal/testserver"
	"github.com/Graylog2/collector-sidecar/superv/persistence"
)

func TestIntegration_Enrollment(t *testing.T) {
	// Create test server
	server, err := testserver.New()
	require.NoError(t, err)

	// Add debug logging
	server.OnCSRReceived = func(uid string, csr *x509.CertificateRequest) {
		t.Logf("SERVER: CSR received from %s, CN: %s", uid, csr.Subject.CommonName)
	}

	serverURL := server.Start()
	defer server.Stop()

	// Create enrollment URL
	enrollmentURL, err := server.CreateEnrollmentURL("test-tenant", time.Hour)
	require.NoError(t, err)

	// Setup dirs
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")
	logger := zaptest.NewLogger(t)

	// Create auth manager with test server's HTTP client
	authMgr := auth.NewManager(logger.Named("auth"), auth.ManagerConfig{
		KeysDir:     keysDir,
		JWTLifetime: 5 * time.Minute,
		HTTPClient:  server.Client(),
	})

	// Verify not enrolled initially
	require.False(t, authMgr.IsEnrolled())

	// Load instance UID
	instanceUID, err := persistence.LoadOrCreateInstanceUID(dir)
	require.NoError(t, err)

	// Phase 1: Prepare enrollment (validates JWT, generates keys, creates CSR)
	result, err := authMgr.PrepareEnrollment(context.Background(), enrollmentURL, instanceUID)
	require.NoError(t, err)
	require.NotEmpty(t, result.CSRPEM)
	require.Equal(t, "test-tenant", result.TenantID)

	// Verify pending state
	require.True(t, authMgr.HasPendingEnrollment())
	require.False(t, authMgr.IsEnrolled())

	// Phase 2: Send CSR via OpAMP WebSocket and receive certificate
	certPEM := sendCSRViaOpAMP(t, serverURL, instanceUID, result.CSRPEM)

	// Phase 3: Complete enrollment with certificate
	err = authMgr.CompleteEnrollment(certPEM)
	require.NoError(t, err)

	// Verify enrolled
	require.False(t, authMgr.HasPendingEnrollment())
	require.True(t, authMgr.IsEnrolled())
	require.NotEmpty(t, authMgr.CertFingerprint())

	// Verify we can generate a JWT
	jwt, err := authMgr.GenerateJWT("test.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, jwt)

	// Verify the JWT can be parsed
	certFP, claims, err := auth.ParseSupervisorJWT(jwt)
	require.NoError(t, err)
	require.Equal(t, instanceUID, claims.Subject)
	require.Contains(t, claims.Audience, "test.example.com")
	require.Equal(t, authMgr.CertFingerprint(), certFP)
}

func TestIntegration_EnrollmentPersistence(t *testing.T) {
	// Create test server
	server, err := testserver.New()
	require.NoError(t, err)

	serverURL := server.Start()
	defer server.Stop()

	// Create enrollment URL
	enrollmentURL, err := server.CreateEnrollmentURL("test-tenant", time.Hour)
	require.NoError(t, err)

	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")
	logger := zaptest.NewLogger(t)

	// First: prepare and complete enrollment
	authMgr1 := auth.NewManager(logger.Named("auth1"), auth.ManagerConfig{
		KeysDir:    keysDir,
		HTTPClient: server.Client(),
	})

	instanceUID, _ := persistence.LoadOrCreateInstanceUID(dir)

	// Prepare enrollment
	result, err := authMgr1.PrepareEnrollment(context.Background(), enrollmentURL, instanceUID)
	require.NoError(t, err)

	// Get certificate via OpAMP
	certPEM := sendCSRViaOpAMP(t, serverURL, instanceUID, result.CSRPEM)

	// Complete enrollment
	err = authMgr1.CompleteEnrollment(certPEM)
	require.NoError(t, err)

	fingerprint1 := authMgr1.CertFingerprint()

	// Second: create new manager and load credentials
	authMgr2 := auth.NewManager(logger.Named("auth2"), auth.ManagerConfig{
		KeysDir: keysDir,
	})

	require.True(t, authMgr2.IsEnrolled())

	err = authMgr2.LoadCredentials()
	require.NoError(t, err)

	// Verify same credentials
	require.Equal(t, fingerprint1, authMgr2.CertFingerprint())

	// Verify JWT generation works
	jwt, err := authMgr2.GenerateJWT("example.com")
	require.NoError(t, err)
	require.NotEmpty(t, jwt)
}

func TestIntegration_EnrollmentWithTenant(t *testing.T) {
	// Create test server
	server, err := testserver.New()
	require.NoError(t, err)

	serverURL := server.Start()
	defer server.Stop()

	// Create enrollment URL with specific tenant
	enrollmentURL, err := server.CreateEnrollmentURL("acme-corp", time.Hour)
	require.NoError(t, err)

	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")
	logger := zaptest.NewLogger(t)

	authMgr := auth.NewManager(logger.Named("auth"), auth.ManagerConfig{
		KeysDir:    keysDir,
		HTTPClient: server.Client(),
	})

	instanceUID, _ := persistence.LoadOrCreateInstanceUID(dir)

	// Prepare enrollment
	result, err := authMgr.PrepareEnrollment(context.Background(), enrollmentURL, instanceUID)
	require.NoError(t, err)
	require.Equal(t, "acme-corp", result.TenantID)

	// Get certificate via OpAMP
	certPEM := sendCSRViaOpAMP(t, serverURL, instanceUID, result.CSRPEM)

	// Complete enrollment
	err = authMgr.CompleteEnrollment(certPEM)
	require.NoError(t, err)

	// Verify the certificate contains the tenant
	cert := authMgr.Certificate()
	require.NotNil(t, cert)
	require.Equal(t, instanceUID, cert.Subject.CommonName)
	require.Contains(t, cert.Subject.Organization, "acme-corp")
}

func TestIntegration_JWTExpiry(t *testing.T) {
	// Create test server
	server, err := testserver.New()
	require.NoError(t, err)

	serverURL := server.Start()
	defer server.Stop()

	enrollmentURL, err := server.CreateEnrollmentURL("test-tenant", time.Hour)
	require.NoError(t, err)

	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")
	logger := zaptest.NewLogger(t)

	// Create manager with very short JWT lifetime (2 seconds)
	authMgr := auth.NewManager(logger.Named("auth"), auth.ManagerConfig{
		KeysDir:     keysDir,
		JWTLifetime: 2 * time.Second,
		HTTPClient:  server.Client(),
	})

	instanceUID, _ := persistence.LoadOrCreateInstanceUID(dir)

	// Prepare enrollment
	result, err := authMgr.PrepareEnrollment(context.Background(), enrollmentURL, instanceUID)
	require.NoError(t, err)

	// Get certificate via OpAMP
	certPEM := sendCSRViaOpAMP(t, serverURL, instanceUID, result.CSRPEM)

	// Complete enrollment
	err = authMgr.CompleteEnrollment(certPEM)
	require.NoError(t, err)

	// Generate JWT
	jwt1, err := authMgr.GenerateJWT("example.com")
	require.NoError(t, err)

	// Parse and check expiry - should not be expired yet
	_, claims1, err := auth.ParseSupervisorJWT(jwt1)
	require.NoError(t, err)
	require.False(t, claims1.IsExpired())

	// Check IsExpiringSoon with different thresholds
	require.True(t, claims1.IsExpiringSoon(5*time.Second))          // expires within 5s - true
	require.False(t, claims1.IsExpiringSoon(500*time.Millisecond)) // expires within 500ms - false (we have ~2s)

	// Wait for expiry
	time.Sleep(2100 * time.Millisecond)

	// Should be expired now
	require.True(t, claims1.IsExpired())

	// But we can generate a new one
	jwt2, err := authMgr.GenerateJWT("example.com")
	require.NoError(t, err)
	require.NotEqual(t, jwt1, jwt2)

	_, claims2, err := auth.ParseSupervisorJWT(jwt2)
	require.NoError(t, err)
	require.False(t, claims2.IsExpired())
}

// sendCSRViaOpAMP sends a CSR to the test server via OpAMP WebSocket and returns the certificate.
func sendCSRViaOpAMP(t *testing.T, serverURL, instanceUID string, csrPEM []byte) []byte {
	t.Helper()

	wsURL := strings.Replace(serverURL, "https://", "wss://", 1) + "/v1/opamp"

	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Create instance UID bytes (truncate or pad to 16 bytes)
	uidBytes := make([]byte, 16)
	copy(uidBytes, []byte(instanceUID))

	// Send message with CSR
	msg := &protobufs.AgentToServer{
		InstanceUid:  uidBytes,
		Capabilities: uint64(protobufs.AgentCapabilities_AgentCapabilities_AcceptsOpAMPConnectionSettings),
		ConnectionSettingsRequest: &protobufs.ConnectionSettingsRequest{
			Opamp: &protobufs.OpAMPConnectionSettingsRequest{
				CertificateRequest: &protobufs.CertificateRequest{
					Csr: csrPEM,
				},
			},
		},
	}

	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	err = conn.WriteMessage(websocket.BinaryMessage, data)
	require.NoError(t, err)

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, respData, err := conn.ReadMessage()
	require.NoError(t, err)

	var respMsg protobufs.ServerToAgent
	err = proto.Unmarshal(respData, &respMsg)
	require.NoError(t, err)

	// Verify we got a certificate back
	require.NotNil(t, respMsg.ConnectionSettings, "expected connection settings in response")
	require.NotNil(t, respMsg.ConnectionSettings.Opamp, "expected opamp settings in response")
	require.NotNil(t, respMsg.ConnectionSettings.Opamp.Certificate, "expected certificate in response")
	certPEM := respMsg.ConnectionSettings.Opamp.Certificate.Cert
	require.NotEmpty(t, certPEM, "expected non-empty certificate")

	// Verify it's a valid PEM certificate
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)

	return certPEM
}
