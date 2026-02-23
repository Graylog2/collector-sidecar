# Bootstrap Config for First-Run Collector Startup

**Date:** 2026-02-23
**Status:** Accepted

## Problem

When a new collector supervisor starts for the first time, there is no cached remote
config on disk. The supervisor launches the collector with `--config collector.yaml`,
but that file does not exist yet because no remote config has been received via OpAMP.
The collector fails immediately:

```
Error: failed to get config: cannot resolve the configuration: ...
open .../config/collector.yaml: no such file or directory
```

The collector stays unhealthy until the OpAMP server pushes a config, but the health
endpoint is never reachable because the collector never started.

Additionally, the local OpAMP server binds to an ephemeral port by default
(`localhost:0`). On restart, the port changes, but the cached config still references
the old port. The collector cannot connect to the local OpAMP server until a remote
config is applied and re-injects the correct endpoint.

## Solution

Add `EnsureBootstrapConfig()` to the config manager. On every startup, before launching
the collector, call this method. It handles two cases:

1. **No config file exists (first run):** Write a bootstrap config containing the opamp
   and health_check extensions plus a minimal nop pipeline (the collector requires at
   least one pipeline to start).
2. **Config file exists (cached from previous run):** Re-inject the opamp and
   health_check extensions to update the local OpAMP endpoint, which may have changed
   if the server binds to an ephemeral port. If the cached config has no pipelines,
   inject a nop pipeline so the collector can start.

In both cases, the nop pipeline (`logs/bootstrap` with nop receiver and nop exporter)
is only injected when the config has no pipelines under `service.pipelines`. Once a
real config arrives from the OpAMP server, it will contain real pipelines and the nop
pipeline will not be re-injected.

### Bootstrap config output

```yaml
extensions:
  health_check:
    endpoint: localhost:13133
    path: /health
  opamp:
    instance_uid: <dynamic>
    server:
      ws:
        endpoint: ws://127.0.0.1:54321/v1/opamp
receivers:
  nop: null
exporters:
  nop: null
service:
  extensions:
    - opamp
    - health_check
  pipelines:
    logs/bootstrap:
      receivers:
        - nop
      exporters:
        - nop
```

The OpAMP endpoint is a full WebSocket URL derived from the local OpAMP server's actual
bound address, formatted as `ws://<host>:<port>` + `config.DefaultOpAMPPath`.

The nop pipeline satisfies the collector's requirement of at least one pipeline. The
`nop` receiver and `nop` exporter are no-ops that do not produce or consume any data.

This gives the collector enough to:
1. Start successfully and report healthy on the health endpoint.
2. Connect to the local OpAMP server so the supervisor can manage it.
3. Wait for the real config to arrive from the upstream OpAMP server.

### Runtime endpoint resolution

The config manager is initialized with the static `LocalServer.Endpoint` (default
`localhost:0`), but the actual bound address is only known after `opampServer.Start()`.
Both `EnsureBootstrapConfig` and `ApplyRemoteConfig` need the runtime-resolved endpoint.

**Approach:** Add `SetLocalEndpoint(endpoint string)` on the config manager. After the
local OpAMP server starts, the supervisor resolves and normalizes the bound address via
`resolveLocalEndpoint()`, then calls `SetLocalEndpoint`. This single resolved value is
used by both `EnsureBootstrapConfig` and `ApplyRemoteConfig` (via `m.cfg.LocalEndpoint`).

**Endpoint normalization:** `opampServer.Addr()` returns `net.Addr.String()`, which may
contain unspecified hosts (`0.0.0.0`, `[::]`) when the server binds to a wildcard
address. These are not dialable by the collector. The `resolveLocalEndpoint` helper
normalizes the address with family-aware loopback replacement:
- `0.0.0.0` → `127.0.0.1` (IPv4)
- `[::]` → `[::1]` (IPv6)

Unparseable addresses cause `resolveLocalEndpoint` to return an error (fail fast),
preventing malformed WebSocket URLs from being written to the collector config.

The path suffix uses the `config.DefaultOpAMPPath` constant (`/v1/opamp`).

### Implementation

**New helper in `supervisor` package:**

`resolveLocalEndpoint(addr string) (string, error)` — parses a `net.Addr.String()`
result, normalizes unspecified hosts to family-aware loopback, and formats as
`ws://<host>:<port>/v1/opamp`. Returns an error if the address cannot be parsed.

**New methods on `configmanager.Manager`:**

`SetLocalEndpoint(endpoint string)` — updates `m.cfg.LocalEndpoint` with the
runtime-resolved WebSocket URL. Called once after the local OpAMP server starts.

`EnsureBootstrapConfig() error` — ensures a valid collector config exists:

1. Ensure the config directory exists (`os.MkdirAll`).
2. Read existing config from `OutputPath`, or start from empty if the file does not
   exist (non-existence errors are expected; other read errors are returned).
3. Call `configmerge.InjectOpAMPExtension` with the current `m.cfg.LocalEndpoint`.
4. Call `configmerge.InjectHealthCheckExtension` with `m.cfg.HealthCheck`.
5. If `configmerge.HasPipelines(config)` returns false, inject a minimal nop pipeline
   (`logs/bootstrap` with nop receiver and nop exporter) via `configmerge.InjectSettings`.
6. Write the result to `OutputPath` via `persistence.WriteFile`.

`configmerge.HasPipelines(config []byte) bool` — parses the YAML config and returns
true if `service.pipelines` exists and has at least one key. Used by
`EnsureBootstrapConfig` to decide whether to inject the nop pipeline.

**Call site:** `supervisor.Start()`, after `opampServer.Start()` but before the worker
queue and OpAMP client are started. At this point the local server's bound address is
known (for the OpAMP endpoint), but no callbacks are running yet, so there is no race
with `ApplyRemoteConfig`.

```go
localEndpoint, err := resolveLocalEndpoint(s.opampServer.Addr())
// handle error with opampServer cleanup
s.configManager.SetLocalEndpoint(localEndpoint)
// handle error with opampServer cleanup
s.configManager.EnsureBootstrapConfig()
```

**Error handling:** If `resolveLocalEndpoint` or `EnsureBootstrapConfig` fails at this
call site, only the local OpAMP server needs cleanup (same as the existing error path
for the worker/client setup that follows).

### Lifecycle

```
First run (no cached config):
  Start()
    -> opampServer.Start()
    -> resolveLocalEndpoint() -> SetLocalEndpoint()
    -> EnsureBootstrapConfig() writes bootstrap config
    -> start worker, OpAMP client, health monitor
    -> commander.Start() succeeds
    -> collector healthy
    -> OpAMP server sends real config
    -> ApplyRemoteConfig() overwrites bootstrap (backed up as .prev)
       (uses same resolved endpoint via m.cfg.LocalEndpoint)
    -> commander.Restart()

Subsequent runs (cached config exists):
  Start()
    -> opampServer.Start()
    -> resolveLocalEndpoint() -> SetLocalEndpoint()
    -> EnsureBootstrapConfig() re-injects extensions with current endpoint
    -> start worker, OpAMP client, health monitor
    -> commander.Start() with updated cached config
```

### Why this approach

- Reuses existing `InjectOpAMPExtension` and `InjectHealthCheckExtension` functions.
- Config manager already owns `OutputPath` and all config writes.
- Minimal change: one helper, two methods, one call site.
- `ApplyRemoteConfig` naturally replaces the bootstrap config when real config arrives.
- Placed before OpAMP client starts: no race with remote config callbacks.
- Both bootstrap and remote config injection use the same resolved endpoint.
- Re-injection on every start prevents stale endpoints in cached configs.
- Family-aware normalization supports both IPv4 and IPv6 environments.
- Fail-fast on unparseable addresses prevents malformed collector configs.

### Alternatives considered

**Delay collector start until remote config arrives:** Would require reworking health
monitoring lifecycle and leave the collector unhealthy for an indeterminate period.
Rejected for complexity.

**Embed static bootstrap YAML via `go:embed`:** The opamp endpoint and instance UID are
dynamic, so the static file would need post-processing anyway, making it equivalent to
this approach with extra steps. Rejected for redundancy.

**Skip existing configs (no-op when file exists):** Would leave stale OpAMP endpoints
in cached configs after restarts with ephemeral ports. Rejected because it breaks
collector→supervisor connectivity.

**IPv6 wildcard → IPv4 loopback:** Would make `[::]` resolve to `127.0.0.1`, which is
unreachable if the listener is IPv6-only. Rejected in favor of family-aware mapping
(`0.0.0.0` → `127.0.0.1`, `[::]` → `[::1]`).

**Silent fallback on unparseable addresses:** Would produce malformed WebSocket URLs in
the collector config. Rejected in favor of returning an error (fail fast).
