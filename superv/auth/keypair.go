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

	"golang.org/x/crypto/curve25519"
)

// GenerateSigningKeypair generates a new Ed25519 keypair for signing.
func GenerateSigningKeypair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

// GenerateEncryptionKeypair generates a new X25519 keypair for encryption.
func GenerateEncryptionKeypair() (publicKey, privateKey []byte, err error) {
	privateKey = make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(privateKey); err != nil {
		return nil, nil, err
	}

	publicKey, err = curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		return nil, nil, err
	}

	return publicKey, privateKey, nil
}
