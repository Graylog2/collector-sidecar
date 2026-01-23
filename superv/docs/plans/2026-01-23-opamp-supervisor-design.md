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

#### JWT-Based Bootstrap

The enrollment JWT contains everything needed for zero-touch provisioning:

```
JWT Claims:
{
  "iss": "opamp.example.com",
  "exp": 1706140800,
  "endpoint": "wss://opamp.example.com/v1/opamp",
  "tenant_id": "acme-corp",
  "agent_labels": {
    "environment": "production",
    "region": "eu-west-1"
  },
  "ca_fingerprint": "sha256:abc123..."
}
```

#### Trust Models

| Mode | Trust Source | Use Case |
|------|--------------|----------|
| Fingerprint (default) | CA fingerprint in JWT | Standard deployment |
| CA-verified | Pre-deployed CA cert | High-security environments |

**Fingerprint mode flow:**
1. Supervisor receives JWT via secure channel
2. Connects to endpoint from JWT
3. Server presents TLS certificate
4. Supervisor verifies CA cert fingerprint matches JWT claim
5. Proceeds with enrollment

**CA-verified mode flow:**
1. Supervisor loads pre-deployed CA cert
2. Validates JWT signature against CA's public key
3. Connects with full TLS chain validation

#### Authentication Lifecycle

**Phase 1: Enrollment (first run)**
```
Supervisor starts with enrollment_token (JWT)
    → Connects to server with enrollment JWT as bearer token
    → Server validates enrollment token, accepts connection
    → Server generates agent-specific JWT
    → Server sends ConnectionSettingsOffers with new headers containing agent JWT
    → Supervisor stores agent JWT, discards enrollment token
    → Supervisor reconnects using agent JWT
```

**Phase 2: Normal operation (subsequent runs)**
```
Supervisor starts with stored agent JWT
    → Connects with agent JWT in Authorization header
    → Server may send new ConnectionSettingsOffers to refresh token
    → Supervisor updates stored token when refreshed
```

**Agent JWT structure:**
```json
{
  "sub": "<instance-uid>",
  "iss": "opamp-server",
  "aud": "opamp-agent",
  "tenant_id": "acme-corp",
  "iat": 1704153600,
  "exp": 1735689600
}
```

**Token refresh options:**
1. **Long-lived tokens** - Agent JWT valid for extended periods
2. **Server-initiated refresh** - Server sends new `ConnectionSettingsOffers` before expiry
3. **Agent-initiated refresh** - Agent requests new token when approaching expiry

**Future option: mTLS with CSR**

For high-security environments, mTLS can be added later:
- Agent generates ed25519 keypair
- Agent sends CSR via `ConnectionSettingsRequest`
- Server returns signed certificate
- Agent reconnects with mTLS

This is not implemented in the initial version but the design supports it.

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
| ConnectionSettingsOffers | Store new agent JWT, reconnect with updated credentials |

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
├── auth/
│   └── agent_token.yaml    # Agent JWT (0600 permissions)
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

**agent_token.yaml (mutable, secure):**
```yaml
token: "eyJhbGciOiJFZDI1NTE5IiwidHlwIjoiSldUIn0..."
received_at: "2024-01-15T10:30:00Z"
expires_at: "2025-01-15T10:30:00Z"
```

#### File Permissions

| File | Created | Modified | Permissions |
|------|---------|----------|-------------|
| instance_uid.yaml | First startup | Never | 0444 (read-only) |
| agent_description.yaml | First startup | As needed | 0644 |
| connection.yaml | First startup | Frequently | 0600 |
| agent_token.yaml | Enrollment | Token refresh | 0600 |

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

bootstrap:
  mode: fingerprint              # fingerprint | ca_verified
  # ca_cert: /etc/supervisor/bootstrap-ca.crt  # For ca_verified mode

auth:
  enrollment_token: "${ENROLLMENT_JWT}"
  token_file: /var/lib/supervisor/auth/agent_token.yaml

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

**Zero-touch (JWT only):**
```bash
supervisor --bootstrap-token "eyJ..."
```

**Standard (config file):**
```bash
supervisor --config /etc/supervisor/config.yaml
```

**Hybrid (config + JWT from env):**
```bash
ENROLLMENT_JWT="eyJ..." supervisor --config /etc/supervisor/config.yaml
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

1. **Agent tokens** - Stored with 0600 permissions, never logged
2. **Enrollment tokens** - Short-lived, discarded after use
3. **Package verification** - Mandatory server attestation, optional publisher signatures
4. **TLS** - Required for upstream connection, optional for localhost
5. **Compliance overrides** - Cannot be modified via OpAMP, local control only
