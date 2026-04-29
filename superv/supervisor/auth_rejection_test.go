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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/Graylog2/collector-sidecar/superv/auth"
	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/persistence"
)

// writeStubCredentials places empty signing.key + signing.crt files in
// keysDir so auth.Manager.IsEnrolled() returns true. The file contents are
// not parsed by handleAuthRejection — IsEnrolled() is a stat-based check.
func writeStubCredentials(t *testing.T, keysDir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(keysDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(keysDir, persistence.SigningKeyFile), []byte("stub"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(keysDir, persistence.SigningCertFile), []byte("stub"), 0o600))
}

// newAuthRejectionTestSupervisor constructs a minimal Supervisor whose only
// wired fields are the ones handleAuthRejection touches: logger, authManager,
// authCfg, persistenceDir, exitFn. exitFn records the invoked code instead of
// actually calling os.Exit.
func newAuthRejectionTestSupervisor(t *testing.T, resetEnabled bool, enrolled bool) (*Supervisor, *int) {
	t.Helper()

	keysDir := t.TempDir()
	persistDir := t.TempDir()
	if enrolled {
		writeStubCredentials(t, keysDir)
	}

	authMgr := auth.NewManager(zaptest.NewLogger(t).Named("auth"), auth.ManagerConfig{
		KeysDir: keysDir,
	})

	var exitCode int = -1 // -1 = "exit not called"
	captureExit := func(code int) { exitCode = code }

	s := &Supervisor{
		logger: zaptest.NewLogger(t),
		authCfg: config.AuthConfig{
			ResetOnAuthRejection: resetEnabled,
		},
		authManager:    authMgr,
		persistenceDir: persistDir,
		exitFn:         captureExit,
	}
	return s, &exitCode
}

// TestHandleAuthRejection_GatedByConfig verifies the 401-while-enrolled
// recovery path only fires when (a) the error is a 401, (b) the agent is
// enrolled, and (c) auth.reset_on_auth_rejection is true in configuration.
func TestHandleAuthRejection_GatedByConfig(t *testing.T) {
	tests := []struct {
		name         string
		resetEnabled bool
		enrolled     bool
		err          error
		wantExit     bool
	}{
		{
			name:         "enabled + enrolled + 401 => exit(1)",
			resetEnabled: true,
			enrolled:     true,
			err:          errors.New("invalid response from server: 401"),
			wantExit:     true,
		},
		{
			name:         "enabled + enrolled + 'unauthorized' substring => exit(1)",
			resetEnabled: true,
			enrolled:     true,
			err:          errors.New("server response code=401 (Unauthorized)"),
			wantExit:     true,
		},
		{
			name:         "disabled + enrolled + 401 => preserved stock behavior (no exit)",
			resetEnabled: false,
			enrolled:     true,
			err:          errors.New("invalid response from server: 401"),
			wantExit:     false,
		},
		{
			name:         "enabled + not enrolled + 401 => retry, no exit",
			resetEnabled: true,
			enrolled:     false,
			err:          errors.New("invalid response from server: 401"),
			wantExit:     false,
		},
		{
			name:         "enabled + enrolled + non-401 error => no exit",
			resetEnabled: true,
			enrolled:     true,
			err:          errors.New("connection reset by peer"),
			wantExit:     false,
		},
		{
			name:         "nil error => no exit",
			resetEnabled: true,
			enrolled:     true,
			err:          nil,
			wantExit:     false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, exitCode := newAuthRejectionTestSupervisor(t, tc.resetEnabled, tc.enrolled)
			s.handleAuthRejection(tc.err)
			if tc.wantExit {
				assert.Equal(t, 1, *exitCode, "exitFn must be called with code 1")
			} else {
				assert.Equal(t, -1, *exitCode, "exitFn must NOT be called")
			}
		})
	}
}

// TestHandleAuthRejection_ClearsPersistentState verifies that when the gate
// fires, credentials and identity.yaml are removed from disk.
func TestHandleAuthRejection_ClearsPersistentState(t *testing.T) {
	s, exitCode := newAuthRejectionTestSupervisor(t, true, true)

	keysDir := s.authManager.GetSigningKeyPath() // returns the path, not the dir
	keysDir = filepath.Dir(keysDir)

	// Seed identity.yaml so ClearInstanceUID has something to remove.
	identityPath := filepath.Join(s.persistenceDir, "identity.yaml")
	require.NoError(t, os.WriteFile(identityPath, []byte("instance_uid: stub\n"), 0o600))

	// Precondition: files exist.
	require.FileExists(t, filepath.Join(keysDir, persistence.SigningKeyFile))
	require.FileExists(t, filepath.Join(keysDir, persistence.SigningCertFile))
	require.FileExists(t, identityPath)

	s.handleAuthRejection(errors.New("invalid response from server: 401"))

	// Postcondition: gate fired (exit called), files removed.
	assert.Equal(t, 1, *exitCode)
	assert.NoFileExists(t, filepath.Join(keysDir, persistence.SigningKeyFile))
	assert.NoFileExists(t, filepath.Join(keysDir, persistence.SigningCertFile))
	assert.NoFileExists(t, identityPath)
}
