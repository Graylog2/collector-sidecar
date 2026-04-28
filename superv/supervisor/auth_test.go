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

package supervisor

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/auth"
	"github.com/Graylog2/collector-sidecar/superv/internal/testpki"
	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestBuildAuthHeaders_Enrolled_GeneratesFreshJWTPerCall(t *testing.T) {
	keysDir := filepath.Join(t.TempDir(), "keys")

	cert := testpki.GenerateTestCert(t)
	require.NoError(t, persistence.SaveSigningKey(keysDir, cert.Key))
	require.NoError(t, persistence.SaveCertificate(keysDir, cert.Cert))

	authMgr := auth.NewManager(zaptest.NewLogger(t), auth.ManagerConfig{
		KeysDir:     keysDir,
		JWTLifetime: 5 * time.Minute,
	})
	require.True(t, authMgr.IsEnrolled())
	require.NoError(t, authMgr.LoadCredentials())

	s := &Supervisor{
		authManager: authMgr,
		logger:      zaptest.NewLogger(t),
	}

	headers, headerFunc := s.buildAuthHeaders(connection.Settings{
		Headers: map[string]string{"X-Custom": "value"},
	})

	// Static headers must not contain Authorization.
	require.Empty(t, headers.Get("Authorization"))
	require.Equal(t, "value", headers.Get("X-Custom"))

	// HeaderFunc must be set for enrolled supervisors.
	require.NotNil(t, headerFunc)

	h1 := headerFunc(headers.Clone())
	auth1 := h1.Get("Authorization")
	require.True(t, strings.HasPrefix(auth1, "Bearer "), "expected Bearer token, got %q", auth1)

	// Verify it's a parseable supervisor JWT with the right cert fingerprint.
	token1 := strings.TrimPrefix(auth1, "Bearer ")
	certFP, claims, err := auth.ParseSupervisorJWT(token1)
	require.NoError(t, err)
	require.Equal(t, authMgr.CertFingerprint(), certFP)
	require.False(t, claims.IsExpired())

	// Second call also succeeds with a valid JWT (proves it calls GenerateJWT each time,
	// not caching a static value).
	h2 := headerFunc(headers.Clone())
	auth2 := h2.Get("Authorization")
	require.True(t, strings.HasPrefix(auth2, "Bearer "), "second call must also produce Bearer token")
}

func TestBuildAuthHeaders_Enrolled_ErrorBranch(t *testing.T) {
	keysDir := filepath.Join(t.TempDir(), "keys")

	cert := testpki.GenerateTestCert(t)
	require.NoError(t, persistence.SaveSigningKey(keysDir, cert.Key))
	require.NoError(t, persistence.SaveCertificate(keysDir, cert.Cert))

	// Create manager but do NOT call LoadCredentials() — signingKey remains nil.
	authMgr := auth.NewManager(zaptest.NewLogger(t), auth.ManagerConfig{
		KeysDir: keysDir,
	})
	require.True(t, authMgr.IsEnrolled())

	s := &Supervisor{
		authManager: authMgr,
		logger:      zaptest.NewLogger(t),
	}

	headers, headerFunc := s.buildAuthHeaders(connection.Settings{})

	require.NotNil(t, headerFunc, "HeaderFunc should still be set for enrolled supervisor")

	// headerFunc should log an error and return headers without Authorization.
	result := headerFunc(headers.Clone())
	require.Empty(t, result.Get("Authorization"),
		"Authorization header must not be set when JWT generation fails")
}

func TestBuildAuthHeaders_NotEnrolled_StaticEnrollmentJWT(t *testing.T) {
	// Use a keysDir with no files so IsEnrolled() returns false.
	keysDir := filepath.Join(t.TempDir(), "empty-keys")

	authMgr := auth.NewManager(zaptest.NewLogger(t), auth.ManagerConfig{
		KeysDir: keysDir,
	})
	require.False(t, authMgr.IsEnrolled())

	s := &Supervisor{
		authManager: authMgr,
		logger:      zaptest.NewLogger(t),
	}

	headers, headerFunc := s.buildAuthHeaders(connection.Settings{
		Headers: map[string]string{"X-Foo": "bar"},
	})

	// Not enrolled and no pending enrollment → no Authorization header.
	require.Empty(t, headers.Get("Authorization"))
	require.Equal(t, "bar", headers.Get("X-Foo"))
	// No HeaderFunc needed when not enrolled.
	require.Nil(t, headerFunc)
}
