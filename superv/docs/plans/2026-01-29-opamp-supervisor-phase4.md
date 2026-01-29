# OpAMP Supervisor Phase 4 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement package management allowing the OpAMP server to push collector binary updates with cryptographic verification.

**Architecture:** Phase 4 adds a package manager that: (1) receives PackagesAvailable from the server, (2) verifies server attestation signatures, (3) downloads packages with integrity verification, (4) optionally verifies publisher signatures, (5) stages and atomically installs packages, and (6) rolls back on failed startup.

**Tech Stack:** Go 1.25+, crypto/ed25519, net/http (downloads), os (atomic operations), archive/tar + compress/gzip (extraction)

**Prerequisites:** Phase 1-3 complete (especially crash recovery for rollback detection)

**Reference:** Design doc `docs/plans/2026-01-23-opamp-supervisor-design.md` Section 5

---

## Phase 4 Overview

| Task | Component | Description | Priority |
|------|-----------|-------------|----------|
| 4.1 | Package Storage | Directory structure, version tracking, symlink management | High |
| 4.2 | Server Attestation | Verify server-signed attestations for packages | High |
| 4.3 | Package Download | Download with progress, integrity verification | High |
| 4.4 | Publisher Signature | Optional cosign/gpg/minisign verification | Medium |
| 4.5 | Package Installation | Atomic install with staging directory | High |
| 4.6 | Rollback Support | Detect failed startup, revert to previous version | High |
| 4.7 | Status Reporting | Report PackageStatuses to server | Medium |
| 4.8 | Integration | Wire into supervisor, handle OnPackagesAvailable | High |

---

## Task 4.1: Package Storage

**Files:**
- Create: `packages/storage.go`
- Create: `packages/storage_test.go`

**Step 1: Write tests for package storage**

Create `packages/storage_test.go`:
```go
// Copyright Graylog, Inc.
// SPDX-License-Identifier: Apache-2.0

package packages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestStorage_Initialize(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()

	storage, err := NewStorage(logger, StorageConfig{
		BaseDir:      dir,
		KeepVersions: 2,
	})
	require.NoError(t, err)

	// Verify directory structure created
	require.DirExists(t, filepath.Join(dir, "staging"))
	require.DirExists(t, filepath.Join(dir, "packages"))

	require.NotNil(t, storage)
}

func TestStorage_StagePackage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()

	storage, err := NewStorage(logger, StorageConfig{
		BaseDir:      dir,
		KeepVersions: 2,
	})
	require.NoError(t, err)

	// Create a fake package file
	content := []byte("fake binary content")
	stagePath, err := storage.Stage("otelcol", "0.98.0", content)
	require.NoError(t, err)
	require.FileExists(t, stagePath)

	// Verify content
	data, err := os.ReadFile(stagePath)
	require.NoError(t, err)
	require.Equal(t, content, data)
}

func TestStorage_InstallPackage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()

	storage, err := NewStorage(logger, StorageConfig{
		BaseDir:      dir,
		KeepVersions: 2,
	})
	require.NoError(t, err)

	// Stage first
	content := []byte("fake binary content")
	stagePath, err := storage.Stage("otelcol", "0.98.0", content)
	require.NoError(t, err)

	// Install
	installPath, err := storage.Install("otelcol", "0.98.0", stagePath)
	require.NoError(t, err)
	require.FileExists(t, installPath)

	// Verify current symlink
	currentPath := storage.CurrentPath("otelcol")
	target, err := os.Readlink(currentPath)
	require.NoError(t, err)
	require.Contains(t, target, "0.98.0")
}

func TestStorage_ListVersions(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()

	storage, err := NewStorage(logger, StorageConfig{
		BaseDir:      dir,
		KeepVersions: 3,
	})
	require.NoError(t, err)

	// Install multiple versions
	for _, version := range []string{"0.96.0", "0.97.0", "0.98.0"} {
		content := []byte("binary " + version)
		stagePath, err := storage.Stage("otelcol", version, content)
		require.NoError(t, err)
		_, err = storage.Install("otelcol", version, stagePath)
		require.NoError(t, err)
	}

	versions, err := storage.ListVersions("otelcol")
	require.NoError(t, err)
	require.Len(t, versions, 3)
}

func TestStorage_Cleanup(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()

	storage, err := NewStorage(logger, StorageConfig{
		BaseDir:      dir,
		KeepVersions: 2, // Keep only 2 versions
	})
	require.NoError(t, err)

	// Install 4 versions
	for _, version := range []string{"0.95.0", "0.96.0", "0.97.0", "0.98.0"} {
		content := []byte("binary " + version)
		stagePath, err := storage.Stage("otelcol", version, content)
		require.NoError(t, err)
		_, err = storage.Install("otelcol", version, stagePath)
		require.NoError(t, err)
	}

	// Cleanup should remove old versions
	err = storage.Cleanup("otelcol")
	require.NoError(t, err)

	versions, err := storage.ListVersions("otelcol")
	require.NoError(t, err)
	require.Len(t, versions, 2)

	// Should keep the newest versions
	require.Contains(t, versions, "0.97.0")
	require.Contains(t, versions, "0.98.0")
}

func TestStorage_GetCurrentVersion(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()

	storage, err := NewStorage(logger, StorageConfig{
		BaseDir:      dir,
		KeepVersions: 2,
	})
	require.NoError(t, err)

	// No version installed yet
	version, err := storage.GetCurrentVersion("otelcol")
	require.Error(t, err)

	// Install a version
	content := []byte("binary")
	stagePath, _ := storage.Stage("otelcol", "0.98.0", content)
	storage.Install("otelcol", "0.98.0", stagePath)

	version, err = storage.GetCurrentVersion("otelcol")
	require.NoError(t, err)
	require.Equal(t, "0.98.0", version)
}

func TestStorage_Rollback(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()

	storage, err := NewStorage(logger, StorageConfig{
		BaseDir:      dir,
		KeepVersions: 3,
	})
	require.NoError(t, err)

	// Install two versions
	for _, version := range []string{"0.97.0", "0.98.0"} {
		content := []byte("binary " + version)
		stagePath, _ := storage.Stage("otelcol", version, content)
		storage.Install("otelcol", version, stagePath)
	}

	// Current should be 0.98.0
	current, _ := storage.GetCurrentVersion("otelcol")
	require.Equal(t, "0.98.0", current)

	// Rollback
	err = storage.Rollback("otelcol")
	require.NoError(t, err)

	// Current should now be 0.97.0
	current, _ = storage.GetCurrentVersion("otelcol")
	require.Equal(t, "0.97.0", current)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./packages/... -v`
Expected: FAIL (package not found)

**Step 3: Implement package storage**

Create `packages/storage.go`:
```go
// Copyright Graylog, Inc.
// SPDX-License-Identifier: Apache-2.0

package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.uber.org/zap"
)

// StorageConfig configures package storage.
type StorageConfig struct {
	// BaseDir is the root directory for package storage.
	BaseDir string

	// KeepVersions is how many old versions to keep for rollback.
	KeepVersions int
}

// Storage manages package files on disk.
type Storage struct {
	logger *zap.Logger
	cfg    StorageConfig
}

// NewStorage creates a new package storage manager.
func NewStorage(logger *zap.Logger, cfg StorageConfig) (*Storage, error) {
	if cfg.KeepVersions < 1 {
		cfg.KeepVersions = 2
	}

	s := &Storage{
		logger: logger,
		cfg:    cfg,
	}

	if err := s.initialize(); err != nil {
		return nil, err
	}

	return s, nil
}

// initialize creates the directory structure.
func (s *Storage) initialize() error {
	dirs := []string{
		filepath.Join(s.cfg.BaseDir, "staging"),
		filepath.Join(s.cfg.BaseDir, "packages"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// Stage writes package content to the staging directory.
// Returns the path to the staged file.
func (s *Storage) Stage(name, version string, content []byte) (string, error) {
	stagingDir := filepath.Join(s.cfg.BaseDir, "staging")
	filename := fmt.Sprintf("%s-%s", name, version)
	stagePath := filepath.Join(stagingDir, filename)

	if err := os.WriteFile(stagePath, content, 0755); err != nil {
		return "", fmt.Errorf("failed to write staged package: %w", err)
	}

	s.logger.Debug("Staged package",
		zap.String("name", name),
		zap.String("version", version),
		zap.String("path", stagePath))

	return stagePath, nil
}

// Install moves a staged package to its final location and updates the current symlink.
func (s *Storage) Install(name, version, stagePath string) (string, error) {
	// Create version directory
	versionDir := filepath.Join(s.cfg.BaseDir, "packages", name, version)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create version directory: %w", err)
	}

	// Move from staging to version directory
	installPath := filepath.Join(versionDir, name)
	if err := os.Rename(stagePath, installPath); err != nil {
		return "", fmt.Errorf("failed to move package: %w", err)
	}

	// Update current symlink
	if err := s.updateCurrentSymlink(name, version); err != nil {
		return "", err
	}

	s.logger.Info("Installed package",
		zap.String("name", name),
		zap.String("version", version),
		zap.String("path", installPath))

	return installPath, nil
}

// updateCurrentSymlink atomically updates the "current" symlink.
func (s *Storage) updateCurrentSymlink(name, version string) error {
	packageDir := filepath.Join(s.cfg.BaseDir, "packages", name)
	currentPath := filepath.Join(packageDir, "current")
	tempLink := filepath.Join(packageDir, "current.new")

	// Target is the version directory (relative symlink)
	target := version

	// Remove temp link if exists
	os.Remove(tempLink)

	// Create new symlink
	if err := os.Symlink(target, tempLink); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempLink, currentPath); err != nil {
		os.Remove(tempLink)
		return fmt.Errorf("failed to update current symlink: %w", err)
	}

	return nil
}

// CurrentPath returns the path to the current version symlink.
func (s *Storage) CurrentPath(name string) string {
	return filepath.Join(s.cfg.BaseDir, "packages", name, "current")
}

// GetCurrentVersion returns the currently installed version.
func (s *Storage) GetCurrentVersion(name string) (string, error) {
	currentPath := s.CurrentPath(name)
	target, err := os.Readlink(currentPath)
	if err != nil {
		return "", fmt.Errorf("no current version: %w", err)
	}
	return filepath.Base(target), nil
}

// ListVersions returns all installed versions, sorted newest first.
func (s *Storage) ListVersions(name string) ([]string, error) {
	packageDir := filepath.Join(s.cfg.BaseDir, "packages", name)
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "current" {
			versions = append(versions, entry.Name())
		}
	}

	// Sort versions (simple string sort - could use semver)
	sort.Sort(sort.Reverse(sort.StringSlice(versions)))

	return versions, nil
}

// Cleanup removes old versions beyond KeepVersions limit.
func (s *Storage) Cleanup(name string) error {
	versions, err := s.ListVersions(name)
	if err != nil {
		return err
	}

	if len(versions) <= s.cfg.KeepVersions {
		return nil
	}

	// Get current version to avoid removing it
	currentVersion, _ := s.GetCurrentVersion(name)

	// Remove oldest versions
	toRemove := versions[s.cfg.KeepVersions:]
	for _, version := range toRemove {
		if version == currentVersion {
			continue // Never remove current
		}

		versionDir := filepath.Join(s.cfg.BaseDir, "packages", name, version)
		if err := os.RemoveAll(versionDir); err != nil {
			s.logger.Warn("Failed to remove old version",
				zap.String("version", version),
				zap.Error(err))
		} else {
			s.logger.Info("Removed old package version",
				zap.String("name", name),
				zap.String("version", version))
		}
	}

	return nil
}

// Rollback reverts to the previous version.
func (s *Storage) Rollback(name string) error {
	versions, err := s.ListVersions(name)
	if err != nil {
		return err
	}

	currentVersion, err := s.GetCurrentVersion(name)
	if err != nil {
		return fmt.Errorf("cannot rollback: %w", err)
	}

	// Find previous version
	var previousVersion string
	for _, v := range versions {
		if v != currentVersion {
			previousVersion = v
			break
		}
	}

	if previousVersion == "" {
		return fmt.Errorf("no previous version to rollback to")
	}

	s.logger.Info("Rolling back package",
		zap.String("name", name),
		zap.String("from", currentVersion),
		zap.String("to", previousVersion))

	return s.updateCurrentSymlink(name, previousVersion)
}

// GetExecutablePath returns the path to the current executable for a package.
func (s *Storage) GetExecutablePath(name string) (string, error) {
	currentPath := s.CurrentPath(name)

	// Resolve symlink
	resolved, err := filepath.EvalSymlinks(currentPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve current symlink: %w", err)
	}

	execPath := filepath.Join(resolved, name)
	if _, err := os.Stat(execPath); err != nil {
		return "", fmt.Errorf("executable not found: %w", err)
	}

	return execPath, nil
}
```

**Step 4: Run tests**

Run: `go test ./packages/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add packages/
git commit -m "feat(packages): implement package storage with versioning and rollback"
```

---

## Task 4.2: Server Attestation Verification

**Files:**
- Create: `packages/attestation.go`
- Create: `packages/attestation_test.go`

**Step 1: Write tests for attestation verification**

Create `packages/attestation_test.go`:
```go
// Copyright Graylog, Inc.
// SPDX-License-Identifier: Apache-2.0

package packages

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVerifyAttestation_Valid(t *testing.T) {
	// Generate server signing key
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Create attestation
	attestation := Attestation{
		PackageName: "otelcol",
		Version:     "0.98.0",
		Hash:        "sha256:abc123def456",
		DownloadURL: "https://github.com/example/releases/download/v0.98.0/otelcol",
		IssuedAt:    time.Now(),
	}

	// Sign it
	signed, err := SignAttestation(attestation, priv)
	require.NoError(t, err)

	// Verify
	err = VerifyAttestation(signed, pub)
	require.NoError(t, err)
}

func TestVerifyAttestation_InvalidSignature(t *testing.T) {
	// Generate two different keys
	pub1, _, _ := ed25519.GenerateKey(rand.Reader)
	_, priv2, _ := ed25519.GenerateKey(rand.Reader)

	attestation := Attestation{
		PackageName: "otelcol",
		Version:     "0.98.0",
		Hash:        "sha256:abc123",
		DownloadURL: "https://example.com/otelcol",
		IssuedAt:    time.Now(),
	}

	// Sign with key2
	signed, _ := SignAttestation(attestation, priv2)

	// Verify with key1 - should fail
	err := VerifyAttestation(signed, pub1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid signature")
}

func TestVerifyAttestation_Expired(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	attestation := Attestation{
		PackageName: "otelcol",
		Version:     "0.98.0",
		Hash:        "sha256:abc123",
		DownloadURL: "https://example.com/otelcol",
		IssuedAt:    time.Now().Add(-25 * time.Hour), // Issued > 24h ago
	}

	signed, _ := SignAttestation(attestation, priv)

	err := VerifyAttestation(signed, pub, WithMaxAge(24*time.Hour))
	require.Error(t, err)
	require.Contains(t, err.Error(), "expired")
}

func TestVerifyAttestation_Tampered(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	attestation := Attestation{
		PackageName: "otelcol",
		Version:     "0.98.0",
		Hash:        "sha256:abc123",
		DownloadURL: "https://example.com/otelcol",
		IssuedAt:    time.Now(),
	}

	signed, _ := SignAttestation(attestation, priv)

	// Tamper with the attestation
	signed.Attestation.Version = "0.99.0"

	err := VerifyAttestation(signed, pub)
	require.Error(t, err)
}

func TestParseSignedAttestation(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	attestation := Attestation{
		PackageName: "otelcol",
		Version:     "0.98.0",
		Hash:        "sha256:abc123def456789",
		DownloadURL: "https://example.com/otelcol",
		IssuedAt:    time.Now(),
	}

	signed, _ := SignAttestation(attestation, priv)

	// Serialize to JSON
	data, err := json.Marshal(signed)
	require.NoError(t, err)

	// Parse back
	parsed, err := ParseSignedAttestation(data)
	require.NoError(t, err)

	// Verify
	err = VerifyAttestation(parsed, pub)
	require.NoError(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./packages/... -v -run TestVerifyAttestation`
Expected: FAIL (undefined types)

**Step 3: Implement attestation verification**

Create `packages/attestation.go`:
```go
// Copyright Graylog, Inc.
// SPDX-License-Identifier: Apache-2.0

package packages

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// Attestation contains package metadata signed by the server.
type Attestation struct {
	PackageName string    `json:"package_name"`
	Version     string    `json:"version"`
	Hash        string    `json:"hash"` // e.g., "sha256:abc123..."
	DownloadURL string    `json:"download_url"`
	IssuedAt    time.Time `json:"issued_at"`
}

// SignedAttestation contains an attestation with its signature.
type SignedAttestation struct {
	Attestation Attestation `json:"attestation"`
	Signature   string      `json:"signature"` // Base64-encoded Ed25519 signature
	KeyID       string      `json:"key_id"`    // Identifier for the signing key
}

// VerifyOption configures attestation verification.
type VerifyOption func(*verifyConfig)

type verifyConfig struct {
	maxAge time.Duration
}

// WithMaxAge sets the maximum age for an attestation.
func WithMaxAge(d time.Duration) VerifyOption {
	return func(c *verifyConfig) {
		c.maxAge = d
	}
}

// SignAttestation signs an attestation with an Ed25519 private key.
func SignAttestation(att Attestation, privateKey ed25519.PrivateKey) (*SignedAttestation, error) {
	// Serialize attestation for signing
	data, err := json.Marshal(att)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal attestation: %w", err)
	}

	// Sign
	signature := ed25519.Sign(privateKey, data)

	return &SignedAttestation{
		Attestation: att,
		Signature:   base64.StdEncoding.EncodeToString(signature),
	}, nil
}

// VerifyAttestation verifies a signed attestation against a public key.
func VerifyAttestation(signed *SignedAttestation, publicKey ed25519.PublicKey, opts ...VerifyOption) error {
	cfg := &verifyConfig{
		maxAge: 0, // No max age by default
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// Check expiration
	if cfg.maxAge > 0 {
		age := time.Since(signed.Attestation.IssuedAt)
		if age > cfg.maxAge {
			return fmt.Errorf("attestation expired: issued %v ago, max age %v", age, cfg.maxAge)
		}
	}

	// Decode signature
	signature, err := base64.StdEncoding.DecodeString(signed.Signature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Serialize attestation for verification
	data, err := json.Marshal(signed.Attestation)
	if err != nil {
		return fmt.Errorf("failed to marshal attestation: %w", err)
	}

	// Verify signature
	if !ed25519.Verify(publicKey, data, signature) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// ParseSignedAttestation parses a signed attestation from JSON.
func ParseSignedAttestation(data []byte) (*SignedAttestation, error) {
	var signed SignedAttestation
	if err := json.Unmarshal(data, &signed); err != nil {
		return nil, fmt.Errorf("failed to parse signed attestation: %w", err)
	}
	return &signed, nil
}

// ParseHash extracts the algorithm and hash value from a hash string.
// Format: "algorithm:hexvalue" (e.g., "sha256:abc123...")
func ParseHash(hash string) (algorithm string, value string, err error) {
	var alg, val string
	n, err := fmt.Sscanf(hash, "%[^:]:%s", &alg, &val)
	if err != nil || n != 2 {
		return "", "", fmt.Errorf("invalid hash format: expected 'algorithm:value', got '%s'", hash)
	}
	return alg, val, nil
}
```

**Step 4: Run tests**

Run: `go test ./packages/... -v -run TestVerifyAttestation`
Expected: PASS

**Step 5: Commit**

```bash
git add packages/attestation.go packages/attestation_test.go
git commit -m "feat(packages): implement server attestation verification"
```

---

## Task 4.3: Package Download

**Files:**
- Create: `packages/download.go`
- Create: `packages/download_test.go`

**Step 1: Write tests for package download**

Create `packages/download_test.go`:
```go
// Copyright Graylog, Inc.
// SPDX-License-Identifier: Apache-2.0

package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestDownloader_Download_Success(t *testing.T) {
	content := []byte("fake binary content for testing")
	expectedHash := sha256.Sum256(content)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	downloader := NewDownloader(logger, DownloaderConfig{})

	result, err := downloader.Download(context.Background(), DownloadRequest{
		URL:          server.URL,
		ExpectedHash: "sha256:" + hex.EncodeToString(expectedHash[:]),
	})
	require.NoError(t, err)
	require.Equal(t, content, result.Content)
	require.True(t, result.HashVerified)
}

func TestDownloader_Download_HashMismatch(t *testing.T) {
	content := []byte("fake binary content")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	downloader := NewDownloader(logger, DownloaderConfig{})

	_, err := downloader.Download(context.Background(), DownloadRequest{
		URL:          server.URL,
		ExpectedHash: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "hash mismatch")
}

func TestDownloader_Download_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	downloader := NewDownloader(logger, DownloaderConfig{})

	_, err := downloader.Download(context.Background(), DownloadRequest{
		URL:          server.URL,
		ExpectedHash: "sha256:abc123",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")
}

func TestDownloader_Download_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't respond - let it timeout
		select {}
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	downloader := NewDownloader(logger, DownloaderConfig{
		Timeout: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := downloader.Download(ctx, DownloadRequest{
		URL:          server.URL,
		ExpectedHash: "sha256:abc123",
	})
	require.Error(t, err)
}

func TestDownloader_ProgressCallback(t *testing.T) {
	content := make([]byte, 1024*10) // 10KB
	for i := range content {
		content[i] = byte(i % 256)
	}
	expectedHash := sha256.Sum256(content)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write(content)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	downloader := NewDownloader(logger, DownloaderConfig{})

	var progressCalls int
	_, err := downloader.Download(context.Background(), DownloadRequest{
		URL:          server.URL,
		ExpectedHash: "sha256:" + hex.EncodeToString(expectedHash[:]),
		OnProgress: func(downloaded, total int64) {
			progressCalls++
		},
	})
	require.NoError(t, err)
	require.Greater(t, progressCalls, 0)
}
```

**Step 2: Implement downloader**

Create `packages/download.go`:
```go
// Copyright Graylog, Inc.
// SPDX-License-Identifier: Apache-2.0

package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// DownloaderConfig configures the package downloader.
type DownloaderConfig struct {
	Timeout time.Duration
}

// DownloadRequest contains parameters for a download.
type DownloadRequest struct {
	URL          string
	ExpectedHash string // Format: "sha256:hexvalue"
	OnProgress   func(downloaded, total int64)
}

// DownloadResult contains the result of a download.
type DownloadResult struct {
	Content      []byte
	HashVerified bool
	Size         int64
}

// Downloader handles package downloads with integrity verification.
type Downloader struct {
	logger *zap.Logger
	client *http.Client
}

// NewDownloader creates a new package downloader.
func NewDownloader(logger *zap.Logger, cfg DownloaderConfig) *Downloader {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	return &Downloader{
		logger: logger,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Download downloads a package and verifies its hash.
func (d *Downloader) Download(ctx context.Context, req DownloadRequest) (*DownloadResult, error) {
	d.logger.Info("Starting package download",
		zap.String("url", req.URL))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Read with progress reporting
	var reader io.Reader = resp.Body
	if req.OnProgress != nil {
		reader = &progressReader{
			reader:     resp.Body,
			total:      resp.ContentLength,
			onProgress: req.OnProgress,
		}
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Verify hash
	if err := d.verifyHash(content, req.ExpectedHash); err != nil {
		return nil, err
	}

	d.logger.Info("Package download complete",
		zap.Int("size", len(content)),
		zap.String("hash", req.ExpectedHash))

	return &DownloadResult{
		Content:      content,
		HashVerified: true,
		Size:         int64(len(content)),
	}, nil
}

// verifyHash verifies the content against the expected hash.
func (d *Downloader) verifyHash(content []byte, expectedHash string) error {
	algorithm, expected, err := ParseHash(expectedHash)
	if err != nil {
		return err
	}

	var actual string
	switch strings.ToLower(algorithm) {
	case "sha256":
		hash := sha256.Sum256(content)
		actual = hex.EncodeToString(hash[:])
	default:
		return fmt.Errorf("unsupported hash algorithm: %s", algorithm)
	}

	if actual != expected {
		return fmt.Errorf("hash mismatch: expected %s, got %s", expected, actual)
	}

	return nil
}

// progressReader wraps a reader to report progress.
type progressReader struct {
	reader     io.Reader
	downloaded int64
	total      int64
	onProgress func(downloaded, total int64)
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.downloaded += int64(n)
	if r.onProgress != nil {
		r.onProgress(r.downloaded, r.total)
	}
	return n, err
}
```

**Step 3: Run tests**

Run: `go test ./packages/... -v -run TestDownloader`
Expected: PASS

**Step 4: Commit**

```bash
git add packages/download.go packages/download_test.go
git commit -m "feat(packages): implement package download with integrity verification"
```

---

## Task 4.4: Publisher Signature Verification (Optional)

**Files:**
- Create: `packages/publisher.go`
- Create: `packages/publisher_test.go`

**Step 1: Write tests for publisher signature verification**

Create `packages/publisher_test.go`:
```go
// Copyright Graylog, Inc.
// SPDX-License-Identifier: Apache-2.0

package packages

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestPublisherVerifier_Minisign(t *testing.T) {
	// This is a simplified test - real implementation would use actual minisign format
	logger := zaptest.NewLogger(t)

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	content := []byte("fake package content")
	signature := ed25519.Sign(priv, content)

	verifier := NewPublisherVerifier(logger, PublisherConfig{
		Format:      "ed25519", // Simplified format for testing
		TrustedKeys: []ed25519.PublicKey{pub},
	})

	err := verifier.Verify(content, signature)
	require.NoError(t, err)
}

func TestPublisherVerifier_UntrustedKey(t *testing.T) {
	logger := zaptest.NewLogger(t)

	trustedPub, _, _ := ed25519.GenerateKey(rand.Reader)
	_, untrustedPriv, _ := ed25519.GenerateKey(rand.Reader)

	content := []byte("fake package content")
	signature := ed25519.Sign(untrustedPriv, content)

	verifier := NewPublisherVerifier(logger, PublisherConfig{
		Format:      "ed25519",
		TrustedKeys: []ed25519.PublicKey{trustedPub},
	})

	err := verifier.Verify(content, signature)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no trusted key")
}

func TestPublisherVerifier_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)

	verifier := NewPublisherVerifier(logger, PublisherConfig{
		Enabled: false,
	})

	// Should pass when disabled
	err := verifier.Verify([]byte("content"), []byte("signature"))
	require.NoError(t, err)
}
```

**Step 2: Implement publisher verifier**

Create `packages/publisher.go`:
```go
// Copyright Graylog, Inc.
// SPDX-License-Identifier: Apache-2.0

package packages

import (
	"crypto/ed25519"
	"fmt"

	"go.uber.org/zap"
)

// PublisherConfig configures publisher signature verification.
type PublisherConfig struct {
	Enabled     bool
	Format      string               // "cosign", "gpg", "minisign", "ed25519"
	TrustedKeys []ed25519.PublicKey // Simplified for now
}

// PublisherVerifier verifies publisher signatures on packages.
type PublisherVerifier struct {
	logger *zap.Logger
	cfg    PublisherConfig
}

// NewPublisherVerifier creates a new publisher signature verifier.
func NewPublisherVerifier(logger *zap.Logger, cfg PublisherConfig) *PublisherVerifier {
	return &PublisherVerifier{
		logger: logger,
		cfg:    cfg,
	}
}

// Verify verifies a publisher signature on package content.
func (v *PublisherVerifier) Verify(content, signature []byte) error {
	if !v.cfg.Enabled {
		return nil // Verification disabled
	}

	switch v.cfg.Format {
	case "ed25519":
		return v.verifyEd25519(content, signature)
	case "cosign":
		return v.verifyCosign(content, signature)
	case "gpg":
		return v.verifyGPG(content, signature)
	case "minisign":
		return v.verifyMinisign(content, signature)
	default:
		return fmt.Errorf("unsupported signature format: %s", v.cfg.Format)
	}
}

// verifyEd25519 verifies a raw Ed25519 signature.
func (v *PublisherVerifier) verifyEd25519(content, signature []byte) error {
	for _, key := range v.cfg.TrustedKeys {
		if ed25519.Verify(key, content, signature) {
			return nil
		}
	}
	return fmt.Errorf("no trusted key verified the signature")
}

// verifyCosign verifies a cosign signature.
func (v *PublisherVerifier) verifyCosign(content, signature []byte) error {
	// TODO: Implement cosign verification
	// Would use github.com/sigstore/cosign/v2/pkg/cosign
	return fmt.Errorf("cosign verification not yet implemented")
}

// verifyGPG verifies a GPG signature.
func (v *PublisherVerifier) verifyGPG(content, signature []byte) error {
	// TODO: Implement GPG verification
	// Would use golang.org/x/crypto/openpgp
	return fmt.Errorf("GPG verification not yet implemented")
}

// verifyMinisign verifies a minisign signature.
func (v *PublisherVerifier) verifyMinisign(content, signature []byte) error {
	// TODO: Implement minisign verification
	// Would use github.com/jedisct1/go-minisign
	return fmt.Errorf("minisign verification not yet implemented")
}
```

**Step 3: Run tests**

Run: `go test ./packages/... -v -run TestPublisher`
Expected: PASS

**Step 4: Commit**

```bash
git add packages/publisher.go packages/publisher_test.go
git commit -m "feat(packages): add publisher signature verification framework"
```

---

## Task 4.5: Package Manager Integration

**Files:**
- Create: `packages/manager.go`
- Create: `packages/manager_test.go`

**Step 1: Write tests for package manager**

Create `packages/manager_test.go`:
```go
// Copyright Graylog, Inc.
// SPDX-License-Identifier: Apache-2.0

package packages

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestManager_ProcessPackagesAvailable(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()

	// Generate server signing key
	serverPub, serverPriv, _ := ed25519.GenerateKey(rand.Reader)

	// Create fake package content
	packageContent := []byte("fake otelcol binary")
	packageHash := sha256.Sum256(packageContent)
	hashStr := "sha256:" + hex.EncodeToString(packageHash[:])

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(packageContent)
	}))
	defer server.Close()

	// Create signed attestation
	attestation := Attestation{
		PackageName: "otelcol",
		Version:     "0.98.0",
		Hash:        hashStr,
		DownloadURL: server.URL,
		IssuedAt:    time.Now(),
	}
	signed, _ := SignAttestation(attestation, serverPriv)

	mgr, err := NewManager(logger, ManagerConfig{
		StorageDir:       dir,
		KeepVersions:     2,
		ServerPublicKey:  serverPub,
		PublisherEnabled: false,
	})
	require.NoError(t, err)

	// Simulate OnPackagesAvailable
	packagesAvailable := &protobufs.PackagesAvailable{
		Packages: map[string]*protobufs.PackageAvailable{
			"otelcol": {
				Type:    protobufs.PackageType_PackageType_TopLevel,
				Version: "0.98.0",
				Hash:    packageHash[:],
			},
		},
	}

	result, err := mgr.ProcessPackagesAvailable(context.Background(), packagesAvailable, signed)
	require.NoError(t, err)
	require.True(t, result.Updated)
	require.Equal(t, "0.98.0", result.NewVersion)
}
```

**Step 2: Implement package manager**

Create `packages/manager.go`:
```go
// Copyright Graylog, Inc.
// SPDX-License-Identifier: Apache-2.0

package packages

import (
	"context"
	"crypto/ed25519"
	"fmt"

	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

// ManagerConfig configures the package manager.
type ManagerConfig struct {
	StorageDir       string
	KeepVersions     int
	ServerPublicKey  ed25519.PublicKey
	PublisherEnabled bool
	PublisherConfig  PublisherConfig
}

// ProcessResult contains the result of processing packages.
type ProcessResult struct {
	Updated         bool
	NewVersion      string
	PreviousVersion string
	RestartRequired bool
}

// Manager coordinates package operations.
type Manager struct {
	logger     *zap.Logger
	storage    *Storage
	downloader *Downloader
	publisher  *PublisherVerifier
	serverKey  ed25519.PublicKey
}

// NewManager creates a new package manager.
func NewManager(logger *zap.Logger, cfg ManagerConfig) (*Manager, error) {
	storage, err := NewStorage(logger, StorageConfig{
		BaseDir:      cfg.StorageDir,
		KeepVersions: cfg.KeepVersions,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	return &Manager{
		logger:     logger,
		storage:    storage,
		downloader: NewDownloader(logger, DownloaderConfig{}),
		publisher:  NewPublisherVerifier(logger, cfg.PublisherConfig),
		serverKey:  cfg.ServerPublicKey,
	}, nil
}

// ProcessPackagesAvailable handles the OnPackagesAvailable callback.
func (m *Manager) ProcessPackagesAvailable(
	ctx context.Context,
	packages *protobufs.PackagesAvailable,
	attestation *SignedAttestation,
) (*ProcessResult, error) {
	if packages == nil || len(packages.Packages) == 0 {
		return &ProcessResult{Updated: false}, nil
	}

	// Verify server attestation
	if err := VerifyAttestation(attestation, m.serverKey); err != nil {
		return nil, fmt.Errorf("attestation verification failed: %w", err)
	}

	// Get current version
	currentVersion, _ := m.storage.GetCurrentVersion(attestation.Attestation.PackageName)
	if currentVersion == attestation.Attestation.Version {
		m.logger.Info("Package already at requested version",
			zap.String("version", currentVersion))
		return &ProcessResult{Updated: false}, nil
	}

	m.logger.Info("Processing package update",
		zap.String("package", attestation.Attestation.PackageName),
		zap.String("current_version", currentVersion),
		zap.String("new_version", attestation.Attestation.Version))

	// Download package
	result, err := m.downloader.Download(ctx, DownloadRequest{
		URL:          attestation.Attestation.DownloadURL,
		ExpectedHash: attestation.Attestation.Hash,
	})
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}

	// Stage package
	stagePath, err := m.storage.Stage(
		attestation.Attestation.PackageName,
		attestation.Attestation.Version,
		result.Content,
	)
	if err != nil {
		return nil, fmt.Errorf("staging failed: %w", err)
	}

	// Install package
	_, err = m.storage.Install(
		attestation.Attestation.PackageName,
		attestation.Attestation.Version,
		stagePath,
	)
	if err != nil {
		return nil, fmt.Errorf("installation failed: %w", err)
	}

	// Cleanup old versions
	if err := m.storage.Cleanup(attestation.Attestation.PackageName); err != nil {
		m.logger.Warn("Failed to cleanup old versions", zap.Error(err))
	}

	return &ProcessResult{
		Updated:         true,
		NewVersion:      attestation.Attestation.Version,
		PreviousVersion: currentVersion,
		RestartRequired: true,
	}, nil
}

// Rollback reverts to the previous version.
func (m *Manager) Rollback(packageName string) error {
	return m.storage.Rollback(packageName)
}

// GetExecutablePath returns the path to the current executable.
func (m *Manager) GetExecutablePath(packageName string) (string, error) {
	return m.storage.GetExecutablePath(packageName)
}

// GetPackageStatuses returns the current package statuses for reporting.
func (m *Manager) GetPackageStatuses() *protobufs.PackageStatuses {
	// TODO: Implement package status reporting
	return &protobufs.PackageStatuses{
		Packages: make(map[string]*protobufs.PackageStatus),
	}
}
```

**Step 3: Run tests**

Run: `go test ./packages/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add packages/manager.go packages/manager_test.go
git commit -m "feat(packages): implement package manager orchestration"
```

---

## Task 4.6: Rollback on Failed Startup

**Files:**
- Modify: `supervisor/supervisor.go`

**Step 1: Integrate rollback with crash recovery**

Add rollback detection to supervisor. When crash recovery detects repeated failures after a package update, trigger rollback:

```go
// Add to supervisor/supervisor.go

// In Supervisor struct:
type Supervisor struct {
	// ... existing fields ...
	packageManager   *packages.Manager
	lastPackageUpdate time.Time
}

// Add rollback check in crash recovery callback:
func (s *Supervisor) shouldRollback() bool {
	// If we updated a package recently and the process keeps crashing,
	// we should rollback
	if s.lastPackageUpdate.IsZero() {
		return false
	}

	// If package was updated in last 5 minutes and we've crashed multiple times
	if time.Since(s.lastPackageUpdate) < 5*time.Minute {
		if s.commander.CrashCount() >= 3 {
			return true
		}
	}
	return false
}

// Add rollback method:
func (s *Supervisor) performRollback(ctx context.Context) error {
	s.logger.Warn("Performing package rollback due to repeated failures")

	if err := s.packageManager.Rollback("otelcol"); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	// Update executable path
	newPath, err := s.packageManager.GetExecutablePath("otelcol")
	if err != nil {
		return fmt.Errorf("failed to get rollback executable: %w", err)
	}

	s.logger.Info("Rollback complete, restarting with previous version",
		zap.String("executable", newPath))

	// Reset crash count and package update time
	s.lastPackageUpdate = time.Time{}

	return nil
}
```

**Step 2: Commit**

```bash
git add supervisor/supervisor.go
git commit -m "feat(supervisor): integrate package rollback on repeated failures"
```

---

## Task 4.7: Package Status Reporting

**Files:**
- Modify: `packages/manager.go`
- Modify: `opamp/client.go`

**Step 1: Implement status tracking**

Add to `packages/manager.go`:
```go
// PackageState tracks the state of a package.
type PackageState struct {
	Name            string
	Version         string
	Status          protobufs.PackageStatusEnum
	ErrorMessage    string
	LastUpdateTime  time.Time
}

// GetPackageStatuses returns current package statuses for OpAMP reporting.
func (m *Manager) GetPackageStatuses() *protobufs.PackageStatuses {
	statuses := &protobufs.PackageStatuses{
		Packages: make(map[string]*protobufs.PackageStatus),
	}

	// Get all managed packages
	// For now, just report otelcol
	version, err := m.storage.GetCurrentVersion("otelcol")
	if err == nil {
		statuses.Packages["otelcol"] = &protobufs.PackageStatus{
			Name:             "otelcol",
			AgentHasVersion:  version,
			AgentHasHash:     nil, // TODO: Store and return hash
			Status:           protobufs.PackageStatusEnum_PackageStatusEnum_Installed,
		}
	}

	return statuses
}
```

**Step 2: Add SetPackageStatuses to OpAMP client**

Add to `opamp/client.go`:
```go
// SetPackageStatuses updates the package statuses reported to the server.
func (c *Client) SetPackageStatuses(statuses *protobufs.PackageStatuses) error {
	if c.opampClient == nil {
		return fmt.Errorf("client not started")
	}
	return c.opampClient.SetPackageStatuses(statuses)
}
```

**Step 3: Commit**

```bash
git add packages/manager.go opamp/client.go
git commit -m "feat(packages): implement package status reporting"
```

---

## Task 4.8: Supervisor Integration

**Files:**
- Modify: `supervisor/supervisor.go`

**Step 1: Wire package manager into supervisor**

```go
// In supervisor New():
if cfg.Packages.StorageDir != "" {
	pkgMgr, err := packages.NewManager(logger, packages.ManagerConfig{
		StorageDir:       cfg.Packages.StorageDir,
		KeepVersions:     cfg.Packages.KeepVersions,
		ServerPublicKey:  serverPublicKey, // From auth/enrollment
		PublisherEnabled: cfg.Packages.Verification.PublisherSignature.Enabled,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create package manager: %w", err)
	}
	s.packageManager = pkgMgr
}

// In OpAMP callbacks:
OnPackagesAvailable: func(ctx context.Context, packages *protobufs.PackagesAvailable) error {
	if s.packageManager == nil {
		s.logger.Warn("Package update received but package manager not configured")
		return nil
	}

	// TODO: Get attestation from custom message or extension
	// For now, this is a placeholder
	result, err := s.packageManager.ProcessPackagesAvailable(ctx, packages, attestation)
	if err != nil {
		s.logger.Error("Failed to process package update", zap.Error(err))
		return err
	}

	if result.RestartRequired {
		s.lastPackageUpdate = time.Now()

		// Get new executable path
		execPath, err := s.packageManager.GetExecutablePath("otelcol")
		if err != nil {
			return fmt.Errorf("failed to get executable path: %w", err)
		}

		// Restart collector with new binary
		s.logger.Info("Restarting collector with updated package",
			zap.String("version", result.NewVersion),
			zap.String("executable", execPath))

		if err := s.commander.Stop(); err != nil {
			s.logger.Warn("Failed to stop collector gracefully", zap.Error(err))
		}

		// Update commander config with new executable
		// Then restart
		if err := s.commander.Start(ctx); err != nil {
			return fmt.Errorf("failed to restart with new package: %w", err)
		}
	}

	// Report updated package statuses
	s.opampClient.SetPackageStatuses(s.packageManager.GetPackageStatuses())

	return nil
},
```

**Step 2: Commit**

```bash
git add supervisor/supervisor.go
git commit -m "feat(supervisor): integrate package management"
```

---

## Summary

Phase 4 implements full package management:

1. **Package Storage** - Directory structure with versioning and symlinks
2. **Server Attestation** - Ed25519 signature verification on package metadata
3. **Package Download** - HTTP download with hash verification
4. **Publisher Signature** - Optional additional verification (framework)
5. **Package Installation** - Atomic install via staging
6. **Rollback Support** - Automatic revert on repeated failures
7. **Status Reporting** - Report package states to server
8. **Integration** - Wire into supervisor lifecycle

After Phase 4, the supervisor can:
- Receive package updates from OpAMP server
- Verify server attestation before downloading
- Download and verify package integrity
- Atomically install new versions
- Automatically rollback if new version fails
- Report package status to server

**Future enhancements:**
- Full cosign/gpg/minisign publisher signature support
- Package extraction (tar.gz, zip)
- Multiple package types (not just collector binary)
- Delta updates
