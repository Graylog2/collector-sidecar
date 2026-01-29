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
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/curve25519"
)

func TestGenerateSigningKeypair(t *testing.T) {
	pub, priv, err := GenerateSigningKeypair()
	require.NoError(t, err)
	require.Len(t, pub, ed25519.PublicKeySize)
	require.Len(t, priv, ed25519.PrivateKeySize)

	// Verify the keypair works for signing
	msg := []byte("test message")
	sig := ed25519.Sign(priv, msg)
	require.True(t, ed25519.Verify(pub, msg, sig))
}

func TestGenerateEncryptionKeypair(t *testing.T) {
	pub, priv, err := GenerateEncryptionKeypair()
	require.NoError(t, err)
	require.Len(t, pub, curve25519.PointSize)
	require.Len(t, priv, curve25519.ScalarSize)

	// Verify ECDH works
	otherPub, otherPriv, err := GenerateEncryptionKeypair()
	require.NoError(t, err)

	shared1, err := curve25519.X25519(priv, otherPub)
	require.NoError(t, err)

	shared2, err := curve25519.X25519(otherPriv, pub)
	require.NoError(t, err)

	require.Equal(t, shared1, shared2)
}

func TestGenerateSigningKeypair_Unique(t *testing.T) {
	pub1, _, err := GenerateSigningKeypair()
	require.NoError(t, err)

	pub2, _, err := GenerateSigningKeypair()
	require.NoError(t, err)

	require.NotEqual(t, pub1, pub2)
}

func TestGenerateEncryptionKeypair_Unique(t *testing.T) {
	pub1, _, err := GenerateEncryptionKeypair()
	require.NoError(t, err)

	pub2, _, err := GenerateEncryptionKeypair()
	require.NoError(t, err)

	require.NotEqual(t, pub1, pub2)
}
