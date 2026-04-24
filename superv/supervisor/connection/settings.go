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
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

const settingsFileName = "connection.yaml"
const maxHeartbeatIntervalSeconds = uint64(math.MaxInt64 / int64(time.Second))

type Settings struct {
	Endpoint          string            `koanf:"endpoint,omitempty"`
	Headers           map[string]string `koanf:"headers,omitempty"`
	CACertPEM         string            `koanf:"ca_cert_pem,omitempty"`
	ClientCertPEM     string            `koanf:"client_cert_pem,omitempty"`
	ClientKeyPEM      string            `koanf:"client_key_pem,omitempty"`
	TLS               TLSSettings       `koanf:"tls,omitempty"`
	ProxyURL          string            `koanf:"proxy_url,omitempty"`
	ProxyHeaders      map[string]string `koanf:"proxy_headers,omitempty"`
	HeartbeatInterval time.Duration     `koanf:"heartbeat_interval,omitempty"`
	UpdatedAt         time.Time         `koanf:"updated_at"`
}

func (s Settings) clone() Settings {
	return Settings{
		Endpoint:          s.Endpoint,
		Headers:           maps.Clone(s.Headers),
		CACertPEM:         s.CACertPEM,
		ClientCertPEM:     s.ClientCertPEM,
		ClientKeyPEM:      s.ClientKeyPEM,
		TLS:               s.TLS.clone(),
		ProxyURL:          s.ProxyURL,
		ProxyHeaders:      maps.Clone(s.ProxyHeaders),
		HeartbeatInterval: s.HeartbeatInterval,
		UpdatedAt:         s.UpdatedAt,
	}
}

func (s Settings) Equal(o Settings) bool {
	return s.Endpoint == o.Endpoint &&
		maps.Equal(s.Headers, o.Headers) &&
		s.CACertPEM == o.CACertPEM &&
		s.ClientCertPEM == o.ClientCertPEM &&
		s.ClientKeyPEM == o.ClientKeyPEM &&
		s.TLS.Equal(o.TLS) &&
		s.ProxyURL == o.ProxyURL &&
		maps.Equal(s.ProxyHeaders, o.ProxyHeaders) &&
		s.HeartbeatInterval == o.HeartbeatInterval
}

type TLSSettings struct {
	Insecure      bool   `koanf:"insecure,omitempty"`
	MinVersion    string `koanf:"min_version,omitempty"`
	MaxVersion    string `koanf:"max_version,omitempty"`
	CAPEMContents string `koanf:"ca_pem_contents,omitempty"`
}

func ToTLSVersion(version string) (uint16, error) {
	switch strings.TrimSpace(version) {
	case "TLSv1.2", "1.2":
		return tls.VersionTLS12, nil
	case "TLSv1.3", "1.3":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("unsupported TLS version %q", version)
	}
}

func (t TLSSettings) ToTLSMinMaxVersion() (uint16, uint16, error) {
	var minVersion uint16
	if t.MinVersion != "" {
		parsedMin, err := ToTLSVersion(t.MinVersion)
		if err != nil {
			return 0, 0, fmt.Errorf("parse TLS min version: %w", err)
		}
		minVersion = parsedMin
	}

	var maxVersion uint16
	if t.MaxVersion != "" {
		parsedMax, err := ToTLSVersion(t.MaxVersion)
		if err != nil {
			return 0, 0, fmt.Errorf("parse TLS max version: %w", err)
		}
		maxVersion = parsedMax
	}

	if minVersion != 0 && maxVersion != 0 && minVersion > maxVersion {
		return 0, 0, fmt.Errorf("invalid TLS version range: min version %q is greater than max version %q", t.MinVersion, t.MaxVersion)
	}

	return minVersion, maxVersion, nil
}

func (t TLSSettings) clone() TLSSettings {
	return TLSSettings{
		Insecure:      t.Insecure,
		MinVersion:    t.MinVersion,
		MaxVersion:    t.MaxVersion,
		CAPEMContents: t.CAPEMContents,
	}
}

func (t TLSSettings) Equal(o TLSSettings) bool {
	return t.Insecure == o.Insecure &&
		t.MinVersion == o.MinVersion &&
		t.MaxVersion == o.MaxVersion &&
		t.CAPEMContents == o.CAPEMContents
}

func NewSettingsManager(logger *zap.Logger, dataDir string) *SettingsManager {
	return &SettingsManager{
		logger:   logger,
		filePath: filepath.Join(dataDir, settingsFileName),
	}
}

type SettingsManager struct {
	logger   *zap.Logger
	filePath string
	mu       sync.RWMutex
	current  *Settings
}

func (s *SettingsManager) currentClone() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.current == nil {
		panic("current settings not initialized")
	}

	return s.current.clone()
}

// GetCurrent returns a clone of the current settings. Panics if current settings have not been initialized yet.
func (s *SettingsManager) GetCurrent() Settings {
	return s.currentClone()
}

// SetCurrent updates the current settings. Clones the provided settings to ensure immutability.
func (s *SettingsManager) SetCurrent(settings Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = new(settings.clone())
}

// SettingsChanged compares the provided OpAMP settings with the current settings and returns a new Settings struct
// with the updates from the OpAMP settings. Returns the new settings and true if any settings have changed.
func (s *SettingsManager) SettingsChanged(settings *protobufs.OpAMPConnectionSettings) (Settings, bool) {
	current := s.currentClone()

	newSettings := s.updateFromOpAMPSettings(current, settings)
	return newSettings, !current.Equal(newSettings)
}

// updateFromOpAMPSettings creates a new Settings struct based on the current settings and updates from the provided OpAMP settings.
func (s *SettingsManager) updateFromOpAMPSettings(current Settings, settings *protobufs.OpAMPConnectionSettings) Settings {
	updated := current.clone()

	// A 0 value for HeartbeatIntervalSeconds is considered as "not provided" according to the OpAMP spec.
	if settings.GetHeartbeatIntervalSeconds() > 0 {
		// According to the OpAMP spec, if HeartbeatIntervalSeconds is provided and greater than 0, the client MUST
		// use it and ignore any previously configured heartbeat interval.
		updated.HeartbeatInterval = heartbeatDurationFromSeconds(settings.GetHeartbeatIntervalSeconds())
	}

	if endpoint := settings.GetDestinationEndpoint(); endpoint != "" {
		updated.Endpoint = endpoint
	}

	if headers := settings.GetHeaders(); headers != nil {
		updated.Headers = convertProtoHeaders(headers)
	}

	if tls := settings.GetTls(); tls != nil {
		// According to the OpAMP spec, if TLS settings are provided, the client MUST use them and ignore any
		// previously configured TLS settings.
		updated.TLS.Insecure = tls.GetInsecureSkipVerify()
		updated.TLS.MinVersion = tls.GetMinVersion()
		updated.TLS.MaxVersion = tls.GetMaxVersion()
		updated.TLS.CAPEMContents = tls.GetCaPemContents()
	}

	if proxy := settings.GetProxy(); proxy != nil {
		if proxyURL := proxy.GetUrl(); proxyURL != "" {
			updated.ProxyURL = proxyURL
		}
		if proxyHeaders := proxy.GetConnectHeaders(); proxyHeaders != nil {
			updated.ProxyHeaders = convertProtoHeaders(proxyHeaders)
		}
	}

	return updated
}

// CaptureSnapshot clones the current connection state for rollback.
func (s *SettingsManager) CaptureSnapshot() Settings {
	return s.currentClone()
}

// LoadPersisted loads any persisted connection settings from disk and returns them.
func (s *SettingsManager) LoadPersisted() (Settings, error) {
	var settings Settings

	if err := persistence.LoadYAMLFile(".", s.filePath, &settings); err != nil {
		return Settings{}, fmt.Errorf("load persisted settings: %w", err)
	}

	return settings, nil
}

// TryLoadPersisted attempts to load persisted settings. Returns the settings, a boolean indicating whether settings
// were found, and an error if loading failed for reasons other than the file not existing.
func (s *SettingsManager) TryLoadPersisted() (Settings, bool, error) {
	settings, err := s.LoadPersisted()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Settings{}, false, nil
		}
		return Settings{}, true, err
	}
	return settings, true, nil
}

func (s *SettingsManager) Persist(settings Settings) error {
	return persistence.WriteYAMLFile(".", s.filePath, &settings)
}

// StageNext stages the provided settings but does not yet make them current. This allows the caller to persist the
// new settings and only switch to them if a condition is met (e.g. successful connection with new settings).
// Calls SetCurrent as a commit callback, so that if the staged file is committed, the new settings become current.
func (s *SettingsManager) StageNext(settings Settings) (persistence.StagedFile, error) {
	stage, err := persistence.StageYAMLFile(".", s.filePath, &settings)
	if err != nil {
		return nil, fmt.Errorf("persist settings: %w", err)
	}
	stage.SetCommitCallback(func() error {
		s.SetCurrent(settings)
		return nil
	})
	return stage, nil
}

func convertProtoHeaders(h *protobufs.Headers) map[string]string {
	if h == nil {
		return nil
	}
	result := make(map[string]string, len(h.GetHeaders()))
	for _, header := range h.GetHeaders() {
		result[header.GetKey()] = header.GetValue()
	}
	return result
}

func heartbeatDurationFromSeconds(seconds uint64) time.Duration {
	if seconds > maxHeartbeatIntervalSeconds {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(seconds) * time.Second
}
