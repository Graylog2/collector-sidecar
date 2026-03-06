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

package ownlogs

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
)

// rebuildTLSConfigFromPEM reconstructs a *tls.Config from the raw PEM bytes
// stored in Settings. Returns nil if no TLS material is present.
func rebuildTLSConfigFromPEM(s Settings) (*tls.Config, error) {
	hasCert := len(s.CertPEM) > 0 && len(s.KeyPEM) > 0
	hasCA := len(s.CACertPEM) > 0
	hasTLSSettings := s.TLSMinVersion != "" || s.TLSMaxVersion != "" || s.InsecureSkipVerify

	if !hasCert && !hasCA && !hasTLSSettings {
		return nil, nil
	}

	cfg := &tls.Config{
		InsecureSkipVerify: s.InsecureSkipVerify,
	}

	if s.TLSMinVersion != "" {
		v, err := connection.ToTLSVersion(s.TLSMinVersion)
		if err != nil {
			return nil, fmt.Errorf("parse TLS min version: %w", err)
		}
		cfg.MinVersion = v
	}
	if s.TLSMaxVersion != "" {
		v, err := connection.ToTLSVersion(s.TLSMaxVersion)
		if err != nil {
			return nil, fmt.Errorf("parse TLS max version: %w", err)
		}
		cfg.MaxVersion = v
	}

	if hasCert {
		clientCert, err := tls.X509KeyPair(s.CertPEM, s.KeyPEM)
		if err != nil {
			return nil, fmt.Errorf("parse client certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{clientCert}
	}

	if hasCA {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(s.CACertPEM) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		cfg.RootCAs = pool
	}

	return cfg, nil
}

const ownLogsFileName = "own_logs.yaml"

// persistedSettings is the on-disk representation including TLS material
// so OTLP export survives restarts in mTLS/custom-CA deployments.
type persistedSettings struct {
	Endpoint           string            `koanf:"endpoint"`
	Headers            map[string]string `koanf:"headers,omitempty"`
	Insecure           bool              `koanf:"insecure,omitempty"`
	CertPEM            []byte            `koanf:"cert_pem,omitempty"`
	KeyPEM             []byte            `koanf:"key_pem,omitempty"`
	CACertPEM          []byte            `koanf:"ca_cert_pem,omitempty"`
	TLSMinVersion      string            `koanf:"tls_min_version,omitempty"`
	TLSMaxVersion      string            `koanf:"tls_max_version,omitempty"`
	InsecureSkipVerify bool              `koanf:"insecure_skip_verify,omitempty"`
}

// Persistence handles saving and loading own_logs settings to disk.
type Persistence struct {
	filePath string
}

// NewPersistence creates a Persistence that stores settings in dataDir.
func NewPersistence(dataDir string) *Persistence {
	return &Persistence{
		filePath: filepath.Join(dataDir, ownLogsFileName),
	}
}

// Save persists the settings to disk including TLS material.
func (p *Persistence) Save(s Settings) error {
	ps := persistedSettings{
		Endpoint:           s.Endpoint,
		Headers:            s.Headers,
		Insecure:           s.Insecure,
		CertPEM:            s.CertPEM,
		KeyPEM:             s.KeyPEM,
		CACertPEM:          s.CACertPEM,
		TLSMinVersion:      s.TLSMinVersion,
		TLSMaxVersion:      s.TLSMaxVersion,
		InsecureSkipVerify: s.InsecureSkipVerify,
	}
	return persistence.WriteYAMLFile(".", p.filePath, &ps)
}

// Load reads persisted settings from disk and rebuilds the TLS config.
// Returns (settings, exists, error).
func (p *Persistence) Load() (Settings, bool, error) {
	if _, err := os.Stat(p.filePath); errors.Is(err, os.ErrNotExist) {
		return Settings{}, false, nil
	}

	var ps persistedSettings
	if err := persistence.LoadYAMLFile(".", p.filePath, &ps); err != nil {
		return Settings{}, true, err
	}

	s := Settings{
		Endpoint:           ps.Endpoint,
		Headers:            ps.Headers,
		Insecure:           ps.Insecure,
		CertPEM:            ps.CertPEM,
		KeyPEM:             ps.KeyPEM,
		CACertPEM:          ps.CACertPEM,
		TLSMinVersion:      ps.TLSMinVersion,
		TLSMaxVersion:      ps.TLSMaxVersion,
		InsecureSkipVerify: ps.InsecureSkipVerify,
	}

	// Rebuild TLSConfig from persisted PEM material
	tlsCfg, err := rebuildTLSConfigFromPEM(s)
	if err != nil {
		return Settings{}, true, fmt.Errorf("rebuild TLS config: %w", err)
	}
	s.TLSConfig = tlsCfg

	return s, true, nil
}
