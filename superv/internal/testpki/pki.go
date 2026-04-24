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

package testpki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type Cert struct {
	Key     *ecdsa.PrivateKey
	KeyDER  []byte
	KeyPEM  []byte
	Cert    *x509.Certificate
	CertDER []byte
	CertPEM []byte
}

func GenerateTestCA(t *testing.T) Cert {
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

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	return Cert{
		Key:     key,
		KeyDER:  keyDER,
		KeyPEM:  keyPEM,
		Cert:    cert,
		CertDER: certDER,
		CertPEM: certPEM,
	}
}

// GenerateTestCert creates a certificate signed by the given CA (or self-signed
// if caCertPEM/caKeyPEM are nil). Returns PEM-encoded certificate and private key.
func GenerateTestCert(t *testing.T, issuerCertPem, issuerKeyPem []byte) Cert {
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

	if issuerCertPem != nil && issuerKeyPem != nil {
		block, _ := pem.Decode(issuerCertPem)
		require.NotNil(t, block)
		parent, err = x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		keyBlock, _ := pem.Decode(issuerKeyPem)
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
	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return Cert{
		Key:     key,
		KeyDER:  keyDER,
		KeyPEM:  keyPEM,
		Cert:    cert,
		CertDER: certDER,
		CertPEM: certPEM,
	}
}
