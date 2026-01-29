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

package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateCSR(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	encPub, _, err := GenerateEncryptionKeypair()
	require.NoError(t, err)

	instanceUID := "01HQ3K5V7X2M4N8P9R0S1T2U3V"

	csrDER, err := CreateCSR(priv, instanceUID, encPub)
	require.NoError(t, err)
	require.NotEmpty(t, csrDER)

	// Parse and verify the CSR
	csr, err := ParseCSR(csrDER)
	require.NoError(t, err)
	require.Equal(t, instanceUID, csr.Subject.CommonName)
	require.NoError(t, csr.CheckSignature())
}

func TestCreateCSR_WithoutEncryptionKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	csrDER, err := CreateCSR(priv, "test-uid", nil)
	require.NoError(t, err)

	csr, err := ParseCSR(csrDER)
	require.NoError(t, err)
	require.Equal(t, "test-uid", csr.Subject.CommonName)
	require.Empty(t, csr.Extensions) // No extensions when no encryption key
}

func TestCreateCSRWithTenant(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	encPub, _, err := GenerateEncryptionKeypair()
	require.NoError(t, err)

	csrDER, err := CreateCSRWithTenant(priv, "instance-123", "acme-corp", encPub)
	require.NoError(t, err)

	csr, err := ParseCSR(csrDER)
	require.NoError(t, err)
	require.Equal(t, "instance-123", csr.Subject.CommonName)
	require.Contains(t, csr.Subject.Organization, "acme-corp")
}

func TestCreateCSR_IncludesEncryptionKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	encPub, _, err := GenerateEncryptionKeypair()
	require.NoError(t, err)

	csrDER, err := CreateCSR(priv, "test-uid", encPub)
	require.NoError(t, err)

	csr, err := ParseCSR(csrDER)
	require.NoError(t, err)

	// Encryption key should be in extensions
	require.NotEmpty(t, csr.Extensions)

	// Find our extension
	var found bool
	for _, ext := range csr.Extensions {
		if ext.Id.Equal(OIDEncryptionPublicKey) {
			require.Equal(t, encPub, ext.Value)
			found = true
			break
		}
	}
	require.True(t, found, "encryption key extension not found")
}

func TestEncodeCSRToPEM(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	csrDER, err := CreateCSR(priv, "test-uid", nil)
	require.NoError(t, err)

	pem := EncodeCSRToPEM(csrDER)
	require.Contains(t, string(pem), "-----BEGIN CERTIFICATE REQUEST-----")
	require.Contains(t, string(pem), "-----END CERTIFICATE REQUEST-----")
}
