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

package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
)

// OIDEncryptionPublicKey is the OID for X25519 encryption public key extension.
// Using a private enterprise number arc for now (1.3.6.1.4.1.99999.1.1).
// This should be replaced with a proper OID once assigned.
var OIDEncryptionPublicKey = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1, 1}

// CreateCSR creates a Certificate Signing Request with the instance UID as CN
// and the X25519 encryption public key as a custom extension.
func CreateCSR(signingKey ed25519.PrivateKey, instanceUID string, encryptionPubKey []byte) ([]byte, error) {
	return createCSR(signingKey, instanceUID, "", encryptionPubKey)
}

// CreateCSRWithTenant creates a CSR with tenant ID in the Organization field.
func CreateCSRWithTenant(signingKey ed25519.PrivateKey, instanceUID, tenantID string, encryptionPubKey []byte) ([]byte, error) {
	return createCSR(signingKey, instanceUID, tenantID, encryptionPubKey)
}

// createCSR is the common implementation for CSR creation.
func createCSR(signingKey ed25519.PrivateKey, instanceUID, tenantID string, encryptionPubKey []byte) ([]byte, error) {
	subject := pkix.Name{
		CommonName: instanceUID,
	}
	if tenantID != "" {
		subject.Organization = []string{tenantID}
	}

	template := &x509.CertificateRequest{
		Subject:            subject,
		SignatureAlgorithm: x509.PureEd25519,
	}

	// Add encryption public key as extension if provided
	if len(encryptionPubKey) > 0 {
		template.ExtraExtensions = []pkix.Extension{
			{
				Id:       OIDEncryptionPublicKey,
				Critical: false,
				Value:    encryptionPubKey,
			},
		}
	}

	return x509.CreateCertificateRequest(rand.Reader, template, signingKey)
}

// ParseCSR parses a DER-encoded CSR.
func ParseCSR(csrDER []byte) (*x509.CertificateRequest, error) {
	return x509.ParseCertificateRequest(csrDER)
}

// EncodeCSRToPEM encodes a DER-encoded CSR to PEM format.
func EncodeCSRToPEM(csrDER []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})
}
