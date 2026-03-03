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
	"fmt"
	"strings"

	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
	"github.com/open-telemetry/opamp-go/protobufs"
)

// ConvertSettings converts OpAMP TelemetryConnectionSettings to ownlogs.Settings.
func ConvertSettings(proto *protobufs.TelemetryConnectionSettings) (Settings, error) {
	if proto == nil {
		return Settings{}, fmt.Errorf("nil TelemetryConnectionSettings")
	}
	if proto.DestinationEndpoint == "" {
		return Settings{}, fmt.Errorf("empty destination endpoint")
	}

	s := Settings{
		Endpoint: proto.DestinationEndpoint,
		Insecure: strings.HasPrefix(proto.DestinationEndpoint, "http://"),
	}

	// Convert headers
	if h := proto.GetHeaders(); h != nil && len(h.Headers) > 0 {
		s.Headers = make(map[string]string, len(h.Headers))
		for _, header := range h.Headers {
			s.Headers[header.Key] = header.Value
		}
	}

	// Build TLS config from Certificate and/or TLSConnectionSettings
	tlsCfg, err := buildTLSConfig(proto.GetCertificate(), proto.GetTls())
	if err != nil {
		return Settings{}, fmt.Errorf("build TLS config: %w", err)
	}
	if tlsCfg != nil {
		s.TLSConfig = tlsCfg
	}

	// Preserve raw PEM material for persistence
	if cert := proto.GetCertificate(); cert != nil {
		s.CertPEM = cert.GetCert()
		s.KeyPEM = cert.GetPrivateKey()
		s.CACertPEM = cert.GetCaCert()
	}
	if tlsSettings := proto.GetTls(); tlsSettings != nil {
		s.TLSMinVersion = tlsSettings.GetMinVersion()
		s.TLSMaxVersion = tlsSettings.GetMaxVersion()
		s.InsecureSkipVerify = tlsSettings.GetInsecureSkipVerify()
		// CaPemContents from TLS settings is also stored for persistence
		if len(s.CACertPEM) == 0 && tlsSettings.GetCaPemContents() != "" {
			s.CACertPEM = []byte(tlsSettings.GetCaPemContents())
		}
	}

	return s, nil
}

func buildTLSConfig(cert *protobufs.TLSCertificate, tlsSettings *protobufs.TLSConnectionSettings) (*tls.Config, error) {
	if cert == nil && tlsSettings == nil {
		return nil, nil
	}

	cfg := &tls.Config{}

	// Client certificate from TLSCertificate
	if cert != nil {
		if len(cert.GetCert()) > 0 && len(cert.GetPrivateKey()) > 0 {
			clientCert, err := tls.X509KeyPair(cert.GetCert(), cert.GetPrivateKey())
			if err != nil {
				return nil, fmt.Errorf("parse client certificate: %w", err)
			}
			cfg.Certificates = []tls.Certificate{clientCert}
		}

		// CA cert from TLSCertificate
		if len(cert.GetCaCert()) > 0 {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(cert.GetCaCert()) {
				return nil, fmt.Errorf("failed to parse CA certificate from TLSCertificate")
			}
			cfg.RootCAs = pool
		}
	}

	// TLSConnectionSettings overrides/supplements
	if tlsSettings != nil {
		if tlsSettings.GetInsecureSkipVerify() {
			cfg.InsecureSkipVerify = true
		}

		// Include system CA pool if requested (before adding any additional CAs).
		// This must happen even when cfg.RootCAs is already set from TLSCertificate.CaCert,
		// because the field means "load system CAs alongside any passed CAs".
		if tlsSettings.GetIncludeSystemCaCertsPool() {
			sysPool, err := x509.SystemCertPool()
			if err != nil {
				sysPool = x509.NewCertPool()
			}
			if cfg.RootCAs != nil {
				// Merge: add the previously configured CAs into the system pool.
				// x509.CertPool has no merge API, so we rebuild by adding PEM from
				// TLSCertificate.CaCert (already validated above) into the system pool.
				if cert != nil && len(cert.GetCaCert()) > 0 {
					sysPool.AppendCertsFromPEM(cert.GetCaCert())
				}
			}
			cfg.RootCAs = sysPool
		}

		// CA from TLSConnectionSettings (supplements existing pool)
		if caPEM := tlsSettings.GetCaPemContents(); caPEM != "" {
			if cfg.RootCAs == nil {
				cfg.RootCAs = x509.NewCertPool()
			}
			if !cfg.RootCAs.AppendCertsFromPEM([]byte(caPEM)) {
				return nil, fmt.Errorf("failed to parse CA certificate from TLSConnectionSettings")
			}
		}

		// Min/max TLS version
		if v := tlsSettings.GetMinVersion(); v != "" {
			parsed, err := parseTLSVersion(v)
			if err != nil {
				return nil, fmt.Errorf("parse TLS min version: %w", err)
			}
			cfg.MinVersion = parsed
		}
		if v := tlsSettings.GetMaxVersion(); v != "" {
			parsed, err := parseTLSVersion(v)
			if err != nil {
				return nil, fmt.Errorf("parse TLS max version: %w", err)
			}
			cfg.MaxVersion = parsed
		}
		if cfg.MinVersion != 0 && cfg.MaxVersion != 0 && cfg.MinVersion > cfg.MaxVersion {
			return nil, fmt.Errorf("TLS min version (%d) is greater than max version (%d)", cfg.MinVersion, cfg.MaxVersion)
		}
	}

	return cfg, nil
}

// parseTLSVersion reuses connection.TLSSettings.ToTLSVersion which accepts
// both "TLSv1.2" and "1.2" forms, trims whitespace, rejects TLS < 1.2,
// and returns an error for unsupported values.
func parseTLSVersion(v string) (uint16, error) {
	return connection.TLSSettings{}.ToTLSVersion(v)
}
