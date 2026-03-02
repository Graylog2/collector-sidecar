# Channel List Filtering for Windows Event Log Receiver

**Date:** 2026-03-02
**Status:** Accepted

## Problem

When managing a fleet of Windows machines with a shared Sidecar configuration, not all machines have the same event log channels available (e.g., Sysmon, IIS, custom application channels). Currently, users work around this by crafting a `query` XML that covers all desired channels, but `EvtSubscribe` fails the entire subscription if any referenced channel does not exist on the machine.

The existing `ignore_channel_errors` flag handles single-channel mode but does not help when a multi-channel query partially references non-existent channels.

## Solution

Add a `channel_list` config field that accepts multiple channel names. At startup, the receiver enumerates available channels on the machine, filters the list to only those that exist, and builds a structured XML query from the surviving channels.

## Design

### Config Changes

**`config_all.go` — new field:**

```go
ChannelList []string `mapstructure:"channel_list,omitempty"`
```

**Backward compatibility:** The existing `channel` field is kept. Internally, a single `channel` is normalized to a one-element `channel_list` in `Config.Build()`.

**Validation (`config_validate.go`):**

**Important:** `validateConfig` canonicalizes `ChannelList` in-place (trim, remove empties, deduplicate) *before* checking source counts. This ensures configs like `["   ", ""]` are rejected at validation time rather than silently becoming empty at runtime.

- Exactly one of `channel`, `channel_list`, or `query` must be set (checked after canonicalization)
- `channel` and `channel_list` are mutually exclusive

### Input Struct Changes

**`input.go`:**

- Replace `channel string` field with `channelList []string`
- Add `filteredQuery *string` — the generated query after channel filtering

### Startup Filtering (Windows-only, `Input.Start`)

Before opening the subscription:

1. Call `ListChannels()` to enumerate available channels
   - **If `ListChannels()` fails:** fail startup immediately with the error. Channel enumeration is a prerequisite — falling back to an unfiltered list would silently subscribe to non-existent channels, defeating the purpose of this feature.
2. Build a case-insensitive set for O(1) lookup (Windows channel names are case-insensitive)
3. Filter `channelList` to only channels that exist
4. Log each skipped channel at `Warn` level with the channel name
5. If filtered list is empty:
   - `ignore_channel_errors=true` → log warning, return nil (start idle)
   - `ignore_channel_errors=false` → return error (fail startup)
6. Build XML query from filtered channels
7. Pass generated query to `Subscription.Open()`

### Query Generation

New function `buildQueryFromChannels(channels []string) string` produces:

```xml
<QueryList><Query Id="0"><Select Path="Security">*</Select><Select Path="Application">*</Select></Query></QueryList>
```

Multiple `<Select Path="...">` elements in a single `<Query>` is the standard way to subscribe to multiple channels in one `EvtSubscribe` call (per Microsoft docs).

**XML safety:** The function MUST use `encoding/xml` marshaling (not string concatenation) so that channel names containing XML-special characters (`&`, `<`, `>`, `"`) are properly escaped. The implementation uses `xml.NewEncoder` to serialize a typed struct, which guarantees correct escaping.

### Persist Key

`getPersistKey()` returns a stable key derived from the **original** (unfiltered, but canonicalized) `channelList`. The key is built by lowercasing each entry, sorting, deduplicating, and joining with `\n`. This ensures the persist key is deterministic regardless of input ordering or casing, and stable even when the set of available channels changes between restarts.

### Acceptance Test Matrix

| Scenario | Input | Expected |
|---|---|---|
| `buildQueryFromChannels` escapes XML-special chars | `[]string{"Foo&Bar"}` | `Path` attr contains `Foo&amp;Bar` |
| `ListChannels()` fails | `listChannels` returns error | `Start()` returns error, receiver does not start |
| All channels filtered out, `ignore_channel_errors=true` | No channels match | `Start()` returns nil, receiver idles |
| All channels filtered out, `ignore_channel_errors=false` | No channels match | `Start()` returns error |
| Persist key stable across order | `["B","A"]` vs `["A","B"]` | Same key |
| Persist key stable across casing | `["Security"]` vs `["SECURITY"]` | Same key |
| Canonicalization deduplicates | `["Sec","sec","SEC"]` | Single entry, single `<Select>` |
| Whitespace-only channel_list rejected at validation | `["   ", ""]` | Config error: "either `channel`, `channel_list`, or `query` must be set" |

### What Stays The Same

- `query` field: used verbatim, no filtering
- Subscription and polling logic: unchanged — receives a generated query
- `ignore_channel_errors` behavior during polling: unchanged
- All error handling in `pollAndRead` and `readBatch`
- Non-Windows builds: `channel_list` validates normally; receiver creation fails with "windows eventlog receiver is only supported on Windows" (from `receiver_others.go`), same as before
