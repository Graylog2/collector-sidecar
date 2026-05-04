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

package identity

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	alicePubBytes, alicePrivBytes, err := GenerateEncryptionKeypair()
	require.NoError(t, err)
	require.Len(t, alicePubBytes, 32)
	require.Len(t, alicePrivBytes, 32)

	// Verify ECDH works
	bobPubBytes, bobPrivBytes, err := GenerateEncryptionKeypair()
	require.NoError(t, err)

	alicePriv, err := ecdh.X25519().NewPrivateKey(alicePrivBytes)
	require.NoError(t, err)
	alicePub, err := ecdh.X25519().NewPublicKey(alicePubBytes)
	require.NoError(t, err)

	assert.Equal(t, alicePriv.PublicKey().Bytes(), alicePub.Bytes())

	bobPriv, err := ecdh.X25519().NewPrivateKey(bobPrivBytes)
	require.NoError(t, err)
	bobPub, err := ecdh.X25519().NewPublicKey(bobPubBytes)
	require.NoError(t, err)

	assert.Equal(t, bobPriv.PublicKey().Bytes(), bobPub.Bytes())

	sharedOne, err := alicePriv.ECDH(bobPub)
	require.NoError(t, err)

	sharedTwo, err := bobPriv.ECDH(alicePub)
	require.NoError(t, err)

	require.Equal(t, sharedOne, sharedTwo)
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
