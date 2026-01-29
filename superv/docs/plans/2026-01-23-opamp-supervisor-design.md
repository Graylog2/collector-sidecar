# OpAMP Supervisor Design

## Overview

A production-ready OpAMP supervisor that manages an OpenTelemetry Collector with full remote management capabilities.

**Dual OpAMP role:**
- **Upstream:** OpAMP client connecting to management server (WebSocket + HTTP)
- **Downstream:** OpAMP server for the managed collector

## Architecture

```
                         ┌─────────────────────────────────────────────┐
                         │               Supervisor                    │
                         │                                             │
    ┌──────────┐         │  ┌─────────────┐       ┌─────────────────┐ │         ┌───────────┐
    │  OpAMP   │◄───────►│  │   OpAMP     │       │   OpAMP Server  │ │◄───────►│ Collector │
    │  Server  │         │  │   Client    │       │   (localhost)   │ │         │ (child)   │
    └──────────┘         │  └──────┬──────┘       └────────┬────────┘ │         └───────────┘
                         │         │                       │          │
                         │         └───────┬───────────────┘          │
                         │                 ▼                          │
                         │         ┌───────────────┐                  │
                         │         │  Core Engine  │                  │
                         │         └───────┬───────┘                  │
                         │                 │                          │
                         │    ┌────────────┼────────────┐             │
                         │    ▼            ▼            ▼             │
                         │ ┌──────┐  ┌──────────┐  ┌─────────┐        │
                         │ │Config│  │Credential│  │ Package │        │
                         │ │Layer │  │ Manager  │  │ Manager │        │
                         │ └──────┘  └──────────┘  └─────────┘        │
                         │                                             │
                         │              Persistence                    │
                         │    /var/lib/supervisor/ (YAML files)       │
                         └─────────────────────────────────────────────┘
```

### Message Flow

**Upstream (Supervisor → OpAMP Server):**
- Combined status (supervisor + collector health)
- Agent description (merged capabilities)
- Effective configuration
- Package statuses
- Custom messages (relayed from collector)

**Downstream (Collector → Supervisor):**
- Collector's own capabilities and status
- Health information
- Custom messages (relayed upstream)
- Effective config confirmation

## Core Features

### 1. Transport

| Direction | Protocols | Notes |
|-----------|-----------|-------|
| Upstream (to server) | WebSocket, HTTP | Configurable at runtime |
| Downstream (to collector) | WebSocket | Localhost only |

### 2. Authentication & Trust Bootstrap

> **Note:** This section has been superseded by the CSR-based trust bootstrap design.
> See [2026-01-29-csr-trust-bootstrap-design.md](2026-01-29-csr-trust-bootstrap-design.md) for the current design.

#### Summary of New Design

The new design uses supervisor-generated keypairs with server-signed certificates:

1. **Enrollment URL** - User receives `https://server.example.com/opamp/enroll/<JWT>`
2. **JWKS Validation** - Supervisor fetches `/.well-known/jwks.json` to validate enrollment JWT
3. **Keypair Generation** - Supervisor generates Ed25519 (signing) + X25519 (encryption) keypairs
4. **CSR Flow** - Supervisor sends CSR, server returns signed certificate
5. **Self-Signed JWT Auth** - Supervisor authenticates with self-signed JWTs (cert fingerprint in header)

Benefits:
- Private keys never leave the supervisor
- Visible enrollment URLs (users can inspect hostnames)
- mTLS infrastructure for OTLP telemetry authentication
- Future: server-side config encryption using supervisor's public key

### 3. Configuration Management

#### Layer Precedence (later wins)

```
1. Remote config from server (AgentConfigMap)
      ↓
2. Supervisor-injected settings (OpAMP endpoint, instance ID, etc.)
      ↓
3. Local override file (compliance, security policies)
      ↓
   Final merged config → written to disk → collector loads
```

#### AgentConfigMap Handling

Server can send multiple named configs:
```protobuf
AgentConfigMap {
  config_map: {
    "collector.yaml": <main config>,
    "receivers.yaml": <additional receivers>,
    "exporters.yaml": <additional exporters>
  }
}
```

#### Supervisor Injections (automatic)

```yaml
extensions:
  opamp:
    server:
      ws:
        endpoint: "localhost:4320"
    instance_uid: "{{.InstanceUID}}"

service:
  extensions: [opamp, ...]
```

#### Local Compliance Overrides

```yaml
# /etc/supervisor/compliance.yaml
processors:
  filter:
    logs:
      exclude:
        match_type: regexp
        record_attributes:
          - key: "user.email"
            value: ".*"

exporters:
  otlp:
    tls:
      insecure: false
      min_version: "1.3"
```

Merge strategy: deep merge with silent override (compliance wins).

### 4. Process Management

#### Lifecycle Operations

| Operation | Method |
|-----------|--------|
| Start | Write config → spawn collector → wait for health |
| Stop | SIGTERM → wait for graceful shutdown → SIGKILL if timeout |
| Reload | Write new config → SIGHUP (Unix) or restart (Windows) |

#### Platform Support

| Platform | Reload Method | Notes |
|----------|---------------|-------|
| Linux | SIGHUP | Native support |
| macOS | SIGHUP | Native support |
| FreeBSD | SIGHUP | Native support |
| Windows | Named event or restart | Fallback to restart if needed |

#### Health Monitoring

Two sources:
1. **OpAMP status** - Collector reports via local OpAMP connection
2. **Health endpoint** - Supervisor polls collector's health extension as backup

#### Crash Recovery

Automatic restart with configurable backoff:
- Exponential backoff (e.g., 1s, 2s, 4s, 8s, 16s)
- Maximum retry count
- Backoff reset after stable operation

### 5. Package Management

#### Package Flow

```
Server sends PackagesAvailable
    ↓
Supervisor compares with current PackageStatuses
    ↓
For each new/updated package:
    → Verify server attestation signature
    → Download from provided URL
    → Verify hash matches attested hash
    → Verify publisher signature (if enabled)
    → Stage in temp location
    → Apply (move to final location)
    → Update PackageStatuses
    ↓
If collector binary updated:
    → Graceful collector shutdown
    → Replace binary
    → Restart collector
```

#### Verification (Mandatory)

**Server attestation (always required):**

The server signs an attestation containing the package metadata:

```yaml
attestation:
  package_name: "otelcol"
  version: "0.98.0"
  hash: "sha256:abc123..."
  download_url: "https://github.com/..."
  issued_at: "2024-01-15T10:00:00Z"

signature:
  algorithm: "ed25519"
  key_id: "server-signing-key-2024"
  value: "base64-encoded-signature..."
```

Trust chain: CA Cert → Server attestation signing key → Attestation (hash + metadata)

**Publisher signature (optional, defense in depth):**

Verify original publisher's signature for packages from external sources:

```yaml
packages:
  verification:
    publisher_signature:
      enabled: true
      format: cosign  # cosign | gpg | minisign
      trusted_keys:
        - /etc/supervisor/keys/otel-release.pub
```

#### Package Storage

```
/var/lib/supervisor/packages/
├── otelcol/
│   ├── current -> v0.98.0/
│   ├── v0.98.0/
│   │   └── otelcol
│   └── v0.97.0/      # Previous version for rollback
│       └── otelcol
└── staging/          # Downloads verified here first
```

#### Rollback Support

- Keep N previous versions
- Auto-rollback if new binary crashes on start

### 6. Message Relay (Supervisor ↔ Collector)

#### Local OpAMP Server

Supervisor runs a local OpAMP server for the collector on localhost.

#### Message Handling

| From Collector | Supervisor Action |
|----------------|-------------------|
| AgentDescription | Merge with own description, report upstream |
| EffectiveConfig | Include in upstream status |
| RemoteConfigStatus | Forward to upstream server |
| PackageStatuses | Forward to upstream server |
| Health | Aggregate with process health |
| CustomCapabilities | Register, prepare to relay messages |
| CustomMessage | Relay to upstream server |

| From Upstream Server | Supervisor Action |
|----------------------|-------------------|
| RemoteConfig | Apply supervisor config, write collector config, notify collector |
| PackagesAvailable | Download packages, may include collector binary updates |
| CustomMessage (for collector) | Relay to collector |
| ConnectionSettingsOffers | Store new certificate (during enrollment/renewal) |

#### Capability Merging

Upstream server sees combined capabilities:
```
supervisor_capabilities ∪ collector_capabilities
```

### 7. Offline Operation

#### Startup Behavior

```
Has cached config?
    ├─ No  → Require server connection (first run must enroll)
    └─ Yes → Start collector with cached config
                 ↓
             Attempt server connection (async)
                 ↓
             Server reachable?
                 ├─ Yes → Normal operation
                 └─ No  → Continue with cached config, retry with backoff
```

#### Offline Capabilities

| Feature | Offline Behavior |
|---------|------------------|
| Collector operation | Runs with last known config |
| Health monitoring | Continues locally |
| Crash recovery | Automatic restart works |
| Config changes | Not available (no server) |
| Package updates | Not available |
| Status reporting | Queued, sent when reconnected |

#### Reconnection Strategy

Exponential backoff with configurable:
- Initial delay
- Maximum delay
- Multiplier

### 8. State Persistence

#### Directory Structure

```
/var/lib/supervisor/
├── instance_uid.yaml       # Immutable - never modified after creation
├── agent_description.yaml  # Mutable - may update over time
├── connection.yaml         # Mutable - connection state
├── keys/                   # Keypairs and certificate (see CSR design)
│   ├── signing.key         # Ed25519 private key (PEM or PKCS#8)
│   ├── signing.crt         # Certificate from server (PEM)
│   ├── encryption.key      # X25519 private key (PEM or PKCS#8)
│   └── bearer_token        # JWT for collector OTLP auth
├── config/
│   ├── supervisor.yaml     # Last known supervisor config
│   ├── collector.yaml      # Last applied collector config (merged)
│   └── remote/             # Raw configs from server (AgentConfigMap)
│       ├── collector.yaml
│       └── receivers.yaml
├── packages/
│   └── ...                 # Package storage
└── state/
    └── package_statuses.yaml
```

#### File Definitions

**instance_uid.yaml (write-once, immutable):**
```yaml
instance_uid: "01HQ3K5V7X2M4N8P9R0S1T2U3V"
created_at: "2024-01-15T10:30:00Z"
```

**agent_description.yaml (mutable):**
```yaml
identifying_attributes:
  service.name: "otel-collector"
  service.namespace: "production"
  host.name: "node-42.example.com"

non_identifying_attributes:
  os.type: "linux"
  os.version: "6.1.0"
  supervisor.version: "1.0.0"

last_updated: "2024-01-15T12:00:00Z"
```

**connection.yaml:**
```yaml
server:
  endpoint: "wss://opamp.example.com/v1/opamp"
  last_connected: "2024-01-15T12:00:00Z"
  last_sequence_num: 42

remote_config:
  hash: "sha256:abc123..."
  received_at: "2024-01-15T11:55:00Z"
  status: APPLIED
```

**keys/ directory:**

See [2026-01-29-csr-trust-bootstrap-design.md](2026-01-29-csr-trust-bootstrap-design.md) for key storage details.

#### File Permissions

| File | Created | Modified | Permissions |
|------|---------|----------|-------------|
| instance_uid.yaml | First startup | Never | 0444 (read-only) |
| agent_description.yaml | First startup | As needed | 0644 |
| connection.yaml | First startup | Frequently | 0600 |
| keys/ directory | Enrollment | - | 0700 |
| keys/*.key | Enrollment | Never | 0600 |
| keys/*.crt | Enrollment | Cert renewal | 0644 |
| keys/bearer_token | First OTLP use | Periodically | 0600 |

### 9. Supervisor Configuration

Configuration sources with precedence: CLI flags > environment variables > config file

#### Example Configuration

```yaml
# /etc/supervisor/config.yaml

server:
  endpoint: "${OPAMP_ENDPOINT}"  # Or from JWT
  transport: websocket           # websocket | http | auto

  connection:
    retry_backoff:
      initial: 1s
      max: 5m
      multiplier: 2

auth:
  # Enrollment URL provided at first run
  enrollment_url: "${ENROLLMENT_URL}"  # e.g., https://server.example.com/opamp/enroll/<JWT>
  jwt_lifetime: 5m                     # Supervisor-signed JWT validity

keys:
  dir: /var/lib/supervisor/keys
  encrypted: false
  passphrase:
    env: "SUPERVISOR_KEY_PASSPHRASE"

local_opamp:
  endpoint: localhost:4320

collector:
  executable: /usr/local/bin/otelcol
  args: ["--config", "{{.ConfigPath}}"]

  config:
    merge_strategy: deep
    local_overrides:
      - /etc/supervisor/compliance.yaml

  health:
    endpoint: http://localhost:13133/health
    interval: 10s
    timeout: 5s

  reload:
    method: auto                 # auto | signal | restart
    windows_reload_event: "otelcol-reload"
    restart_on_reload_failure: true

  restart:
    max_retries: 5
    backoff: [1s, 2s, 4s, 8s, 16s]

  shutdown:
    graceful_timeout: 30s

packages:
  storage_dir: /var/lib/supervisor/packages
  keep_versions: 2

  verification:
    publisher_signature:
      enabled: false
      format: cosign
      trusted_keys:
        - /etc/supervisor/keys/otel-release.pub

persistence:
  dir: /var/lib/supervisor

logging:
  format: json                   # json | text
  level: info                    # debug | info | warn | error
```

### 10. Observability

- Structured JSON logging
- Configurable log levels
- Logs include: connection events, config changes, package operations, health status changes

## Bootstrap Modes

**Zero-touch (enrollment URL):**
```bash
supervisor --enrollment-url "https://opamp.example.com/opamp/enroll/eyJ..."
```

**Standard (config file, already enrolled):**
```bash
supervisor --config /etc/supervisor/config.yaml
```

**Hybrid (config + enrollment URL from env):**
```bash
ENROLLMENT_URL="https://..." supervisor --config /etc/supervisor/config.yaml
```

## Platform Support

### Build Targets

| OS | Architectures | Status |
|----|---------------|--------|
| Linux | amd64, arm64 | Primary |
| macOS | amd64, arm64 | Primary |
| Windows | amd64, arm64 | Primary |
| FreeBSD | amd64, arm64 | Community supported |

### Platform Abstractions

```go
type ProcessController interface {
    Start(executable string, args []string) error
    Stop(graceful time.Duration) error
    Reload() error  // SIGHUP on Unix, alternative on Windows
    IsRunning() bool
}
```

## Security Considerations

1. **Private keys** - Stored with 0600 permissions, never logged, optionally PKCS#8 encrypted
2. **Enrollment tokens** - Short-lived, discarded after successful CSR flow
3. **Supervisor-signed JWTs** - Short-lived (default 5m), generated per connection
4. **Package verification** - Mandatory server attestation, optional publisher signatures
5. **TLS** - Required for upstream connection, optional for localhost
6. **Compliance overrides** - Cannot be modified via OpAMP, local control only
7. **Config encryption** (future) - Server can encrypt configs for specific supervisors
