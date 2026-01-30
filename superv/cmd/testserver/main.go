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

// Command testserver runs a test OpAMP server for development and testing.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"

	"github.com/Graylog2/collector-sidecar/superv/internal/testserver"
)

func main() {
	var (
		addr      string
		tenantID  string
		jwtExpiry time.Duration
		printJWKS bool
		printCurl bool
	)

	flag.StringVar(&addr, "addr", ":8443", "Address to listen on")
	flag.StringVar(&tenantID, "tenant", "test-tenant", "Tenant ID for enrollment JWT")
	flag.DurationVar(&jwtExpiry, "jwt-expiry", 24*time.Hour, "Enrollment JWT expiry duration")
	flag.BoolVar(&printJWKS, "print-jwks", false, "Print JWKS and exit")
	flag.BoolVar(&printCurl, "print-curl", false, "Print curl commands for testing")
	flag.Parse()

	server, err := testserver.New()
	if err != nil {
		log.Fatalf("Failed to create test server: %v", err)
	}

	if printJWKS {
		jwks := map[string]any{
			"keys": []map[string]any{
				{
					"kty": "OKP",
					"crv": "Ed25519",
					"kid": server.KeyID,
					"x":   base64.RawURLEncoding.EncodeToString(server.ServerPublicKey),
					"use": "sig",
				},
			},
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(jwks)
		return
	}

	// Create the server with custom address
	mux := http.NewServeMux()

	// JWKS endpoint
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[JWKS] %s %s", r.Method, r.URL.Path)
		server.HandleJWKS(w, r)
	})

	// OpAMP WebSocket endpoint
	mux.HandleFunc("/v1/opamp", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[OpAMP] %s %s", r.Method, r.URL.Path)
		if r.Header.Get("Authorization") != "" {
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 50 {
				authHeader = authHeader[:50] + "..."
			}
			log.Printf("[OpAMP] Authorization: %s", authHeader)
		}
		server.HandleOpAMP(w, r)
	})

	// Catch-all handler to log unknown paths (only for paths we don't explicitly handle)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only handle if this is actually root or an unknown path
		if r.URL.Path != "/" {
			log.Printf("[UNKNOWN] %s %s (404)", r.Method, r.URL.Path)
		}
		http.NotFound(w, r)
	})

	// Set callbacks for logging
	server.OnAgentConnect = func(instanceUID string, conn *testserver.AgentConnection) {
		log.Printf("[OpAMP] Agent connected: %s", instanceUID)
	}
	server.OnAgentDisconnect = func(instanceUID string) {
		log.Printf("[OpAMP] Agent disconnected: %s", instanceUID)
	}
	server.OnAgentMessage = func(instanceUID string, msg *protobufs.AgentToServer) {
		log.Printf("[OpAMP] Message from %s (capabilities: %d)", instanceUID, msg.Capabilities)
		if msg.ConnectionSettingsRequest != nil {
			log.Printf("[OpAMP] Agent %s sent CSR request", instanceUID)
		}
	}
	server.OnCSRReceived = func(instanceUID string, csr *x509.CertificateRequest) {
		log.Printf("[CSR] Received CSR from %s (CN: %s, O: %v)", instanceUID, csr.Subject.CommonName, csr.Subject.Organization)
	}

	// Generate self-signed TLS cert
	tlsConfig, err := generateTLSConfig()
	if err != nil {
		log.Fatalf("Failed to generate TLS config: %v", err)
	}

	httpServer := &http.Server{
		Addr:      addr,
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	// Create enrollment JWT
	enrollmentJWT, err := server.CreateEnrollmentJWT(tenantID, jwtExpiry)
	if err != nil {
		log.Fatalf("Failed to create enrollment JWT: %v", err)
	}

	fmt.Println("========================================")
	fmt.Println("Test OpAMP Server")
	fmt.Println("========================================")
	fmt.Printf("Listening on: https://localhost%s\n", addr)
	fmt.Printf("Tenant ID: %s\n", tenantID)
	fmt.Printf("JWT Expiry: %s\n", jwtExpiry)
	fmt.Println()
	fmt.Println("Endpoints:")
	fmt.Printf("  JWKS:        https://localhost%s/.well-known/jwks.json\n", addr)
	fmt.Printf("  OpAMP (WS):  wss://localhost%s/v1/opamp\n", addr)
	fmt.Printf("  OpAMP (HTTP): https://localhost%s/v1/opamp\n", addr)
	fmt.Println()
	fmt.Println("Enrollment URL:")
	fmt.Printf("  https://localhost%s/opamp/enroll/%s\n", addr, enrollmentJWT)
	fmt.Println()

	if printCurl {
		fmt.Println("Curl commands for testing:")
		fmt.Println()
		fmt.Println("# Fetch JWKS:")
		fmt.Printf("curl -k https://localhost%s/.well-known/jwks.json | jq .\n", addr)
		fmt.Println()
		fmt.Println("# Test enrollment URL parsing (supervisor command):")
		fmt.Printf("./supervisor --enrollment-url 'https://localhost%s/opamp/enroll/%s'\n", addr, enrollmentJWT)
		fmt.Println()
	}

	fmt.Println("Press Ctrl+C to stop")
	fmt.Println("========================================")

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		httpServer.Close()
	}()

	// Start server
	if err := httpServer.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

func generateTLSConfig() (*tls.Config, error) {
	// Generate ECDSA private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}, nil
}
