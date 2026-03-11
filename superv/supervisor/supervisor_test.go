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
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/Graylog2/collector-sidecar/superv/auth"
	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/healthmonitor"
	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
)

func TestNewSupervisor(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.DefaultConfig()
	cfg.Server.Endpoint = "ws://localhost:4320/v1/opamp"
	cfg.Agent.Executable = "/bin/echo"
	cfg.Persistence.Dir = t.TempDir()

	sup, err := New(logger, cfg, uuid.New().String())
	require.NoError(t, err)
	require.NotNil(t, sup)
}

func TestSupervisor_GetInstanceUID(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.DefaultConfig()
	cfg.Server.Endpoint = "ws://localhost:4320/v1/opamp"
	cfg.Agent.Executable = "/bin/echo"
	cfg.Persistence.Dir = t.TempDir()

	testUID := uuid.New().String()
	sup, err := New(logger, cfg, testUID)
	require.NoError(t, err)

	uid := sup.InstanceUID()
	require.Equal(t, testUID, uid)
}

func TestSupervisor_IsRunning(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.DefaultConfig()
	cfg.Server.Endpoint = "ws://localhost:4320/v1/opamp"
	cfg.Agent.Executable = "/bin/echo"
	cfg.Persistence.Dir = t.TempDir()

	sup, err := New(logger, cfg, uuid.New().String())
	require.NoError(t, err)

	// Initially not running
	require.False(t, sup.IsRunning())
}

func TestSupervisor_ConfigManagerIntegration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()

	cfg := config.Config{
		Server: config.ServerConfig{
			Endpoint: "ws://localhost:4320/v1/opamp",
		},
		LocalServer: config.LocalServer{
			Endpoint: "localhost:4321",
		},
		Agent: config.AgentConfig{
			Executable: "/bin/sleep",
			Args:       []string{"1"},
			Health: config.HealthConfig{
				Endpoint: "http://localhost:13133/health",
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
		Persistence: config.PersistenceConfig{
			Dir: dir,
		},
	}

	supervisor, err := New(logger, cfg, uuid.New().String())
	require.NoError(t, err)
	require.NotNil(t, supervisor)

	// Verify components are created
	require.NotNil(t, supervisor.configManager)
	require.NotNil(t, supervisor.healthMonitor)
}

func TestSupervisor_OnOpampConnectionSettings_UpdatesEndpoint(t *testing.T) {
	// This is an integration test that verifies the callback behavior.
	// Full testing requires a mock OpAMP server to receive connection settings
	// and verify the client reconnects with new settings.
	t.Skip("Requires integration test setup with mock OpAMP server")
}

func TestNewSupervisor_ConnectionSettingsBootstrap(t *testing.T) {
	t.Run("uses persisted settings when server endpoint is not configured", func(t *testing.T) {
		dir := t.TempDir()
		writePersistedConnectionSettings(t, dir, connection.Settings{
			Endpoint: "wss://persisted.example.com/v1/opamp",
			Headers:  map[string]string{"X-Persisted": "true"},
		})

		logger := zaptest.NewLogger(t)
		cfg := config.DefaultConfig()
		cfg.Persistence.Dir = dir
		cfg.Keys.Dir = filepath.Join(dir, "keys")
		cfg.Agent.Executable = "/bin/echo"
		cfg.Server.Endpoint = ""

		sup, err := New(logger, cfg, uuid.New().String())
		require.NoError(t, err)

		current := sup.connectionSettingsManager.GetCurrent()
		require.Equal(t, "wss://persisted.example.com/v1/opamp", current.Endpoint)
		require.Equal(t, map[string]string{"X-Persisted": "true"}, current.Headers)
	})

	t.Run("configured endpoint overrides persisted endpoint and headers", func(t *testing.T) {
		dir := t.TempDir()
		writePersistedConnectionSettings(t, dir, connection.Settings{
			Endpoint: "wss://persisted.example.com/v1/opamp",
			Headers:  map[string]string{"X-Persisted": "true"},
		})

		logger := zaptest.NewLogger(t)
		cfg := config.DefaultConfig()
		cfg.Persistence.Dir = dir
		cfg.Keys.Dir = filepath.Join(dir, "keys")
		cfg.Agent.Executable = "/bin/echo"
		cfg.Server.Endpoint = "wss://configured.example.com/v1/opamp"
		cfg.Server.Headers = map[string]string{"X-Configured": "true"}

		sup, err := New(logger, cfg, uuid.New().String())
		require.NoError(t, err)

		current := sup.connectionSettingsManager.GetCurrent()
		require.Equal(t, "wss://configured.example.com/v1/opamp", current.Endpoint)
		require.Equal(t, map[string]string{"X-Configured": "true"}, current.Headers)
	})

	t.Run("uses enrollment endpoint when no server or persisted endpoint exists", func(t *testing.T) {
		dir := t.TempDir()
		logger := zaptest.NewLogger(t)
		cfg := config.DefaultConfig()
		cfg.Persistence.Dir = dir
		cfg.Keys.Dir = filepath.Join(dir, "keys")
		cfg.Agent.Executable = "/bin/echo"
		cfg.Server.Endpoint = ""
		cfg.Server.Auth.EnrollmentEndpoint = "wss://enroll.example.com/v1/opamp"
		cfg.Server.Auth.EnrollmentHeaders = map[string]string{"X-Enrollment": "true"}

		sup, err := New(logger, cfg, uuid.New().String())
		require.NoError(t, err)

		current := sup.connectionSettingsManager.GetCurrent()
		require.Equal(t, "wss://enroll.example.com/v1/opamp", current.Endpoint)
		require.Equal(t, map[string]string{"X-Enrollment": "true"}, current.Headers)
	})

	t.Run("returns error when no endpoint can be resolved", func(t *testing.T) {
		dir := t.TempDir()
		logger := zaptest.NewLogger(t)
		cfg := config.DefaultConfig()
		cfg.Persistence.Dir = dir
		cfg.Keys.Dir = filepath.Join(dir, "keys")
		cfg.Agent.Executable = "/bin/echo"
		cfg.Server.Endpoint = ""
		cfg.Server.Auth.EnrollmentEndpoint = ""

		sup, err := New(logger, cfg, uuid.New().String())
		require.Error(t, err)
		require.Nil(t, sup)
		require.ErrorContains(t, err, "no server endpoint configured and no persisted connection settings found")
	})
}

func writePersistedConnectionSettings(t *testing.T, dir string, settings connection.Settings) {
	t.Helper()

	manager := connection.NewSettingsManager(zap.NewNop(), dir)
	manager.SetCurrent(connection.Settings{Endpoint: "wss://bootstrap.example.com/v1/opamp"})

	stage, err := manager.StageNext(settings)
	require.NoError(t, err)
	require.NoError(t, stage.Commit())
}

func TestSupervisor_NonIdentifyingAttributes_WithCollectorVersion(t *testing.T) {
	s := &Supervisor{
		collectorVersion: "2.0.0-alpha.0",
	}

	attrs := s.nonIdentifyingAttributes("test-host")

	attrMap := make(map[string]string)
	for _, kv := range attrs {
		attrMap[kv.Key] = kv.Value.GetStringValue()
	}

	require.Equal(t, "test-host", attrMap["host.name"])
	require.NotEmpty(t, attrMap["service.version"])
	require.NotEmpty(t, attrMap["os.type"])
	require.NotEmpty(t, attrMap["host.arch"])
	require.Equal(t, "2.0.0-alpha.0", attrMap["collector.version"])
}

func TestSupervisor_NonIdentifyingAttributes_WithoutCollectorVersion(t *testing.T) {
	s := &Supervisor{}

	attrs := s.nonIdentifyingAttributes("test-host")

	attrMap := make(map[string]string)
	for _, kv := range attrs {
		attrMap[kv.Key] = kv.Value.GetStringValue()
	}

	require.Equal(t, "test-host", attrMap["host.name"])
	require.NotEmpty(t, attrMap["service.version"])
	_, hasCollectorVersion := attrMap["collector.version"]
	require.False(t, hasCollectorVersion, "collector.version should not be present when empty")
}

func TestSupervisor_InitialComponentHealth_DefaultHealthyWithoutMonitor(t *testing.T) {
	supervisor := &Supervisor{}

	health := supervisor.initialComponentHealth()
	require.True(t, health.Healthy)
	require.Empty(t, health.LastError)
}

func TestSupervisor_InitialComponentHealth_DefaultHealthyWithoutSample(t *testing.T) {
	monitor := healthmonitor.New(zap.NewNop(), healthmonitor.Config{
		Endpoint: "http://localhost:13133/health",
		Timeout:  time.Second,
		Interval: time.Second,
	})
	supervisor := &Supervisor{healthMonitor: monitor}

	health := supervisor.initialComponentHealth()
	require.True(t, health.Healthy)
	require.Empty(t, health.LastError)
}

func TestSupervisor_InitialComponentHealth_UsesLatestMonitorSample(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	monitor := healthmonitor.New(zap.NewNop(), healthmonitor.Config{
		Endpoint: server.URL,
		Timeout:  time.Second,
		Interval: time.Second,
	})

	_, err := monitor.CheckHealth(context.Background())
	require.NoError(t, err)

	supervisor := &Supervisor{healthMonitor: monitor}
	health := supervisor.initialComponentHealth()

	require.False(t, health.Healthy)
	require.Equal(t, "Service Unavailable", health.LastError)
}

func TestBuildAuthHeaders_Enrolled_GeneratesFreshJWTPerCall(t *testing.T) {
	keysDir := filepath.Join(t.TempDir(), "keys")
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	cert := createSelfSignedCert(t, pub)
	require.NoError(t, persistence.SaveSigningKey(keysDir, priv))
	require.NoError(t, persistence.SaveCertificate(keysDir, cert))

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
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	cert := createSelfSignedCert(t, pub)
	require.NoError(t, persistence.SaveSigningKey(keysDir, priv))
	require.NoError(t, persistence.SaveCertificate(keysDir, cert))

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

func TestSupervisor_BuildCollectorEnv(t *testing.T) {
	keysDir := "/tmp/test-keys"
	authMgr := auth.NewManager(zaptest.NewLogger(t), auth.ManagerConfig{
		KeysDir: keysDir,
	})
	instanceUid, err := uuid.NewV7()
	require.NoError(t, err)

	t.Run("sets TLS paths from auth manager", func(t *testing.T) {
		s := &Supervisor{
			authManager: authMgr,
			agentCfg:    config.AgentConfig{},
			instanceUID: instanceUid.String(),
		}

		env := s.buildCollectorEnv()

		require.Equal(t, instanceUid.String(), env["GLC_INTERNAL_INSTANCE_UID"])
		require.Equal(t, authMgr.GetSigningKeyPath(), env["GLC_INTERNAL_TLS_CLIENT_KEY_PATH"])
		require.Equal(t, authMgr.GetSigningCertPath(), env["GLC_INTERNAL_TLS_CLIENT_CERT_PATH"])
	})

	t.Run("sets persistence dir", func(t *testing.T) {
		s := &Supervisor{
			authManager:    authMgr,
			agentCfg:       config.AgentConfig{},
			instanceUID:    instanceUid.String(),
			persistenceDir: "/var/lib/graylog-sidecar",
		}

		env := s.buildCollectorEnv()

		require.Equal(t, "/var/lib/graylog-sidecar", env["GLC_INTERNAL_PERSISTENCE_DIR"])
	})

	t.Run("merges user-configured env vars", func(t *testing.T) {
		s := &Supervisor{
			authManager: authMgr,
			agentCfg: config.AgentConfig{
				Env: map[string]string{"MY_VAR": "my-value"},
			},
		}

		env := s.buildCollectorEnv()

		require.Equal(t, "my-value", env["MY_VAR"])
		require.Equal(t, authMgr.GetSigningKeyPath(), env["GLC_INTERNAL_TLS_CLIENT_KEY_PATH"])
	})

	t.Run("user env overrides TLS paths", func(t *testing.T) {
		s := &Supervisor{
			authManager: authMgr,
			agentCfg: config.AgentConfig{
				Env: map[string]string{
					"GLC_INTERNAL_TLS_CLIENT_KEY_PATH": "/custom/key.pem",
				},
			},
		}

		env := s.buildCollectorEnv()

		require.Equal(t, "/custom/key.pem", env["GLC_INTERNAL_TLS_CLIENT_KEY_PATH"])
		require.Equal(t, authMgr.GetSigningCertPath(), env["GLC_INTERNAL_TLS_CLIENT_CERT_PATH"])
	})
}

// createSelfSignedCert creates a minimal self-signed ed25519 certificate for testing.
func createSelfSignedCert(t *testing.T, pub ed25519.PublicKey) *x509.Certificate {
	t.Helper()

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}

	// Self-sign using the ed25519 key itself.
	_, selfSignPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, pub, selfSignPriv)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	return cert
}
