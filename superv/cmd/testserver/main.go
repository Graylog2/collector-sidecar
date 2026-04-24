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

	"github.com/Graylog2/collector-sidecar/superv/internal/testserver"
)

func main() {
	var (
		addr        string
		issuer      string
		jwtExpiry   time.Duration
		printJWKS   bool
		verbose     bool
		veryVerbose bool
		jsonLogs    bool
	)

	flag.StringVar(&addr, "addr", ":8443", "Address to listen on")
	flag.StringVar(&issuer, "issuer", "test", "Issuer for enrollment JWT")
	flag.DurationVar(&jwtExpiry, "jwt-expiry", 24*time.Hour, "Enrollment JWT expiry duration")
	flag.BoolVar(&printJWKS, "print-jwks", false, "Print JWKS and exit")
	flag.BoolVar(&verbose, "v", false, "Detailed logging (description, effective config, packages)")
	flag.BoolVar(&veryVerbose, "vv", false, "Full logging (includes complete protobuf dumps)")
	flag.BoolVar(&jsonLogs, "json", false, "Output logs as JSON")
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
		_ = enc.Encode(jwks)
		return
	}

	// Determine verbosity level
	verbosity := testserver.VerbosityDefault
	if verbose {
		verbosity = testserver.VerbosityDetailed
	}
	if veryVerbose {
		verbosity = testserver.VerbosityFull
	}

	// Set up logger
	logger := testserver.NewDebugLogger(verbosity, jsonLogs)
	server.Logger = logger

	// Create the server with custom address
	mux := http.NewServeMux()

	// JWKS endpoint
	mux.HandleFunc("/.well-known/jwks.json", server.HandleJWKS)

	// OpAMP endpoint
	mux.HandleFunc("/v1/opamp", server.HandleOpAMP)

	// Generate self-signed TLS cert
	tlsConfig, err := generateTLSConfig()
	if err != nil {
		log.Fatalf("Failed to generate TLS config: %v", err)
	}

	httpServer := &http.Server{
		Addr:        addr,
		Handler:     mux,
		TLSConfig:   tlsConfig,
		ReadTimeout: 5 * time.Second,
	}

	// Create enrollment JWT
	enrollmentJWT, err := server.CreateEnrollmentJWT(issuer, jwtExpiry)
	if err != nil {
		log.Fatalf("Failed to create enrollment JWT: %v", err)
	}

	fmt.Println("========================================")
	fmt.Println("Test OpAMP Server")
	fmt.Println("========================================")
	fmt.Printf("Listening on: https://localhost%s\n", addr)
	fmt.Printf("Issuer: %s\n", issuer)
	fmt.Printf("JWT Expiry: %s\n", jwtExpiry)
	fmt.Printf("Verbosity: %d\n", verbosity)
	fmt.Printf("JSON logs: %v\n", jsonLogs)
	fmt.Println()
	fmt.Println("Endpoints:")
	fmt.Printf("  JWKS:         https://localhost%s/.well-known/jwks.json\n", addr)
	fmt.Printf("  OpAMP (WS):   wss://localhost%s/v1/opamp\n", addr)
	fmt.Printf("  OpAMP (HTTP): https://localhost%s/v1/opamp\n", addr)
	fmt.Println()
	fmt.Println("Enrollment endpoint:")
	fmt.Printf("  https://localhost%s\n", addr)
	fmt.Println("Enrollment token:")
	fmt.Printf("  %s\n", enrollmentJWT)
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println("========================================")

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		_ = logger.Sync()
		_ = httpServer.Close()
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
