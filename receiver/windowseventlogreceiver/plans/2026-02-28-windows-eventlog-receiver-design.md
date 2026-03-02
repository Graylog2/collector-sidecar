# Custom Windows Event Log Receiver

## Status

Proposed — 2026-02-28

## Context

The Graylog Collector uses an OpenTelemetry Collector built with OCB. The contrib
`windowseventlogreceiver` provides Windows Event Log collection but has several
shortcomings that affect message fidelity, operational resilience, and usability
for Graylog customers.

This design describes a custom receiver (`windowseventlog`) that forks the
OTel contrib stanza operator code and adds targeted enhancements. The fork reuses
approximately 75% of the existing code — syscall wrappers, subscription management,
bookmark handling, buffer management, XML parsing, and the polling loop — and adds
new capabilities on top.

## Decision

Fork `pkg/stanza/operator/input/windows/` from OpenTelemetry Collector Contrib
(v0.146.0) into a Graylog-owned receiver module. Enhance with SID resolution,
parameter expansion, message template fallback, improved error recovery, UserData
parsing, locale control, and robustness fixes. Register as a new receiver type
replacing the contrib receiver in the collector build.

## Module Structure

```
receiver/windowseventlogreceiver/
├── go.mod
├── factory.go                    # OTel receiver factory
├── config.go                     # config struct + validation
├── receiver_windows.go           # receiver lifecycle (Start/Shutdown)
├── receiver_others.go            # build stub for non-Windows
└── internal/
    └── windows/
        ├── api.go                # wevtapi syscalls (forked)
        ├── subscription.go       # subscription management (forked)
        ├── bookmark.go           # bookmark handling (forked)
        ├── buffer.go             # UTF-16/UTF-8 buffer (forked)
        ├── input.go              # polling loop (forked, enhanced)
        ├── event.go              # event rendering (forked, enhanced)
        ├── xml.go                # XML unmarshaling (forked)
        ├── publisher.go          # publisher handle management (forked)
        ├── publishercache.go     # publisher LRU cache (forked, extended)
        ├── sid.go                # SID resolution + LRU cache (new)
        ├── msgtemplate.go        # message template cache + fallback (new)
        ├── paramexpand.go        # %%ID parameter expansion (new)
        └── backoff.go            # exponential backoff (new)
```

## Integration

The receiver is registered in `builder/builder-config.yaml` as
`windowseventlog`, replacing the contrib `windowseventlogreceiver`.
Bookmark persistence uses the OTel `file_storage` extension, which should be
pre-configured in the default Graylog Collector config.

**Fork origin:** OpenTelemetry Collector Contrib commit
[`f214dc8`](https://github.com/open-telemetry/opentelemetry-collector-contrib/commit/f214dc8da19d8a1e2d42d83693b61f5f17228f50)
(v0.146.0). The receiver module includes a README recording this SHA for future
upstream tracking.

## Improvements

### 1. SID-to-Account Resolution

**Problem:** The contrib receiver passes raw SID strings (e.g. `S-1-5-21-...`)
through without resolving them. SIDs for local accounts can only be resolved on
the originating machine.

**Solution:** New file `sid.go`. Calls `LookupAccountSid` via
`golang.org/x/sys/windows` to resolve SID strings to account name, domain, and
account type.

An LRU cache (default 1024 entries) avoids repeated lookups. Cache entries have a
TTL (5 minutes) so account renames are eventually picked up. Failed lookups are
cached too (negative cache) to avoid hammering the API for non-resolvable SIDs.

Lookup errors are non-blocking — the raw SID is preserved as fallback and the
event is emitted regardless.

**Output fields added to the structured event map:**

| Field | Description |
|---|---|
| `security.user_id` | Raw SID string (always present, same as today) |
| `security.user_name` | Resolved account name (if resolved) |
| `security.domain` | Resolved domain (if resolved) |
| `security.user_type` | Account type: User, Group, WellKnownGroup, Computer, etc. |

**Config:**

| Option | Default | Description |
|---|---|---|
| `resolve_sids` | `true` | Enable/disable SID resolution |
| `sid_cache_size` | `1024` | SID LRU cache capacity |

### 2. Parameter Message Expansion

**Problem:** Event messages and data values sometimes contain `%%1234`-style
references. These are parameter message IDs that should be resolved from the
provider's parameter message table. The contrib receiver does not resolve them,
leaving raw `%%` tokens in the output.

**Solution:** New file `paramexpand.go`. After rendering a message (either via
`EvtFormatMessage` or the template fallback), scan the result for `%%\d+` patterns.
For each match, call `EvtFormatMessage` with `EvtFormatMessageId` against the
publisher handle and replace the token with the resolved string.

Resolved ID-to-string mappings are cached per provider on the publisher cache
entry. Unresolvable IDs are left as-is in the output and cached as negative
results.

Expansion applies to:

- Rendered message strings (post-`EvtFormatMessage`)
- Event data values containing `%%` references
- Template fallback output (see next section)

Always enabled when deep rendering is active (`suppress_rendering_info: false`).
No additional config options.

### 3. Message Template Fallback

**Problem:** When `EvtFormatMessage` fails (DLL not found, provider not registered,
etc.), the contrib receiver falls back to simple XML rendering with no message at
all. Many events end up with empty messages.

**Solution:** New file `msgtemplate.go`. A secondary rendering path that uses
cached message templates from publisher metadata.

**How it works:**

1. On first encounter of a provider, after opening the publisher handle, enumerate
   event metadata via `EvtOpenEventMetadataEnum` / `EvtNextEventMetadata`.
2. For each event definition, extract the message template string via
   `EvtFormatMessage` with `EvtFormatMessageEvent` on the metadata handle.
3. Store templates in a map keyed by `(eventID, version)` on the publisher cache
   entry.
4. Windows message templates use `%1`, `%2` for positional parameters. Convert to
   Go `text/template` syntax at cache time using a safe helper function
   (`eventParam`) that returns the parameter value if present or preserves the
   original `%N` placeholder if the index is out of range. This avoids template
   execution failures when a provider template references more parameters than the
   event supplies.
5. When deep rendering fails, look up the template by event ID + version, execute
   it with the event's data values (from `<EventData>` or `<UserData>`, whichever
   is present), then apply `%%ID` expansion to the result.

Templates are cached lazily per provider and evicted together with the publisher
handle when the LRU cache evicts a provider.

**Rendering fallback chain:**

1. `EvtFormatMessage` (deep rendering) — primary path
2. Cached message template with Go `text/template` — secondary path
3. No message (simple XML rendering) — last resort

### 4. UserData Parsing

**Problem:** The contrib receiver's `EventXML` struct has an `EventData` field but
no `UserData` field. Many Windows events carry their structured data exclusively
in `<UserData>` instead of `<EventData>` — for example, Security event 1102
(audit log cleared) uses `<UserData><LogFileCleared>` with fields like
SubjectUserSid, SubjectUserName, and SubjectDomainName. These payloads are
silently dropped.

**Solution:** Add a `UserData` field to the `EventXML` struct in `xml.go`. The
UserData element contains a single provider-specific child element (e.g.
`<LogFileCleared>`) with nested data fields. Parse the child element's name and
its data fields into the same key-value structure used for EventData.

Emit UserData in the structured output body as `user_data` alongside the existing
`event_data`. Include the child element's XML local name as `user_data.xml_name`
for context.

No new config options — UserData is always parsed when present.

### 5. Locale/Language Control

**Problem:** The contrib receiver hardcodes locale `0` (system default) in the
call to `evtOpenPublisherMetadata`. On non-English Windows systems, rendered
strings (level names, task names, keyword names) come back in the system locale,
but the severity mapping is hardcoded to English strings ("Critical", "Error",
"Warning", "Information"). This causes incorrect severity levels on non-English
systems.

**Solution:** Add a `language` config option (LCID as uint32, default 0 for system
locale). Pass it to `evtOpenPublisherMetadata` so publishers render strings in the
requested locale.

Additionally, fix severity mapping to prefer the numeric `Level` field (1=Critical,
2=Error, 3=Warning, 4=Information) over the rendered string. The rendered string
becomes a secondary source only. This makes severity mapping locale-independent.

**Config:**

| Option | Default | Description |
|---|---|---|
| `language` | `0` | Windows LCID for publisher metadata rendering (0 = system default) |

### 6. Transient Subscription-Open Recovery

**Problem:** When `subscription.Open()` fails with a transient error at startup
(e.g. Event Log service temporarily unavailable), the contrib receiver logs the
error but never starts the polling goroutine. The receiver silently does nothing.
Runtime subscription reopen is only implemented for remote subscriptions, not
local ones.

**Solution:** Modify the startup path in `input.go` to always start the polling
goroutine. If the initial `subscription.Open()` fails with a transient error, the
goroutine retries with exponential backoff (integrating with the backoff from
improvement #7). Also extend the runtime reopen logic to handle local
subscriptions, not just remote ones — on `ERROR_INVALID_HANDLE` or handle-not-open
errors during read, close and reopen the local subscription.

No new config options.

### 7. Timestamp and Severity Robustness

**Problem:** Two issues in the contrib receiver: (a) invalid timestamps silently
become `time.Now()` with no indication of data loss, and (b) severity mapping uses
hardcoded English strings that fail on non-English systems.

**Solution:**

For timestamps: keep the `time.Now()` fallback as a last resort but log a warning
when it's used, making the data quality issue observable. The warning includes the
original timestamp string for debugging.

For severity: change `parseSeverity()` to prefer the numeric `Level` field as the
primary source (1→Fatal, 2→Error, 3→Warn, 4→Info). Use the rendered level string
only when the numeric level is absent or unrecognized. This makes severity mapping
work regardless of the Windows system locale.

No new config options.

### 8. XML Query Validation

**Problem:** The contrib receiver checks that either `channel` or `query` is set
(not both) but does not validate the XML syntax of the query string. A malformed
query is only detected at runtime when `EvtSubscribe` fails.

**Solution:** Add `xml.Unmarshal` validation of the query string in `Build()`.
This catches malformed XML at config time with a clear error message. It does not
validate the XPath semantics — only the XML structure.

No new config options.

### 9. XML Input Sanitization

**Problem:** The contrib receiver passes raw event XML directly to Go's
`encoding/xml.Unmarshal`. Windows event XML can occasionally contain invalid XML
characters (control characters 0x00-0x08, 0x0B, 0x0C, 0x0E-0x1F) which cause
parse failures. Winlogbeat uses a safe reader wrapper that strips these before
decoding.

**Solution:** Add a `sanitizeXML` function that strips invalid XML 1.0 characters
before calling `xml.Unmarshal` in `unmarshalEventXML()`. This is a small defensive
wrapper that prevents parse failures on malformed event output.

No new config options.

### 10. Strict Bookmark Recovery

**Problem:** The contrib receiver uses `EvtSubscribeStartAfterBookmark` without
the `EvtSubscribeStrict` flag. If the bookmarked event no longer exists (e.g. the
event log was cleared or rotated), `EvtSubscribe` may silently skip events or
behave unpredictably rather than returning a detectable error.

**Solution:** Add the `EvtSubscribeStrict` flag when resuming from a bookmark in
`subscription.go`. This causes `EvtSubscribe` to return specific error codes
(`ERROR_NOT_FOUND`, `ERROR_EVT_QUERY_RESULT_STALE`,
`ERROR_EVT_QUERY_RESULT_INVALID_POSITION`) when the bookmark is stale. On any of
these errors, fall back to `EvtSubscribeStartAtOldestRecord` to ensure no events
are missed.

No new config options.

### 11. ProcessingErrorData Diagnostics

**Problem:** When Windows fails to render an event (e.g. missing message DLL), it
includes a `<ProcessingErrorData>` element in the XML with an `ErrorCode` and
`DataItemName`. The contrib receiver's `EventXML` struct does not parse these
fields, making it harder to troubleshoot rendering failures.

**Solution:** Add `RenderErrorCode` (uint32) and `RenderErrorDataItemName` (string)
fields to `EventXML` in `xml.go`, mapped to `ProcessingErrorData>ErrorCode` and
`ProcessingErrorData>DataItemName`. Emit them in the structured body as
`error.code` and `error.data_item_name` when non-zero/non-empty.

No new config options.

### 12. Audit Outcome Enrichment

**Problem:** Security channel events carry audit success/failure information in
the Keywords bitmask (bit 0x10000000000000 = failure, bit 0x20000000000000 =
success). The contrib receiver stores keywords as strings but does not derive a
semantic `outcome` field, forcing downstream rules to parse hex keyword values.

**Solution:** In `formattedBody()` in `xml.go`, parse the raw Keywords hex string
(e.g. `"0x8020000000000000"`), check the two audit bits, and add an `outcome`
field to the structured body with value `"success"` or `"failure"`. Only add the
field when one of the audit bits is set.

No new config options.

### 13. Exponential Backoff and Error Classification

**Problem:** The contrib receiver uses a fixed `poll_interval` for retries and
classifies fewer Windows error codes as recoverable. This leads to either
unnecessary restarts on transient failures or slow recovery.

**Solution:** New file `backoff.go`. Classifies Windows error codes and integrates
exponential backoff into the polling loop. This backoff is also used by the
transient open recovery (improvement #6).

**Error classification:**

| Error Code | Classification | Action |
|---|---|---|
| `ERROR_NO_MORE_ITEMS` | Normal | Continue at `poll_interval` |
| `RPC_S_INVALID_BOUND` | Recoverable | Halve batch size, reopen (existing behavior) |
| `ERROR_INVALID_HANDLE` | Recoverable | Reopen subscription, backoff |
| `RPC_S_SERVER_UNAVAILABLE` | Recoverable | Backoff, retry |
| `RPC_S_CALL_CANCELLED` | Recoverable | Backoff, retry |
| `ERROR_EVT_QUERY_RESULT_STALE` | Recoverable | Reset bookmark, reopen |
| `ERROR_INVALID_PARAMETER` | Recoverable | Backoff, retry |
| `ERROR_EVT_PUBLISHER_DISABLED` | Recoverable | Backoff, retry |
| `ERROR_EVT_CHANNEL_NOT_FOUND` | Fatal | Stop (unless `ignore_channel_errors`) |
| `ERROR_ACCESS_DENIED` | Fatal | Stop |
| Everything else | Fatal | Stop |

**Backoff parameters (hardcoded constants):**

- Initial delay: 5 seconds
- Multiplier: 2x
- Maximum delay: 60 seconds
- Reset on successful read
- Sequence: 5s → 10s → 20s → 40s → 60s → 60s...

The existing `poll_interval` config is preserved for normal operation. On
recoverable error, the loop switches to backoff delays. On recovery, it resets
to `poll_interval`.

## Forked Code Changes Summary

| File | Source | Modifications |
|---|---|---|
| `api.go` | Forked | Remove remote session APIs (`EvtOpenSession`), add `EvtOpenEventMetadataEnum`, `EvtNextEventMetadata` |
| `subscription.go` | Forked | Remove remote session reconnection, add local reopen on error, add `EvtSubscribeStrict` + stale bookmark recovery |
| `bookmark.go` | Forked | None |
| `buffer.go` | Forked | None |
| `xml.go` | Forked | Add `UserData` field, `ProcessingErrorData` fields, `user_data`/`error.*`/`outcome` to `formattedBody()`, fix severity mapping to prefer numeric Level, add XML input sanitization, flatten `event_data` to flat map (winlogbeat-aligned), apply consistent empty-value/duplicate handling to `user_data` |
| `security.go` | Forked | **Removed** — parsed human-readable message text into display-label keys (e.g. "Account Name"), redundant with `event_data` which has the same data via manifest identifiers (e.g. "SubjectUserName") |
| `publisher.go` | Forked | Accept locale parameter, pass to `evtOpenPublisherMetadata` |
| `publishercache.go` | Forked | Extend entries to hold message templates and `%%ID` cache |
| `event.go` | Forked | Add template fallback, `%%ID` expansion, SID resolution hooks |
| `input.go` | Forked | Add backoff integration, transient open retry, remove remote config, add timestamp warning |
| `config_all.go` | Forked | Add `resolve_sids`, `sid_cache_size`, `language`; remove `remote.*` |
| `config_windows.go` | Forked | Add XML query syntax validation in `Build()` |
| `sid.go` | New | SID resolution + LRU cache |
| `msgtemplate.go` | New | Message template cache + fallback rendering |
| `paramexpand.go` | New | `%%ID` parameter expansion |
| `backoff.go` | New | Exponential backoff + error classification |

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

All options from the contrib receiver are preserved except:

- `remote.*` (server, username, password, domain) — removed, local only
- `max_events_per_poll` — removed, rarely used
- `include_log_record_original` — kept (see below)

New options:

- `resolve_sids` (bool, default `true`) — enable SID-to-account resolution
- `sid_cache_size` (int, default `1024`) — SID LRU cache capacity
- `language` (uint32, default `0`) — Windows LCID for publisher metadata rendering

## Data Flow

```
EvtSubscribe
  → poll loop (with backoff on error)
  → EvtNext (batch)
  → deep render (EvtFormatMessage)
      success → apply %%ID expansion
      failure → template fallback → apply %%ID expansion
  → SID resolution (LRU cached)
  → structured EventXML
  → OTLP log record
  → pipeline
```

## Out of Scope

The following items were considered and deliberately excluded from the initial
implementation. They can be revisited in future iterations.

### Offline .evtx File Ingestion

Reading exported Windows Event Log files (.evtx) via `EvtQuery` with
`EvtQueryFilePath` instead of `EvtSubscribe`. Would require a separate code path
for file detection (`os.Stat`), query-based reading instead of subscription-based,
and modified bookmark/seek logic for file positions. Excluded because Graylog
Collector is a live collection agent and offline .evtx analysis is a different use
case typically handled by forensic tools.

### Forwarded Events Handling

Forwarded events (arriving via Windows Event Forwarding into the ForwardedEvents
channel) may use publisher metadata from the originating machine that doesn't
match locally installed providers. A dedicated rendering path that relies on
XML-embedded RenderingInfo instead of local metadata would prevent cache pollution
and incorrect message rendering. Excluded because Graylog Collector is typically
installed per-machine and WEF is not a primary use case.

### Query Builder

A structured config for event ID ranges/exclusions, level filtering, provider
filtering, and time-based filters that generates XPath queries automatically.
Would include automatic query splitting for the 21-clause XPath limit and a
Windows Server 2025 workaround for a known Event Log service crash with query
filters. Excluded to keep the receiver simple — users can write raw XPath queries
in the `query` config field, and Graylog server can provide query templates.

### Event Data Parameter Name Recovery

Resolving parameter names from publisher metadata event templates when XML `<Data>`
elements lack `Name` attributes. Would require xxhash fingerprinting of property
count and types to match events to the correct template version, plus fallback
naming (`param1`, `param2`, etc.). Excluded because most modern providers include
names in XML, and the fingerprinting logic is complex. Events from legacy providers
will have unnamed parameters.

### Native Data Type Extraction

Extracting typed event data via `EvtRender` with `EvtRenderContextUser` and
interpreting `EvtVariant` structures to preserve native types (integers, GUIDs,
timestamps, etc.). Currently all event data values are strings from XML parsing.
Excluded because Graylog pipeline rules can handle type conversions downstream
and the `EvtVariant` handling adds significant complexity (~15 data types).

### ComplexData Support

Some Microsoft providers use `ComplexData` elements in EventData for structured
binary data. Currently only `Data[]` and `Binary` elements are parsed. Excluded
because very few providers use ComplexData in practice.

### Remote Event Log Collection

Collecting events from remote Windows machines via `EvtOpenSession` / EvtRPC
without installing an agent on each machine. The contrib receiver supports this.
Excluded because Graylog Collector is deployed per-machine and removing remote
support simplifies subscription management and error recovery.

### Stop-on-EOF / Finite Read Mode

A configurable `no_more_events: stop` mode that signals EOF when no more events
are available, allowing one-shot collection. Only relevant for finite reads such
as .evtx file ingestion. Excluded because the receiver is continuous polling only
and .evtx support is out of scope.

### Full Metadata Cache

Caching per-provider keywords (bitmask-to-name), opcodes, levels, and tasks in
addition to publisher handles and message templates. Would reduce per-event API
call overhead and allow metadata rendering even when individual `EvtFormatMessage`
calls fail for specific fields. Excluded to limit cache complexity — only publisher
handles and message templates are cached.
