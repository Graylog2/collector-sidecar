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
3. Reads the tenant ID from `m.certificate.Subject.Organization[0]` (if the
   `Organization` slice is non-empty; otherwise no tenant ID)
4. Calls `CreateCSR` / `CreateCSRWithTenant` with `m.signingKey`, `instanceUID`,
   and the encryption public key
5. Returns the PEM-encoded CSR via `EncodeCSRToPEM`

Unlike `PrepareEnrollment`, this does not generate new keypairs, fetch JWKS, or
validate enrollment tokens. It does not set pending state since keys aren't changing.

### 3. Handling the Renewal Certificate Response

Rename `handleEnrollmentCertificate` to **`handleCertificateResponse`** to reflect
its dual role. The call site in `prepareConnectionSettings` must be updated
accordingly. The logic becomes:

1. If `authManager.HasPendingEnrollment()` → call `CompleteEnrollment` (existing flow)
2. Else if `s.pendingCSR != nil` → this is a renewal response, call
   `authManager.CompleteRenewal(certPEM)`
3. Else → ignore (no pending request)

**Branch ordering is significant:** during enrollment, both `HasPendingEnrollment()`
and `s.pendingCSR != nil` are true. Branch 1 must be checked first so enrollment
takes the correct path. During renewal, only `s.pendingCSR != nil` is true
(`HasPendingEnrollment()` is false because no pending signing key exists).

New method **`CompleteRenewal(certPEM []byte) error`** on `auth.Manager`:

1. Parses the certificate PEM
2. Verifies the new cert's public key matches the existing `m.signingKey` using
   `bytes.Equal(cert.PublicKey.(ed25519.PublicKey), m.signingKey.Public().(ed25519.PublicKey))`
   — reject if the server issued a cert for a different key
3. Rejects if `newCert.NotAfter <= oldCert.NotAfter` — returns an error, keeping the
   old cert in place. This prevents overwriting a known-better cert with one that is
   equal or worse (e.g., server bug issuing an already-expired cert). The retry
   backoff kicks in and the server may eventually issue a valid cert.
4. Saves the certificate to disk via `persistence.SaveCertificate`
5. Updates `m.certificate` in memory (under write lock — see section 8)

### 4. Integration into the Health Check Loop

The existing health status consumer goroutine (`s.healthWg.Go`) iterates over a
channel that only emits on health status *changes*. Since the collector typically
stays healthy, this channel fires rarely — too infrequently for a renewal check.

The solution is to add a dedicated renewal ticker within the same goroutine, using
`select` over both the health status channel and the renewal ticker:

```go
renewalTicker := time.NewTicker(s.agentCfg.Health.Interval) // same cadence as health
defer renewalTicker.Stop()

for {
    select {
    case status, ok := <-healthUpdates:
        if !ok {
            return
        }
        // existing health reporting logic...

    case <-renewalTicker.C:
        s.checkCertificateRenewal()
    }
}
```

**`checkCertificateRenewal()`** (called on every renewal tick):
```
if !s.authManager.IsEnrolled() {
    return
}
if s.authManager.CertificateExpired() {
    s.logger.Error("Certificate expired, renewal pending")
}
if s.pendingCSR == nil {
    if s.authManager.CertificateNeedsRenewal(s.authCfg.RenewalFraction) {
        s.requestCertificateRenewal()
    }
} else if !s.authManager.HasPendingEnrollment() {
    // Renewal is pending — check retry/response timeout
    if time.Now().After(s.nextRenewalRetry) {
        s.requestCertificateRenewal()
    }
}
```

Note: `IsEnrolled()` checks file existence on disk (two `stat` calls). This is
acceptable at the health check interval (default 10s) but could be cached after
enrollment if profiling shows it matters.

**`requestCertificateRenewal()`**:
1. Calls `authManager.PrepareRenewal(instanceUID)` to get the CSR PEM
2. Sets `s.pendingCSR` to the CSR (under `s.mu`)
3. Calls `s.opampClient.RequestConnectionSettings(csrPEM)` — note that "success"
   here means the request was queued/sent, not that the server accepted it
4. On error: advances `s.nextRenewalRetry` with exponential backoff, logs at Warn.
   Backoff uses `server.connection.retry_backoff` parameters (`Initial`, `Max`,
   `Multiplier`). The current backoff interval is tracked in a new
   `s.renewalBackoff time.Duration` field (protected by `s.mu`), reset to zero
   on successful renewal. Jitter is not applied (the health tick interval provides
   sufficient natural jitter).
5. On success: sets `s.nextRenewalRetry = time.Now().Add(renewalResponseTimeout)` to
   prevent re-sending on the next tick while awaiting the server's response. The
   `renewalResponseTimeout` is a constant (2 minutes). If the server does not respond
   within this window, the next tick past the deadline treats it as a failure and
   retries with normal exponential backoff. Logs at Info level.

### 5. Post-Renewal Actions

When `handleCertificateResponse` successfully processes a renewal:

1. Persist new certificate to disk (in `CompleteRenewal`)
2. Update `m.certificate` in memory (in `CompleteRenewal`)
3. Clear `s.pendingCSR` and reset `s.nextRenewalRetry`/`s.renewalBackoff` (under
   `s.mu`). **Lock ordering:** `CompleteRenewal` acquires the `auth.Manager` mutex
   first, then `s.mu` is acquired separately afterward to clear supervisor state.
   `CompleteRenewal` must NOT be called while holding `s.mu`, to avoid deadlock.
   This matches the existing enrollment pattern where `CompleteEnrollment` runs
   first, then `s.mu.Lock()` clears `s.pendingCSR`.
4. Restart/reload the collector via `s.commander` so it re-reads the cert/key files
   from disk (it receives paths via `GLC_INTERNAL_TLS_CLIENT_KEY_PATH` and
   `GLC_INTERNAL_TLS_CLIENT_CERT_PATH` environment variables)
5. Reconnect the supervisor's own-logs OTLP exporter via
   `s.ownLogsManager.ReloadClientCert(ctx, certPath, keyPath)` — this calls
   `ownlogs.LoadClientCert` to build a new `tls.Config` from the on-disk files,
   updates the TLS config in the current `Settings`, and calls `Apply()` using the
   last-applied settings and resource (stored on `Manager` or passed from
   `s.currentOwnLogs`)
6. Log at Info level: "Certificate renewed successfully", including the new cert's
   `NotAfter` and the old cert's `NotAfter` for operational observability

Steps 4 and 5 are best-effort. If either fails, log the error but do not retry —
the new cert is already persisted to disk. The collector will pick it up on its next
restart (config change, crash recovery, etc.), and the own-logs exporter will reload
on the next `TelemetryConnectionSettings` update from the server.

### 6. Retry and Failure Behavior

No separate backoff timer or goroutine. The renewal ticker drives retries:

- Track `nextRenewalRetry time.Time` and `renewalBackoff time.Duration` on the
  supervisor, both protected by `s.mu` (read/written from both the health/renewal
  goroutine and the opamp-go callback goroutine)
- On each renewal tick, if `pendingCSR != nil` (renewal) and
  `time.Now().After(nextRenewalRetry)`, attempt again
- Each retry generates a fresh CSR (same keys, new CSR bytes) — re-sending is
  idempotent from the server's perspective since the public key material is unchanged
- Advance `renewalBackoff` using `server.connection.retry_backoff` parameters:
  start at `Initial` (default 1s), multiply by `Multiplier` (default 2.0), cap at
  `Max` (default 5 minutes). Set `nextRenewalRetry = time.Now().Add(renewalBackoff)`.
  Reset `renewalBackoff` to zero on successful renewal.
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

Validation at config load time: zero (Go's default for unset `float64`) defaults to
0.75 in `DefaultConfig()`. Values that are explicitly negative or >= 1 are rejected
with an error.

### 8. Concurrency in `auth.Manager`

Renewal introduces concurrent mutation of `m.certificate`: `CompleteRenewal` writes
it from the opamp-go callback goroutine, while `GenerateJWT` reads it from the
`HeaderFunc` goroutine, and `CertificateNeedsRenewal`/`CertificateExpired` read it
from the health goroutine. Today these fields are plain struct fields with no
synchronization because they were write-once (set during startup or enrollment).

Add a `sync.RWMutex` to `auth.Manager`:

- **Write lock:** `CompleteRenewal`, `CompleteEnrollment`, `LoadCredentials`
- **Read lock:** `GenerateJWT`, `Certificate()`, `CertFingerprint()`,
  `CertificateNeedsRenewal()`, `CertificateExpired()`

The signing key (`m.signingKey`) does not change during renewal, but protecting it
under the same lock is simpler and avoids a partial-update window where the cert and
key are temporarily inconsistent (relevant during enrollment when both change).

## Files Changed

| File | Change |
|------|--------|
| `auth/manager.go` | Add `sync.RWMutex`, `CertificateNeedsRenewal`, `CertificateExpired`, `PrepareRenewal`, `CompleteRenewal`; add read/write locks to existing accessors |
| `supervisor/supervisor.go` | Rename `handleEnrollmentCertificate` → `handleCertificateResponse` (update call site in `prepareConnectionSettings`), add renewal ticker + `checkCertificateRenewal` in health goroutine, add `nextRenewalRetry`/`renewalBackoff` fields (under `s.mu`), add `requestCertificateRenewal`, add `renewalResponseTimeout` constant |
| `ownlogs/manager.go` | Add `ReloadClientCert` method |
| `config/types.go` | Add `RenewalFraction float64` to `AuthConfig` |
| `config/types.go` | Default `RenewalFraction` to 0.75 in `DefaultConfig()` |

## State Machine

```
                    ┌──────────────┐
                    │   Enrolled   │
                    │  (cert valid)│
                    └──────┬───────┘
                           │ renewal tick: CertificateNeedsRenewal() == true
                           ▼
              ┌───►┌──────────────┐
              │    │  CSR Sent    │
              │    │  (awaiting   │
              │    │   response)  │
              │    └──┬────────┬──┘
              │       │        │
              │       │ recv   │ timeout (2 min)
              │       ▼        │
              │    ┌────────┐  │
              │    │Validate│  │
              │    │Response│  │
              │    └─┬────┬─┘  │
              │   ok │    │bad │
              │      │    │    │
              │      ▼    ▼    ▼
              │    ┌────┐ ┌──────┐
              │    │Done│ │Retry │
              │    │    │ │(bkof)│
              │    └──┬─┘ └──┬───┘
              │       │      │
              │       ▼      │
              │    ┌──────┐  │
              └────│Enrold│◄─┘
                   │(new) │ (retry loops back to
                   └──────┘  CSR Sent on next tick)

Done = persist cert, restart collector, reload own-logs
```
