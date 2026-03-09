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
	"net/url"
	"os"
	"strings"

	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
	"github.com/open-telemetry/opamp-go/protobufs"
)

// ConvertSettings converts OpAMP TelemetryConnectionSettings to ownlogs.Settings.
// clientCertPath and clientKeyPath are the paths to the mTLS client certificate
// and key files (e.g. the sidecar's signing cert/key). The exporter will not
// start if the files cannot be loaded.
func ConvertSettings(proto *protobufs.TelemetryConnectionSettings, clientCertPath, clientKeyPath string) (Settings, error) {
	if proto == nil {
		return Settings{}, fmt.Errorf("nil TelemetryConnectionSettings")
	}
	if proto.DestinationEndpoint == "" {
		return Settings{}, fmt.Errorf("empty destination endpoint")
	}

	endpoint := proto.DestinationEndpoint
	var tlsServerName string

	// Extract ?tls_server_name=<value> from the endpoint URL. This allows
	// the OpAMP server to specify a TLS ServerName override (e.g. a cluster
	// ID) when the server certificate CN/SAN doesn't match the hostname.
	if u, err := url.Parse(endpoint); err == nil {
		if sn := u.Query().Get("tls_server_name"); sn != "" {
			tlsServerName = sn
			q := u.Query()
			q.Del("tls_server_name")
			u.RawQuery = q.Encode()
			endpoint = u.String()
		}
	}

	s := Settings{
		Endpoint: endpoint,
		Insecure: strings.HasPrefix(endpoint, "http://"),
	}

	// Convert headers
	if h := proto.GetHeaders(); h != nil && len(h.Headers) > 0 {
		s.Headers = make(map[string]string, len(h.Headers))
		for _, header := range h.Headers {
			s.Headers[header.Key] = header.Value
		}
	}

	// Build TLS config from Certificate and/or TLSConnectionSettings
	tlsCfg, err := buildTLSConfig(proto.GetCertificate(), proto.GetTls(), tlsServerName)
	if err != nil {
		return Settings{}, fmt.Errorf("build TLS config: %w", err)
	}
	if tlsCfg != nil {
		s.TLSConfig = tlsCfg
	}
	s.TLSServerName = tlsServerName

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
		s.IncludeSystemCACertsPool = tlsSettings.GetIncludeSystemCaCertsPool()
		if caPEM := tlsSettings.GetCaPemContents(); caPEM != "" {
			s.TLSCAPemContents = caPEM
		}
	}

	// Convert proxy settings
	if proxy := proto.GetProxy(); proxy != nil {
		if proxyURL := proxy.GetUrl(); proxyURL != "" {
			s.ProxyURL = proxyURL
		}
		if proxyHeaders := proxy.GetConnectHeaders(); proxyHeaders != nil && len(proxyHeaders.Headers) > 0 {
			s.ProxyHeaders = make(map[string]string, len(proxyHeaders.Headers))
			for _, header := range proxyHeaders.Headers {
				s.ProxyHeaders[header.Key] = header.Value
			}
		}
	}

	// LoadClientCert overwrites any client certificate from the proto with the
	// sidecar's signing cert/key for mTLS.
	if err := s.LoadClientCert(clientCertPath, clientKeyPath); err != nil {
		return Settings{}, fmt.Errorf("load client certificate: %w", err)
	}

	return s, nil
}

// LoadClientCert reads a client certificate and key from the given file paths
// and adds them to the TLS config for mTLS. The file contents are not stored
// in the settings, so they are not persisted.
func (s *Settings) LoadClientCert(certPath, keyPath string) error {
	if certPath == "" || keyPath == "" {
		return fmt.Errorf("client cert path and key path must not be empty")
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("read client cert %s: %w", certPath, err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read client key %s: %w", keyPath, err)
	}
	clientCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("parse client certificate: %w", err)
	}
	if s.TLSConfig == nil {
		s.TLSConfig = &tls.Config{}
	}
	s.TLSConfig.Certificates = []tls.Certificate{clientCert}
	return nil
}

func buildTLSConfig(cert *protobufs.TLSCertificate, tlsSettings *protobufs.TLSConnectionSettings, serverName string) (*tls.Config, error) {
	if cert == nil && tlsSettings == nil && serverName == "" {
		return nil, nil
	}

	cfg := &tls.Config{
		ServerName: serverName,
	}

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
			parsed, err := connection.ToTLSVersion(v)
			if err != nil {
				return nil, fmt.Errorf("parse TLS min version: %w", err)
			}
			cfg.MinVersion = parsed
		}
		if v := tlsSettings.GetMaxVersion(); v != "" {
			parsed, err := connection.ToTLSVersion(v)
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
