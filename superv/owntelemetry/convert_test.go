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

package owntelemetry

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestCA creates a self-signed CA certificate and returns the PEM-encoded
// certificate and private key.
func generateTestCA(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

// generateTestCert creates a certificate signed by the given CA (or self-signed
// if caCertPEM/caKeyPEM are nil). Returns PEM-encoded certificate and private key.
func generateTestCert(t *testing.T, caCertPEM, caKeyPEM []byte) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Test Client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	// Determine signer
	var parent *x509.Certificate
	var signerKey *ecdsa.PrivateKey

	if caCertPEM != nil && caKeyPEM != nil {
		block, _ := pem.Decode(caCertPEM)
		require.NotNil(t, block)
		parent, err = x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		keyBlock, _ := pem.Decode(caKeyPEM)
		require.NotNil(t, keyBlock)
		signerKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
		require.NoError(t, err)
	} else {
		// Self-signed
		parent = tmpl
		signerKey = key
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, signerKey)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

// writeTestClientCert creates a temporary client cert/key pair and returns
// the file paths. Used by ConvertSettings tests which require valid cert paths.
func writeTestClientCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	caCertPEM, caKeyPEM := generateTestCA(t)
	clientCertPEM, clientKeyPEM := generateTestCert(t, caCertPEM, caKeyPEM)
	dir := t.TempDir()
	certPath = filepath.Join(dir, "client.crt")
	keyPath = filepath.Join(dir, "client.key")
	require.NoError(t, os.WriteFile(certPath, clientCertPEM, 0o600))
	require.NoError(t, os.WriteFile(keyPath, clientKeyPEM, 0o600))
	return certPath, keyPath
}

func TestConvertSettings_ExportInterval(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/metrics?export_interval=15s",
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Equal(t, 15*time.Second, s.ExportInterval)
	// Query param should be stripped from the endpoint
	assert.NotContains(t, s.Endpoint, "export_interval")
}

func TestConvertSettings_Endpoint(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com:4318/v1/logs", s.Endpoint)
	assert.False(t, s.Insecure)
}

func TestConvertSettings_Headers(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Headers: &protobufs.Headers{
			Headers: []*protobufs.Header{
				{Key: "Authorization", Value: "Bearer token123"},
				{Key: "X-Custom", Value: "value"},
			},
		},
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Equal(t, "Bearer token123", s.Headers["Authorization"])
	assert.Equal(t, "value", s.Headers["X-Custom"])
}

func TestConvertSettings_InsecureHTTP(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "http://example.com:4318/v1/logs",
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.True(t, s.Insecure)
}

func TestConvertSettings_TLSCertificate(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	caCertPEM, caKeyPEM := generateTestCA(t)
	clientCertPEM, clientKeyPEM := generateTestCert(t, caCertPEM, caKeyPEM)

	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Certificate: &protobufs.TLSCertificate{
			Cert:       clientCertPEM,
			PrivateKey: clientKeyPEM,
			CaCert:     caCertPEM,
		},
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	require.NotNil(t, s.TLSConfig)
	assert.NotNil(t, s.TLSConfig.RootCAs)
	// Client cert from file overrides the one from proto — but both are valid
	assert.Len(t, s.TLSConfig.Certificates, 1)
	// Raw PEM bytes are preserved for persistence
	assert.Equal(t, clientCertPEM, s.CertPEM)
	assert.Equal(t, clientKeyPEM, s.KeyPEM)
	assert.Equal(t, caCertPEM, s.CACertPEM)
}

func TestConvertSettings_TLSConnectionSettings(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	caCertPEM, _ := generateTestCA(t)

	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Tls: &protobufs.TLSConnectionSettings{
			CaPemContents:            string(caCertPEM),
			IncludeSystemCaCertsPool: true,
			InsecureSkipVerify:       true,
			MinVersion:               "1.3",
		},
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	require.NotNil(t, s.TLSConfig)
	assert.True(t, s.TLSConfig.InsecureSkipVerify)
	assert.Equal(t, uint16(tls.VersionTLS13), s.TLSConfig.MinVersion)
	assert.NotNil(t, s.TLSConfig.RootCAs)
}

func TestConvertSettings_BothCASources(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	caCertPEM, _ := generateTestCA(t)
	tlsCAPEM, _ := generateTestCA(t)

	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Certificate: &protobufs.TLSCertificate{
			CaCert: caCertPEM,
		},
		Tls: &protobufs.TLSConnectionSettings{
			CaPemContents:            string(tlsCAPEM),
			IncludeSystemCaCertsPool: true,
		},
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Equal(t, caCertPEM, s.CACertPEM)
	assert.Equal(t, string(tlsCAPEM), s.TLSCAPemContents)
	assert.True(t, s.IncludeSystemCACertsPool)
	require.NotNil(t, s.TLSConfig)
	assert.NotNil(t, s.TLSConfig.RootCAs)
}

func TestConvertSettings_InvalidClientCertificate(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Certificate: &protobufs.TLSCertificate{
			Cert:       []byte("invalid pem"),
			PrivateKey: []byte("invalid pem"),
		},
	}
	_, err := ConvertSettings(proto, certPath, keyPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse client certificate")
}

func TestConvertSettings_InvalidCAFromCertificate(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Certificate: &protobufs.TLSCertificate{
			CaCert: []byte("invalid ca pem"),
		},
	}
	_, err := ConvertSettings(proto, certPath, keyPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CA certificate from TLSCertificate")
}

func TestConvertSettings_InvalidCAFromTLSSettings(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Tls: &protobufs.TLSConnectionSettings{
			CaPemContents: "invalid ca pem",
		},
	}
	_, err := ConvertSettings(proto, certPath, keyPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CA certificate from TLSConnectionSettings")
}

func TestConvertSettings_InvalidTLSVersion(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Tls: &protobufs.TLSConnectionSettings{
			MinVersion: "1.0",
		},
	}
	_, err := ConvertSettings(proto, certPath, keyPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse TLS min version")
}

func TestConvertSettings_SystemCACertsWithExistingCA(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	caCertPEM, _ := generateTestCA(t)

	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Certificate: &protobufs.TLSCertificate{
			CaCert: caCertPEM,
		},
		Tls: &protobufs.TLSConnectionSettings{
			IncludeSystemCaCertsPool: true,
		},
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	require.NotNil(t, s.TLSConfig)
	assert.NotNil(t, s.TLSConfig.RootCAs)
}

func TestConvertSettings_TLSMinGreaterThanMax(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Tls: &protobufs.TLSConnectionSettings{
			MinVersion: "1.3",
			MaxVersion: "1.2",
		},
	}
	_, err := ConvertSettings(proto, certPath, keyPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min version")
	assert.Contains(t, err.Error(), "greater than max")
}

func TestConvertSettings_NilProto(t *testing.T) {
	_, err := ConvertSettings(nil, "", "")
	require.Error(t, err)
}

func TestConvertSettings_EmptyEndpoint(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{}
	_, err := ConvertSettings(proto, "", "")
	require.Error(t, err)
}

func TestConvertSettings_MissingClientCert(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
	}
	_, err := ConvertSettings(proto, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load client certificate")
}

func TestConvertSettings_TLSServerName(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/?tls_server_name=my-cluster-id",
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com:4318/", s.Endpoint)
	assert.Equal(t, "my-cluster-id", s.TLSServerName)
	require.NotNil(t, s.TLSConfig)
	assert.Equal(t, "my-cluster-id", s.TLSConfig.ServerName)
}

func TestConvertSettings_TLSServerNameWithPath(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs?tls_server_name=my-cluster-id",
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com:4318/v1/logs", s.Endpoint)
	assert.Equal(t, "my-cluster-id", s.TLSServerName)
}

func TestConvertSettings_TLSServerNameWithOtherParams(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs?foo=bar&tls_server_name=cluster-42&baz=qux",
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Equal(t, "cluster-42", s.TLSServerName)
	assert.NotContains(t, s.Endpoint, "tls_server_name")
	assert.Contains(t, s.Endpoint, "foo=bar")
	assert.Contains(t, s.Endpoint, "baz=qux")
}

func TestConvertSettings_NoTLSServerName(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com:4318/v1/logs", s.Endpoint)
	assert.Empty(t, s.TLSServerName)
}

func TestConvertSettings_LogLevel(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs?log_level=debug",
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com:4318/v1/logs", s.Endpoint)
	assert.Equal(t, "debug", s.LogLevel)
}

func TestConvertSettings_LogLevelWithOtherParams(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs?foo=bar&log_level=warn&baz=qux",
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Equal(t, "warn", s.LogLevel)
	assert.NotContains(t, s.Endpoint, "log_level")
	assert.Contains(t, s.Endpoint, "foo=bar")
	assert.Contains(t, s.Endpoint, "baz=qux")
}

func TestConvertSettings_NoLogLevel(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Empty(t, s.LogLevel)
}

func TestLoadClientCert(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)

	s := Settings{Endpoint: "https://example.com:4318/v1/logs"}
	require.NoError(t, s.LoadClientCert(certPath, keyPath))
	require.NotNil(t, s.TLSConfig)
	assert.Len(t, s.TLSConfig.Certificates, 1)
	// PEM bytes should NOT be stored
	assert.Empty(t, s.CertPEM)
	assert.Empty(t, s.KeyPEM)
}

func TestLoadClientCert_PreservesTLSConfig(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)

	s := Settings{
		Endpoint:  "https://example.com:4318/",
		TLSConfig: &tls.Config{ServerName: "my-cluster"},
	}
	require.NoError(t, s.LoadClientCert(certPath, keyPath))
	assert.Equal(t, "my-cluster", s.TLSConfig.ServerName)
	assert.Len(t, s.TLSConfig.Certificates, 1)
}

func TestLoadClientCert_EmptyPaths(t *testing.T) {
	s := Settings{}
	err := s.LoadClientCert("", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestLoadClientCert_MissingFiles(t *testing.T) {
	s := Settings{}
	err := s.LoadClientCert("/nonexistent/cert.pem", "/nonexistent/key.pem")
	require.Error(t, err)
}

func TestConvertSettings_Proxy(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Proxy: &protobufs.ProxyConnectionSettings{
			Url: "http://proxy:8080",
			ConnectHeaders: &protobufs.Headers{
				Headers: []*protobufs.Header{
					{Key: "Proxy-Authorization", Value: "Basic abc123"},
				},
			},
		},
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Equal(t, "http://proxy:8080", s.ProxyURL)
	assert.Equal(t, "Basic abc123", s.ProxyHeaders["Proxy-Authorization"])
}

func TestConvertSettings_ProxyNil(t *testing.T) {
	certPath, keyPath := writeTestClientCert(t)
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
	}
	s, err := ConvertSettings(proto, certPath, keyPath)
	require.NoError(t, err)
	assert.Empty(t, s.ProxyURL)
	assert.Nil(t, s.ProxyHeaders)
}
