# Certificate Renewal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Proactively renew the supervisor's X.509 certificate before expiry by sending a new CSR via OpAMP.

**Architecture:** The renewal check piggybacks on the health goroutine via a dedicated ticker. When the certificate reaches a configurable fraction of its lifetime, `auth.Manager` generates a renewal CSR with the existing keys. The CSR is sent via `RequestConnectionSettings`. On receiving the new cert, the supervisor persists it, restarts the collector, and reloads own-logs.

**Tech Stack:** Go, OpAMP (`opamp-go`), Ed25519/X25519 (`crypto/ed25519`, `golang.org/x/crypto/curve25519`), `x509`

**Spec:** `docs/plans/2026-03-24-certificate-renewal-design.md`

**Working directory:** All commands must be run from the `superv/` directory.

---

### Task 1: Add `sync.RWMutex` to `auth.Manager`

Add concurrency protection to `auth.Manager` credential fields. This is a prerequisite for all subsequent tasks since renewal introduces concurrent mutation.

**Files:**
- Modify: `auth/manager.go`
- Test: `auth/manager_test.go`

- [ ] **Step 1: Add the mutex field**

In `auth/manager.go`, add `mu sync.RWMutex` to the `Manager` struct and add `"sync"` to imports:

```go
type Manager struct {
	logger      *zap.Logger
	keysDir     string
	httpClient  *http.Client
	jwtLifetime time.Duration

	mu sync.RWMutex // protects signingKey and certificate

	// Cached credentials
	signingKey  ed25519.PrivateKey
	certificate *x509.Certificate

	// Enrollment state (before CSR is submitted)
	pendingSigningKey    ed25519.PrivateKey
	pendingEncryptionKey []byte
	pendingTenantID      string
	pendingEnrollmentJWT string
}
```

- [ ] **Step 2: Add write locks to `LoadCredentials` and `CompleteEnrollment`**

In `LoadCredentials`, wrap the credential assignment:

```go
func (m *Manager) LoadCredentials() error {
	signingKey, err := persistence.LoadSigningKey(m.keysDir)
	if err != nil {
		return fmt.Errorf("failed to load signing key: %w", err)
	}

	cert, err := persistence.LoadCertificate(m.keysDir)
	if err != nil {
		return fmt.Errorf("failed to load certificate: %w", err)
	}

	m.mu.Lock()
	m.signingKey = signingKey
	m.certificate = cert
	m.mu.Unlock()
	return nil
}
```

In `CompleteEnrollment`, wrap the state update (lines 244-252):

```go
	// Update manager state
	m.mu.Lock()
	m.signingKey = m.pendingSigningKey
	m.certificate = cert
	m.mu.Unlock()

	// Clear pending state
	m.pendingSigningKey = nil
	m.pendingEncryptionKey = nil
	m.pendingTenantID = ""
	m.pendingEnrollmentJWT = ""
```

- [ ] **Step 3: Add read locks to read-only accessors**

Wrap reads in `GenerateJWT`:

```go
func (m *Manager) GenerateJWT() (string, error) {
	m.mu.RLock()
	signingKey := m.signingKey
	cert := m.certificate
	m.mu.RUnlock()

	if signingKey == nil || cert == nil {
		return "", errors.New("credentials not loaded")
	}

	instanceUID := cert.Subject.CommonName
	return CreateSupervisorJWT(signingKey, cert, instanceUID, m.jwtLifetime)
}
```

Wrap `Certificate()`:

```go
func (m *Manager) Certificate() *x509.Certificate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.certificate
}
```

Wrap `CertFingerprint()`:

```go
func (m *Manager) CertFingerprint() string {
	m.mu.RLock()
	cert := m.certificate
	m.mu.RUnlock()
	if cert == nil {
		return ""
	}
	return CertificateHexFingerprint(cert)
}
```

- [ ] **Step 4: Run existing tests to verify no regressions**

Run: `go test ./auth/ -v -count=1`

Expected: All existing tests pass unchanged.

- [ ] **Step 5: Commit**

```
feat(auth): add RWMutex to Manager for concurrent credential access
```

---

### Task 2: Add `CertificateNeedsRenewal` and `CertificateExpired`

Pure query methods on `auth.Manager` for checking certificate expiry.

**Files:**
- Modify: `auth/manager.go`
- Test: `auth/manager_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `auth/manager_test.go`:

```go
func TestManager_CertificateNeedsRenewal(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")
	logger := zaptest.NewLogger(t)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = persistence.SaveSigningKey(keysDir, priv)

	t.Run("no certificate loaded", func(t *testing.T) {
		m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
		require.False(t, m.CertificateNeedsRenewal(0.75))
	})

	t.Run("cert not yet due for renewal", func(t *testing.T) {
		// Cert valid from now to now+24h, fraction 0.75 => threshold at now+18h
		cert := createManagerTestCertWithValidity(t, priv.Public().(ed25519.PublicKey), time.Now(), time.Now().Add(24*time.Hour))
		_ = persistence.SaveCertificate(keysDir, cert)

		m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
		_ = m.LoadCredentials()
		require.False(t, m.CertificateNeedsRenewal(0.75))
	})

	t.Run("cert past renewal threshold", func(t *testing.T) {
		// Cert valid from 24h ago to 1h from now. Fraction 0.75 => threshold at 24h ago + 18.75h = 5.25h ago
		cert := createManagerTestCertWithValidity(t, priv.Public().(ed25519.PublicKey), time.Now().Add(-24*time.Hour), time.Now().Add(1*time.Hour))
		_ = persistence.SaveCertificate(keysDir, cert)

		m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
		_ = m.LoadCredentials()
		require.True(t, m.CertificateNeedsRenewal(0.75))
	})

	t.Run("expired cert needs renewal", func(t *testing.T) {
		cert := createManagerTestCertWithValidity(t, priv.Public().(ed25519.PublicKey), time.Now().Add(-48*time.Hour), time.Now().Add(-1*time.Hour))
		_ = persistence.SaveCertificate(keysDir, cert)

		m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
		_ = m.LoadCredentials()
		require.True(t, m.CertificateNeedsRenewal(0.75))
	})
}

func TestManager_CertificateExpired(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")
	logger := zaptest.NewLogger(t)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = persistence.SaveSigningKey(keysDir, priv)

	t.Run("no certificate loaded", func(t *testing.T) {
		m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
		require.False(t, m.CertificateExpired())
	})

	t.Run("cert not expired", func(t *testing.T) {
		cert := createManagerTestCertWithValidity(t, priv.Public().(ed25519.PublicKey), time.Now(), time.Now().Add(24*time.Hour))
		_ = persistence.SaveCertificate(keysDir, cert)

		m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
		_ = m.LoadCredentials()
		require.False(t, m.CertificateExpired())
	})

	t.Run("cert expired", func(t *testing.T) {
		cert := createManagerTestCertWithValidity(t, priv.Public().(ed25519.PublicKey), time.Now().Add(-48*time.Hour), time.Now().Add(-1*time.Hour))
		_ = persistence.SaveCertificate(keysDir, cert)

		m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
		_ = m.LoadCredentials()
		require.True(t, m.CertificateExpired())
	})
}
```

Add the test helper `createManagerTestCertWithValidity` (below existing helpers):

```go
func createManagerTestCertWithValidity(t *testing.T, pub ed25519.PublicKey, notBefore, notAfter time.Time) *x509.Certificate {
	t.Helper()

	seed := make([]byte, ed25519.SeedSize)
	signingKey := ed25519.NewKeyFromSeed(seed)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-instance-uid"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, pub, signingKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	return cert
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./auth/ -run "TestManager_CertificateNeedsRenewal|TestManager_CertificateExpired" -v`

Expected: FAIL — methods not defined.

- [ ] **Step 3: Implement the methods**

Add to `auth/manager.go`:

```go
// CertificateNeedsRenewal returns true if the certificate has passed the renewal
// threshold. The threshold is computed as NotBefore + fraction * (NotAfter - NotBefore).
func (m *Manager) CertificateNeedsRenewal(renewalFraction float64) bool {
	m.mu.RLock()
	cert := m.certificate
	m.mu.RUnlock()

	if cert == nil {
		return false
	}

	lifetime := cert.NotAfter.Sub(cert.NotBefore)
	threshold := cert.NotBefore.Add(time.Duration(float64(lifetime) * renewalFraction))
	return time.Now().After(threshold)
}

// CertificateExpired returns true if the certificate's NotAfter is in the past.
func (m *Manager) CertificateExpired() bool {
	m.mu.RLock()
	cert := m.certificate
	m.mu.RUnlock()

	if cert == nil {
		return false
	}

	return time.Now().After(cert.NotAfter)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./auth/ -run "TestManager_CertificateNeedsRenewal|TestManager_CertificateExpired" -v`

Expected: PASS

- [ ] **Step 5: Run all auth tests**

Run: `go test ./auth/ -v -count=1`

Expected: All pass.

- [ ] **Step 6: Commit**

```
feat(auth): add CertificateNeedsRenewal and CertificateExpired methods
```

---

### Task 3: Add `PrepareRenewal` to `auth.Manager`

Generates a renewal CSR using existing keys.

**Files:**
- Modify: `auth/manager.go`
- Test: `auth/manager_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `auth/manager_test.go`:

```go
func TestManager_PrepareRenewal(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")
	logger := zaptest.NewLogger(t)

	// Set up enrolled state: signing key, encryption key, certificate
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = persistence.SaveSigningKey(keysDir, priv)

	encPub, encPriv, _ := GenerateEncryptionKeypair()
	_ = persistence.SaveEncryptionKey(keysDir, encPriv)

	cert := createManagerTestCertWithOrg(t, priv.Public().(ed25519.PublicKey), "test-tenant")
	_ = persistence.SaveCertificate(keysDir, cert)

	m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
	err := m.LoadCredentials()
	require.NoError(t, err)

	csrPEM, err := m.PrepareRenewal("test-instance-uid")
	require.NoError(t, err)
	require.NotEmpty(t, csrPEM)

	// Parse and verify the CSR
	block, _ := pem.Decode(csrPEM)
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE REQUEST", block.Type)

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)
	require.Equal(t, "test-instance-uid", csr.Subject.CommonName)
	require.Equal(t, []string{"test-tenant"}, csr.Subject.Organization)

	// Verify the CSR is signed by the existing signing key
	require.Equal(t, priv.Public(), csr.PublicKey)

	// Verify the encryption public key extension is present
	var foundEncKey bool
	for _, ext := range csr.Extensions {
		if ext.Id.Equal(OIDEncryptionPublicKey) {
			require.Equal(t, encPub, ext.Value)
			foundEncKey = true
		}
	}
	require.True(t, foundEncKey, "encryption public key extension not found in CSR")
}

func TestManager_PrepareRenewal_NoTenant(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")
	logger := zaptest.NewLogger(t)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = persistence.SaveSigningKey(keysDir, priv)

	_, encPriv, _ := GenerateEncryptionKeypair()
	_ = persistence.SaveEncryptionKey(keysDir, encPriv)

	// Cert without Organization
	cert := createManagerTestCert(t, priv.Public().(ed25519.PublicKey))
	_ = persistence.SaveCertificate(keysDir, cert)

	m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
	_ = m.LoadCredentials()

	csrPEM, err := m.PrepareRenewal("test-instance-uid")
	require.NoError(t, err)

	block, _ := pem.Decode(csrPEM)
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)
	require.Empty(t, csr.Subject.Organization)
}

func TestManager_PrepareRenewal_NotLoaded(t *testing.T) {
	dir := t.TempDir()
	logger := zaptest.NewLogger(t)
	m := NewManager(logger, ManagerConfig{KeysDir: dir})

	_, err := m.PrepareRenewal("test-instance-uid")
	require.Error(t, err)
	require.ErrorContains(t, err, "credentials not loaded")
}
```

Add the test helper `createManagerTestCertWithOrg`:

```go
func createManagerTestCertWithOrg(t *testing.T, pub ed25519.PublicKey, org string) *x509.Certificate {
	t.Helper()

	seed := make([]byte, ed25519.SeedSize)
	signingKey := ed25519.NewKeyFromSeed(seed)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "test-instance-uid",
			Organization: []string{org},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, pub, signingKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	return cert
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./auth/ -run "TestManager_PrepareRenewal" -v`

Expected: FAIL — method not defined.

- [ ] **Step 3: Implement `PrepareRenewal`**

Add to `auth/manager.go` (add `"golang.org/x/crypto/curve25519"` to imports):

```go
// PrepareRenewal creates a CSR for certificate renewal using the existing keys.
// Unlike PrepareEnrollment, this does not generate new keypairs or validate tokens.
func (m *Manager) PrepareRenewal(instanceUID string) ([]byte, error) {
	m.mu.RLock()
	signingKey := m.signingKey
	cert := m.certificate
	m.mu.RUnlock()

	if signingKey == nil || cert == nil {
		return nil, errors.New("credentials not loaded")
	}

	// Load encryption private key from disk and derive public key
	encPriv, err := persistence.LoadEncryptionKey(m.keysDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load encryption key: %w", err)
	}

	encPub, err := curve25519.X25519(encPriv, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("failed to derive encryption public key: %w", err)
	}

	// Read tenant ID from certificate Organization field
	var tenantID string
	if len(cert.Subject.Organization) > 0 {
		tenantID = cert.Subject.Organization[0]
	}

	// Create CSR with existing signing key
	var csrDER []byte
	if tenantID != "" {
		csrDER, err = CreateCSRWithTenant(signingKey, instanceUID, tenantID, encPub)
	} else {
		csrDER, err = CreateCSR(signingKey, instanceUID, encPub)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create renewal CSR: %w", err)
	}

	m.logger.Info("Renewal CSR prepared", zap.String("instance_uid", instanceUID))
	return EncodeCSRToPEM(csrDER), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./auth/ -run "TestManager_PrepareRenewal" -v`

Expected: PASS

- [ ] **Step 5: Run all auth tests**

Run: `go test ./auth/ -v -count=1`

Expected: All pass.

- [ ] **Step 6: Commit**

```
feat(auth): add PrepareRenewal for certificate renewal CSR generation
```

---

### Task 4: Add `CompleteRenewal` to `auth.Manager`

Validates and persists a renewed certificate.

**Files:**
- Modify: `auth/manager.go`
- Test: `auth/manager_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `auth/manager_test.go`:

```go
func TestManager_CompleteRenewal(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")
	logger := zaptest.NewLogger(t)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = persistence.SaveSigningKey(keysDir, priv)

	oldCert := createManagerTestCertWithValidity(t, priv.Public().(ed25519.PublicKey), time.Now().Add(-24*time.Hour), time.Now().Add(1*time.Hour))
	_ = persistence.SaveCertificate(keysDir, oldCert)

	m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
	_ = m.LoadCredentials()

	// Create a new cert with later NotAfter, signed by a CA but with the same public key
	newCert := createManagerTestCertWithValidity(t, priv.Public().(ed25519.PublicKey), time.Now(), time.Now().Add(24*time.Hour))
	newCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: newCert.Raw})

	err := m.CompleteRenewal(newCertPEM)
	require.NoError(t, err)

	// Verify the new cert is loaded
	require.Equal(t, newCert.NotAfter.Unix(), m.Certificate().NotAfter.Unix())

	// Verify it was persisted
	loaded, err := persistence.LoadCertificate(keysDir)
	require.NoError(t, err)
	require.Equal(t, newCert.NotAfter.Unix(), loaded.NotAfter.Unix())
}

func TestManager_CompleteRenewal_WrongKey(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")
	logger := zaptest.NewLogger(t)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = persistence.SaveSigningKey(keysDir, priv)

	oldCert := createManagerTestCert(t, priv.Public().(ed25519.PublicKey))
	_ = persistence.SaveCertificate(keysDir, oldCert)

	m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
	_ = m.LoadCredentials()

	// Create a cert with a DIFFERENT public key
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	badCert := createManagerTestCertWithValidity(t, otherPriv.Public().(ed25519.PublicKey), time.Now(), time.Now().Add(48*time.Hour))
	badCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: badCert.Raw})

	err := m.CompleteRenewal(badCertPEM)
	require.Error(t, err)
	require.ErrorContains(t, err, "public key mismatch")

	// Old cert should still be loaded
	require.Equal(t, oldCert.NotAfter.Unix(), m.Certificate().NotAfter.Unix())
}

func TestManager_CompleteRenewal_NotAfterNotExtended(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")
	logger := zaptest.NewLogger(t)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = persistence.SaveSigningKey(keysDir, priv)

	oldCert := createManagerTestCertWithValidity(t, priv.Public().(ed25519.PublicKey), time.Now(), time.Now().Add(24*time.Hour))
	_ = persistence.SaveCertificate(keysDir, oldCert)

	m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
	_ = m.LoadCredentials()

	// Create a cert with same NotAfter (not extended)
	sameCert := createManagerTestCertWithValidity(t, priv.Public().(ed25519.PublicKey), time.Now(), oldCert.NotAfter)
	sameCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: sameCert.Raw})

	err := m.CompleteRenewal(sameCertPEM)
	require.Error(t, err)
	require.ErrorContains(t, err, "NotAfter")

	// Old cert should still be loaded
	require.Equal(t, oldCert.NotAfter.Unix(), m.Certificate().NotAfter.Unix())
}

func TestManager_CompleteRenewal_InvalidPEM(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "keys")
	logger := zaptest.NewLogger(t)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = persistence.SaveSigningKey(keysDir, priv)
	cert := createManagerTestCert(t, priv.Public().(ed25519.PublicKey))
	_ = persistence.SaveCertificate(keysDir, cert)

	m := NewManager(logger, ManagerConfig{KeysDir: keysDir})
	_ = m.LoadCredentials()

	err := m.CompleteRenewal([]byte("not-a-cert"))
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid certificate PEM")
}

func TestManager_CompleteRenewal_NotLoaded(t *testing.T) {
	dir := t.TempDir()
	logger := zaptest.NewLogger(t)
	m := NewManager(logger, ManagerConfig{KeysDir: dir})

	err := m.CompleteRenewal([]byte("anything"))
	require.Error(t, err)
	require.ErrorContains(t, err, "credentials not loaded")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./auth/ -run "TestManager_CompleteRenewal" -v`

Expected: FAIL — method not defined.

- [ ] **Step 3: Implement `CompleteRenewal`**

Add to `auth/manager.go` (add `"bytes"` to imports):

```go
// CompleteRenewal validates and persists a renewed certificate received from the server.
// The new cert must have the same public key as the current signing key and a later NotAfter.
func (m *Manager) CompleteRenewal(certPEM []byte) error {
	m.mu.RLock()
	signingKey := m.signingKey
	oldCert := m.certificate
	m.mu.RUnlock()

	if signingKey == nil || oldCert == nil {
		return errors.New("credentials not loaded")
	}

	// Parse certificate
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return errors.New("invalid certificate PEM")
	}

	newCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Verify public key matches
	newPub, ok := newCert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return errors.New("renewed certificate does not contain an Ed25519 public key")
	}
	if !bytes.Equal(newPub, signingKey.Public().(ed25519.PublicKey)) {
		return errors.New("public key mismatch: renewed certificate has a different public key")
	}

	// Reject if NotAfter is not extended
	if !newCert.NotAfter.After(oldCert.NotAfter) {
		return fmt.Errorf("renewed certificate NotAfter (%s) is not later than current (%s)",
			newCert.NotAfter.Format(time.RFC3339), oldCert.NotAfter.Format(time.RFC3339))
	}

	// Persist
	if err := persistence.SaveCertificate(m.keysDir, newCert); err != nil {
		return fmt.Errorf("failed to save renewed certificate: %w", err)
	}

	// Update in memory
	m.mu.Lock()
	m.certificate = newCert
	m.mu.Unlock()

	m.logger.Info("Certificate renewed",
		zap.String("cert_fingerprint", CertificateHexFingerprint(newCert)),
		zap.Time("old_not_after", oldCert.NotAfter),
		zap.Time("new_not_after", newCert.NotAfter),
	)

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./auth/ -run "TestManager_CompleteRenewal" -v`

Expected: PASS

- [ ] **Step 5: Run all auth tests**

Run: `go test ./auth/ -v -count=1`

Expected: All pass.

- [ ] **Step 6: Commit**

```
feat(auth): add CompleteRenewal for certificate renewal validation and persistence
```

---

### Task 5: Add `RenewalFraction` to config

**Files:**
- Modify: `config/types.go`
- Modify: `config/validate.go`
- Test: `config/validate_test.go`

- [ ] **Step 1: Write the failing validation test**

Add to `config/validate_test.go` (add `"time"` to imports if not already present):

```go
func TestValidateRenewalFraction(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		wantErr string
	}{
		{name: "valid 0.75", value: 0.75},
		{name: "valid 0.5", value: 0.5},
		{name: "valid 0.01", value: 0.01},
		{name: "valid 0.99", value: 0.99},
		{name: "zero defaults to 0.75", value: 0},
		{name: "negative", value: -0.5, wantErr: "renewal_fraction"},
		{name: "one", value: 1.0, wantErr: "renewal_fraction"},
		{name: "greater than one", value: 1.5, wantErr: "renewal_fraction"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := AuthConfig{
				JWTLifetime:     5 * time.Minute,
				RenewalFraction: tt.value,
			}
			err := cfg.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./config/ -run "TestValidateRenewalFraction" -v`

Expected: FAIL — `RenewalFraction` field not found.

- [ ] **Step 3: Add field to `AuthConfig`**

In `config/types.go`, add `RenewalFraction` to `AuthConfig`:

```go
type AuthConfig struct {
	EnrollmentEndpoint string            `koanf:"enrollment_endpoint"`
	EnrollmentToken    string            `koanf:"enrollment_token"`
	EnrollmentHeaders  map[string]string `koanf:"enrollment_headers"`
	InsecureTLS        bool              `koanf:"insecure_tls"`
	JWTLifetime        time.Duration     `koanf:"jwt_lifetime"`
	RenewalFraction    float64           `koanf:"renewal_fraction"`
}
```

- [ ] **Step 4: Add default in `DefaultConfig()`**

In `config/types.go`, in the `DefaultConfig()` function, update the `Auth` block:

```go
Auth: AuthConfig{
	JWTLifetime:     5 * time.Minute,
	RenewalFraction: 0.75,
},
```

- [ ] **Step 5: Add validation**

In `config/validate.go`, in `AuthConfig.Validate()`:

```go
func (k AuthConfig) Validate() error {
	if k.JWTLifetime < minJWTLifetime {
		return fmt.Errorf("server.auth.jwt_lifetime: JWT lifetime must be at least %s", minJWTLifetime)
	}
	// Zero means unset — DefaultConfig() provides 0.75. But if the user explicitly
	// sets 0, CertificateNeedsRenewal(0) would always trigger, so reject it too.
	if k.RenewalFraction != 0 && (k.RenewalFraction <= 0 || k.RenewalFraction >= 1) {
		return fmt.Errorf("server.auth.renewal_fraction: must be between 0 (exclusive) and 1 (exclusive), got %g", k.RenewalFraction)
	}
	return nil
}
```

Note: The `DefaultConfig()` default (0.75) applies when the field is unset in the config
file. The validation allows zero (Go's zero-value for unset `float64`) but rejects
negative and >= 1. The call site in `checkCertificateRenewal` must coalesce zero to
the default (see Task 7).

- [ ] **Step 6: Run validation tests**

Run: `go test ./config/ -run "TestValidateRenewalFraction" -v`

Expected: PASS

- [ ] **Step 7: Run all config tests**

Run: `go test ./config/ -v -count=1`

Expected: All pass.

- [ ] **Step 8: Commit**

```
feat(config): add renewal_fraction setting for certificate renewal threshold
```

---

### Task 6: Rename `handleEnrollmentCertificate` and add renewal branch

Rename the method and add the renewal response handling path.

**Files:**
- Modify: `supervisor/supervisor.go`

- [ ] **Step 1: Rename `handleEnrollmentCertificate` to `handleCertificateResponse`**

In `supervisor/supervisor.go`, rename the function (line 922) and update the call
site in `prepareConnectionSettings` (line 861).

Old:
```go
newlyEnrolled, err := s.handleEnrollmentCertificate(settings)
```

New:
```go
newlyEnrolled, err := s.handleCertificateResponse(settings)
```

- [ ] **Step 2: Add the renewal branch to `handleCertificateResponse`**

Replace the body of `handleCertificateResponse` (formerly `handleEnrollmentCertificate`):

```go
func (s *Supervisor) handleCertificateResponse(settings *protobufs.OpAMPConnectionSettings) (bool, error) {
	cert := settings.GetCertificate()
	if cert == nil {
		return false, nil
	}

	certPEM := cert.GetCert()
	if len(certPEM) == 0 {
		return false, nil
	}

	s.logger.Info("Received certificate from server")

	// Branch 1: enrollment (HasPendingEnrollment takes precedence — see design spec section 3)
	if s.authManager.HasPendingEnrollment() {
		s.logger.Info("Completing enrollment with received certificate")
		if err := s.authManager.CompleteEnrollment(certPEM); err != nil {
			return false, fmt.Errorf("failed to complete enrollment: %w", err)
		}

		s.mu.Lock()
		s.pendingCSR = nil
		s.mu.Unlock()

		s.logger.Info("Enrollment completed successfully",
			zap.String("cert_fingerprint", s.authManager.CertFingerprint()),
		)
		return true, nil
	}

	// Branch 2: renewal
	s.mu.RLock()
	hasPendingCSR := s.pendingCSR != nil
	s.mu.RUnlock()

	if hasPendingCSR {
		s.logger.Info("Completing certificate renewal")
		if err := s.authManager.CompleteRenewal(certPEM); err != nil {
			return false, fmt.Errorf("failed to complete renewal: %w", err)
		}

		// Clear renewal state (lock ordering: auth.Manager mutex was released by CompleteRenewal,
		// now safe to take s.mu)
		s.mu.Lock()
		s.pendingCSR = nil
		s.nextRenewalRetry = time.Time{}
		s.renewalBackoff = 0
		s.mu.Unlock()

		// Best-effort: restart collector to pick up new cert
		if s.commander != nil {
			if err := s.commander.Restart(context.Background()); err != nil {
				s.logger.Error("Failed to restart collector after certificate renewal", zap.Error(err))
			}
		}

		// Best-effort: reload own-logs TLS
		if s.ownLogsManager != nil {
			if err := s.reloadOwnLogsCert(context.Background()); err != nil {
				s.logger.Warn("Failed to reload own-logs certificate", zap.Error(err))
			}
		}

		s.logger.Info("Certificate renewal completed successfully",
			zap.String("cert_fingerprint", s.authManager.CertFingerprint()),
		)
		return true, nil
	}

	// Branch 3: no pending request
	s.logger.Debug("No pending enrollment or renewal, ignoring certificate")
	return false, nil
}
```

- [ ] **Step 3: Add the `nextRenewalRetry` and `renewalBackoff` fields to `Supervisor`**

Add after the `pendingCSR` field:

```go
	// Pending enrollment CSR (set during enrollment, cleared after completion)
	pendingCSR []byte

	// Certificate renewal retry state (protected by mu)
	nextRenewalRetry time.Time
	renewalBackoff   time.Duration
```

Add `"time"` to imports if not already present.

- [ ] **Step 4: Add `reloadOwnLogsCert` helper**

```go
// reloadOwnLogsCert reloads the own-logs OTLP exporter's client certificate
// after a certificate renewal. Must be called from the work queue goroutine
// to avoid racing with applyOwnLogs.
func (s *Supervisor) reloadOwnLogsCert(ctx context.Context) error {
	if s.currentOwnLogs == nil || s.currentOwnLogs.TLSConfig == nil {
		return nil
	}

	certPath := s.authManager.GetSigningCertPath()
	keyPath := s.authManager.GetSigningKeyPath()

	if err := s.currentOwnLogs.LoadClientCert(certPath, keyPath); err != nil {
		return fmt.Errorf("load client cert: %w", err)
	}

	res := ownlogs.BuildResource(ServiceName, version.Version(), s.instanceUID)
	return s.ownLogsManager.Apply(ctx, *s.currentOwnLogs, res)
}
```

Note: `s.currentOwnLogs` is also mutated by the own-logs callback which runs on the
work queue. To avoid a data race, the `reloadOwnLogsCert` call in
`handleCertificateResponse` should be dispatched on the work queue. However, since
`handleCertificateResponse` runs synchronously in the opamp-go callback, and the
existing own-logs callback also runs there before being dispatched to the work queue,
consider dispatching the reload as a `workFunc` instead of calling it inline. If
this adds too much complexity for a best-effort operation, document the race and
accept it — the worst case is a stale TLS config that gets fixed on the next
`TelemetryConnectionSettings` update.

- [ ] **Step 5: Run existing supervisor tests**

Run: `go test ./supervisor/ -v -count=1 -timeout 120s`

Expected: All existing tests pass. The rename should not break anything since the
method is private.

- [ ] **Step 6: Commit**

```
refactor(supervisor): rename handleEnrollmentCertificate, add renewal response handling
```

---

### Task 7: Add renewal check to health goroutine

Wire the renewal ticker into the supervisor's health goroutine and implement the
check/request methods.

**Files:**
- Modify: `supervisor/supervisor.go`

- [ ] **Step 1: Add the `renewalResponseTimeout` constant**

At the top of `supervisor/supervisor.go`, near other constants:

```go
// renewalResponseTimeout is how long to wait for a certificate renewal response
// before treating the request as failed and retrying.
const renewalResponseTimeout = 2 * time.Minute
```

- [ ] **Step 2: Modify the health goroutine to add the renewal ticker**

In `supervisor.go`, replace the health goroutine (starting at line 431) with:

```go
	s.healthWg.Go(func() {
		renewalTicker := time.NewTicker(s.agentCfg.Health.Interval)
		defer renewalTicker.Stop()

		for {
			select {
			case status, ok := <-healthUpdates:
				if !ok {
					return
				}
				if s.authManager.IsEnrolled() {
					s.mu.RLock()
					client := s.opampClient
					s.mu.RUnlock()

					if client != nil {
						if err := client.SetHealth(status.ToComponentHealth(nil)); err != nil {
							s.logger.Warn("Failed to report health", zap.Error(err))
						}
					}
				}

			case <-renewalTicker.C:
				s.checkCertificateRenewal()
			}
		}
	})
```

Note: The existing code uses `s.healthWg.Go(func() { ... })`. The `healthWg` field
is declared as `sync.WaitGroup` at line 78 — Go 1.22+ added `WaitGroup.Go()`.

- [ ] **Step 3: Implement `checkCertificateRenewal`**

```go
// checkCertificateRenewal checks if the certificate needs renewal and initiates
// or retries the renewal process.
func (s *Supervisor) checkCertificateRenewal() {
	if !s.authManager.IsEnrolled() {
		return
	}

	if s.authManager.CertificateExpired() {
		s.logger.Error("Certificate expired, renewal pending")
	}

	s.mu.RLock()
	hasPendingCSR := s.pendingCSR != nil
	hasPendingEnrollment := s.authManager.HasPendingEnrollment()
	nextRetry := s.nextRenewalRetry
	s.mu.RUnlock()

	if hasPendingEnrollment {
		return // enrollment in progress, not our concern
	}

	if !hasPendingCSR {
		fraction := s.authCfg.RenewalFraction
		if fraction == 0 {
			fraction = 0.75 // default if unset
		}
		if s.authManager.CertificateNeedsRenewal(fraction) {
			s.requestCertificateRenewal()
		}
		return
	}

	// Renewal pending — check retry/response timeout
	if time.Now().After(nextRetry) {
		s.requestCertificateRenewal()
	}
}
```

- [ ] **Step 4: Implement `requestCertificateRenewal`**

```go
// requestCertificateRenewal generates a renewal CSR and sends it via OpAMP.
func (s *Supervisor) requestCertificateRenewal() {
	csrPEM, err := s.authManager.PrepareRenewal(s.instanceUID)
	if err != nil {
		s.logger.Error("Failed to prepare renewal CSR", zap.Error(err))
		s.advanceRenewalBackoff()
		return
	}

	s.mu.Lock()
	s.pendingCSR = csrPEM
	s.mu.Unlock()

	s.mu.RLock()
	client := s.opampClient
	s.mu.RUnlock()

	if client == nil {
		s.logger.Warn("OpAMP client not available for certificate renewal")
		s.advanceRenewalBackoff()
		return
	}

	if err := client.RequestConnectionSettings(csrPEM); err != nil {
		s.logger.Warn("Failed to send certificate renewal request", zap.Error(err))
		s.advanceRenewalBackoff()
		return
	}

	// Request sent successfully — set response timeout
	s.mu.Lock()
	s.nextRenewalRetry = time.Now().Add(renewalResponseTimeout)
	s.mu.Unlock()

	s.logger.Info("Certificate renewal requested, awaiting response")
}

// advanceRenewalBackoff advances the exponential backoff for renewal retries.
func (s *Supervisor) advanceRenewalBackoff() {
	s.mu.Lock()
	if s.renewalBackoff == 0 {
		s.renewalBackoff = s.renewalBackoffCfg.Initial
	} else {
		s.renewalBackoff = time.Duration(float64(s.renewalBackoff) * s.renewalBackoffCfg.Multiplier)
	}
	if s.renewalBackoff > s.renewalBackoffCfg.Max {
		s.renewalBackoff = s.renewalBackoffCfg.Max
	}
	s.nextRenewalRetry = time.Now().Add(s.renewalBackoff)
	s.mu.Unlock()
}
```

The backoff config lives on `ServerConfig.Connection.RetryBackoff`, not on `AuthConfig`.
Add the field to `Supervisor`:

```go
	renewalBackoffCfg config.BackoffConfig
```

Initialize it in `New()` from `cfg.Server.Connection.RetryBackoff`:

```go
	renewalBackoffCfg: cfg.Server.Connection.RetryBackoff,
```

- [ ] **Step 5: Run tests**

Run: `go test ./supervisor/ -v -count=1 -timeout 120s`

Expected: All pass.

- [ ] **Step 6: Commit**

```
feat(supervisor): add certificate renewal check to health goroutine
```

---

### Task 8: Own-logs cert reload (SKIPPED)

The supervisor's `reloadOwnLogsCert` helper (added in Task 6) directly calls
`Settings.LoadClientCert` and `Manager.Apply`. A separate `ReloadClientCert` method
on `ownlogs.Manager` would just be a pass-through wrapper with no added value.
The own-logs reload is best-effort, so the supervisor helper is sufficient.

---

### Task 9: Integration test

Verify the end-to-end renewal flow.

**Files:**
- Test: `supervisor/supervisor_test.go` or `supervisor/renewal_test.go`

- [ ] **Step 1: Write an integration test**

The test should:
1. Create a supervisor with an enrolled certificate that is past the renewal threshold
2. Set up a mock OpAMP server that responds to `RequestConnectionSettings` with a new cert
3. Start the supervisor and verify:
   - The renewal CSR is sent
   - The new cert is received and persisted
   - `pendingCSR` is cleared
   - The auth manager has the new certificate

Look at the existing `supervisor/integration_test.go` for patterns on how the
test infrastructure is set up (mock OpAMP server, test certificates, etc.).

- [ ] **Step 2: Run the integration test**

Run: `go test ./supervisor/ -run "TestCertificateRenewal" -v -timeout 120s`

Expected: PASS

- [ ] **Step 3: Run all tests**

Run: `go test ./... -v -count=1 -timeout 300s`

Expected: All pass.

- [ ] **Step 4: Commit**

```
test(supervisor): add certificate renewal integration test
```

---

### Task 10: Format and final check

- [ ] **Step 1: Run formatter**

Run: `make fmt`

- [ ] **Step 2: Run go fix**

Run: `go fix ./...`

- [ ] **Step 3: Run full test suite**

Run: `go test ./... -count=1 -timeout 300s`

Expected: All pass.

- [ ] **Step 4: Commit if formatting changed anything**

```
chore: format code
```
