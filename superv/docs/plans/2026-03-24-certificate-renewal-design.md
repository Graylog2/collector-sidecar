# Certificate Renewal Design

## Problem

The supervisor's X.509 certificate (used for JWT-based authentication and collector
mTLS) has a finite lifetime but no renewal mechanism. When the certificate expires,
the supervisor can no longer authenticate with the server and the collector's OTLP
exporter loses its mTLS credentials. There is currently no code that checks
`NotAfter` or initiates renewal.

## Requirements

1. The supervisor must proactively renew its certificate before it expires by sending
   a new CSR to the server via the existing OpAMP `RequestConnectionSettings` mechanism.
2. Renewal reuses the existing Ed25519 signing key and X25519 encryption key — no new
   keypairs are generated.
3. Renewal is triggered by a timer based on a configurable fraction of the certificate's
   lifetime (default 0.75). For example, a 90-day cert renews after day 67.
4. On failure, retry with exponential backoff indefinitely. No shutdown or degraded
   mode — the supervisor keeps running and retrying, logging at escalating severity.
5. On success, persist the new certificate, restart/reload the collector (so it picks
   up the new cert for its OTLP exporter mTLS), and reconnect the supervisor's own-logs
   OTLP exporter.

## Design

### 1. Certificate Expiry Checks in `auth.Manager`

Two new methods on `auth.Manager`:

**`CertificateNeedsRenewal(renewalFraction float64) bool`**
- Computes `threshold := NotBefore + fraction * (NotAfter - NotBefore)`
- Returns `true` if `time.Now()` is past `threshold`
- Returns `false` if no certificate is loaded

**`CertificateExpired() bool`**
- Returns `true` if `time.Now()` is past `certificate.NotAfter`
- Returns `false` if no certificate is loaded

Both are pure queries on the already-loaded `m.certificate` field. No disk I/O,
safe to call on every health tick.

**Invariant:** `m.certificate` is always non-nil when the supervisor's health loop
runs. The startup sequence is: `LoadCredentials()` (which fails hard if the cert
file is corrupt) → `Start()` → health loop begins. If `LoadCredentials()` fails,
the supervisor does not start, so the health loop never runs with a nil certificate.

### 2. Renewal CSR Generation in `auth.Manager`

New method **`PrepareRenewal(instanceUID string) ([]byte, error)`**:

1. Loads the existing encryption private key from disk via
   `persistence.LoadEncryptionKey(m.keysDir)`
2. Derives the X25519 public key: `curve25519.X25519(privKey, curve25519.Basepoint)`
3. Reads the tenant ID from `m.certificate.Subject.Organization[0]` (if present)
4. Calls `CreateCSR` / `CreateCSRWithTenant` with `m.signingKey`, `instanceUID`,
   and the encryption public key
5. Returns the PEM-encoded CSR via `EncodeCSRToPEM`

Unlike `PrepareEnrollment`, this does not generate new keypairs, fetch JWKS, or
validate enrollment tokens. It does not set pending state since keys aren't changing.

### 3. Handling the Renewal Certificate Response

Rename `handleEnrollmentCertificate` to **`handleCertificateResponse`** to reflect
its dual role. The logic becomes:

1. If `authManager.HasPendingEnrollment()` → call `CompleteEnrollment` (existing flow)
2. Else if `s.pendingCSR != nil` → this is a renewal response, call
   `authManager.CompleteRenewal(certPEM)`
3. Else → ignore (no pending request)

New method **`CompleteRenewal(certPEM []byte) error`** on `auth.Manager`:

1. Parses the certificate PEM
2. Verifies the new cert's public key matches the existing `m.signingKey` using
   `bytes.Equal(cert.PublicKey.(ed25519.PublicKey), m.signingKey.Public().(ed25519.PublicKey))`
   — reject if the server issued a cert for a different key
3. Warns if `newCert.NotAfter <= oldCert.NotAfter` (server bug — would cause a
   tight renewal loop bounded by retry backoff)
4. Saves the certificate to disk via `persistence.SaveCertificate`
5. Updates `m.certificate` in memory

### 4. Integration into the Health Check Loop

The renewal check piggybacks on the existing health monitor polling loop. In the
supervisor's health status consumer goroutine (`s.healthWg.Go`), after reporting
health to the OpAMP server:

```
if s.authManager.IsEnrolled() && s.pendingCSR == nil {
    if s.authManager.CertificateNeedsRenewal(s.renewalFraction) {
        s.requestCertificateRenewal()
    }
} else if s.pendingCSR != nil && !s.authManager.HasPendingEnrollment() {
    // Renewal is pending — check retry backoff
    if time.Now().After(s.nextRenewalRetry) {
        s.requestCertificateRenewal()
    }
}
```

**`requestCertificateRenewal()`**:
1. Calls `authManager.PrepareRenewal(instanceUID)` to get the CSR PEM
2. Sets `s.pendingCSR` to the CSR (under lock)
3. Calls `s.opampClient.RequestConnectionSettings(csrPEM)`
4. On error: sets `s.nextRenewalRetry` with exponential backoff, logs at Warn level
5. On success: logs at Info level ("Certificate renewal requested, awaiting response")

### 5. Post-Renewal Actions

When `handleCertificateResponse` successfully processes a renewal:

1. Persist new certificate to disk (in `CompleteRenewal`)
2. Update `m.certificate` in memory (in `CompleteRenewal`)
3. Clear `s.pendingCSR` (under lock)
4. Restart/reload the collector via `s.commander` so it re-reads the cert/key files
   from disk (it receives paths via `GLC_INTERNAL_TLS_CLIENT_KEY_PATH` and
   `GLC_INTERNAL_TLS_CLIENT_CERT_PATH` environment variables)
5. Reconnect the supervisor's own-logs OTLP exporter via
   `s.ownLogsManager.ReloadClientCert()` — this re-reads the cert/key files from
   disk and calls `Apply()` with updated `Settings.TLSConfig` to rebuild the exporter
6. Reset `s.nextRenewalRetry` to zero
7. Log at Info level: "Certificate renewed successfully"

### 6. Retry and Failure Behavior

No separate backoff timer or goroutine. The health tick drives retries:

- Track `nextRenewalRetry time.Time` on the supervisor, protected by `s.mu`
  (read/written from both the health goroutine and the opamp-go callback goroutine)
- On each health tick, if `pendingCSR != nil` (renewal) and
  `time.Now().After(nextRenewalRetry)`, attempt again
- Each retry generates a fresh CSR (same keys, new CSR bytes) — re-sending is
  idempotent from the server's perspective since the public key material is unchanged
- Advance `nextRenewalRetry` with exponential backoff capped at
  `server.connection.retry_backoff.max` (default 5 minutes)
- No shutdown or degraded mode — the supervisor keeps running indefinitely

**Restart recovery:** Renewal state (`pendingCSR`, `nextRenewalRetry`) is not
persisted to disk. If the supervisor restarts mid-renewal, it loads the existing
certificate via `LoadCredentials()`, the health tick re-evaluates
`CertificateNeedsRenewal()`, and renewal is re-triggered naturally from the
certificate's expiry time. No special recovery logic is needed.

**Log levels:**

| Event | Level |
|-------|-------|
| Renewal threshold reached, CSR sent | Info |
| Renewal request failed, will retry | Warn |
| Renewal succeeded, new cert installed | Info |
| Certificate expired, still retrying | Error (every tick) |
| Own-logs reconnect failed | Warn |
| Collector restart failed | Error |

### 7. Configuration

Add `RenewalFraction` to `AuthConfig` in `config/types.go`:

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

Config file usage:

```yaml
server:
  auth:
    renewal_fraction: 0.75  # default
```

Validation: must be > 0 and < 1. If unset or zero, default to 0.75.

## Files Changed

| File | Change |
|------|--------|
| `auth/manager.go` | Add `CertificateNeedsRenewal`, `CertificateExpired`, `PrepareRenewal`, `CompleteRenewal` |
| `supervisor/supervisor.go` | Rename `handleEnrollmentCertificate` → `handleCertificateResponse`, add renewal check in health loop, add `nextRenewalRetry` field, add `requestCertificateRenewal` |
| `ownlogs/manager.go` | Add `ReloadClientCert` method |
| `config/types.go` | Add `RenewalFraction float64` to `AuthConfig` |
| `config/defaults.go` (or equivalent) | Default `RenewalFraction` to 0.75 |

## State Machine

```
                    ┌──────────────┐
                    │   Enrolled   │
                    │  (cert valid)│
                    └──────┬───────┘
                           │ health tick: CertificateNeedsRenewal() == true
                           ▼
                    ┌──────────────┐
            ┌──────│   Renewing   │◄────────┐
            │      │ (pendingCSR) │         │
            │      └──────┬───────┘         │
            │             │                 │
            │    success  │    failure       │
            │             ▼                 │
            │      ┌─────────────┐    retry after backoff
            │      │  Cert       │──────────┘
            │      │  Response?  │
            │      └──────┬──────┘
            │             │ yes
            │             ▼
            │      ┌──────────────┐
            │      │  Renewed     │
            │      │  (restart    │
            │      │   collector, │
            │      │   reload     │
            │      │   own-logs)  │
            │      └──────┬───────┘
            │             │
            │             ▼
            │      ┌──────────────┐
            └─────►│   Enrolled   │
                   │  (new cert)  │
                   └──────────────┘
```
