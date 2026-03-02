# wevttest CLI Design

**Date:** 2026-02-28
**Status:** Accepted

## Context

The custom Windows Event Log receiver needs a standalone test tool to verify behavior on Windows machines without running the full Graylog Sidecar + OTel collector pipeline. The tool should be small, zero-dependency (beyond what the receiver already uses), and focused on two tasks: streaming events and listing available channels.

## CLI Interface

```
wevttest — Windows Event Log receiver test tool

Usage:
  wevttest <command> [flags] [args...]

Commands:
  stream    Live-stream events from one or more channels
  list      List all available event log channels

Stream usage:
  wevttest stream [--format xml|json|otel] [--start-at beginning|end] [channel...]

  Defaults: format=json, start-at=end, channels=Application,System,Security

  Formats:
    xml   — Raw Windows event XML
    json  — Structured JSON (receiver's formattedBody output, pretty-printed)
    otel  — OTLP JSON (plog.Logs marshaled via plog.JSONMarshaler)

List usage:
  wevttest list

  Prints one channel name per line, sorted alphabetically.
```

Channels are positional arguments to the stream command. The `--format` flag controls output. The `--start-at` flag controls whether to read from the beginning or end of the log (default: end, meaning only new events).

## Architecture

### Stream Command

1. For each channel, create a receiver config with that channel name and the chosen `start_at` value.
2. Set `include_log_record_original: true` (needed for XML output format).
3. Create a custom `consumer.Logs` that intercepts log records and formats them to stdout.
4. The consumer callback formats each `plog.LogRecord` according to `--format`:
   - **json**: Extract the body (`pcommon.Map` from `formattedBody()`), convert via `AsRaw()`, then `json.MarshalIndent`.
   - **xml**: Extract the `log.record.original` attribute value and print.
   - **otel**: Use `plog.JSONMarshaler` to marshal the full `plog.Logs`.
5. Start all receivers, block on `signal.NotifyContext(ctx, os.Interrupt)`, then shut down gracefully.

Each event is printed as a complete JSON object or XML document, separated by newlines for easy piping to `jq` or other tools.

### List Command

Uses `EvtOpenChannelEnum` / `EvtNextChannelPath` Windows API calls (added to `internal/windows/`) to enumerate all available event log channels. Results are sorted alphabetically and printed one per line.

### File Structure

```
receiver/windowseventlogreceiver/cmd/wevttest/
├── main.go           — Entry point, subcommand dispatch
├── stream.go         — Stream command (platform-independent orchestration)
├── stream_windows.go — Windows-specific receiver instantiation
├── stream_others.go  — Non-Windows stub (error: "requires Windows")
├── list_windows.go   — Channel enumeration via Windows API
├── list_others.go    — Non-Windows stub
└── format.go         — Output formatting (json, xml, otel)
```

Channel enumeration API bindings are added to `internal/windows/` for reusability:
- `EvtOpenChannelEnum` and `EvtNextChannelPath` syscall wrappers in `api.go`
- A `ListChannels()` function that returns `[]string`

### CLI Framework

Go subcommand style using `flag.FlagSet` per subcommand. No external CLI framework dependencies.

## Output Formats

### JSON (default)

Pretty-printed structured event map — the same structure the receiver produces in `formattedBody()`:

```json
{
  "event_id": { "id": 7036, "qualifiers": 0 },
  "provider": { "name": "Service Control Manager" },
  "system_time": "2024-01-15T10:30:00.000Z",
  "computer": "DESKTOP-ABC",
  "channel": "System",
  "record_id": 12345,
  "level": "Information",
  "message": "The Windows Update service entered the running state.",
  "event_data": { "param1": "Windows Update", "param2": "running" }
}
```

### XML

Raw Windows event XML as rendered by the receiver:

```xml
<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
  <System>...</System>
  <EventData>...</EventData>
</Event>
```

### OTel (OTLP JSON)

Full OTLP JSON representation of `plog.Logs` as the OTLP exporter would produce:

```json
{
  "resourceLogs": [{
    "scopeLogs": [{
      "logRecords": [{
        "timeUnixNano": "...",
        "severityNumber": 9,
        "body": { "kvlistValue": { "values": [...] } }
      }]
    }]
  }]
}
```

## Decisions

- **Location:** Inside the receiver module at `cmd/wevttest/` — co-located with the code it tests, can import internal packages.
- **Wiring:** Minimal OTel pipeline with the real receiver and a custom consumer — tests the actual code path including stanza adapter, bookmark, SID resolution, rendering fallback chain.
- **Default channels:** Application, System, Security.
- **Scope:** Minimal flags only (format, start-at). This is a test/debug tool, not a production collector.
- **Channel enumeration:** Added to `internal/windows/` for reusability.
