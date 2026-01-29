# CSR-Based Trust Bootstrap Design

## Overview

This design replaces server-issued JWTs with a CSR-based flow where supervisors generate their own keypairs and obtain server-signed certificates. This provides:

1. **Supervisor-owned identity** - Private keys never leave the supervisor
2. **mTLS infrastructure** - Certificates can authenticate OTLP telemetry connections
3. **Config encryption** (future) - Server can encrypt configs for specific supervisors
4. **Local encryption** (future) - Supervisor can encrypt config files at rest

**Key changes from previous design:**
- Enrollment URL replaces endpoint-in-JWT (visible, inspectable URLs)
- Supervisor generates Ed25519 (signing) + X25519 (encryption) keypairs
- Server signs certificate containing supervisor's public keys
- Supervisor authenticates with self-signed JWTs (server validates via certificate)

## Enrollment Flow

### Enrollment URL Format

Users receive a URL containing the enrollment JWT:
```
https://opamp.example.com/opamp/enroll/eyJhbGciOiJFZDI1NTE5...
```

Benefits:
- URL is visible and inspectable - users can spot malicious hostnames
- HTTPS connection provides trust anchor via system CA store
- No chicken-and-egg problem validating JWT that contains its own endpoint

### Enrollment JWT Structure

```json
Header:
{
  "alg": "EdDSA",
  "typ": "JWT",
  "kid": "<key-id>"
}

Claims:
{
  "iss": "opamp.example.com",
  "exp": 1706140800,
  "tenant_id": "acme-corp",
  "key_algorithm": "Ed25519",
  "agent_labels": {
    "environment": "production",
    "region": "eu-west-1"
  }
}
```

### Enrollment Steps

```
1. User provides enrollment URL to supervisor
2. Supervisor extracts hostname from URL
3. Supervisor fetches https://<hostname>/.well-known/jwks.json
4. Supervisor validates JWT signature against JWKS (matched by kid)
5. If valid, supervisor extracts claims and proceeds to CSR flow
6. Supervisor connects to OpAMP endpoint with enrollment JWT as bearer token
```

### JWKS Endpoint

Server exposes public keys at `/.well-known/jwks.json`:
```json
{
  "keys": [
    {
      "kty": "OKP",
      "crv": "Ed25519",
      "kid": "enrollment-key-2024",
      "x": "<base64url-public-key>",
      "use": "sig"
    }
  ]
}
```

## CSR Flow & Certificate Issuance

### Keypair Generation

On first enrollment, supervisor generates two keypairs:

| Purpose | Algorithm | Usage |
|---------|-----------|-------|
| Signing | Ed25519 | Sign JWTs for authentication, sign CSR |
| Encryption | X25519 | Future: decrypt configs from server, encrypt local files |

### CSR Content

Supervisor creates a CSR containing:
- **Subject CN**: Instance UID (e.g., `CN=01HQ3K5V7X2M4N8P9R0S1T2U3V`)
- **Public Key**: Ed25519 signing key
- **Custom Extension**: X25519 encryption public key

### CSR Flow (per OpAMP spec)

```
         Supervisor                                    Server

              │  (1) Connect with enrollment JWT          │
              ├──────────────────────────────────────────►│
              │                                           │
┌───────────┐ │                                           │
│ Generate  │ │  (2) AgentToServer{CSR}                   │
│ keypairs  ├─┼──────────────────────────────────────────►│
│ and CSR   │ │                                           │
└───────────┘ │                                           │
              │                                           │  ┌──────────┐
              │                                           │  │ Validate │
              │                                           ├─►│ & Sign   │
              │                                           │  │ Cert     │
              │                                           │  └────┬─────┘
              │                                           │       │
              │  (3) ServerToAgent{ConnectionSettings     │◄──────┘
              │       with certificate}                   │
              │◄──────────────────────────────────────────┤
              │                                           │
              │  (4) Disconnect                           │
              ├─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─►│
              │                                           │
              │  (5) Reconnect with supervisor-signed JWT │
              ├──────────────────────────────────────────►│
              │                                           │
```

### Issued Certificate Content

Server signs a certificate containing:

| Field | Value |
|-------|-------|
| Subject CN | Instance UID |
| Subject O | Tenant ID |
| Public Key | Ed25519 signing key |
| Custom Extension (OID TBD) | X25519 encryption public key |
| Validity | Server-configured (e.g., 90 days, 1 year) |

## Supervisor Authentication (Post-Enrollment)

### Supervisor-Signed JWT

After receiving its certificate, the supervisor authenticates by signing its own JWTs:

```json
Header:
{
  "alg": "EdDSA",
  "typ": "JWT",
  "x5t#S256": "<cert-sha256-fingerprint>"
}

Claims:
{
  "sub": "01HQ3K5V7X2M4N8P9R0S1T2U3V",
  "aud": "opamp.example.com",
  "iat": 1704153600,
  "exp": 1704153900
}
```

| Field | Purpose |
|-------|---------|
| `x5t#S256` | Certificate fingerprint - enables O(1) lookup of public key |
| `sub` | Instance UID - identity being asserted |
| `aud` | Server DNS name - prevents token reuse against other servers |
| `iat/exp` | Short validity window (default: 5 minutes, configurable) |

### Authentication Flow

```
1. Supervisor generates fresh JWT (signed with Ed25519 private key)
2. Connects to OpAMP endpoint with JWT as bearer token
3. Server extracts cert fingerprint from JWT header
4. Server looks up certificate by fingerprint
5. Server validates JWT signature against certificate's public key
6. Server verifies sub matches certificate's CN
7. Connection authenticated
```

### JWT Lifetime

- Default: 5 minutes (configurable)
- New JWT generated per connection attempt
- Short lifetime limits replay window if token is intercepted

### Certificate Renewal

When certificate approaches expiration:
1. Supervisor reuses existing keypairs
2. Generates new CSR (same keys, fresh request)
3. Sends CSR over authenticated connection
4. Server issues new certificate
5. Supervisor stores new certificate, continues operating

## Collector Authentication (OTLP)

The supervisor's credentials can authenticate collector telemetry to OTLP endpoints. Two methods available:

### Method 1: mTLS (Client Certificate)

Supervisor writes credentials to files, injects paths into collector config:

```
<persistence_dir>/keys/
├── signing.key          # Ed25519 private key (PEM)
├── signing.crt          # Certificate (PEM)
└── encryption.key       # X25519 private key (PEM, future)
```

Injected into collector's OTLP exporter config:
```yaml
exporters:
  otlp:
    endpoint: "otlp.example.com:4317"
    tls:
      cert_file: "/var/lib/supervisor/keys/signing.crt"
      key_file: "/var/lib/supervisor/keys/signing.key"
```

### Method 2: Bearer Token (Supervisor-Signed JWT)

For endpoints that don't support mTLS:

1. Supervisor generates JWT (same structure as its own auth JWT)
2. Writes JWT to file: `<persistence_dir>/keys/bearer_token`
3. Periodically refreshes before expiration
4. Collector reads token from file

Injected into collector config:
```yaml
exporters:
  otlp:
    endpoint: "otlp.example.com:4317"
    headers:
      authorization: "${file:/var/lib/supervisor/keys/bearer_token}"
```

### Credential File Permissions

| File | Permissions | Notes |
|------|-------------|-------|
| `signing.key` | 0600 | Private key - supervisor + collector only |
| `signing.crt` | 0644 | Public certificate |
| `encryption.key` | 0600 | Private key |
| `bearer_token` | 0600 | Sensitive credential |

## Config Encryption (Future - Capability Gated)

### Overview

Optional feature announced via capability flag. When enabled:
- Server encrypts collector configs before sending
- Supervisor decrypts using its X25519 private key
- Supervisor can also encrypt configs at rest

### Encryption Algorithm

X25519 ECDH + AES-256-GCM (ECIES-style hybrid encryption):

```
Encrypt (Server):
1. Generate ephemeral X25519 keypair
2. ECDH(ephemeral_private, supervisor_public) → shared_secret
3. HKDF(shared_secret, "opamp-config-encryption") → aes_key
4. AES-256-GCM(plaintext, aes_key) → ciphertext + tag
5. Output: ephemeral_public || nonce || ciphertext || tag

Decrypt (Supervisor):
1. ECDH(supervisor_private, ephemeral_public) → shared_secret
2. HKDF(shared_secret, "opamp-config-encryption") → aes_key
3. AES-256-GCM decrypt → plaintext
```

### Wire Format (Envelope)

Encrypted config in AgentConfigMap uses envelope with metadata:

```yaml
# Config name indicates encryption: "collector.yaml.enc"
encrypted_config:
  version: 1
  algorithm: "X25519-AES256GCM"
  key_fingerprint: "<supervisor-x25519-pubkey-fingerprint>"
  ephemeral_public: "<base64>"
  nonce: "<base64>"
  ciphertext: "<base64>"
  tag: "<base64>"
```

### Capability Advertisement

Supervisor advertises encryption support in AgentDescription:

```
capabilities: ... | AcceptsEncryptedConfig
```

Server only encrypts when capability is present - backward compatible with supervisors that don't support encryption.

### At-Rest Encryption

Supervisor can encrypt stored configs using same algorithm:
- Encrypts to its own X25519 public key
- Decrypts on load using private key
- Protects configs if disk is compromised

## Key Storage

### Directory Structure

```
<persistence_dir>/
├── keys/
│   ├── signing.key      # Ed25519 private key (PEM or PKCS#8)
│   ├── signing.crt      # Certificate from server (PEM)
│   ├── encryption.key   # X25519 private key (PEM or PKCS#8)
│   └── bearer_token     # Current JWT for collector (refreshed periodically)
├── auth/
│   └── (enrollment token - removed after successful enrollment)
└── ...
```

### Key Format

Standard PEM files, optionally PKCS#8 encrypted:

```
# Unencrypted (default)
-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIL...
-----END PRIVATE KEY-----

# PKCS#8 Encrypted (optional)
-----BEGIN ENCRYPTED PRIVATE KEY-----
MIGbMFcGCSqGSIb3DQEFDTBKMCkGCSqGSIb3DQEFDDAcBAi...
-----END ENCRYPTED PRIVATE KEY-----
```

### Passphrase Sources

For PKCS#8 encrypted keys, passphrase can come from:

| Source | Configuration |
|--------|---------------|
| Environment variable | `key_passphrase_env: "SUPERVISOR_KEY_PASSPHRASE"` |
| File | `key_passphrase_file: "/run/secrets/key-passphrase"` |
| External command | `key_passphrase_cmd: ["vault", "read", "-field=passphrase", "secret/supervisor"]` |

### Configuration Example

```yaml
keys:
  dir: /var/lib/supervisor/keys
  encrypted: true  # Enable PKCS#8 encryption
  passphrase:
    # One of:
    env: "SUPERVISOR_KEY_PASSPHRASE"
    # file: "/run/secrets/key-passphrase"
    # cmd: ["vault", "read", "-field=passphrase", "secret/supervisor"]
```

### File Permissions

| File | Permissions | Notes |
|------|-------------|-------|
| `keys/` directory | 0700 | Restricted directory |
| `*.key` | 0600 | Private keys |
| `*.crt` | 0644 | Public certificate |
| `bearer_token` | 0600 | Sensitive |

## Configuration & State Machine

### Supervisor Configuration

```yaml
server:
  # Enrollment URL provided at first run (contains JWT)
  enrollment_url: "https://opamp.example.com/opamp/enroll/eyJhbGciOi..."

  # Or endpoint for subsequent runs (derived from enrollment URL)
  endpoint: "wss://opamp.example.com/v1/opamp"

keys:
  dir: /var/lib/supervisor/keys
  encrypted: false
  passphrase:
    env: "SUPERVISOR_KEY_PASSPHRASE"
    # file: "/run/secrets/key-passphrase"
    # cmd: ["vault", "read", "..."]

auth:
  jwt_lifetime: 5m  # Supervisor-signed JWT validity (default: 5m)
```

### State Machine

```
                    ┌─────────────────────────────────────────┐
                    │                                         │
                    ▼                                         │
              ┌──────────┐                                    │
              │   INIT   │                                    │
              └────┬─────┘                                    │
                   │                                          │
         Has keys & cert?                                     │
          /            \                                      │
        No              Yes                                   │
        /                \                                    │
       ▼                  ▼                                   │
┌─────────────┐    ┌─────────────┐                           │
│  ENROLLING  │    │ OPERATIONAL │◄──────────────────────────┤
└──────┬──────┘    └──────┬──────┘                           │
       │                  │                                   │
       │ 1. Fetch JWKS    │ Connect with                     │
       │ 2. Validate JWT  │ supervisor-signed JWT            │
       │ 3. Generate keys │          │                       │
       │ 4. Send CSR      │          ▼                       │
       │ 5. Receive cert  │   ┌─────────────┐                │
       │                  │   │  CONNECTED  │                │
       ▼                  │   └──────┬──────┘                │
┌─────────────┐           │          │                       │
│  ENROLLED   │───────────┘          │ Cert expiring?        │
└─────────────┘                      │        │              │
                                     │       Yes             │
                                     │        │              │
                                     │        ▼              │
                                     │ ┌─────────────┐       │
                                     │ │  RENEWING   │───────┘
                                     │ │  (new CSR)  │
                                     │ └─────────────┘
                                     │
                                     │ Disconnect?
                                     ▼
                              ┌─────────────┐
                              │RECONNECTING │───► OPERATIONAL
                              └─────────────┘
```

### Persistence After Enrollment

| File | Created | Updated |
|------|---------|---------|
| `keys/signing.key` | Enrollment | Never (unless compromised) |
| `keys/signing.crt` | Enrollment | Certificate renewal |
| `keys/encryption.key` | Enrollment | Never |
| `keys/bearer_token` | First OTLP use | Periodically refreshed |
| `connection.yaml` | First connect | Each reconnect |

## Implementation Notes

### Changes from Previous Design

| Aspect | Previous Design | New Design |
|--------|-----------------|------------|
| Enrollment | JWT contains endpoint | URL contains JWT; endpoint visible |
| Trust anchor | CA fingerprint in JWT | HTTPS + system CA store |
| Identity | Server-issued JWT | Supervisor-generated keypairs + server-signed cert |
| Authentication | Server JWT as bearer | Supervisor-signed JWT with cert fingerprint |
| Key ownership | Server holds signing authority | Supervisor holds private keys |
| OTLP auth | Not addressed | mTLS or supervisor-signed JWT |
| Config encryption | Not addressed | Future: X25519 + AES-GCM |

### Implementation Phases

| Phase | Scope |
|-------|-------|
| 1 | Enrollment URL + JWKS validation |
| 2 | Keypair generation + CSR flow |
| 3 | Supervisor-signed JWT authentication |
| 4 | Certificate renewal |
| 5 | Collector credential injection (mTLS + JWT) |
| 6 | PKCS#8 encrypted key storage |
| 7 (future) | Config encryption (capability-gated) |

### Library Requirements

| Language | Primitives | Source |
|----------|------------|--------|
| Go | Ed25519 | `crypto/ed25519` (stdlib) |
| Go | X25519 | `golang.org/x/crypto/curve25519` |
| Go | AES-GCM | `crypto/aes` + `crypto/cipher` (stdlib) |
| Go | HKDF | `golang.org/x/crypto/hkdf` |
| Java | Ed25519 | JCA (Java 15+) |
| Java | X25519 | JCA (Java 11+) |
| Java | AES-GCM | JCA |
