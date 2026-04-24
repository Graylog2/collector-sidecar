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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/auth"
	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/internal/testprotos"
	"github.com/Graylog2/collector-sidecar/superv/keen"
	"github.com/Graylog2/collector-sidecar/superv/ownlogs"
	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSupervisor_HandleOwnLogs(t *testing.T) {
	newSupervisorWithCommander := func(t *testing.T, logger *zap.Logger) *Supervisor {
		t.Helper()

		keysDir := t.TempDir()
		persistDir := t.TempDir()

		// Write a self-signed cert to the auth manager's expected paths so
		// ConvertSettings.LoadClientCert can read them.
		writeTestSigningCert(t, keysDir)

		authMgr := auth.NewManager(zap.NewNop(), auth.ManagerConfig{
			KeysDir: keysDir,
		})

		cmd, err := keen.New(logger, t.TempDir(), keen.Config{
			Executable: "/bin/true",
		}, keen.NewBackoff(keen.BackoffConfig{}))
		require.NoError(t, err)

		return &Supervisor{
			logger:             logger,
			authManager:        authMgr,
			persistenceDir:     persistDir,
			instanceUID:        "test-instance",
			ownLogsManager:     ownlogs.NewManager(config.TelemetryLogsConfig{}),
			ownLogsPersistence: ownlogs.NewPersistence(persistDir, authMgr.GetSigningCertPath(), authMgr.GetSigningKeyPath()),
			commander:          cmd,
		}
	}

	t.Run("disable path deletes file and restarts collector", func(t *testing.T) {
		core, observed := observer.New(zap.InfoLevel)
		logger := zap.New(core)
		s := newSupervisorWithCommander(t, logger)

		// Pre-create an own-logs.yaml file
		err := s.ownLogsPersistence.Save(ownlogs.Settings{
			Endpoint: "https://example.com:4318/v1/logs",
		})
		require.NoError(t, err)

		// Call handleOwnLogs with empty endpoint (disable)
		s.handleOwnLogs(context.Background(), &protobufs.TelemetryConnectionSettings{
			DestinationEndpoint: "",
		})

		// Verify file was deleted
		_, exists, err := s.ownLogsPersistence.Load()
		require.NoError(t, err)
		require.False(t, exists)

		// Verify restart was attempted (log message present)
		msgs := observed.FilterMessage("Restarting collector to apply own_logs changes")
		require.Equal(t, 1, msgs.Len(), "expected restart log message")
	})

	t.Run("enable path persists file and restarts collector", func(t *testing.T) {
		core, observed := observer.New(zap.DebugLevel)
		logger := zap.New(core)
		s := newSupervisorWithCommander(t, logger)

		// Use an HTTPS test server because ConvertSettings always loads TLS
		// client certs, which conflicts with plain HTTP endpoints.
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		s.handleOwnLogs(context.Background(), &protobufs.TelemetryConnectionSettings{
			DestinationEndpoint: ts.URL + "/v1/logs?tls_server_name=localhost",
			Certificate:         testprotos.CreateTLSCertificate(t),
		})

		// Verify file was persisted
		loaded, exists, err := s.ownLogsPersistence.Load()
		require.NoError(t, err)
		require.True(t, exists, "own-logs.yaml should be persisted; logs: %v", observed.All())
		require.Equal(t, ts.URL+"/v1/logs", loaded.Endpoint)

		// Verify restart was attempted (log message present)
		msgs := observed.FilterMessage("Restarting collector to apply own_logs changes")
		require.Equal(t, 1, msgs.Len(), "expected restart log message")
	})

	t.Run("unchanged settings do not restart collector again", func(t *testing.T) {
		core, observed := observer.New(zap.DebugLevel)
		logger := zap.New(core)
		s := newSupervisorWithCommander(t, logger)

		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		settings := &protobufs.TelemetryConnectionSettings{
			DestinationEndpoint: ts.URL + "/v1/logs?log_level=info&tls_server_name=localhost",
			Certificate:         testprotos.CreateTLSCertificate(t),
		}

		s.handleOwnLogs(context.Background(), settings)
		s.handleOwnLogs(context.Background(), settings)

		restarts := observed.FilterMessage("Restarting collector to apply own_logs changes")
		require.Equal(t, 1, restarts.Len(), "expected only one restart for unchanged own_logs settings")

		skips := observed.FilterMessage("Own logs settings unchanged, skipping apply")
		require.Equal(t, 1, skips.Len(), "expected unchanged own_logs settings to be skipped")
	})

	t.Run("no own logs manager logs warning", func(t *testing.T) {
		core, observed := observer.New(zap.WarnLevel)
		logger := zap.New(core)
		s := &Supervisor{
			logger: logger,
		}

		s.handleOwnLogs(context.Background(), &protobufs.TelemetryConnectionSettings{
			DestinationEndpoint: "https://example.com:4318/v1/logs",
		})

		msgs := observed.FilterMessage("Received own_logs settings but own logs manager is not configured")
		require.Equal(t, 1, msgs.Len())
	})
}

func TestCreateOpAMPCallbacks_OnOwnLogs_EnqueuesWork(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	ctx, cancel := context.WithCancel(context.Background())

	s := &Supervisor{
		logger:    logger,
		workQueue: make(chan workFunc),
		ctx:       ctx,
		cancel:    cancel,
	}
	s.workWg.Add(1)
	go s.runWorker()
	defer func() {
		cancel()
		s.workWg.Wait()
	}()

	callbacks := s.createOpAMPCallbacks()
	callbacks.OnOwnLogs(context.Background(), &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
	})

	require.Eventually(t, func() bool {
		return observed.FilterMessage("Received own_logs settings but own logs manager is not configured").Len() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestCreateOpAMPCallbacks_OnOwnLogs_EnqueueFailsAfterWorkerStop(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	ctx, cancel := context.WithCancel(context.Background())

	s := &Supervisor{
		logger:    logger,
		workQueue: make(chan workFunc),
		ctx:       ctx,
		cancel:    cancel,
	}
	s.workWg.Add(1)
	go s.runWorker()

	cancel()
	s.workWg.Wait()

	callbacks := s.createOpAMPCallbacks()
	callbacks.OnOwnLogs(context.Background(), &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
	})

	require.Equal(t, 1, observed.FilterMessage("Skipping own_logs apply during shutdown").Len())
	require.Zero(t, observed.FilterMessage("Failed to enqueue own_logs apply").Len())
}

// writeTestSigningCert generates a self-signed ECDSA cert/key pair and writes
// them to keysDir/signing.crt and keysDir/signing.key for use by auth.Manager.
func writeTestSigningCert(t *testing.T, keysDir string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	require.NoError(t, os.WriteFile(filepath.Join(keysDir, persistence.SigningCertFile), certPEM, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(keysDir, persistence.SigningKeyFile), keyPEM, 0o600))
}
