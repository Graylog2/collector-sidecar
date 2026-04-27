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
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/Graylog2/collector-sidecar/superv/internal/testpki"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/curve25519"
)

func TestSaveAndLoadSigningKey(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	// Generate a test keypair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	err = SaveSigningKey(keysDir, priv)
	require.NoError(t, err)

	loaded, err := LoadSigningKey(keysDir)
	require.NoError(t, err)
	require.Equal(t, priv, loaded)
	require.Equal(t, pub, loaded.Public())
}

func TestSaveSigningKey_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	err = SaveSigningKey(keysDir, priv)
	require.NoError(t, err)

	filePath := filepath.Join(keysDir, "signing.key")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestLoadSigningKey_NotExists(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadSigningKey(dir)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestSaveAndLoadEncryptionKey(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	// Generate X25519 keypair
	priv := make([]byte, curve25519.ScalarSize)
	_, err := rand.Read(priv)
	require.NoError(t, err)

	err = SaveEncryptionKey(keysDir, priv)
	require.NoError(t, err)

	loaded, err := LoadEncryptionKey(keysDir)
	require.NoError(t, err)
	require.Equal(t, priv, loaded)
}

func TestSaveAndLoadCertificate(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	cert := testpki.GenerateTestCert(t)

	err := SaveCertificate(keysDir, cert.Cert)
	require.NoError(t, err)

	loaded, err := LoadCertificate(keysDir)
	require.NoError(t, err)
	require.Equal(t, cert.Cert.Raw, loaded.Raw)
}

func TestSaveCertificate_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	cert := testpki.GenerateTestCert(t)

	err := SaveCertificate(keysDir, cert.Cert)
	require.NoError(t, err)

	filePath := filepath.Join(keysDir, "signing.crt")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	// Certificates are public, so 0o644 is fine
	require.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func TestKeysExist(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	// Initially no keys exist
	require.False(t, SigningKeyExists(keysDir))
	require.False(t, EncryptionKeyExists(keysDir))
	require.False(t, CertificateExists(keysDir))

	// Create signing key
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, SaveSigningKey(keysDir, priv))
	require.True(t, SigningKeyExists(keysDir))

	// Create encryption key
	encPriv := make([]byte, curve25519.ScalarSize)
	_, _ = rand.Read(encPriv)
	require.NoError(t, SaveEncryptionKey(keysDir, encPriv))
	require.True(t, EncryptionKeyExists(keysDir))

	// Create certificate
	cert := testpki.GenerateTestCert(t)
	require.NoError(t, SaveCertificate(keysDir, cert.Cert))
	require.True(t, CertificateExists(keysDir))
}

func TestCertificateFingerprint(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")

	cert := testpki.GenerateTestCert(t)
	require.NoError(t, SaveCertificate(keysDir, cert.Cert))

	fp, err := CertificateFingerprint(keysDir)
	require.NoError(t, err)
	require.NotEmpty(t, fp)
	// SHA-256 fingerprint should be 64 hex chars
	require.Len(t, fp, 64)
}
