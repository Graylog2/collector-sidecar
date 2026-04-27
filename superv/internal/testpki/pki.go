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
	"crypto/ed25519"
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
	Key       ed25519.PrivateKey
	KeyPEM    []byte
	Cert      *x509.Certificate
	CertDER   []byte
	CertPEM   []byte
	CACertPEM []byte
}

type certOptions struct {
	NotBefore  *time.Time
	NotAfter   *time.Time
	Issuer     *Cert
	PrivateKey ed25519.PrivateKey
	CSR        *x509.CertificateRequest
	Serial     *int64
	Subject    *string
	Seed       []byte
}

type CertOption func(options *certOptions)

func WithNotBefore(notBefore time.Time) CertOption {
	return func(options *certOptions) {
		options.NotBefore = &notBefore
	}
}

func WithNotAfter(notAfter time.Time) CertOption {
	return func(options *certOptions) {
		options.NotAfter = &notAfter
	}
}

func WithIssuer(issuer Cert) CertOption {
	return func(options *certOptions) {
		options.Issuer = &issuer
	}
}

func WithPrivateKey(priv ed25519.PrivateKey) CertOption {
	return func(options *certOptions) {
		options.PrivateKey = priv
	}
}

func WithCSR(csr *x509.CertificateRequest) CertOption {
	return func(options *certOptions) {
		options.CSR = csr
	}
}

func WithSerial(serial int) CertOption {
	return func(options *certOptions) {
		options.Serial = new(int64(serial))
	}
}

func WithSubject(subject string) CertOption {
	return func(options *certOptions) {
		options.Subject = &subject
	}
}

func WithSeed(seed []byte) CertOption {
	return func(options *certOptions) {
		options.Seed = seed
	}
}

func GenerateTestCA(t *testing.T, opts ...CertOption) Cert {
	t.Helper()
	return generateCert(t, opts, &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	})
}

// GenerateTestCert creates a certificate signed by the given CA (or self-signed
// if caCertPEM/caKeyPEM are nil). Returns PEM-encoded certificate and private key.
func GenerateTestCert(t *testing.T, opts ...CertOption) Cert {
	t.Helper()
	return generateCert(t, opts, &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Test Client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
}

func generateCert(t *testing.T, o []CertOption, tmpl *x509.Certificate) Cert {
	t.Helper()
	opts := &certOptions{}
	for _, option := range o {
		option(opts)
	}

	if opts.NotBefore != nil {
		tmpl.NotBefore = *opts.NotBefore
	}
	if opts.NotAfter != nil {
		tmpl.NotAfter = *opts.NotAfter
	}
	if opts.Serial != nil {
		tmpl.SerialNumber = big.NewInt(*opts.Serial)
	}
	if opts.Subject != nil {
		tmpl.Subject = pkix.Name{CommonName: *opts.Subject}
	}

	if opts.PrivateKey != nil && opts.Seed != nil {
		t.Fatalf("Only one of PrivateKey or Seed CertOption can be used")
	}

	var key ed25519.PrivateKey
	switch {
	case opts.PrivateKey != nil:
		key = opts.PrivateKey
	case opts.Seed != nil:
		key = ed25519.NewKeyFromSeed(opts.Seed)
	default:
		_, privKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		key = privKey
	}

	var pubKey any
	if opts.CSR != nil {
		pubKey = opts.CSR.PublicKey
	} else {
		pubKey = key.Public()
	}

	// Determine signer
	issuer := opts.Issuer
	var issuerCert *x509.Certificate
	var issuerKey ed25519.PrivateKey

	if issuer != nil {
		issuerCert = issuer.Cert
		issuerKey = issuer.Key
	} else {
		// Self-signed
		issuerCert = tmpl
		issuerKey = key
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, issuerCert, pubKey, issuerKey)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})

	var caCertPEM []byte
	if issuer != nil {
		caCertPEM = issuer.CertPEM
	} else {
		caCertPEM = certPEM
	}

	return Cert{
		Key:       key,
		KeyPEM:    keyPEM,
		Cert:      cert,
		CertDER:   certDER,
		CertPEM:   certPEM,
		CACertPEM: caCertPEM,
	}
}
