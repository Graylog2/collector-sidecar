# OpAMP Spec Compliance Design

Date: 2026-02-02
Status: Draft
Context: Addressing gaps identified in `docs/2026-02-02-opamp-supervisor-review.md`

## Overview

This design addresses OpAMP specification gaps for Graylog integration. The Graylog server uses remote config, health monitoring, enrollment, and full connection settings. We're implementing missing features and cleaning up implementation quality issues.

## Scope

### High Priority: Connection Settings

Full handling of `OpAMPConnectionSettings` message fields.

#### Settings to Handle

| Field | Action |
|-------|--------|
| `destination_endpoint` | Update endpoint, reconnect client |
| `headers` | Update headers, reconnect client |
| `certificate` | Update client cert for mTLS, reconnect |
| `ca_certificate` | Update CA for server verification, reconnect |
| `proxy_settings` | Update proxy URL, reconnect |
| `heartbeat_interval_seconds` | Update heartbeat ticker (no reconnect) |

#### Implementation Flow

```
OnOpampConnectionSettings(ctx, settings):
    1. Snapshot current connection config (for rollback)

    2. Extract new values from settings:
       - endpoint := settings.OpampConnectionSettings.DestinationEndpoint
       - headers := settings.OpampConnectionSettings.Headers
       - clientCert := settings.OpampConnectionSettings.Certificate
       - caCert := settings.OpampConnectionSettings.CaCertificate
       - proxy := settings.OpampConnectionSettings.ProxySettings
       - heartbeatSec := settings.OpampConnectionSettings.HeartbeatIntervalSeconds

    3. If heartbeat interval changed:
       - Update heartbeat ticker (no reconnect)

    4. If endpoint/headers/TLS/proxy changed:
       - Stop current OpAMP client
       - Build new client config with updated values
       - Attempt to start new client
       - If start fails:
           - Restore snapshot config
           - Restart client with old config
           - Report ConnectionSettingsStatus with error
           - Return

    5. Persist new config to disk (only after successful reconnect)

    6. Report ConnectionSettingsStatus with success
```

#### Persistence Strategy

- Keep old config on disk until new connection is verified working
- Only overwrite once new client connects successfully
- If supervisor crashes mid-transition, restarts with last known-good config
- No backup files needed - persist-on-success provides automatic rollback

#### Status Reporting

| Scenario | Status | Error Message |
|----------|--------|---------------|
| All settings applied, reconnect succeeded | SUCCESS | (empty) |
| Heartbeat-only change, applied | SUCCESS | (empty) |
| Reconnect failed, rolled back | FAILED | "connection failed: <reason>" |
| Invalid endpoint URL | FAILED | "invalid endpoint: <reason>" |
| TLS certificate parse error | FAILED | "invalid certificate: <reason>" |

```go
s.opampClient.SetConnectionSettingsStatus(&protobufs.ConnectionSettingsStatus{
    Status:       protobufs.ConnectionSettingsStatus_SUCCESS,
    ErrorMessage: "",
})
```

### Medium Priority: Heartbeat

#### Current State
No heartbeat capability or interval handling.

#### Implementation

Add `ReportsHeartbeat` to capabilities when starting the client.

```go
type Supervisor struct {
    // ... existing fields
    heartbeatTicker   *time.Ticker
    heartbeatInterval time.Duration  // default 30s
}

func (s *Supervisor) startHeartbeat() {
    s.heartbeatTicker = time.NewTicker(s.heartbeatInterval)
    go func() {
        for range s.heartbeatTicker.C {
            s.opampClient.SendHeartbeat()
        }
    }()
}

func (s *Supervisor) updateHeartbeatInterval(seconds uint64) {
    s.heartbeatInterval = time.Duration(seconds) * time.Second
    s.heartbeatTicker.Reset(s.heartbeatInterval)
}
```

Ticker updates without reconnecting - just changes ping frequency.

### Medium Priority: Available Components

#### Current State
Does not report available components.

#### Implementation

- Add `ReportsAvailableComponents` capability
- Query collector at startup via `--components` flag or build info endpoint
- Discover available receivers, processors, exporters, extensions
- Report via `AgentDescription.available_components`
- Re-query on collector restart if component set could change

### Medium Priority: Custom Messages

#### Current State
Does not handle custom messages.

#### Implementation

- Add `AcceptsCustomMessages` capability
- Forward `CustomMessage` from server to agent and vice versa
- Buffer messages during reconnection if needed
- Pass through `CustomCapabilities` declarations

### Low Priority: Cleanup

#### Instance UID Validation

Current `parseInstanceUID` accepts arbitrary strings and truncates/pads to 16 bytes. Since we generate valid UUIDs at creation time, this fallback is dead code.

Change:
- Remove fallback byte-copy logic
- Add strict validation: must be valid UUID, exactly 16 bytes
- Return error for invalid input instead of silently mangling

#### ReportsStatus Capability

Ensure `ReportsStatus` is always included in capabilities (spec requirement for all agents).

```go
func defaultCapabilities() protobufs.AgentCapabilities {
    return protobufs.AgentCapabilities_ReportsStatus |
           protobufs.AgentCapabilities_ReportsHeartbeat |
           // ... other capabilities
}
```

#### Broadcast Lock Scope

Current `Server.Broadcast` holds mutex while sending to each connection. Slow connections can stall others.

Change:
```go
func (s *Server) Broadcast(msg *protobufs.ServerToAgent) {
    s.mu.Lock()
    conns := make([]*connection, 0, len(s.connections))
    for _, c := range s.connections {
        conns = append(conns, c)
    }
    s.mu.Unlock()

    for _, c := range conns {
        c.Send(msg)  // outside lock
    }
}
```

#### Context Propagation

`SetEffectiveConfig` uses `context.Background()` when calling `UpdateEffectiveConfig`.

Change: Pass caller's context through to respect cancellation.

## File Changes

### `superv/opamp/client.go`
- Add `ReportsHeartbeat`, `ReportsAvailableComponents`, `AcceptsCustomMessages` to capabilities
- Enforce `ReportsStatus` is always set
- Add `SetConnectionSettingsStatus()` method
- Add `SendHeartbeat()` method
- Remove `parseInstanceUID` fallback, add strict validation
- Fix context propagation in `SetEffectiveConfig`

### `superv/supervisor/supervisor.go`
- Extend `OnOpampConnectionSettings` with full settings handling
- Add heartbeat ticker fields and goroutine management
- Add `updateHeartbeatInterval()` method
- Add rollback logic with config snapshot
- Add available components discovery on startup
- Add custom message forwarding

### `superv/persistence/connection.go` (new)
- Persistence for connection settings (endpoint, headers, TLS paths, proxy)
- Persist only after successful reconnect
- Load on startup to restore last-known-good settings

### `superv/opamp/server.go`
- Snapshot connections before broadcast to reduce lock hold time

### `superv/config/validate.go`
- Add validation for endpoint URL scheme (ws:// or wss://)
- Add TLS certificate validation helper

## Out of Scope

- Command field validation logging (observability only, not functional)
- `InsecureSkipVerify` warning logs (acceptable config escape hatch)

## Testing Strategy

- Unit tests for connection settings parsing and validation
- Unit tests for rollback on connection failure
- Unit tests for heartbeat ticker reset
- Integration test: server sends new endpoint, verify reconnect
- Integration test: server sends bad endpoint, verify rollback
- Integration test: verify `ConnectionSettingsStatus` reported correctly

## References

- Review document: `docs/2026-02-02-opamp-supervisor-review.md`
- OpAMP spec: `docs/opamp-specification.md`
- Reference implementation: `/home/bernd/graylog/sidecar/.src/opentelemetry-collector-contrib/cmd/opampsupervisor/`
