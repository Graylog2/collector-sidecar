# wevttest CLI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a standalone CLI tool (`wevttest`) to test the Windows Event Log receiver on Windows machines, with `stream` and `list` subcommands and XML/JSON/OTel output formats.

**Architecture:** Go subcommand-style CLI using `flag.FlagSet`. The `stream` command wires up the real OTel receiver with a custom consumer that formats output to stdout. The `list` command uses new `EvtOpenChannelEnum`/`EvtNextChannelPath` bindings added to `internal/windows/`.

**Tech Stack:** Go stdlib (`flag`, `encoding/json`, `os/signal`), OTel pdata/plog for marshaling, existing receiver factory and internal packages.

---

### Task 1: Add channel enumeration API bindings to internal/windows/

**Files:**
- Modify: `receiver/windowseventlogreceiver/internal/windows/api.go`
- Create: `receiver/windowseventlogreceiver/internal/windows/channels.go` (build tag: `windows`)
- Create: `receiver/windowseventlogreceiver/internal/windows/channels_others.go` (build tag: `!windows`)

**Step 1: Add syscall proc declarations to api.go**

Add to the `var` block in `api.go`:

```go
openChannelEnumProc  SyscallProc = api.NewProc("EvtOpenChannelEnum")
nextChannelPathProc  SyscallProc = api.NewProc("EvtNextChannelPath")
```

**Step 2: Create channels.go with Windows implementation**

```go
//go:build windows

package windows

import (
	"errors"
	"sort"
	"syscall"
	"unsafe"
)

// evtOpenChannelEnum opens a handle to enumerate event log channels.
var evtOpenChannelEnum = func(session uintptr, flags uint32) (uintptr, error) {
	handle, _, err := openChannelEnumProc.Call(session, uintptr(flags))
	if !errors.Is(err, ErrorSuccess) {
		return 0, err
	}
	return handle, nil
}

// evtNextChannelPath retrieves the next channel path from the enumerator.
// Returns the channel path, or ("", ErrorNoMoreItems) when done.
var evtNextChannelPath = func(channelEnum uintptr, channelPathBufferSize uint32, channelPathBuffer *uint16, channelPathBufferUsed *uint32) error {
	_, _, err := nextChannelPathProc.Call(
		channelEnum,
		uintptr(channelPathBufferSize),
		uintptr(unsafe.Pointer(channelPathBuffer)),
		uintptr(unsafe.Pointer(channelPathBufferUsed)),
	)
	if !errors.Is(err, ErrorSuccess) {
		return err
	}
	return nil
}

// ListChannels enumerates all available Windows Event Log channels
// and returns them sorted alphabetically.
func ListChannels() ([]string, error) {
	handle, err := evtOpenChannelEnum(0, 0)
	if err != nil {
		return nil, fmt.Errorf("EvtOpenChannelEnum: %w", err)
	}
	defer evtClose(handle)

	var channels []string
	buf := make([]uint16, 512)

	for {
		var used uint32
		err := evtNextChannelPath(handle, uint32(len(buf)), &buf[0], &used)
		if errors.Is(err, ErrorNoMoreItems) {
			break
		}
		if errors.Is(err, ErrorInsufficientBuffer) {
			buf = make([]uint16, used)
			err = evtNextChannelPath(handle, uint32(len(buf)), &buf[0], &used)
			if err != nil {
				return nil, fmt.Errorf("EvtNextChannelPath: %w", err)
			}
		} else if err != nil {
			return nil, fmt.Errorf("EvtNextChannelPath: %w", err)
		}
		channels = append(channels, syscall.UTF16ToString(buf[:used]))
	}

	sort.Strings(channels)
	return channels, nil
}
```

Note: add `"fmt"` to the import block.

**Step 3: Create channels_others.go stub**

```go
//go:build !windows

package windows

import "errors"

// ListChannels is only supported on Windows.
func ListChannels() ([]string, error) {
	return nil, errors.New("channel enumeration is only supported on Windows")
}
```

**Step 4: Verify compilation**

Run from repo root: `cd receiver/windowseventlogreceiver && go vet ./internal/windows/`

(This only checks non-Windows build since we're on Linux. Full verification happens on Windows.)

---

### Task 2: Create CLI entry point with subcommand dispatch

**Files:**
- Create: `receiver/windowseventlogreceiver/cmd/wevttest/main.go`

**Step 1: Write main.go**

```go
package main

import (
	"flag"
	"fmt"
	"os"
)

const usage = `wevttest — Windows Event Log receiver test tool

Usage:
  wevttest <command> [flags] [args...]

Commands:
  stream    Live-stream events from one or more channels
  list      List all available event log channels

Run 'wevttest <command> -h' for command-specific help.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "stream":
		if err := runStream(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "list":
		if err := runList(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
}

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	fs.Parse(args)
	return listChannels()
}
```

**Step 2: Verify it compiles (will fail until stream/list are implemented)**

This is a checkpoint — the remaining tasks will add the missing functions.

---

### Task 3: Implement the list command

**Files:**
- Create: `receiver/windowseventlogreceiver/cmd/wevttest/list_windows.go`
- Create: `receiver/windowseventlogreceiver/cmd/wevttest/list_others.go`

**Step 1: Write list_windows.go**

```go
//go:build windows

package main

import (
	"fmt"

	"github.com/Graylog2/collector-sidecar/receiver/windowseventlogreceiver/internal/windows"
)

func listChannels() error {
	channels, err := windows.ListChannels()
	if err != nil {
		return err
	}
	for _, ch := range channels {
		fmt.Println(ch)
	}
	return nil
}
```

**Step 2: Write list_others.go**

```go
//go:build !windows

package main

import "errors"

func listChannels() error {
	return errors.New("the list command is only supported on Windows")
}
```

---

### Task 4: Implement output formatting

**Files:**
- Create: `receiver/windowseventlogreceiver/cmd/wevttest/format.go`

**Step 1: Write format.go**

```go
package main

import (
	"encoding/json"
	"fmt"
	"io"

	"go.opentelemetry.io/collector/pdata/plog"
)

type outputFormat string

const (
	formatJSON outputFormat = "json"
	formatXML  outputFormat = "xml"
	formatOTel outputFormat = "otel"
)

func validFormat(s string) (outputFormat, error) {
	switch outputFormat(s) {
	case formatJSON, formatXML, formatOTel:
		return outputFormat(s), nil
	default:
		return "", fmt.Errorf("invalid format %q (valid: json, xml, otel)", s)
	}
}

// formatter writes log records to the given writer in the specified format.
type formatter struct {
	format outputFormat
	w      io.Writer
}

func (f *formatter) writeLogs(ld plog.Logs) error {
	switch f.format {
	case formatOTel:
		return f.writeOTel(ld)
	case formatJSON:
		return f.writeJSON(ld)
	case formatXML:
		return f.writeXML(ld)
	default:
		return fmt.Errorf("unsupported format: %s", f.format)
	}
}

func (f *formatter) writeOTel(ld plog.Logs) error {
	m := &plog.JSONMarshaler{}
	data, err := m.MarshalLogs(ld)
	if err != nil {
		return err
	}
	// Pretty-print the JSON
	var raw json.RawMessage = data
	pretty, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f.w, "%s\n", pretty)
	return err
}

func (f *formatter) writeJSON(ld plog.Logs) error {
	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		rl := ld.ResourceLogs().At(i)
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			for k := 0; k < sl.LogRecords().Len(); k++ {
				lr := sl.LogRecords().At(k)
				body := lr.Body()
				if body.Type() == plog.Logs{}.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().Body().Type() {
					// Default empty — skip type check, just use AsRaw
				}
				raw := body.AsRaw()
				pretty, err := json.MarshalIndent(raw, "", "  ")
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintf(f.w, "%s\n", pretty); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (f *formatter) writeXML(ld plog.Logs) error {
	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		rl := ld.ResourceLogs().At(i)
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			for k := 0; k < sl.LogRecords().Len(); k++ {
				lr := sl.LogRecords().At(k)
				// log.record.original contains the raw XML
				val, ok := lr.Attributes().Get("log.record.original")
				if !ok {
					fmt.Fprintf(f.w, "<!-- no original XML available -->\n")
					continue
				}
				if _, err := fmt.Fprintf(f.w, "%s\n", val.Str()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
```

Note: The body type check is awkward — simplify to just calling `body.AsRaw()` directly, which returns `nil` for empty bodies and a `map[string]any` for map bodies. Clean this up during implementation.

---

### Task 5: Implement the stream command

**Files:**
- Create: `receiver/windowseventlogreceiver/cmd/wevttest/stream.go` (shared logic)
- Create: `receiver/windowseventlogreceiver/cmd/wevttest/stream_windows.go` (Windows receiver wiring)
- Create: `receiver/windowseventlogreceiver/cmd/wevttest/stream_others.go` (stub)

**Step 1: Write stream.go with flag parsing and orchestration**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
)

var defaultChannels = []string{"Application", "System", "Security"}

func runStream(args []string) error {
	fs := flag.NewFlagSet("stream", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: wevttest stream [flags] [channel...]

Live-stream events from Windows Event Log channels.

Default channels: Application, System, Security

Flags:
`)
		fs.PrintDefaults()
	}

	formatStr := fs.String("format", "json", "output format: json, xml, otel")
	startAt := fs.String("start-at", "end", "start position: beginning, end")
	fs.Parse(args)

	format, err := validFormat(*formatStr)
	if err != nil {
		return err
	}

	if *startAt != "beginning" && *startAt != "end" {
		return fmt.Errorf("invalid --start-at value %q (valid: beginning, end)", *startAt)
	}

	channels := fs.Args()
	if len(channels) == 0 {
		channels = defaultChannels
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	f := &formatter{format: format, w: os.Stdout}
	return streamEvents(ctx, channels, *startAt, format == formatXML, f)
}
```

**Step 2: Write stream_windows.go with OTel pipeline wiring**

```go
//go:build windows

package main

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"

	wel "github.com/Graylog2/collector-sidecar/receiver/windowseventlogreceiver"
)

func streamEvents(ctx context.Context, channels []string, startAt string, includeOriginalXML bool, f *formatter) error {
	logger, _ := zap.NewDevelopment()

	cons, err := consumer.NewLogs(func(ctx context.Context, ld plog.Logs) error {
		return f.writeLogs(ld)
	})
	if err != nil {
		return fmt.Errorf("creating consumer: %w", err)
	}

	factory := wel.NewFactory()
	var receivers []receiver.Logs

	for _, ch := range channels {
		cfg := factory.CreateDefaultConfig().(*wel.WindowsLogConfig)
		cfg.Config.Channel = ch
		cfg.Config.StartAt = startAt
		cfg.Config.IncludeLogRecordOriginal = includeOriginalXML

		settings := receiver.Settings{
			TelemetrySettings: component.TelemetrySettings{
				Logger: logger.Named(ch),
			},
		}

		r, err := factory.CreateLogs(ctx, settings, cfg, cons)
		if err != nil {
			return fmt.Errorf("creating receiver for channel %q: %w", ch, err)
		}
		receivers = append(receivers, r)
	}

	// Start all receivers.
	for _, r := range receivers {
		if err := r.Start(ctx, componentHost{}); err != nil {
			return fmt.Errorf("starting receiver: %w", err)
		}
	}

	logger.Info("streaming events", zap.Strings("channels", channels), zap.String("format", string(f.format)))
	fmt.Fprintf(os.Stderr, "Streaming from %v (press Ctrl+C to stop)...\n", channels)

	// Block until interrupted.
	<-ctx.Done()

	fmt.Fprintln(os.Stderr, "\nShutting down...")

	// Shut down all receivers.
	shutdownCtx := context.Background()
	var wg sync.WaitGroup
	for _, r := range receivers {
		wg.Add(1)
		go func(r receiver.Logs) {
			defer wg.Done()
			r.Shutdown(shutdownCtx)
		}(r)
	}
	wg.Wait()

	return nil
}

// componentHost is a minimal implementation of component.Host.
type componentHost struct{}

func (componentHost) GetFactory(component.Kind, component.Type) component.Factory { return nil }
func (componentHost) GetExtensions() map[component.ID]component.Component          { return nil }
```

Note: The exact `component.Host` interface may have changed in v1.52.0. Verify during implementation by checking `go doc go.opentelemetry.io/collector/component.Host`. Add `"os"` to the import block.

**Step 3: Write stream_others.go stub**

```go
//go:build !windows

package main

import "errors"

func streamEvents(_ context.Context, _ []string, _ string, _ bool, _ *formatter) error {
	return errors.New("the stream command is only supported on Windows")
}
```

Note: add `"context"` import.

---

### Task 6: Update go.mod and verify compilation

**Step 1: Add pdata dependency to go.mod**

Run: `cd receiver/windowseventlogreceiver && go get go.opentelemetry.io/collector/pdata@v1.52.0`

(The `plog` package lives in `pdata`.)

**Step 2: Tidy modules**

Run: `cd receiver/windowseventlogreceiver && go mod tidy`

**Step 3: Verify non-Windows compilation**

Run: `cd receiver/windowseventlogreceiver && go vet ./cmd/wevttest/`

**Step 4: Verify Windows cross-compilation**

Run: `cd receiver/windowseventlogreceiver && GOOS=windows GOARCH=amd64 go build ./cmd/wevttest/`

This produces a `wevttest.exe` in the module directory. Delete it after: `rm receiver/windowseventlogreceiver/wevttest.exe`

**Step 5: Format**

Run: `make fmt` from the repo root.

---

### Task 7: Add the CLI to the builder (optional)

Check if the builder config at `builder/builder-config.yaml` should reference the new CLI. If not (since this is a standalone test tool, not part of the collector), skip this task.

Most likely this is NOT needed — `wevttest` is built independently via `go build ./cmd/wevttest/`, not through the OTel collector builder.

---

### Task 8: Test on Windows

This is a manual step. Copy `wevttest.exe` to a Windows machine and run:

```powershell
# List all channels
.\wevttest.exe list

# Stream default channels (Application, System, Security) in JSON
.\wevttest.exe stream

# Stream specific channel in XML
.\wevttest.exe stream --format xml Application

# Stream from beginning in OTel format
.\wevttest.exe stream --format otel --start-at beginning System
```

---

## Notes for Implementer

1. **component.Host interface**: The exact methods on `component.Host` may differ in collector v1.52.0. Check with `go doc` and implement the minimum required methods.

2. **TelemetrySettings**: Creating `component.TelemetrySettings` may require more fields than just `Logger` in v1.52.0 (e.g., `MeterProvider`, `TracerProvider`). Use `component.TelemetrySettings{Logger: logger}` and add noop providers if the compiler complains.

3. **plog.JSONMarshaler**: Verify this exists in `go.opentelemetry.io/collector/pdata/plog` v1.52.0. Alternative: `go.opentelemetry.io/collector/pdata/plog/plogotlp`.

4. **format.go body type check**: The awkward body type comparison in `writeJSON` should just be `body.AsRaw()` — if the body is a map it returns `map[string]any`, if empty it returns `nil`. Handle `nil` gracefully.

5. **stream_windows.go imports**: Will need `"os"` for `os.Stderr` and `"context"` in the `_others.go` stub.

6. **Cross-compilation**: `GOOS=windows go build` checks type correctness but doesn't link Windows DLLs, so it's a good smoke test from Linux.
