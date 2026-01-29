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

package testserver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Graylog2/collector-sidecar/superv/auth"
)

func TestServer_JWKS(t *testing.T) {
	server, err := New()
	require.NoError(t, err)

	url := server.Start()
	defer server.Stop()

	// Fetch JWKS
	client := server.Client()
	keys, err := auth.FetchJWKS(client, url)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, server.KeyID, keys[0].KeyID)
}

func TestServer_EnrollmentJWT(t *testing.T) {
	server, err := New()
	require.NoError(t, err)

	url := server.Start()
	defer server.Stop()

	// Create enrollment JWT
	jwt, err := server.CreateEnrollmentJWT("test-tenant", time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, jwt)

	// Validate it using our auth package
	client := server.Client()
	keys, err := auth.FetchJWKS(client, url)
	require.NoError(t, err)

	claims, err := auth.ValidateEnrollmentJWT(jwt, keys)
	require.NoError(t, err)
	require.Equal(t, "test-tenant", claims.TenantID)
	require.Equal(t, "Ed25519", claims.KeyAlgorithm)
}

func TestServer_CreateEnrollmentURL(t *testing.T) {
	server, err := New()
	require.NoError(t, err)

	url := server.Start()
	defer server.Stop()

	enrollURL, err := server.CreateEnrollmentURL("test-tenant", time.Hour)
	require.NoError(t, err)
	require.Contains(t, enrollURL, url)
	require.Contains(t, enrollURL, "/opamp/enroll/")
}
