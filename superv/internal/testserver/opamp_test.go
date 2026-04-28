// Copyright (C)  2026 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.

package testserver

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func doPOST(t *testing.T, client *http.Client, url string, data []byte) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, bytes.NewReader(data))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing POST request: %w", err)
	}
	return resp, nil
}

func TestServer_OpAMP_WebSocket(t *testing.T) {
	server, err := New()
	require.NoError(t, err)
	server.RequireAuth = false

	url := server.Start()
	defer server.Stop()

	// Convert to websocket URL
	wsURL := strings.Replace(url, "https://", "wss://", 1) + "/v1/opamp"
	t.Logf("WebSocket URL: %s", wsURL)

	// Create WebSocket dialer with TLS config
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// Connect
	conn, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v (response: %v)", err, resp)
	}
	defer conn.Close()
	defer resp.Body.Close()
	t.Log("Connected!")

	// Create a test message
	instanceUID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	msg := &protobufs.AgentToServer{
		InstanceUid:  instanceUID,
		Capabilities: uint64(protobufs.AgentCapabilities_AgentCapabilities_AcceptsOpAMPConnectionSettings),
	}

	// Marshal and send
	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	err = conn.WriteMessage(websocket.BinaryMessage, data)
	require.NoError(t, err)
	t.Log("Sent message")

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, respData, err := conn.ReadMessage()
	require.NoError(t, err)
	t.Logf("Got response: %d bytes", len(respData))

	var respMsg protobufs.ServerToAgent
	err = proto.Unmarshal(respData, &respMsg)
	require.NoError(t, err)
	t.Logf("Response instance UID: %x", respMsg.GetInstanceUid())
}

func TestServer_OpAMP_HTTP(t *testing.T) {
	server, err := New()
	require.NoError(t, err)
	server.RequireAuth = false

	url := server.Start()
	defer server.Stop()

	httpURL := url + "/v1/opamp"
	t.Logf("HTTP URL: %s", httpURL)

	// Create a test message
	instanceUID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	msg := &protobufs.AgentToServer{
		InstanceUid:  instanceUID,
		Capabilities: uint64(protobufs.AgentCapabilities_AgentCapabilities_AcceptsOpAMPConnectionSettings),
	}

	// Marshal message
	data, err := proto.Marshal(msg)
	require.NoError(t, err)

	// Send POST request
	resp, err := doPOST(t, server.Client(), httpURL, data)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Read response
	respData, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	t.Logf("Got response: %d bytes", len(respData))

	var respMsg protobufs.ServerToAgent
	err = proto.Unmarshal(respData, &respMsg)
	require.NoError(t, err)
	t.Logf("Response instance UID: %x", respMsg.GetInstanceUid())
}

func TestServer_OpAMP_HTTP_CSR(t *testing.T) {
	server, err := New()
	require.NoError(t, err)
	server.RequireAuth = false

	recorder := NewTestRecorder()
	server.Logger = recorder

	url := server.Start()
	defer server.Stop()

	httpURL := url + "/v1/opamp"

	// Generate a real CSR for testing
	_, signingPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "test-instance-http",
			Organization: []string{"test"},
		},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, signingPriv)
	require.NoError(t, err)

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	// Send message with CSR
	instanceUID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	msg := &protobufs.AgentToServer{
		InstanceUid:  instanceUID,
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

	resp, err := doPOST(t, server.Client(), httpURL, data)
	require.NoError(t, err)
	defer resp.Body.Close()
	t.Log("Sent CSR message via HTTP")

	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Wait for CSR event
	event, err := recorder.WaitForKind(EventCSRReceived, 2*time.Second)
	require.NoError(t, err)
	csr := event.Data.(*x509.CertificateRequest)
	t.Logf("CSR received, CN: %s", csr.Subject.CommonName)

	// Read response
	respData, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var respMsg protobufs.ServerToAgent
	err = proto.Unmarshal(respData, &respMsg)
	require.NoError(t, err)

	// Verify we got a certificate back
	require.NotNil(t, respMsg.GetConnectionSettings())
	require.NotNil(t, respMsg.GetConnectionSettings().GetOpamp())
	require.NotNil(t, respMsg.GetConnectionSettings().GetOpamp().GetCertificate())
	require.NotEmpty(t, respMsg.GetConnectionSettings().GetOpamp().GetCertificate().GetCert())

	t.Logf("Got certificate: %d bytes", len(respMsg.GetConnectionSettings().GetOpamp().GetCertificate().GetCert()))

	// Verify it's a valid PEM certificate
	block, _ := pem.Decode(respMsg.GetConnectionSettings().GetOpamp().GetCertificate().GetCert())
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.Equal(t, "test-instance-http", cert.Subject.CommonName)
	require.Contains(t, cert.Subject.Organization, "test")
}

func TestServer_OpAMP_CSR(t *testing.T) {
	server, err := New()
	require.NoError(t, err)
	server.RequireAuth = false

	recorder := NewTestRecorder()
	server.Logger = recorder

	url := server.Start()
	defer server.Stop()

	wsURL := strings.Replace(url, "https://", "wss://", 1) + "/v1/opamp"

	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	conn, resp, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()
	defer resp.Body.Close()

	// Generate a real CSR for testing
	_, signingPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "test-instance",
			Organization: []string{"test"},
		},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, signingPriv)
	require.NoError(t, err)

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	// Send message with CSR
	instanceUID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	msg := &protobufs.AgentToServer{
		InstanceUid:  instanceUID,
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
	t.Log("Sent CSR message")

	// Wait for CSR event
	event, err := recorder.WaitForKind(EventCSRReceived, 2*time.Second)
	require.NoError(t, err)
	csr := event.Data.(*x509.CertificateRequest)
	t.Logf("CSR received, CN: %s", csr.Subject.CommonName)

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, respData, err := conn.ReadMessage()
	require.NoError(t, err)

	var respMsg protobufs.ServerToAgent
	err = proto.Unmarshal(respData, &respMsg)
	require.NoError(t, err)

	// Verify we got a certificate back
	require.NotNil(t, respMsg.GetConnectionSettings())
	require.NotNil(t, respMsg.GetConnectionSettings().GetOpamp())
	require.NotNil(t, respMsg.GetConnectionSettings().GetOpamp().GetCertificate())
	require.NotEmpty(t, respMsg.GetConnectionSettings().GetOpamp().GetCertificate().GetCert())

	t.Logf("Got certificate: %d bytes", len(respMsg.GetConnectionSettings().GetOpamp().GetCertificate().GetCert()))

	// Verify it's a valid PEM certificate
	block, _ := pem.Decode(respMsg.GetConnectionSettings().GetOpamp().GetCertificate().GetCert())
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.Equal(t, "test-instance", cert.Subject.CommonName)
	require.Contains(t, cert.Subject.Organization, "test")
}

func TestServer_RequireAuth_Unauthenticated(t *testing.T) {
	server, err := New()
	require.NoError(t, err)
	server.RequireAuth = true

	url := server.Start()
	defer server.Stop()

	httpURL := url + "/v1/opamp"

	// Try to connect without auth header
	msg := &protobufs.AgentToServer{
		InstanceUid:  []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Capabilities: uint64(protobufs.AgentCapabilities_AgentCapabilities_AcceptsOpAMPConnectionSettings),
	}

	data, err := proto.Marshal(msg)
	require.NoError(t, err)

	resp, err := doPOST(t, server.Client(), httpURL, data)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should be rejected
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServer_RequireAuth_EnrollmentJWT(t *testing.T) {
	server, err := New()
	require.NoError(t, err)
	server.RequireAuth = true

	url := server.Start()
	defer server.Stop()

	httpURL := url + "/v1/opamp"

	// Create enrollment JWT
	enrollmentJWT, err := server.CreateEnrollmentJWT("test", time.Hour)
	require.NoError(t, err)

	// Generate CSR
	_, signingPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "test-instance-auth",
			Organization: []string{"test"},
		},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, signingPriv)
	require.NoError(t, err)

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	// Send message with CSR and enrollment JWT
	msg := &protobufs.AgentToServer{
		InstanceUid:  []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
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

	client := server.Client()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, httpURL, bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Authorization", "Bearer "+enrollmentJWT)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should be accepted
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Should receive certificate
	respData, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var respMsg protobufs.ServerToAgent
	err = proto.Unmarshal(respData, &respMsg)
	require.NoError(t, err)

	require.NotNil(t, respMsg.GetConnectionSettings())
	require.NotNil(t, respMsg.GetConnectionSettings().GetOpamp())
	require.NotNil(t, respMsg.GetConnectionSettings().GetOpamp().GetCertificate())
	require.NotEmpty(t, respMsg.GetConnectionSettings().GetOpamp().GetCertificate().GetCert())
}

func TestServer_RequireAuth_SupervisorJWT(t *testing.T) {
	server, err := New()
	require.NoError(t, err)
	server.RequireAuth = true

	url := server.Start()
	defer server.Stop()

	httpURL := url + "/v1/opamp"
	instanceUID := "test-instance-supervisor"

	// First, enroll with enrollment JWT
	enrollmentJWT, err := server.CreateEnrollmentJWT("test", time.Hour)
	require.NoError(t, err)

	// Generate keys
	_, signingPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   instanceUID,
			Organization: []string{"test"},
		},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, signingPriv)
	require.NoError(t, err)

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	// Send CSR with enrollment JWT
	msg := &protobufs.AgentToServer{
		InstanceUid:  []byte(instanceUID)[:16],
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

	client := server.Client()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, httpURL, bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Authorization", "Bearer "+enrollmentJWT)

	resp, err := client.Do(req)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Get certificate from response
	respData, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)

	var respMsg protobufs.ServerToAgent
	err = proto.Unmarshal(respData, &respMsg)
	require.NoError(t, err)

	certPEM := respMsg.GetConnectionSettings().GetOpamp().GetCertificate().GetCert()
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)

	_, err = x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Now create a supervisor JWT signed with our private key
	supervisorJWT := createTestSupervisorJWT(t, signingPriv, instanceUID, "localhost")

	// Send a regular message with supervisor JWT
	msg2 := &protobufs.AgentToServer{
		InstanceUid:  []byte(instanceUID)[:16],
		Capabilities: uint64(protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus),
	}

	data2, err := proto.Marshal(msg2)
	require.NoError(t, err)

	req2, err := http.NewRequestWithContext(t.Context(), http.MethodPost, httpURL, bytes.NewReader(data2))
	require.NoError(t, err)
	req2.Header.Set("Content-Type", "application/x-protobuf")
	req2.Header.Set("Authorization", "Bearer "+supervisorJWT)

	resp2, err := client.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	// Should be accepted
	require.Equal(t, http.StatusOK, resp2.StatusCode)
}

// createTestSupervisorJWT creates a supervisor JWT for testing.
func createTestSupervisorJWT(t *testing.T, privateKey ed25519.PrivateKey, instanceUID, audience string) string {
	t.Helper()

	now := time.Now()
	claims := jwt.MapClaims{
		"sub": instanceUID,
		"aud": audience,
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tokenString, err := token.SignedString(privateKey)
	require.NoError(t, err)

	return tokenString
}
