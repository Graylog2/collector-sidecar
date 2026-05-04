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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Graylog2/collector-sidecar/superv/identity"
	"github.com/Graylog2/collector-sidecar/superv/internal/testpki"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestInitIdentity_CreatesAllArtifacts(t *testing.T) {
	persistDir, keysDir := t.TempDir(), t.TempDir()

	require.NoError(t, InitIdentity(zaptest.NewLogger(t), persistDir, keysDir))

	data, err := LoadInstanceData(persistDir)
	require.NoError(t, err)
	assert.NotEmpty(t, data.InstanceUID)
	assert.False(t, data.CreatedAt.IsZero())

	assert.True(t, SigningKeyExists(keysDir))
	assert.True(t, EncryptionKeyExists(keysDir))
}

func TestInitIdentity_IdentityFileIsReadOnly(t *testing.T) {
	persistDir, keysDir := t.TempDir(), t.TempDir()

	require.NoError(t, InitIdentity(zaptest.NewLogger(t), persistDir, keysDir))

	info, err := os.Stat(filepath.Join(persistDir, instanceUIDFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o444), info.Mode().Perm())
}

func TestInitIdentity_Idempotent(t *testing.T) {
	persistDir, keysDir := t.TempDir(), t.TempDir()
	logger := zaptest.NewLogger(t)

	require.NoError(t, InitIdentity(logger, persistDir, keysDir))

	identityBefore, err := os.ReadFile(filepath.Join(persistDir, instanceUIDFile))
	require.NoError(t, err)
	signingBefore, err := os.ReadFile(filepath.Join(keysDir, SigningKeyFile))
	require.NoError(t, err)
	encryptionBefore, err := LoadEncryptionKey(keysDir)
	require.NoError(t, err)

	require.NoError(t, InitIdentity(logger, persistDir, keysDir))

	identityAfter, err := os.ReadFile(filepath.Join(persistDir, instanceUIDFile))
	require.NoError(t, err)
	signingAfter, err := os.ReadFile(filepath.Join(keysDir, SigningKeyFile))
	require.NoError(t, err)
	encryptionAfter, err := LoadEncryptionKey(keysDir)
	require.NoError(t, err)

	assert.Equal(t, identityBefore, identityAfter, "identity file should not be rewritten")
	assert.Equal(t, signingBefore, signingAfter, "signing key should not be regenerated")
	assert.Equal(t, encryptionBefore.Bytes(), encryptionAfter.Bytes(), "encryption key should not be regenerated")
}

func TestInitIdentity_PreservesExistingIdentity(t *testing.T) {
	persistDir, keysDir := t.TempDir(), t.TempDir()
	logger := zaptest.NewLogger(t)

	require.NoError(t, InitIdentity(logger, persistDir, keysDir))
	original, err := LoadInstanceData(persistDir)
	require.NoError(t, err)

	// Remove only the keys to confirm a second run regenerates them
	// without touching the identity file.
	require.NoError(t, os.RemoveAll(keysDir))
	require.NoError(t, os.MkdirAll(keysDir, 0o700))

	require.NoError(t, InitIdentity(logger, persistDir, keysDir))

	after, err := LoadInstanceData(persistDir)
	require.NoError(t, err)
	assert.Equal(t, original.InstanceUID, after.InstanceUID)
	assert.True(t, original.CreatedAt.Equal(after.CreatedAt))
}

func TestInitIdentity_PreservesExistingSigningKey(t *testing.T) {
	persistDir, keysDir := t.TempDir(), t.TempDir()
	logger := zaptest.NewLogger(t)

	_, priv, err := identity.GenerateSigningKeypair()
	require.NoError(t, err)
	require.NoError(t, SaveSigningKey(keysDir, priv))

	before, err := os.ReadFile(filepath.Join(keysDir, SigningKeyFile))
	require.NoError(t, err)

	require.NoError(t, InitIdentity(logger, persistDir, keysDir))

	after, err := os.ReadFile(filepath.Join(keysDir, SigningKeyFile))
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.True(t, EncryptionKeyExists(keysDir), "encryption key should still be created")
}

func TestInitIdentity_PreservesExistingEncryptionKey(t *testing.T) {
	persistDir, keysDir := t.TempDir(), t.TempDir()
	logger := zaptest.NewLogger(t)

	_, priv, err := identity.GenerateEncryptionKeypair()
	require.NoError(t, err)
	require.NoError(t, SaveEncryptionKey(keysDir, priv))

	before, err := LoadEncryptionKey(keysDir)
	require.NoError(t, err)

	require.NoError(t, InitIdentity(logger, persistDir, keysDir))

	after, err := LoadEncryptionKey(keysDir)
	require.NoError(t, err)
	assert.Equal(t, before.Bytes(), after.Bytes())
	assert.True(t, SigningKeyExists(keysDir), "signing key should still be created")
}

func TestInitIdentity_RejectsCertificateWithoutSigningKey(t *testing.T) {
	persistDir, keysDir := t.TempDir(), t.TempDir()
	cert := testpki.GenerateTestCert(t)
	require.NoError(t, SaveCertificate(keysDir, cert.Cert))

	err := InitIdentity(zaptest.NewLogger(t), persistDir, keysDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "certificate exists but identity keys are incomplete")
	assert.False(t, SigningKeyExists(keysDir), "signing key should not be regenerated for an existing certificate")
}

func TestInitIdentity_RejectsCertificateWithoutEncryptionKey(t *testing.T) {
	persistDir, keysDir := t.TempDir(), t.TempDir()
	cert := testpki.GenerateTestCert(t)
	require.NoError(t, SaveSigningKey(keysDir, cert.Key))
	require.NoError(t, SaveCertificate(keysDir, cert.Cert))

	err := InitIdentity(zaptest.NewLogger(t), persistDir, keysDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "certificate exists but identity keys are incomplete")
	assert.False(t, EncryptionKeyExists(keysDir), "encryption key should not be regenerated for an existing certificate")
}

func TestInitIdentity_StatErrorOnIdentity(t *testing.T) {
	// Make the persistence dir a regular file so that stat() on
	// <dir>/identity.yaml fails with a non-IsNotExist error (ENOTDIR).
	tmp := t.TempDir()
	persistPath := filepath.Join(tmp, "not-a-dir")
	require.NoError(t, os.WriteFile(persistPath, []byte("x"), 0o600))

	err := InitIdentity(zaptest.NewLogger(t), persistPath, t.TempDir())
	require.Error(t, err)
	assert.False(t, errors.Is(err, os.ErrNotExist), "stat error should not be ErrNotExist")
}
