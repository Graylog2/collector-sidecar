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

package persistence

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	signingKeyFile    = "signing.key"
	signingCertFile   = "signing.crt"
	encryptionKeyFile = "encryption.key"
	bearerTokenFile   = "bearer_token"
)

// SaveSigningKey saves an Ed25519 private key to disk in PEM format.
func SaveSigningKey(keysDir string, key ed25519.PrivateKey) error {
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return err
	}

	// TODO: Implement password protection for PKCS#8 keys

	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshaling PKCS8 private key: %w", err)
	}

	block := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8,
	}

	filePath := filepath.Join(keysDir, signingKeyFile)
	return os.WriteFile(filePath, pem.EncodeToMemory(block), 0600)
}

// LoadSigningKey loads an Ed25519 private key from disk.
func LoadSigningKey(keysDir string) (ed25519.PrivateKey, error) {
	filePath := filepath.Join(keysDir, signingKeyFile)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(content)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing PKCS8 private key: %w", err)
	}

	ed25519Key, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("key is not Ed25519")
	}

	return ed25519Key, nil
}

// SaveEncryptionKey saves an X25519 private key to disk in PEM format.
func SaveEncryptionKey(keysDir string, key []byte) error {
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return err
	}

	block := &pem.Block{
		Type:  "X25519 PRIVATE KEY",
		Bytes: key,
	}

	filePath := filepath.Join(keysDir, encryptionKeyFile)
	return os.WriteFile(filePath, pem.EncodeToMemory(block), 0600)
}

// LoadEncryptionKey loads an X25519 private key from disk.
func LoadEncryptionKey(keysDir string) ([]byte, error) {
	filePath := filepath.Join(keysDir, encryptionKeyFile)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(content)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	return block.Bytes, nil
}

// SaveCertificate saves an X.509 certificate to disk in PEM format.
func SaveCertificate(keysDir string, cert *x509.Certificate) error {
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return err
	}

	block := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}

	filePath := filepath.Join(keysDir, signingCertFile)
	return os.WriteFile(filePath, pem.EncodeToMemory(block), 0644)
}

// LoadCertificate loads an X.509 certificate from disk.
func LoadCertificate(keysDir string) (*x509.Certificate, error) {
	filePath := filepath.Join(keysDir, signingCertFile)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(content)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	return x509.ParseCertificate(block.Bytes)
}

// SigningKeyExists returns true if the signing key file exists.
func SigningKeyExists(keysDir string) bool {
	filePath := filepath.Join(keysDir, signingKeyFile)
	_, err := os.Stat(filePath)
	return err == nil
}

// EncryptionKeyExists returns true if the encryption key file exists.
func EncryptionKeyExists(keysDir string) bool {
	filePath := filepath.Join(keysDir, encryptionKeyFile)
	_, err := os.Stat(filePath)
	return err == nil
}

// CertificateExists returns true if the certificate file exists.
func CertificateExists(keysDir string) bool {
	filePath := filepath.Join(keysDir, signingCertFile)
	_, err := os.Stat(filePath)
	return err == nil
}

// CertificateFingerprint returns the SHA-256 fingerprint of the certificate.
func CertificateFingerprint(keysDir string) (string, error) {
	cert, err := LoadCertificate(keysDir)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:]), nil
}

// SaveBearerToken saves a bearer token to disk for collector OTLP auth.
func SaveBearerToken(keysDir string, token string) error {
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return err
	}

	filePath := filepath.Join(keysDir, bearerTokenFile)
	return os.WriteFile(filePath, []byte(token), 0600)
}

// LoadBearerToken loads a bearer token from disk.
func LoadBearerToken(keysDir string) (string, error) {
	filePath := filepath.Join(keysDir, bearerTokenFile)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// BearerTokenExists returns true if the bearer token file exists.
func BearerTokenExists(keysDir string) bool {
	filePath := filepath.Join(keysDir, bearerTokenFile)
	_, err := os.Stat(filePath)
	return err == nil
}
