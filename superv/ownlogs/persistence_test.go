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
	"path/filepath"
	"testing"

	"github.com/Graylog2/collector-sidecar/superv/internal/testpki"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistence_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeClientCredentials(t, testpki.GenerateTestCert(t))
	p := NewPersistence(dir, certPath, keyPath)

	settings := Settings{
		Endpoint: "https://example.com:4318/v1/logs",
		Headers:  map[string]string{"Authorization": "Bearer tok"},
		Insecure: false,
	}

	err := p.Save(settings)
	require.NoError(t, err)

	loaded, exists, err := p.Load()
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, settings.Endpoint, loaded.Endpoint)
	assert.Equal(t, settings.Headers["Authorization"], loaded.Headers["Authorization"])
	// Client cert should be loaded from file paths
	require.NotNil(t, loaded.TLSConfig)
	assert.Len(t, loaded.TLSConfig.Certificates, 1)
}

func TestPersistence_SaveAndLoad_WithTLS(t *testing.T) {
	dir := t.TempDir()
	cert := testpki.GenerateTestCert(t)
	certPath, keyPath := writeClientCredentials(t, cert)
	p := NewPersistence(dir, certPath, keyPath)

	settings := Settings{
		Endpoint:           "https://example.com:4318/v1/logs",
		CertPEM:            cert.CertPEM,
		KeyPEM:             cert.KeyPEM,
		CACertPEM:          cert.CACertPEM,
		TLSMinVersion:      "1.3",
		InsecureSkipVerify: false,
	}

	err := p.Save(settings)
	require.NoError(t, err)

	loaded, exists, err := p.Load()
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, settings.CertPEM, loaded.CertPEM)
	assert.Equal(t, settings.KeyPEM, loaded.KeyPEM)
	assert.Equal(t, settings.CACertPEM, loaded.CACertPEM)
	assert.Equal(t, "1.3", loaded.TLSMinVersion)
	// TLSConfig should be rebuilt from persisted PEM material
	require.NotNil(t, loaded.TLSConfig)
	assert.NotNil(t, loaded.TLSConfig.RootCAs)
	assert.Len(t, loaded.TLSConfig.Certificates, 1)
}

func TestPersistence_SaveAndLoad_WithSystemCACertsPool(t *testing.T) {
	dir := t.TempDir()
	cert := testpki.GenerateTestCert(t)
	certPath, keyPath := writeClientCredentials(t, cert)
	p := NewPersistence(dir, certPath, keyPath)

	settings := Settings{
		Endpoint:                 "https://example.com:4318/v1/logs",
		CACertPEM:                cert.CACertPEM,
		IncludeSystemCACertsPool: true,
	}

	err := p.Save(settings)
	require.NoError(t, err)

	loaded, exists, err := p.Load()
	require.NoError(t, err)
	require.True(t, exists)
	assert.True(t, loaded.IncludeSystemCACertsPool)
	require.NotNil(t, loaded.TLSConfig)
	assert.NotNil(t, loaded.TLSConfig.RootCAs)
}

func TestPersistence_SaveAndLoad_WithDualCASources(t *testing.T) {
	dir := t.TempDir()
	cert1 := testpki.GenerateTestCert(t)
	certPath, keyPath := writeClientCredentials(t, cert1)
	p := NewPersistence(dir, certPath, keyPath)

	cert2 := testpki.GenerateTestCert(t) // second, distinct CA

	settings := Settings{
		Endpoint:         "https://example.com:4318/v1/logs",
		CACertPEM:        cert1.CACertPEM,
		TLSCAPemContents: string(cert2.CACertPEM),
	}

	err := p.Save(settings)
	require.NoError(t, err)

	loaded, exists, err := p.Load()
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, cert1.CACertPEM, loaded.CACertPEM)
	assert.Equal(t, string(cert2.CACertPEM), loaded.TLSCAPemContents)
	require.NotNil(t, loaded.TLSConfig)
	assert.NotNil(t, loaded.TLSConfig.RootCAs)
}

func TestPersistence_SaveAndLoad_WithTLSServerName(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeClientCredentials(t, testpki.GenerateTestCert(t))
	p := NewPersistence(dir, certPath, keyPath)

	settings := Settings{
		Endpoint:      "https://example.com:4318/v1/logs",
		TLSServerName: "my-cluster-id",
	}

	err := p.Save(settings)
	require.NoError(t, err)

	loaded, exists, err := p.Load()
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, "my-cluster-id", loaded.TLSServerName)
	require.NotNil(t, loaded.TLSConfig)
	assert.Equal(t, "my-cluster-id", loaded.TLSConfig.ServerName)
}

func TestPersistence_Delete(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeClientCredentials(t, testpki.GenerateTestCert(t))
	p := NewPersistence(dir, certPath, keyPath)

	// Save then delete
	err := p.Save(Settings{Endpoint: "https://example.com:4318/v1/logs"})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, ownLogsFileName))

	err = p.Delete()
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(dir, ownLogsFileName))

	// Load should report no file
	_, exists, err := p.Load()
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestPersistence_Delete_NoFile(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeClientCredentials(t, testpki.GenerateTestCert(t))
	p := NewPersistence(dir, certPath, keyPath)

	// Deleting when no file exists should not error
	err := p.Delete()
	require.NoError(t, err)
}

func TestPersistence_Load_NoFile(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeClientCredentials(t, testpki.GenerateTestCert(t))
	p := NewPersistence(dir, certPath, keyPath)

	_, exists, err := p.Load()
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestPersistence_SaveAndLoad_WithProxy(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeClientCredentials(t, testpki.GenerateTestCert(t))
	p := NewPersistence(dir, certPath, keyPath)

	settings := Settings{
		Endpoint:     "https://example.com:4318/v1/logs",
		ProxyURL:     "http://proxy:8080",
		ProxyHeaders: map[string]string{"Proxy-Authorization": "Basic abc"},
	}

	err := p.Save(settings)
	require.NoError(t, err)

	loaded, exists, err := p.Load()
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, "http://proxy:8080", loaded.ProxyURL)
	assert.Equal(t, "Basic abc", loaded.ProxyHeaders["Proxy-Authorization"])
}

func TestPersistence_FileLocation(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeClientCredentials(t, testpki.GenerateTestCert(t))
	p := NewPersistence(dir, certPath, keyPath)

	err := p.Save(Settings{Endpoint: "https://example.com:4318/v1/logs"})
	require.NoError(t, err)

	// File should exist at expected path
	assert.FileExists(t, filepath.Join(dir, ownLogsFileName))
}
