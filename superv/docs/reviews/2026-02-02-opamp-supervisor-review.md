# OpAMP Supervisor Code Review

Date: 2026-02-02
Scope: `superv/opamp/*.go`, `superv/supervisor/supervisor.go`, supporting paths `superv/healthmonitor/monitor.go`, `superv/auth/enrollment.go`.
Spec reference: `superv/docs/opamp-specification.md`.

## Executive Summary

The supervisor implementation uses opamp-go wrappers cleanly and integrates with health monitoring and enrollment flows. Several key gaps remain against the OpAMP specification, especially around instance UID requirements, connection settings management, and heartbeat reporting. There are also small Go idiomatic and concurrency concerns (error handling, validation, locking duration). These should be addressed before claiming full spec compliance.

## Spec Alignment Review

### Instance UID Requirements

Spec:
- AgentToServer.instance_uid MUST be 16 bytes.
- SHOULD be UUID v7.

Current implementation:
- `parseInstanceUID` accepts any string, attempts UUID parse, and if it fails, copies raw bytes into a `[16]byte`, potentially truncating or padding arbitrary input.
- `ClientConfig.Validate` does not enforce 16-byte length or UUID v7 format.

Impact:
- Non-compliant instance UID length and format can cause server-side state tracking issues or interoperability problems.

Files:
- `superv/opamp/client.go`

### Capabilities

Spec:
- AgentToServer.capabilities MUST be set.
- ReportsStatus capability MUST be set by all agents.
- Capabilities should be accurate to supported features.

Current implementation:
- Capabilities are configured, but no validation enforces ReportsStatus.
- There is no logic preventing invalid combinations.

Impact:
- Risk of claiming a capability not implemented or missing the required ReportsStatus bit.

Files:
- `superv/opamp/client.go`
- `superv/supervisor/supervisor.go`

### Heartbeat Reporting

Spec:
- If ReportsHeartbeat capability is set, client SHOULD send heartbeats.
- Default interval is 30s, or server-provided `OpAMPConnectionSettings.heartbeat_interval_seconds`.

Current implementation:
- No ReportsHeartbeat capability or heartbeat emission.
- No handling of `heartbeat_interval_seconds` when connection settings are received.

Impact:
- Missed heartbeat capability and behavior expected by servers.

Files:
- `superv/opamp/client.go`
- `superv/supervisor/supervisor.go`

### Connection Settings Management

Spec:
- Client should apply connection settings and report status (connection settings workflow).
- Settings include destination endpoint, headers, certificates, TLS, proxy, and heartbeat interval.

Current implementation:
- `OnOpampConnectionSettings` only handles certificate returned from enrollment response.
- No application of endpoint changes, headers, TLS/CA/proxy settings, or heartbeat interval.
- No reporting via ConnectionSettingsStatus.

Impact:
- Partial compliance with connection settings workflow and security model.
- Server-driven endpoint and TLS changes are ignored.

Files:
- `superv/supervisor/supervisor.go`

### Remote Config Status

Spec:
- Agent should report RemoteConfigStatus including hash and status after processing.

Current implementation:
- Remote config is applied and effective config is reported.
- No explicit status reporting is visible in supervisor logic; relies on opamp-go defaults.

Impact:
- Server may not receive detailed status for configuration acceptance or errors.

Files:
- `superv/opamp/callbacks.go`
- `superv/supervisor/supervisor.go`

### ServerToAgent.command Handling

Spec:
- Command message must not be set with other fields except instance_uid and capabilities; other fields are ignored.

Current implementation:
- Command handling is delegated through opamp-go callbacks; supervisor does not validate or log if message contains incompatible fields.

Impact:
- Limited observability; may accept malformed server messages without noticing.

Files:
- `superv/opamp/callbacks.go`
- `superv/supervisor/supervisor.go`

## Go Idioms and Implementation Quality

### Validation and Errors

- `ClientConfig.Validate` only checks presence of endpoint and instance UID. It should validate scheme and instance UID format/length.
- `parseInstanceUID` silently falls back to raw bytes. This is surprising and should either error or generate a compliant UUID.

Files:
- `superv/opamp/client.go`

### Context Usage

- `SetEffectiveConfig` uses `context.Background()` when calling `UpdateEffectiveConfig`, ignoring caller context cancellation.

Files:
- `superv/opamp/client.go`

### Mutable Configuration

- `Supervisor.Start` modifies `s.cfg.Server.Endpoint` based on enrollment URL. Mutating shared config during runtime can produce unexpected side effects.

Files:
- `superv/supervisor/supervisor.go`

## Security Review

- `InsecureSkipVerify` can be enabled without warning, which is acceptable as a config escape hatch but should be visible through logs for safety.
- Enrollment URL is enforced as HTTPS, and JWT is validated using Ed25519 and JWKS (good).
- TLS and proxy settings from `OpAMPConnectionSettings` are ignored, so server-directed trust settings are not applied.

Files:
- `superv/supervisor/supervisor.go`
- `superv/auth/enrollment.go`

## Concurrency and Lifecycle

### Broadcast Lock Scope

- `Server.Broadcast` holds the mutex while sending to each connection. Slow or blocked connections can stall other operations.

Files:
- `superv/opamp/server.go`

### Health Polling

- Health polling uses a cancellable context and channel closure correctly. Initial status is always sent.
- `Monitor` stores last and lastSent states safely.

Files:
- `superv/healthmonitor/monitor.go`

## Recommended Remediation Plan

1. Enforce `instance_uid` format and length (16 bytes, UUID v7). Reject invalid inputs early or generate compliant values.
2. Ensure ReportsStatus capability is always set and only set supported capabilities.
3. Implement heartbeat reporting and apply `heartbeat_interval_seconds` from connection settings.
4. Apply full connection settings: endpoint updates, headers, TLS CA / client cert, proxy settings; report ConnectionSettingsStatus.
5. Report RemoteConfigStatus after apply success/failure, including hash and error details.
6. Reduce lock hold time in `Server.Broadcast` by snapshotting connections before send.
7. Use caller context when updating effective config or when triggering OpAMP updates.
8. Add warnings when `InsecureSkipVerify` is true to make insecure mode explicit.

## Reference Implementation Comparison

Reference path: `/home/bernd/graylog/sidecar/.src/opentelemetry-collector-contrib/cmd/opampsupervisor/`
Compared areas: OpAMP client/server setup, capabilities, connection settings, heartbeat, config handling, and status reporting.

### Key Differences (Spec-Relevant)

1. Instance UID handling
   - Reference: uses persistent UUID and validates collector instance UID during bootstrap.
   - Our supervisor: `parseInstanceUID` accepts arbitrary strings and truncates into 16 bytes.
   - Gap: stricter enforcement of 16-byte UUID (UUID v7 recommended) is needed.

2. Capabilities enforcement
   - Reference: `SupportedCapabilities()` always includes ReportsStatus and reflects configured support.
   - Our supervisor: capabilities are set manually; ReportsStatus is not enforced.
   - Gap: enforce ReportsStatus and validate supported capabilities centrally.

3. Connection settings workflow
   - Reference: applies endpoint, headers, TLS certs; restarts client; rolls back on failure; updates heartbeat interval.
   - Our supervisor: only handles enrollment certificate, ignores endpoint/headers/TLS/proxy and heartbeat.
   - Gap: implement full OpAMPConnectionSettings and ConnectionSettingsStatus reporting.

4. Heartbeat
   - Reference: enables ReportsHeartbeat and uses server-provided heartbeat interval.
   - Our supervisor: no heartbeat capability or interval handling.
   - Gap: implement heartbeat capability and interval honoring.

5. Remote config lifecycle
   - Reference: merges remote config with own telemetry and local config, persists last received config, and updates effective config.
   - Our supervisor: applies remote config and reports effective config only; status reporting is minimal.
   - Gap: track remote config hash and status per spec, persist last received config.

6. Available components
   - Reference: collects available components during bootstrap and supports ReportAvailableComponents flag.
   - Our supervisor: does not report available components.
   - Gap: implement ReportsAvailableComponents when needed.

7. Custom messages
   - Reference: forwards CustomCapabilities and CustomMessage between agent and server with buffering.
   - Our supervisor: does not handle custom messages.
   - Gap: implement custom message forwarding if required.

8. Endpoint validation and TLS config
   - Reference: validates endpoint scheme and TLS configuration up front.
   - Our supervisor: only checks for non-empty endpoint.
   - Gap: add scheme validation and TLS config validation to avoid invalid OpAMP URLs.

### Suggested Gap-to-File Mapping

- Instance UID validation and generation: `superv/opamp/client.go`, `superv/persistence/instance.go`
- Capability enforcement: `superv/opamp/client.go`, `superv/supervisor/supervisor.go`
- Connection settings application: `superv/supervisor/supervisor.go`
- Heartbeat interval support: `superv/opamp/client.go`, `superv/supervisor/supervisor.go`
- Remote config status persistence: `superv/configmanager/manager.go`, `superv/opamp/callbacks.go`
- Available components reporting: `superv/opamp/client.go`, `superv/supervisor/supervisor.go`
- Endpoint/TLS validation: `superv/config/validate.go`, `superv/opamp/client.go`

## Quick Reference: Key Files

- `superv/opamp/client.go` (client config, capabilities, instance UID parsing)
- `superv/opamp/callbacks.go` (callback wiring, remote config handling)
- `superv/opamp/server.go` (local OpAMP server, broadcast)
- `superv/supervisor/supervisor.go` (overall OpAMP lifecycle, enrollment, config, health)
- `superv/healthmonitor/monitor.go` (health status to ComponentHealth)
- `superv/auth/enrollment.go` (enrollment URL parsing and JWT validation)
