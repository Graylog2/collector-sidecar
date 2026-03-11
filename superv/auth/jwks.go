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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// JWK represents a JSON Web Key.
type JWK struct {
	KeyID     string
	PublicKey ed25519.PublicKey
}

type jwksResponse struct {
	Keys []jwkEntry `json:"keys"`
}

type jwkEntry struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	X   string `json:"x"`
	Use string `json:"use"`
}

// FetchJWKS fetches the JWKS from the server's well-known endpoint.
func FetchJWKS(client *http.Client, baseURL string) ([]JWK, error) {
	resp, err := client.Get(baseURL + "/.well-known/jwks.json")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS request failed with status %d", resp.StatusCode)
	}

	const maxJWKSSize = 1 << 20 // 1 MB
	var jwks jwksResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJWKSSize)).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to decode JWKS: %w", err)
	}

	var keys []JWK
	for _, entry := range jwks.Keys {
		if entry.Kty != "OKP" || entry.Crv != "Ed25519" {
			continue // Skip non-Ed25519 keys
		}

		pubBytes, err := base64.RawURLEncoding.DecodeString(entry.X)
		if err != nil {
			continue
		}

		if len(pubBytes) != ed25519.PublicKeySize {
			continue
		}

		keys = append(keys, JWK{
			KeyID:     entry.Kid,
			PublicKey: pubBytes,
		})
	}

	if len(keys) == 0 {
		return nil, errors.New("no valid Ed25519 keys found in JWKS")
	}

	return keys, nil
}

// GetKeyByID finds a key by its ID in the JWKS.
func GetKeyByID(keys []JWK, kid string) (*JWK, error) {
	for _, k := range keys {
		if k.KeyID == kid {
			return &k, nil
		}
	}
	return nil, errors.New("key not found in JWKS")
}
