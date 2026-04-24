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
	hasTLSCA := s.TLSCAPemContents != ""
	hasTLSSettings := s.TLSMinVersion != "" || s.TLSMaxVersion != "" || s.InsecureSkipVerify || s.IncludeSystemCACertsPool
	hasServerName := s.TLSServerName != ""

	if !hasCert && !hasCA && !hasTLSCA && !hasTLSSettings && !hasServerName {
		return nil, nil
	}

	cfg := &tls.Config{
		ServerName:         s.TLSServerName,
		InsecureSkipVerify: s.InsecureSkipVerify, //nolint:gosec // Intentionally configurable
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
	if cfg.MinVersion != 0 && cfg.MaxVersion != 0 && cfg.MinVersion > cfg.MaxVersion {
		return nil, fmt.Errorf("TLS min version (%d) is greater than max version (%d)", cfg.MinVersion, cfg.MaxVersion)
	}

	if hasCert {
		clientCert, err := tls.X509KeyPair(s.CertPEM, s.KeyPEM)
		if err != nil {
			return nil, fmt.Errorf("parse client certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{clientCert}
	}

	// Build CA pool: mirror the merge logic from buildTLSConfig in convert.go
	if hasCA || hasTLSCA || s.IncludeSystemCACertsPool {
		var pool *x509.CertPool
		if s.IncludeSystemCACertsPool {
			var err error
			pool, err = x509.SystemCertPool()
			if err != nil {
				pool = x509.NewCertPool()
			}
		} else {
			pool = x509.NewCertPool()
		}
		if hasCA {
			if !pool.AppendCertsFromPEM(s.CACertPEM) {
				return nil, fmt.Errorf("failed to parse CA certificate")
			}
		}
		if hasTLSCA {
			if !pool.AppendCertsFromPEM([]byte(s.TLSCAPemContents)) {
				return nil, fmt.Errorf("failed to parse TLS CA certificate")
			}
		}
		cfg.RootCAs = pool
	}

	return cfg, nil
}

const ownLogsFileName = "own-logs.yaml"

// persistedSettings is the on-disk representation including TLS material
// so OTLP export survives restarts in mTLS/custom-CA deployments.
type persistedSettings struct {
	Endpoint                 string            `koanf:"endpoint"`
	Headers                  map[string]string `koanf:"headers,omitempty"`
	Insecure                 bool              `koanf:"insecure,omitempty"`
	CertPEM                  []byte            `koanf:"cert_pem,omitempty"`
	KeyPEM                   []byte            `koanf:"key_pem,omitempty"`
	CACertPEM                []byte            `koanf:"ca_cert_pem,omitempty"`
	TLSMinVersion            string            `koanf:"tls_min_version,omitempty"`
	TLSMaxVersion            string            `koanf:"tls_max_version,omitempty"`
	InsecureSkipVerify       bool              `koanf:"insecure_skip_verify,omitempty"`
	IncludeSystemCACertsPool bool              `koanf:"include_system_ca_certs_pool,omitempty"`
	TLSCAPemContents         string            `koanf:"tls_ca_pem_contents,omitempty"`
	TLSServerName            string            `koanf:"tls_server_name,omitempty"`
	ProxyURL                 string            `koanf:"proxy_url,omitempty"`
	ProxyHeaders             map[string]string `koanf:"proxy_headers,omitempty"`
	LogLevel                 string            `koanf:"log_level,omitempty"`
}

// Persistence handles saving and loading own_logs settings to disk.
type Persistence struct {
	filePath       string
	clientCertPath string
	clientKeyPath  string
}

// NewPersistence creates a Persistence that stores settings in dataDir.
// clientCertPath and clientKeyPath are the paths to the mTLS client certificate
// and key files that will be loaded when restoring settings from disk.
func NewPersistence(dataDir, clientCertPath, clientKeyPath string) *Persistence {
	return &Persistence{
		filePath:       filepath.Join(dataDir, ownLogsFileName),
		clientCertPath: clientCertPath,
		clientKeyPath:  clientKeyPath,
	}
}

// Delete removes the persisted settings file. It is not an error if the file
// does not exist.
func (p *Persistence) Delete() error {
	if err := os.Remove(p.filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Save persists the settings to disk including TLS material.
func (p *Persistence) Save(s Settings) error {
	ps := persistedSettings{
		Endpoint:                 s.Endpoint,
		Headers:                  s.Headers,
		Insecure:                 s.Insecure,
		CertPEM:                  s.CertPEM,
		KeyPEM:                   s.KeyPEM,
		CACertPEM:                s.CACertPEM,
		TLSMinVersion:            s.TLSMinVersion,
		TLSMaxVersion:            s.TLSMaxVersion,
		InsecureSkipVerify:       s.InsecureSkipVerify,
		IncludeSystemCACertsPool: s.IncludeSystemCACertsPool,
		TLSCAPemContents:         s.TLSCAPemContents,
		TLSServerName:            s.TLSServerName,
		ProxyURL:                 s.ProxyURL,
		ProxyHeaders:             s.ProxyHeaders,
		LogLevel:                 s.LogLevel,
	}
	return persistence.WriteYAMLFile(".", p.filePath, &ps)
}

// Load reads persisted settings from disk and rebuilds the TLS config.
// Returns (settings, exists, error).
func (p *Persistence) Load() (Settings, bool, error) {
	var ps persistedSettings
	if err := persistence.LoadYAMLFile(".", p.filePath, &ps); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Settings{}, false, nil
		}
		return Settings{}, true, err
	}

	s := Settings{
		Endpoint:                 ps.Endpoint,
		Headers:                  ps.Headers,
		Insecure:                 ps.Insecure,
		CertPEM:                  ps.CertPEM,
		KeyPEM:                   ps.KeyPEM,
		CACertPEM:                ps.CACertPEM,
		TLSMinVersion:            ps.TLSMinVersion,
		TLSMaxVersion:            ps.TLSMaxVersion,
		InsecureSkipVerify:       ps.InsecureSkipVerify,
		IncludeSystemCACertsPool: ps.IncludeSystemCACertsPool,
		TLSCAPemContents:         ps.TLSCAPemContents,
		TLSServerName:            ps.TLSServerName,
		ProxyURL:                 ps.ProxyURL,
		ProxyHeaders:             ps.ProxyHeaders,
		LogLevel:                 ps.LogLevel,
	}

	// Rebuild TLSConfig from persisted PEM material
	tlsCfg, err := rebuildTLSConfigFromPEM(s)
	if err != nil {
		return Settings{}, true, fmt.Errorf("rebuild TLS config: %w", err)
	}
	s.TLSConfig = tlsCfg

	// Load mTLS client certificate from file paths
	if err := s.LoadClientCert(p.clientCertPath, p.clientKeyPath); err != nil {
		return Settings{}, true, fmt.Errorf("load client certificate: %w", err)
	}

	return s, true, nil
}
