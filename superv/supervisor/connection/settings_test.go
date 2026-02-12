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

package connection

import (
	"crypto/tls"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSettingsCloneIncludesProxyHeaders(t *testing.T) {
	original := Settings{
		Headers: map[string]string{
			"Authorization": "Bearer token",
		},
		ProxyHeaders: map[string]string{
			"Proxy-Authorization": "Basic abc123",
		},
	}

	cloned := original.clone()
	require.Equal(t, original.Headers, cloned.Headers)
	require.Equal(t, original.ProxyHeaders, cloned.ProxyHeaders)

	cloned.Headers["Authorization"] = "changed"
	cloned.ProxyHeaders["Proxy-Authorization"] = "changed"

	require.Equal(t, "Bearer token", original.Headers["Authorization"])
	require.Equal(t, "Basic abc123", original.ProxyHeaders["Proxy-Authorization"])
}

func TestSettingsEqual(t *testing.T) {
	base := Settings{
		Endpoint:          "wss://example.com/v1/opamp",
		Headers:           map[string]string{"Authorization": "Bearer token"},
		CACertPEM:         "ca-cert",
		ClientCertPEM:     "client-cert",
		ClientKeyPEM:      "client-key",
		TLS:               TLSSettings{Insecure: false, MinVersion: "TLSv1.2", MaxVersion: "TLSv1.3", CAPEMContents: "ca-pem"},
		ProxyURL:          "http://proxy:8080",
		ProxyHeaders:      map[string]string{"Proxy-Auth": "Basic abc"},
		HeartbeatInterval: 30 * time.Second,
	}

	t.Run("identical", func(t *testing.T) {
		other := base.clone()
		require.True(t, base.Equal(other))
	})

	t.Run("different endpoint", func(t *testing.T) {
		other := base.clone()
		other.Endpoint = "wss://other.com"
		require.False(t, base.Equal(other))
	})

	t.Run("different headers", func(t *testing.T) {
		other := base.clone()
		other.Headers["Authorization"] = "changed"
		require.False(t, base.Equal(other))
	})

	t.Run("different ca cert", func(t *testing.T) {
		other := base.clone()
		other.CACertPEM = "different"
		require.False(t, base.Equal(other))
	})

	t.Run("different client cert", func(t *testing.T) {
		other := base.clone()
		other.ClientCertPEM = "different"
		require.False(t, base.Equal(other))
	})

	t.Run("different client key", func(t *testing.T) {
		other := base.clone()
		other.ClientKeyPEM = "different"
		require.False(t, base.Equal(other))
	})

	t.Run("different tls insecure", func(t *testing.T) {
		other := base.clone()
		other.TLS.Insecure = true
		require.False(t, base.Equal(other))
	})

	t.Run("different proxy url", func(t *testing.T) {
		other := base.clone()
		other.ProxyURL = "http://other-proxy:9090"
		require.False(t, base.Equal(other))
	})

	t.Run("different proxy headers", func(t *testing.T) {
		other := base.clone()
		other.ProxyHeaders["Proxy-Auth"] = "changed"
		require.False(t, base.Equal(other))
	})

	t.Run("different heartbeat interval", func(t *testing.T) {
		other := base.clone()
		other.HeartbeatInterval = 60 * time.Second
		require.False(t, base.Equal(other))
	})

	t.Run("UpdatedAt is not compared", func(t *testing.T) {
		other := base.clone()
		other.UpdatedAt = time.Now().Add(time.Hour)
		require.True(t, base.Equal(other))
	})
}

func TestToTLSVersion(t *testing.T) {
	var s TLSSettings

	tests := []struct {
		input     string
		expected  uint16
		expectErr bool
	}{
		{"TLSv1.2", tls.VersionTLS12, false},
		{"TLSv1.3", tls.VersionTLS13, false},
		{"1.2", tls.VersionTLS12, false},
		{"1.3", tls.VersionTLS13, false},
		{" TLSv1.2 ", tls.VersionTLS12, false},
		{"", 0, true},
		{"unknown", 0, true},
	}

	for _, test := range tests {
		version, err := s.ToTLSVersion(test.input)

		if test.expectErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}

		require.Equal(t, test.expected, version)
	}
}

func TestToTLSMinMaxVersion(t *testing.T) {
	tests := []struct {
		name      string
		settings  TLSSettings
		wantMin   uint16
		wantMax   uint16
		expectErr bool
	}{
		{
			name:      "both empty uses defaults",
			settings:  TLSSettings{},
			wantMin:   0,
			wantMax:   0,
			expectErr: false,
		},
		{
			name:      "min only",
			settings:  TLSSettings{MinVersion: "1.2"},
			wantMin:   tls.VersionTLS12,
			wantMax:   0,
			expectErr: false,
		},
		{
			name:      "max only",
			settings:  TLSSettings{MaxVersion: "TLSv1.3"},
			wantMin:   0,
			wantMax:   tls.VersionTLS13,
			expectErr: false,
		},
		{
			name:      "valid range",
			settings:  TLSSettings{MinVersion: "TLSv1.2", MaxVersion: "TLSv1.3"},
			wantMin:   tls.VersionTLS12,
			wantMax:   tls.VersionTLS13,
			expectErr: false,
		},
		{
			name:      "invalid min",
			settings:  TLSSettings{MinVersion: "TLSv1.1"},
			wantMin:   0,
			wantMax:   0,
			expectErr: true,
		},
		{
			name:      "invalid max",
			settings:  TLSSettings{MaxVersion: "TLSv1.1"},
			wantMin:   0,
			wantMax:   0,
			expectErr: true,
		},
		{
			name:      "min greater than max",
			settings:  TLSSettings{MinVersion: "1.3", MaxVersion: "1.2"},
			wantMin:   0,
			wantMax:   0,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minVersion, maxVersion, err := tt.settings.ToTLSMinMaxVersion()
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantMin, minVersion)
			require.Equal(t, tt.wantMax, maxVersion)
		})
	}
}

func TestConvertProtoHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    *protobufs.Headers
		expected map[string]string
	}{
		{
			name:     "nil headers",
			input:    nil,
			expected: nil,
		},
		{
			name: "empty headers",
			input: &protobufs.Headers{
				Headers: []*protobufs.Header{},
			},
			expected: map[string]string{},
		},
		{
			name: "single header",
			input: &protobufs.Headers{
				Headers: []*protobufs.Header{
					{Key: "Authorization", Value: "Bearer token"},
				},
			},
			expected: map[string]string{
				"Authorization": "Bearer token",
			},
		},
		{
			name: "multiple headers",
			input: &protobufs.Headers{
				Headers: []*protobufs.Header{
					{Key: "Authorization", Value: "Bearer token"},
					{Key: "X-Custom", Value: "value"},
				},
			},
			expected: map[string]string{
				"Authorization": "Bearer token",
				"X-Custom":      "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertProtoHeaders(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestHeartbeatDurationFromSecondsClampsOverflow(t *testing.T) {
	require.Equal(t, time.Duration(0), heartbeatDurationFromSeconds(0))
	require.Equal(t, 60*time.Second, heartbeatDurationFromSeconds(60))
	require.Equal(t, time.Duration(math.MaxInt64), heartbeatDurationFromSeconds(math.MaxUint64))
	require.Equal(t, time.Duration(math.MaxInt64), heartbeatDurationFromSeconds(maxHeartbeatIntervalSeconds+1))
	require.Equal(t, time.Duration(maxHeartbeatIntervalSeconds)*time.Second, heartbeatDurationFromSeconds(maxHeartbeatIntervalSeconds))
}

func TestUpdateFromOpAMPSettings(t *testing.T) {
	current := Settings{
		Endpoint:          "wss://original.example.com/v1/opamp",
		Headers:           map[string]string{"Authorization": "Bearer old"},
		CACertPEM:         "old-ca",
		TLS:               TLSSettings{MinVersion: "TLSv1.2", MaxVersion: "TLSv1.3"},
		ProxyURL:          "http://proxy:8080",
		ProxyHeaders:      map[string]string{"Proxy-Auth": "old"},
		HeartbeatInterval: 30 * time.Second,
	}

	manager := NewSettingsManager(zap.NewNop(), t.TempDir())

	t.Run("endpoint updated", func(t *testing.T) {
		result := manager.updateFromOpAMPSettings(current, &protobufs.OpAMPConnectionSettings{
			DestinationEndpoint: "wss://new.example.com/v1/opamp",
		})
		require.Equal(t, "wss://new.example.com/v1/opamp", result.Endpoint)
	})

	t.Run("empty endpoint preserves current", func(t *testing.T) {
		result := manager.updateFromOpAMPSettings(current, &protobufs.OpAMPConnectionSettings{
			DestinationEndpoint: "",
		})
		require.Equal(t, "wss://original.example.com/v1/opamp", result.Endpoint)
	})

	t.Run("headers updated", func(t *testing.T) {
		result := manager.updateFromOpAMPSettings(current, &protobufs.OpAMPConnectionSettings{
			Headers: &protobufs.Headers{
				Headers: []*protobufs.Header{
					{Key: "Authorization", Value: "Bearer new"},
				},
			},
		})
		require.Equal(t, map[string]string{"Authorization": "Bearer new"}, result.Headers)
	})

	t.Run("nil headers preserves current", func(t *testing.T) {
		result := manager.updateFromOpAMPSettings(current, &protobufs.OpAMPConnectionSettings{})
		require.Equal(t, current.Headers, result.Headers)
	})

	t.Run("tls settings replaced", func(t *testing.T) {
		result := manager.updateFromOpAMPSettings(current, &protobufs.OpAMPConnectionSettings{
			Tls: &protobufs.TLSConnectionSettings{
				InsecureSkipVerify: true,
				MinVersion:         "TLSv1.3",
				MaxVersion:         "TLSv1.3",
				CaPemContents:      "new-ca-pem",
			},
		})
		require.True(t, result.TLS.Insecure)
		require.Equal(t, "TLSv1.3", result.TLS.MinVersion)
		require.Equal(t, "TLSv1.3", result.TLS.MaxVersion)
		require.Equal(t, "new-ca-pem", result.TLS.CAPEMContents)
	})

	t.Run("nil tls preserves current", func(t *testing.T) {
		result := manager.updateFromOpAMPSettings(current, &protobufs.OpAMPConnectionSettings{})
		require.Equal(t, current.TLS, result.TLS)
	})

	t.Run("proxy url updated", func(t *testing.T) {
		result := manager.updateFromOpAMPSettings(current, &protobufs.OpAMPConnectionSettings{
			Proxy: &protobufs.ProxyConnectionSettings{
				Url: "http://new-proxy:9090",
			},
		})
		require.Equal(t, "http://new-proxy:9090", result.ProxyURL)
	})

	t.Run("empty proxy url preserves current", func(t *testing.T) {
		result := manager.updateFromOpAMPSettings(current, &protobufs.OpAMPConnectionSettings{
			Proxy: &protobufs.ProxyConnectionSettings{
				Url: "",
			},
		})
		require.Equal(t, "http://proxy:8080", result.ProxyURL)
	})

	t.Run("proxy headers updated", func(t *testing.T) {
		result := manager.updateFromOpAMPSettings(current, &protobufs.OpAMPConnectionSettings{
			Proxy: &protobufs.ProxyConnectionSettings{
				ConnectHeaders: &protobufs.Headers{
					Headers: []*protobufs.Header{
						{Key: "Proxy-Auth", Value: "new"},
					},
				},
			},
		})
		require.Equal(t, map[string]string{"Proxy-Auth": "new"}, result.ProxyHeaders)
	})

	t.Run("heartbeat interval updated", func(t *testing.T) {
		result := manager.updateFromOpAMPSettings(current, &protobufs.OpAMPConnectionSettings{
			HeartbeatIntervalSeconds: 60,
		})
		require.Equal(t, 60*time.Second, result.HeartbeatInterval)
	})

	t.Run("heartbeat interval zero preserves current", func(t *testing.T) {
		result := manager.updateFromOpAMPSettings(current, &protobufs.OpAMPConnectionSettings{
			HeartbeatIntervalSeconds: 0,
		})
		require.Equal(t, current.HeartbeatInterval, result.HeartbeatInterval)
	})

	t.Run("does not mutate current", func(t *testing.T) {
		original := current.clone()
		manager.updateFromOpAMPSettings(current, &protobufs.OpAMPConnectionSettings{
			DestinationEndpoint:      "wss://changed.example.com",
			HeartbeatIntervalSeconds: 120,
		})
		require.Equal(t, original, current)
	})
}

func TestSettingsChanged(t *testing.T) {
	manager := NewSettingsManager(zap.NewNop(), t.TempDir())
	manager.SetCurrent(Settings{
		Endpoint:          "wss://example.com/v1/opamp",
		HeartbeatInterval: 30 * time.Second,
	})

	t.Run("returns false when nothing changed", func(t *testing.T) {
		_, changed := manager.SettingsChanged(&protobufs.OpAMPConnectionSettings{
			HeartbeatIntervalSeconds: 30,
		})
		require.False(t, changed)
	})

	t.Run("returns false when heartbeat interval is not provided", func(t *testing.T) {
		_, changed := manager.SettingsChanged(&protobufs.OpAMPConnectionSettings{
			HeartbeatIntervalSeconds: 0,
		})
		require.False(t, changed)
	})

	t.Run("returns true when endpoint changed", func(t *testing.T) {
		updated, changed := manager.SettingsChanged(&protobufs.OpAMPConnectionSettings{
			DestinationEndpoint:      "wss://new.example.com/v1/opamp",
			HeartbeatIntervalSeconds: 30,
		})
		require.True(t, changed)
		require.Equal(t, "wss://new.example.com/v1/opamp", updated.Endpoint)
	})

	t.Run("clamps heartbeat overflow", func(t *testing.T) {
		updated, changed := manager.SettingsChanged(&protobufs.OpAMPConnectionSettings{
			HeartbeatIntervalSeconds: math.MaxUint64,
		})
		require.True(t, changed)
		require.Equal(t, time.Duration(math.MaxInt64), updated.HeartbeatInterval)
	})
}

func TestGetCurrentPanicsWhenUninitialized(t *testing.T) {
	manager := NewSettingsManager(zap.NewNop(), t.TempDir())
	require.Panics(t, func() { manager.GetCurrent() })
}

func TestSetCurrentGetCurrentRoundTrip(t *testing.T) {
	manager := NewSettingsManager(zap.NewNop(), t.TempDir())
	settings := Settings{
		Endpoint: "wss://example.com/v1/opamp",
		Headers:  map[string]string{"Auth": "token"},
	}
	manager.SetCurrent(settings)

	got := manager.GetCurrent()
	require.True(t, settings.Equal(got))

	// Mutating the returned value must not affect the stored value.
	got.Endpoint = "mutated"
	require.Equal(t, "wss://example.com/v1/opamp", manager.GetCurrent().Endpoint)
}

func TestCaptureSnapshotReturnsCopy(t *testing.T) {
	manager := NewSettingsManager(zap.NewNop(), t.TempDir())
	manager.SetCurrent(Settings{Endpoint: "wss://example.com"})

	snapshot := manager.CaptureSnapshot()
	require.Equal(t, "wss://example.com", snapshot.Endpoint)

	manager.SetCurrent(Settings{Endpoint: "wss://changed.example.com"})
	require.Equal(t, "wss://example.com", snapshot.Endpoint)
}

func TestLoadPersisted(t *testing.T) {
	t.Run("loads persisted settings", func(t *testing.T) {
		dir := t.TempDir()
		manager := NewSettingsManager(zap.NewNop(), dir)

		original := Settings{
			Endpoint:          "wss://example.com/v1/opamp",
			Headers:           map[string]string{"Authorization": "Bearer token"},
			HeartbeatInterval: 30 * time.Second,
		}
		// Write settings via StageNext + Commit so the file exists.
		manager.SetCurrent(Settings{Endpoint: "initial"})
		stage, err := manager.StageNext(original)
		require.NoError(t, err)
		require.NoError(t, stage.Commit())

		loaded, err := manager.LoadPersisted()
		require.NoError(t, err)
		require.Equal(t, original.Endpoint, loaded.Endpoint)
		require.Equal(t, original.Headers, loaded.Headers)
		require.Equal(t, original.HeartbeatInterval, loaded.HeartbeatInterval)
	})

	t.Run("returns error when file does not exist", func(t *testing.T) {
		manager := NewSettingsManager(zap.NewNop(), t.TempDir())
		_, err := manager.LoadPersisted()
		require.Error(t, err)
	})
}

func TestTryLoadPersisted(t *testing.T) {
	t.Run("returns settings when file exists", func(t *testing.T) {
		dir := t.TempDir()
		manager := NewSettingsManager(zap.NewNop(), dir)
		manager.SetCurrent(Settings{Endpoint: "initial"})

		original := Settings{Endpoint: "wss://example.com"}
		stage, err := manager.StageNext(original)
		require.NoError(t, err)
		require.NoError(t, stage.Commit())

		loaded, exists, err := manager.TryLoadPersisted()
		require.NoError(t, err)
		require.True(t, exists)
		require.Equal(t, "wss://example.com", loaded.Endpoint)
	})

	t.Run("returns false when file does not exist", func(t *testing.T) {
		manager := NewSettingsManager(zap.NewNop(), t.TempDir())
		_, exists, err := manager.TryLoadPersisted()
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("returns error for corrupt file", func(t *testing.T) {
		dir := t.TempDir()
		// Write invalid YAML to the settings file.
		err := os.WriteFile(filepath.Join(dir, settingsFileName), []byte(":\x00invalid"), 0600)
		require.NoError(t, err)

		manager := NewSettingsManager(zap.NewNop(), dir)
		_, exists, err := manager.TryLoadPersisted()
		require.Error(t, err)
		require.True(t, exists)
	})
}

func TestStageNext(t *testing.T) {
	t.Run("commit makes settings current", func(t *testing.T) {
		dir := t.TempDir()
		manager := NewSettingsManager(zap.NewNop(), dir)
		manager.SetCurrent(Settings{Endpoint: "wss://old.example.com"})

		next := Settings{Endpoint: "wss://new.example.com"}
		stage, err := manager.StageNext(next)
		require.NoError(t, err)

		// Before commit, current is still old.
		require.Equal(t, "wss://old.example.com", manager.GetCurrent().Endpoint)

		require.NoError(t, stage.Commit())

		// After commit, current is updated via the callback.
		require.Equal(t, "wss://new.example.com", manager.GetCurrent().Endpoint)

		// File is persisted and loadable.
		loaded, err := manager.LoadPersisted()
		require.NoError(t, err)
		require.Equal(t, "wss://new.example.com", loaded.Endpoint)
	})

	t.Run("returns error for unwritable path", func(t *testing.T) {
		// Create a regular file, then use it as a parent directory so MkdirAll fails portably.
		blocker := filepath.Join(t.TempDir(), "notadir")
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0600))

		manager := NewSettingsManager(zap.NewNop(), blocker)
		manager.SetCurrent(Settings{Endpoint: "wss://old.example.com"})

		_, err := manager.StageNext(Settings{Endpoint: "wss://new.example.com"})
		require.Error(t, err)
	})

	t.Run("cleanup does not make settings current", func(t *testing.T) {
		dir := t.TempDir()
		manager := NewSettingsManager(zap.NewNop(), dir)
		manager.SetCurrent(Settings{Endpoint: "wss://old.example.com"})

		next := Settings{Endpoint: "wss://new.example.com"}
		stage, err := manager.StageNext(next)
		require.NoError(t, err)

		require.NoError(t, stage.Cleanup())

		// Current unchanged after cleanup.
		require.Equal(t, "wss://old.example.com", manager.GetCurrent().Endpoint)
	})
}

// TestSettingsManagerConcurrentAccess verifies that concurrent reads and writes
// do not race. Run with -race to detect data races. The post-condition assertions
// verify that the manager remains in a consistent state after concurrent access.
func TestSettingsManagerConcurrentAccess(t *testing.T) {
	manager := NewSettingsManager(zap.NewNop(), t.TempDir())
	manager.SetCurrent(Settings{
		Endpoint: "wss://initial.example.com/v1/opamp",
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if j%2 == 0 {
					manager.SetCurrent(Settings{
						Endpoint: fmt.Sprintf("wss://%d-%d.example.com/v1/opamp", worker, j),
					})
					continue
				}

				got := manager.GetCurrent()
				assert.NotEmpty(t, got.Endpoint)

				_, _ = manager.SettingsChanged(&protobufs.OpAMPConnectionSettings{
					DestinationEndpoint: fmt.Sprintf("wss://new-%d.example.com/v1/opamp", worker),
				})

				snap := manager.CaptureSnapshot()
				assert.NotEmpty(t, snap.Endpoint)
			}
		}(i)
	}

	wg.Wait()

	// After all goroutines finish, the manager must still be usable and consistent.
	final := manager.GetCurrent()
	require.NotEmpty(t, final.Endpoint)
	require.True(t, final.Equal(manager.CaptureSnapshot()))
}
