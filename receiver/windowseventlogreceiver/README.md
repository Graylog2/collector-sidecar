# Windows Event Log Receiver

| Status    |                                    |
|-----------|------------------------------------|
| Stability | alpha                              |
| Platforms | Windows only (darwin, linux: stub) |

Tails and parses logs from the Windows Event Log API. Forked from the
[OpenTelemetry Collector Contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib)
`windowseventlogreceiver` with additional enhancements for Graylog.

## Fork Origin

- **Upstream:** `pkg/stanza/operator/input/windows/` and `receiver/windowseventlogreceiver/`
- **Commit:** [`f214dc8`](https://github.com/open-telemetry/opentelemetry-collector-contrib/commit/f214dc8da19d8a1e2d42d83693b61f5f17228f50)
- **Version:** v0.146.0

## Changes from Upstream

See `docs/plans/2026-02-28-windows-eventlog-receiver-design.md` for the full
design document. Summary of enhancements:

- **SID-to-account resolution** — resolves raw SID strings to account name,
  domain, and type via `LookupAccountSid`, with an LRU cache
- **Parameter message expansion** — resolves `%%1234`-style parameter message
  IDs from provider message tables
- **Message template fallback** — when `EvtFormatMessage` fails, falls back to
  cached message templates from provider metadata before giving up
- **UserData parsing** — parses `<UserData>` payloads alongside `<EventData>`
- **Locale control** — configurable LCID for publisher metadata rendering
- **Numeric severity mapping** — prefers numeric `Level` field over rendered
  string, making severity mapping locale-independent
- **Timestamp warning** — logs a warning when timestamp parsing falls back to
  `time.Now()`
- **XML query validation** — validates query XML syntax at config time
- **XML input sanitization** — strips invalid XML 1.0 characters before parsing
- **Strict bookmark recovery** — uses `EvtSubscribeStrict` and falls back to
  the configured `start_at` when the bookmark is stale
- **ProcessingErrorData diagnostics** — exposes rendering error codes in output
- **Audit outcome enrichment** — derives `outcome` field from keyword audit bits
- **Exponential backoff** — classifies Windows error codes and uses exponential
  backoff for transient failures
- **Transient subscription recovery** — retries `subscription.Open()` on
  transient errors instead of silently stopping

Removed from upstream:

- **Remote event log collection** (`remote.*` config) — Graylog Collector is
  deployed per-machine
- **`max_events_per_poll`** — rarely used, removed for simplicity

## Configuration

```yaml
receivers:
  windowseventlog:
    channel: Security
    start_at: end
    poll_interval: 1s
    max_reads: 100
    raw: false
    suppress_rendering_info: false
    exclude_providers: []
    ignore_channel_errors: false
    include_log_record_original: false
    storage: file_storage
    resolve_sids: true
    sid_cache_size: 1024
    language: 0
```

### Configuration Fields

| Field                      | Default | Description                                                                                                                                               |
|----------------------------|---------|-----------------------------------------------------------------------------------------------------------------------------------------------------------|
| `channel`                  | —       | The Windows Event Log channel to monitor (e.g. `Security`, `Application`, `System`). Either `channel` or `query` must be set, but not both.               |
| `query`                    | —       | An XML query for filtering events. Mutually exclusive with `channel`. See [XML Event Queries](#xml-queries) below.                                        |
| `start_at`                 | `end`   | Where to start reading on first startup. Options: `beginning` or `end`.                                                                                   |
| `poll_interval`            | `1s`    | How often to check for new events. The next poll begins after all available events have been read.                                                         |
| `max_reads`                | `100`   | Maximum number of events read per batch before yielding.                                                                                                  |
| `raw`                      | `false` | If `true`, emit the original XML string as the log body. If `false`, emit a structured map.                                                               |
| `suppress_rendering_info`  | `false` | If `false`, call `EvtFormatMessage` for detailed rendering (message, level, task, opcode, keywords). If `true`, only the raw XML fields are used.         |
| `exclude_providers`        | `[]`    | Provider names to skip (events from these providers are dropped).                                                                                         |
| `ignore_channel_errors`    | `false` | If `true`, log a warning instead of failing when the channel cannot be opened.                                                                            |
| `include_log_record_original` | `false` | If `true`, add the original XML string as the `log.record.original` attribute.                                                                         |
| `storage`                  | —       | ID of a storage extension for bookmark persistence. Without this, bookmarks are in-memory only and lost on restart.                                       |
| `resolve_sids`             | `true`  | Resolve SID strings to account name, domain, and type via `LookupAccountSid`. Results are cached.                                                         |
| `sid_cache_size`           | `1024`  | Maximum number of entries in the SID LRU cache. Failed lookups are also cached (negative cache).                                                          |
| `language`                 | `0`     | Windows LCID passed to `EvtOpenPublisherMetadata`. `0` means system default locale.                                                                       |
| `attributes`               | `{}`    | A map of `key: value` pairs added to each log record's attributes.                                                                                        |
| `resource`                 | `{}`    | A map of `key: value` pairs added to each log record's resource.                                                                                          |
| `operators`                | `[]`    | An array of [stanza operators](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/stanza/docs/operators) for log processing. |
| `retry_on_failure.enabled` | `false` | If `true`, pause and retry sending the current batch on downstream errors.                                                                                |
| `retry_on_failure.initial_interval` | `1s` | Time to wait after the first retry failure.                                                                                                          |
| `retry_on_failure.max_interval`     | `30s` | Upper bound on retry backoff interval.                                                                                                              |
| `retry_on_failure.max_elapsed_time` | `5m`  | Maximum total time spent retrying before discarding the batch. `0` means retry forever.                                                             |

## Structured Output

When `raw: false` (the default), the log body is a structured map with the
following fields:

| Field                          | Type              | Description                                                                 |
|--------------------------------|-------------------|-----------------------------------------------------------------------------|
| `event_id.id`                  | uint32            | The event identifier                                                        |
| `event_id.qualifiers`          | uint16            | Event ID qualifiers                                                         |
| `provider.name`                | string            | Provider (source) name                                                      |
| `provider.guid`                | string            | Provider GUID                                                               |
| `provider.event_source`        | string            | Legacy event source name                                                    |
| `channel`                      | string            | Event log channel                                                           |
| `computer`                     | string            | Computer name                                                               |
| `record_id`                    | uint64            | Event record ID                                                             |
| `system_time`                  | string            | Event timestamp (ISO 8601)                                                  |
| `level`                        | string            | Rendered level or numeric level                                             |
| `task`                         | string            | Rendered task or numeric task                                               |
| `opcode`                       | string            | Rendered opcode or numeric opcode                                           |
| `keywords`                     | []string          | Rendered keywords or raw keyword values                                     |
| `message`                      | string            | Rendered event message                                                      |
| `version`                      | uint8             | Event version                                                               |
| `event_data`                   | map               | Parsed `<EventData>` fields                                                 |
| `user_data`                    | map               | Parsed `<UserData>` fields (when present). Includes `xml_name` for the child element name. |
| `security.user_id`             | string            | Raw SID string                                                              |
| `security.user_name`           | string            | Resolved account name (when `resolve_sids: true`)                           |
| `security.domain`              | string            | Resolved domain (when `resolve_sids: true`)                                 |
| `security.user_type`           | string            | Account type: User, Group, WellKnownGroup, Computer, etc.                   |
| `execution`                    | map               | Process/thread info (`process_id`, `thread_id`, etc.)                       |
| `correlation`                  | map               | Activity and related activity IDs (when present)                            |
| `outcome`                      | string            | `"success"` or `"failure"` derived from audit keyword bits (Security events) |
| `error.code`                   | uint32            | Windows rendering error code (when `ProcessingErrorData` is present)        |
| `error.data_item_name`         | string            | Name of the field that failed rendering (when `ProcessingErrorData` is present) |

## Rendering Fallback Chain

When `suppress_rendering_info: false`, the receiver attempts to render each
event message through a three-stage fallback:

1. **`EvtFormatMessage`** (deep rendering) — the primary path using the Windows
   API to fully render the message with all substitutions.
2. **Cached message template** — if deep rendering fails (e.g. missing DLL),
   the receiver looks up the provider's message template by event ID and version
   from cached metadata, and executes it with the event's data values.
3. **No message** (simple XML rendering) — if neither path produces a message,
   the event is emitted with an empty message and the raw XML fields.

After rendering, `%%NNNN` parameter message tokens in the message are resolved
from the provider's parameter message table.

## Error Recovery

The receiver classifies Windows error codes and handles them differently:

- **Recoverable errors** (`ERROR_INVALID_HANDLE`, `RPC_S_SERVER_UNAVAILABLE`,
  `RPC_S_CALL_CANCELLED`, `ERROR_EVT_QUERY_RESULT_STALE`,
  `ERROR_INVALID_PARAMETER`, `ERROR_EVT_PUBLISHER_DISABLED`) trigger exponential
  backoff: 5s → 10s → 20s → 40s → 60s (capped). The backoff resets on
  successful read.
- **Non-recoverable errors** (`ERROR_EVT_CHANNEL_NOT_FOUND`,
  `ERROR_ACCESS_DENIED`) stop the receiver (unless `ignore_channel_errors: true`
  for channel errors).
- **Stale bookmark** errors cause the receiver to fall back to the configured
  `start_at` (`end` or `beginning`). Resumption from a stale bookmark is not
  possible, so some gap between the last recorded bookmark and the new start
  position may occur.

## Example Configurations

### Simple — Monitor a Single Channel

```yaml
receivers:
  windowseventlog:
    channel: Application
```

### Security Channel with SID Resolution

```yaml
receivers:
  windowseventlog:
    channel: Security
    resolve_sids: true
    sid_cache_size: 2048
    storage: file_storage
```

### XML Query — Multiple Providers

Use an XML query to collect from specific providers across channels. When using
`query`, do not set `channel`.

```yaml
receivers:
  windowseventlog:
    query: |
      <QueryList>
        <Query Id="0">
          <Select Path="Application">*[System[Provider[@Name='foo']]]</Select>
          <Select Path="Application">*[System[Provider[@Name='bar']]]</Select>
        </Query>
      </QueryList>
    storage: file_storage
```

### Raw XML Output

```yaml
receivers:
  windowseventlog:
    channel: System
    raw: true
```

### Non-English Locale

Force English rendering on a non-English Windows system (LCID `1033` = en-US):

```yaml
receivers:
  windowseventlog:
    channel: Application
    language: 1033
```

### Operators Pipeline

```yaml
receivers:
  windowseventlog:
    channel: Application
    operators:
      - type: filter
        expr: 'body.level == "Information"'
```

## Operators

Each operator performs a simple responsibility, such as parsing a timestamp or
JSON. Chain together operators to process logs into a desired format.

- Every operator has a `type`.
- Every operator can be given a unique `id`. If you use the same type of
  operator more than once in a pipeline, you must specify an `id`. Otherwise,
  the `id` defaults to the value of `type`.
- Operators output to the next operator in the pipeline. The last operator emits
  from the receiver. Optionally, use the `output` parameter to specify the `id`
  of another operator to route logs directly.
- Only parsers and general-purpose operators should be used.
