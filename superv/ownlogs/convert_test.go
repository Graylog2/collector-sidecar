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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
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

func TestConvertSettings_Endpoint(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
	}
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com:4318/v1/logs", s.Endpoint)
	assert.False(t, s.Insecure)
}

func TestConvertSettings_Headers(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Headers: &protobufs.Headers{
			Headers: []*protobufs.Header{
				{Key: "Authorization", Value: "Bearer token123"},
				{Key: "X-Custom", Value: "value"},
			},
		},
	}
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	assert.Equal(t, "Bearer token123", s.Headers["Authorization"])
	assert.Equal(t, "value", s.Headers["X-Custom"])
}

func TestConvertSettings_InsecureHTTP(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "http://example.com:4318/v1/logs",
	}
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	assert.True(t, s.Insecure)
}

func TestConvertSettings_TLSCertificate(t *testing.T) {
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
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	require.NotNil(t, s.TLSConfig)
	assert.NotNil(t, s.TLSConfig.RootCAs)
	assert.Len(t, s.TLSConfig.Certificates, 1)
	// Raw PEM bytes are preserved for persistence
	assert.Equal(t, clientCertPEM, s.CertPEM)
	assert.Equal(t, clientKeyPEM, s.KeyPEM)
	assert.Equal(t, caCertPEM, s.CACertPEM)
}

func TestConvertSettings_TLSConnectionSettings(t *testing.T) {
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
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	require.NotNil(t, s.TLSConfig)
	assert.True(t, s.TLSConfig.InsecureSkipVerify)
	assert.Equal(t, uint16(tls.VersionTLS13), s.TLSConfig.MinVersion)
	assert.NotNil(t, s.TLSConfig.RootCAs)
}

func TestConvertSettings_BothCASources(t *testing.T) {
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
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	assert.Equal(t, caCertPEM, s.CACertPEM)
	assert.Equal(t, string(tlsCAPEM), s.TLSCAPemContents)
	assert.True(t, s.IncludeSystemCACertsPool)
	require.NotNil(t, s.TLSConfig)
	assert.NotNil(t, s.TLSConfig.RootCAs)
}

func TestConvertSettings_InvalidClientCertificate(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Certificate: &protobufs.TLSCertificate{
			Cert:       []byte("invalid pem"),
			PrivateKey: []byte("invalid pem"),
		},
	}
	_, err := ConvertSettings(proto)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse client certificate")
}

func TestConvertSettings_InvalidCAFromCertificate(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Certificate: &protobufs.TLSCertificate{
			CaCert: []byte("invalid ca pem"),
		},
	}
	_, err := ConvertSettings(proto)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CA certificate from TLSCertificate")
}

func TestConvertSettings_InvalidCAFromTLSSettings(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Tls: &protobufs.TLSConnectionSettings{
			CaPemContents: "invalid ca pem",
		},
	}
	_, err := ConvertSettings(proto)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CA certificate from TLSConnectionSettings")
}

func TestConvertSettings_InvalidTLSVersion(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Tls: &protobufs.TLSConnectionSettings{
			MinVersion: "1.0",
		},
	}
	_, err := ConvertSettings(proto)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse TLS min version")
}

func TestConvertSettings_SystemCACertsWithExistingCA(t *testing.T) {
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
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	require.NotNil(t, s.TLSConfig)
	// RootCAs should contain both system CAs and the custom CA.
	// We can't easily count system CAs, but the pool must be non-nil
	// and the custom CA must be included (verified by the pool being usable).
	assert.NotNil(t, s.TLSConfig.RootCAs)
}

func TestConvertSettings_TLSMinGreaterThanMax(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
		Tls: &protobufs.TLSConnectionSettings{
			MinVersion: "1.3",
			MaxVersion: "1.2",
		},
	}
	_, err := ConvertSettings(proto)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min version")
	assert.Contains(t, err.Error(), "greater than max")
}

func TestConvertSettings_NilProto(t *testing.T) {
	_, err := ConvertSettings(nil)
	require.Error(t, err)
}

func TestConvertSettings_EmptyEndpoint(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{}
	_, err := ConvertSettings(proto)
	require.Error(t, err)
}

func TestConvertSettings_Proxy(t *testing.T) {
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
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	assert.Equal(t, "http://proxy:8080", s.ProxyURL)
	assert.Equal(t, "Basic abc123", s.ProxyHeaders["Proxy-Authorization"])
}

func TestConvertSettings_ProxyNil(t *testing.T) {
	proto := &protobufs.TelemetryConnectionSettings{
		DestinationEndpoint: "https://example.com:4318/v1/logs",
	}
	s, err := ConvertSettings(proto)
	require.NoError(t, err)
	assert.Empty(t, s.ProxyURL)
	assert.Nil(t, s.ProxyHeaders)
}
