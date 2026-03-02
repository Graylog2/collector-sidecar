# Windows Event Log Receiver Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a custom OTel receiver (`windowseventlog`) by forking the contrib stanza Windows Event Log operator and adding SID resolution, `%%ID` expansion, message template fallback, UserData parsing, locale control, exponential backoff, strict bookmark recovery, ProcessingErrorData diagnostics, audit outcome enrichment, and robustness fixes.

**Architecture:** Fork `pkg/stanza/operator/input/windows/` from OTel contrib into `receiver/windowseventlogreceiver/internal/windows/`. Wrap it with a thin OTel receiver factory. New capabilities are added as separate files (sid.go, paramexpand.go, msgtemplate.go, backoff.go) and wired into the forked event processing pipeline. Platform-independent logic is testable on Linux; Windows API calls are behind build tags.

**Tech Stack:** Go 1.26, OTel Collector SDK v0.146, wevtapi.dll syscalls, `golang.org/x/sys/windows`, `text/template`

**Design doc:** `docs/plans/2026-02-28-windows-eventlog-receiver-design.md`

---

## Task Dependency Graph

```
Task 1 (scaffold)
  → Task 2 (fork base files)
    → Task 3 (XML query validation)
    → Task 4 (UserData parsing)
    → Task 5 (timestamp/severity)
    → Task 5a (audit outcome enrichment)
    → Task 5b (ProcessingErrorData diagnostics)
    → Task 5c (XML input sanitization)
    → Task 6 (locale/language)
    → Task 7 (backoff)
      → Task 8 (transient open recovery)
      → Task 8a (strict bookmark recovery)
    → Task 9 (SID resolution)
    → Task 10 (%%ID expansion)
      → Task 11 (message template fallback)
    → Task 12 (wire into event pipeline)
      → Task 13 (builder integration)
```

Tasks 3-11 can be done in parallel after Task 2. Task 12 depends on all of 3-11. Task 13 depends on 12.

---

### Task 1: Scaffold the Module

Create the Go module structure and receiver wrapper files. No forked code yet — just the skeleton that compiles.

**Files:**
- Create: `receiver/windowseventlogreceiver/go.mod`
- Create: `receiver/windowseventlogreceiver/factory.go`
- Create: `receiver/windowseventlogreceiver/config.go`
- Create: `receiver/windowseventlogreceiver/receiver_windows.go`
- Create: `receiver/windowseventlogreceiver/receiver_others.go`
- Create: `receiver/windowseventlogreceiver/internal/windows/.gitkeep`

**Step 1: Create the go.mod**

```
receiver/windowseventlogreceiver/go.mod
```

The module name should be `github.com/Graylog2/collector-sidecar/receiver/windowseventlogreceiver`. Use `go 1.26.0`. Dependencies needed:
- `go.opentelemetry.io/collector/receiver` (same version as builder/go.mod — v1.52.0 range)
- `go.opentelemetry.io/collector/component`
- `github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza` (for adapter and helper types — v0.146.0)
- `go.uber.org/zap`

Run `go mod tidy` after creating. Use the builder/go.mod as reference for exact versions.

**Step 2: Create factory.go**

Model after: `3rd-party/opentelemetry-collector-contrib/receiver/windowseventlogreceiver/factory.go`

```go
package windowseventlogreceiver

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/receiver"
)

const typeStr = "windowseventlog"

func NewFactory() receiver.Factory {
	return newFactoryAdapter()
}

func createDefaultConfig() component.Config {
	return &Config{}
}
```

The `newFactoryAdapter()` function is platform-specific (next files).

**Step 3: Create config.go**

The config struct consolidates all options. Model after the OTel `config_all.go` but add our new fields and remove remote config.

```go
package windowseventlogreceiver

import (
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator/helper"
)

type Config struct {
	helper.InputConfig       `mapstructure:",squash"`
	Channel                  string        `mapstructure:"channel"`
	IgnoreChannelErrors      bool          `mapstructure:"ignore_channel_errors,omitempty"`
	MaxReads                 int           `mapstructure:"max_reads,omitempty"`
	StartAt                  string        `mapstructure:"start_at,omitempty"`
	PollInterval             time.Duration `mapstructure:"poll_interval,omitempty"`
	Raw                      bool          `mapstructure:"raw,omitempty"`
	IncludeLogRecordOriginal bool          `mapstructure:"include_log_record_original,omitempty"`
	SuppressRenderingInfo    bool          `mapstructure:"suppress_rendering_info,omitempty"`
	ExcludeProviders         []string      `mapstructure:"exclude_providers,omitempty"`
	Query                    *string       `mapstructure:"query,omitempty"`
	// New fields
	ResolveSIDs  bool   `mapstructure:"resolve_sids,omitempty"`
	SIDCacheSize int    `mapstructure:"sid_cache_size,omitempty"`
	Language     uint32 `mapstructure:"language,omitempty"`
}
```

Defaults (set in a `NewConfig()` or equivalent):
- `MaxReads`: 100
- `StartAt`: "end"
- `PollInterval`: 1s
- `ResolveSIDs`: true
- `SIDCacheSize`: 1024
- `Language`: 0

**Step 4: Create receiver_others.go (non-Windows build stub)**

```go
//go:build !windows

package windowseventlogreceiver

import (
	"fmt"

	"go.opentelemetry.io/collector/receiver"
)

func newFactoryAdapter() receiver.Factory {
	// Return a factory that errors on non-Windows platforms
	// Model after OTel's receiver_others.go
}
```

**Step 5: Create receiver_windows.go (Windows build) — placeholder**

This file will be completed in Task 2 Step 8 after the internal package is forked. For now create a minimal placeholder so the module compiles on Linux (only `receiver_others.go` is active).

```go
//go:build windows

package windowseventlogreceiver

import (
	"go.opentelemetry.io/collector/receiver"
)

func newFactoryAdapter() receiver.Factory {
	// Completed in Task 2 after internal/windows is available
	panic("not yet wired")
}
```

**Step 6: Verify the module compiles**

Run: `cd receiver/windowseventlogreceiver && go mod tidy && go vet ./...`
Expected: No errors (on Linux, the non-Windows stub is used)

**Step 7: Commit**

```
feat: scaffold windowseventlog receiver module
```

---

### Task 2: Fork Base Files from OTel Contrib

Copy the stanza operator source files into our module, strip remote support, and verify compilation.

**Source:** `3rd-party/opentelemetry-collector-contrib/pkg/stanza/operator/input/windows/`
**Target:** `receiver/windowseventlogreceiver/internal/windows/`

**Step 1: Create README.md**

Create `receiver/windowseventlogreceiver/README.md` documenting the fork origin:

```markdown
# Windows Event Log Receiver

Custom OTel receiver for Windows Event Log collection, forked from
[opentelemetry-collector-contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib).

## Fork Origin

- **Upstream:** `pkg/stanza/operator/input/windows/` and `receiver/windowseventlogreceiver/`
- **Commit:** [`f214dc8da19d8a1e2d42d83693b61f5f17228f50`](https://github.com/open-telemetry/opentelemetry-collector-contrib/commit/f214dc8da19d8a1e2d42d83693b61f5f17228f50)
- **Version:** v0.146.0

## Changes from Upstream

See `docs/plans/2026-02-28-windows-eventlog-receiver-design.md` for the full
design document describing all improvements and out-of-scope items.
```

**Step 2: Copy all source files**

Copy these files from source to target, preserving names:
- `api.go`
- `subscription.go`
- `bookmark.go`
- `buffer.go`
- `input.go`
- `event.go`
- `xml.go`
- `publisher.go`
- `publishercache.go`
- `config_all.go`
- `config_windows.go`

Also copy test files:
- `xml_test.go`
- `buffer_test.go`
- `publishercache_test.go`

And test fixtures:
- `testdata/` directory (all XML sample files)

**Step 2: Update package declarations**

Change all `package windows` declarations to match the new package path. The package name stays `windows` but the import path changes to `github.com/Graylog2/collector-sidecar/receiver/windowseventlogreceiver/internal/windows`.

**Step 3: Strip remote support from api.go**

Remove:
- `EvtOpenSession` procedure and its `evtOpenSession` wrapper function
- `EvtRPCLogin` struct
- `EvtRPCLoginClass` constant

Keep everything else (all other wevtapi procedures, constants, error codes).

**Step 4: Strip remote support from subscription.go**

- Remove the `Server` field from `Subscription` struct
- Remove `NewRemoteSubscription()` function (keep `NewLocalSubscription()` or equivalent)
- In `Open()`: remove `sessionHandle` parameter, always pass 0 for session
- In `Read()`: remove remote-specific error handling branch

**Step 5: Strip remote support from input.go**

- Remove `RemoteConfig` usage from `Input` struct
- Remove `remoteSessionHandle`, `startRemoteSession` fields
- Remove `isRemote()` method
- Remove `startRemoteSession()` / `stopRemoteSession()` methods
- Remove `defaultStartRemoteSession()` function
- In `Start()`: remove remote session opening logic
- In `readBatch()`: remove remote resubscription branch (will be replaced with local reopen in Task 8)

**Step 6: Adapt config files**

Config ownership model: the internal `config_all.go` and `config_windows.go` stay
in `internal/windows/` and own the `Config` struct and `Build()` function. The
top-level `config.go` (from Task 1) becomes a thin wrapper that embeds the
internal config for the adapter. This matches the OTel contrib pattern where
`receiver.go` defines `WindowsLogConfig` embedding the internal `windows.Config`.

In `internal/windows/config_all.go`:
- Remove `RemoteConfig` struct and `Remote` field
- Remove `MaxEventsPerPoll` field
- Add `ResolveSIDs bool`, `SIDCacheSize int`, `Language uint32` fields
- Update `NewConfig()` defaults to include new fields

In `internal/windows/config_windows.go`:
- Remove remote config validation from `Build()`
- Remove `MaxEventsPerPoll` wiring from `Build()`
- Keep `Build()` as the constructor for `Input`

Update top-level `receiver/windowseventlogreceiver/config.go` to embed the
internal config:

```go
package windowseventlogreceiver

import (
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/adapter"
	"github.com/Graylog2/collector-sidecar/receiver/windowseventlogreceiver/internal/windows"
)

type WindowsLogConfig struct {
	windows.Config `mapstructure:",squash"`
	adapter.BaseConfig `mapstructure:",squash"`
}
```

**Step 7: Copy and adapt test fixtures**

Ensure `testdata/` directory has all XML sample files. Update import paths in test files.

**Step 8: Complete receiver_windows.go adapter wiring**

Now that the internal package exists, complete the receiver factory. Model after
the OTel contrib `receiver_windows.go` and `receiver.go`:

```go
//go:build windows

package windowseventlogreceiver

import (
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/adapter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/receiver"

	"github.com/Graylog2/collector-sidecar/receiver/windowseventlogreceiver/internal/windows"
)

func newFactoryAdapter() receiver.Factory {
	return adapter.NewFactory(&receiverType{}, component.StabilityLevelAlpha)
}

type receiverType struct{}

func (r *receiverType) Type() component.Type {
	return component.MustNewType(typeStr)
}

func (r *receiverType) CreateDefaultConfig() component.Config {
	return createDefaultConfig()
}

func (r *receiverType) BaseConfig(cfg component.Config) adapter.BaseConfig {
	return cfg.(*WindowsLogConfig).BaseConfig
}

func (r *receiverType) InputConfig(cfg component.Config) operator.Config {
	return operator.NewConfig(&cfg.(*WindowsLogConfig).Config)
}
```

Also update `createDefaultConfig()` in `factory.go` to return `*WindowsLogConfig`
with the internal config defaults populated.

**Step 9: Verify compilation and tests**

Run: `cd receiver/windowseventlogreceiver && go mod tidy && go vet ./...`
Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestParse -v`

The XML parsing tests (TestParseValidTimestamp, TestParseSeverity, TestParseBody,
TestParseEventData, TestUnmarshalWithUserData, TestParseNoRendered) should pass on
Linux since they don't use Windows APIs.

Expected: All platform-independent tests pass.

**Step 10: Commit**

```
feat: fork OTel stanza windows operator, strip remote support
```

---

### Task 3: XML Query Validation

Add XML syntax validation for the `query` config option in `Build()`.

**Files:**
- Modify: `receiver/windowseventlogreceiver/internal/windows/config_windows.go`
- Create: `receiver/windowseventlogreceiver/internal/windows/config_test.go`

**Step 1: Write the failing test**

Create `config_test.go` (no Windows build tag — test the validation logic):

```go
package windows

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateQueryXML_Valid(t *testing.T) {
	q := `<QueryList><Query><Select Path="Security">*</Select></Query></QueryList>`
	err := validateQueryXML(q)
	require.NoError(t, err)
}

func TestValidateQueryXML_Malformed(t *testing.T) {
	q := `<QueryList><Query><Select Path="Security">*</Select></Query>`  // missing closing tag
	err := validateQueryXML(q)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid xml_query")
}

func TestValidateQueryXML_Empty(t *testing.T) {
	err := validateQueryXML("")
	require.NoError(t, err) // empty is fine, means not set
}
```

**Step 2: Run test to verify it fails**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestValidateQueryXML -v`
Expected: FAIL — `validateQueryXML` not defined

**Step 3: Implement validateQueryXML**

Add to a new file or to config validation code (platform-independent):

```go
func validateQueryXML(query string) error {
	if query == "" {
		return nil
	}
	if err := xml.Unmarshal([]byte(query), &struct{}{}); err != nil {
		return fmt.Errorf("invalid xml_query: %w", err)
	}
	return nil
}
```

Call this from `Build()` when `c.Query != nil`:

```go
if c.Query != nil {
	if err := validateQueryXML(*c.Query); err != nil {
		return nil, err
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestValidateQueryXML -v`
Expected: PASS

**Step 5: Commit**

```
feat: validate XML query syntax at config time
```

---

### Task 4: UserData Parsing

Add `UserData` field to `EventXML` and emit it in structured output.

**Files:**
- Modify: `receiver/windowseventlogreceiver/internal/windows/xml.go`
- Modify: `receiver/windowseventlogreceiver/internal/windows/xml_test.go`

**Step 1: Write the failing test**

The OTel codebase already has `testdata/xmlSampleUserData.xml` and `TestUnmarshalWithUserData`. The existing test expects UserData to be DROPPED. Change it to expect UserData to be PARSED.

First, understand the UserData XML structure. The test file `xmlSampleUserData.xml` contains:
```xml
<UserData>
  <LogFileCleared xmlns="...">
    <SubjectUserSid>S-1-5-21-...</SubjectUserSid>
    <SubjectUserName>test_user</SubjectUserName>
    ...
  </LogFileCleared>
</UserData>
```

Add a new test:

```go
func TestFormattedBodyWithUserData(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "xmlSampleUserData.xml"))
	require.NoError(t, err)

	event, err := unmarshalEventXML(data)
	require.NoError(t, err)

	body := formattedBody(event)
	ud, ok := body["user_data"].(map[string]any)
	require.True(t, ok, "user_data should be present")
	require.Equal(t, "LogFileCleared", ud["xml_name"])
	require.Equal(t, "S-1-5-21-1148437859-4135665037-1195073887-1000", ud["SubjectUserSid"])
	require.Equal(t, "test_user", ud["SubjectUserName"])
	require.Equal(t, "TEST", ud["SubjectDomainName"])
}
```

**Step 2: Run test to verify it fails**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestFormattedBodyWithUserData -v`
Expected: FAIL — `user_data` key not present in body

**Step 3: Implement UserData parsing**

UserData XML has a dynamic child element name (e.g. `<LogFileCleared>`, `<EventData>` etc.). We need a custom XML unmarshaler.

Add to `xml.go`:

```go
// UserData represents the <UserData> element which contains a single
// provider-specific child element with key-value data fields.
type UserData struct {
	Name xml.Name // The child element's name (e.g. LogFileCleared)
	Data []Data   // Parsed child elements as key-value pairs
}

func (u *UserData) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	// Read the single child element (e.g. <LogFileCleared>)
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			u.Name = t.Name
			// Read all grandchild elements as Data pairs
			var inner struct {
				Data []Data `xml:",any"`
			}
			// We need to decode the children of this element
			// Use a struct with xml:",any" to capture all children
			if err := d.DecodeElement(&inner, &t); err != nil {
				return err
			}
			u.Data = inner.Data
		case xml.EndElement:
			return nil
		}
	}
}
```

The `Data` struct already exists in the codebase for EventData. It has `Name` and `Value` fields. Reuse it.

Add `UserData` field to `EventXML`:

```go
type EventXML struct {
	// ... existing fields ...
	EventData EventData    `xml:"EventData"`
	UserData  *UserData    `xml:"UserData"`  // NEW
	// ...
}
```

Add to `formattedBody()`:

```go
if e.UserData != nil && len(e.UserData.Data) > 0 {
	ud := map[string]any{}
	ud["xml_name"] = e.UserData.Name.Local
	for _, d := range e.UserData.Data {
		ud[d.Name] = d.Value
	}
	body["user_data"] = ud
}
```

Note: The `Data` struct for UserData children may need adjustment. In EventData, `Data` elements have `<Data Name="key">value</Data>`. In UserData, children are `<SubjectUserSid>value</SubjectUserSid>` — the element name IS the key, not a Name attribute. We need to handle this differently:

```go
// UserDataEntry represents a child element under the UserData wrapper.
// Unlike EventData where Name is an attribute, here the XML element name is the key.
type UserDataEntry struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}
```

The `UnmarshalXML` for UserData should capture these as `[]UserDataEntry`.

**Step 4: Run tests to verify they pass**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestFormattedBodyWithUserData -v`
Expected: PASS

Also run existing XML tests to ensure no regression:
Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestParse -v`
Expected: All pass (events without UserData unaffected)

**Step 5: Commit**

```
feat: parse UserData payloads from Windows Event Log XML
```

---

### Task 5: Timestamp and Severity Robustness

Fix `parseSeverity()` to prefer numeric Level and add warning for timestamp fallback.

**Files:**
- Modify: `receiver/windowseventlogreceiver/internal/windows/xml.go`
- Modify: `receiver/windowseventlogreceiver/internal/windows/xml_test.go`

**Step 1: Write failing tests for severity**

```go
func TestParseSeverity_NumericPreferred(t *testing.T) {
	// Numeric level should take priority over rendered string
	require.Equal(t, entry.Error, parseSeverity("Fehler", "2"))   // German "Error"
	require.Equal(t, entry.Warn, parseSeverity("Warnung", "3"))   // German "Warning"
	require.Equal(t, entry.Fatal, parseSeverity("Critique", "1")) // French "Critical"
	require.Equal(t, entry.Info, parseSeverity("情報", "4"))       // Japanese "Information"
}

func TestParseSeverity_FallbackToRendered(t *testing.T) {
	// When numeric level is empty or unrecognized, fall back to rendered string
	require.Equal(t, entry.Error, parseSeverity("Error", ""))
	require.Equal(t, entry.Warn, parseSeverity("Warning", "99"))
}
```

**Step 2: Run test to verify it fails**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestParseSeverity_Numeric -v`
Expected: FAIL — current implementation checks rendered string first

**Step 3: Rewrite parseSeverity**

```go
func parseSeverity(renderedLevel, level string) entry.Severity {
	// Prefer numeric level (locale-independent)
	switch level {
	case "1":
		return entry.Fatal
	case "2":
		return entry.Error
	case "3":
		return entry.Warn
	case "4":
		return entry.Info
	}
	// Fall back to rendered string (English)
	switch renderedLevel {
	case "Critical":
		return entry.Fatal
	case "Error":
		return entry.Error
	case "Warning":
		return entry.Warn
	case "Information":
		return entry.Info
	}
	return entry.Default
}
```

**Step 4: Write failing test for timestamp warning**

The `parseTimestamp` function currently returns `time.Now()` silently. Change its signature to also return a boolean indicating fallback was used, or accept a logger. Since the function is called from `sendEvent` in `input.go`, the simplest approach is to return `(time.Time, error)`:

```go
func TestParseTimestamp_InvalidReturnsError(t *testing.T) {
	ts, err := parseTimestamp("not-a-timestamp")
	require.Error(t, err)
	// Should still return a time (now) for the caller to use
	require.WithinDuration(t, time.Now(), ts, time.Second)
}

func TestParseTimestamp_ValidNoError(t *testing.T) {
	ts, err := parseTimestamp("2022-04-22T10:20:52.3778625Z")
	require.NoError(t, err)
	require.Equal(t, 2022, ts.Year())
}
```

**Step 5: Implement parseTimestamp change**

```go
func parseTimestamp(ts string) (time.Time, error) {
	if timestamp, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return timestamp, nil
	}
	return time.Now(), fmt.Errorf("invalid timestamp %q, using current time", ts)
}
```

Update all callers in `input.go` (`sendEvent` function) to log the error as a warning:

```go
ts, err := parseTimestamp(eventXML.TimeCreated.SystemTime)
if err != nil {
	i.Logger().Warn("Timestamp parse failed, using current time", zap.Error(err))
}
```

**Step 6: Run all tests**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestParse -v`
Expected: All pass. Update any existing tests that depend on the old `parseSeverity` behavior (check that `TestParseSeverity` in the forked `xml_test.go` still passes with the reordered logic — it should, since numeric-first still handles all the same cases).

**Step 7: Commit**

```
fix: prefer numeric severity level, warn on timestamp fallback
```

---

### Task 5a: Audit Outcome Enrichment

Derive `outcome` field from keyword audit bits in Security channel events.

**Files:**
- Modify: `receiver/windowseventlogreceiver/internal/windows/xml.go`
- Modify: `receiver/windowseventlogreceiver/internal/windows/xml_test.go`

**Step 1: Write failing test**

```go
func TestFormattedBody_AuditSuccess(t *testing.T) {
	event := &EventXML{
		Keywords: []string{"0x8020000000000000"}, // audit success bit set
		// ... minimal required fields ...
	}
	body := formattedBody(event)
	require.Equal(t, "success", body["outcome"])
}

func TestFormattedBody_AuditFailure(t *testing.T) {
	event := &EventXML{
		Keywords: []string{"0x8010000000000000"}, // audit failure bit set
		// ... minimal required fields ...
	}
	body := formattedBody(event)
	require.Equal(t, "failure", body["outcome"])
}

func TestFormattedBody_NoAuditKeyword(t *testing.T) {
	event := &EventXML{
		Keywords: []string{"0x8000000000000000"},
		// ... minimal required fields ...
	}
	body := formattedBody(event)
	_, hasOutcome := body["outcome"]
	require.False(t, hasOutcome)
}
```

**Step 2: Run test to verify it fails**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestFormattedBody_Audit -v`
Expected: FAIL

**Step 3: Implement outcome derivation in formattedBody**

Add to `xml.go`:

```go
const (
	keywordAuditFailure = 0x10000000000000
	keywordAuditSuccess = 0x20000000000000
)

// parseKeywordBits extracts the raw keyword bitmask from the Keywords field.
// Windows stores it as a hex string like "0x8020000000000000".
func parseKeywordBits(keywords []string) uint64 {
	for _, kw := range keywords {
		if strings.HasPrefix(kw, "0x") || strings.HasPrefix(kw, "0X") {
			if v, err := strconv.ParseUint(kw[2:], 16, 64); err == nil {
				return v
			}
		}
	}
	return 0
}
```

In `formattedBody()`, after building the body map:

```go
kwBits := parseKeywordBits(e.Keywords)
if kwBits&keywordAuditFailure != 0 {
	body["outcome"] = "failure"
} else if kwBits&keywordAuditSuccess != 0 {
	body["outcome"] = "success"
}
```

**Step 4: Run tests**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestFormattedBody_Audit -v`
Expected: All pass

**Step 5: Commit**

```
feat: derive audit outcome from keyword bits
```

---

### Task 5b: ProcessingErrorData Diagnostics

Parse rendering error metadata from event XML.

**Files:**
- Modify: `receiver/windowseventlogreceiver/internal/windows/xml.go`
- Modify: `receiver/windowseventlogreceiver/internal/windows/xml_test.go`

**Step 1: Write failing test**

Create a test fixture `testdata/xmlSampleProcessingError.xml` with a `<ProcessingErrorData>` element:

```xml
<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
  <System>
    <Provider Name="TestProvider"/>
    <EventID>1000</EventID>
    <Level>4</Level>
    <Channel>Application</Channel>
    <Computer>test</Computer>
    <TimeCreated SystemTime="2024-01-15T10:00:00.000Z"/>
    <EventRecordID>1</EventRecordID>
  </System>
  <ProcessingErrorData>
    <ErrorCode>15027</ErrorCode>
    <DataItemName>message</DataItemName>
  </ProcessingErrorData>
</Event>
```

```go
func TestFormattedBody_ProcessingError(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "xmlSampleProcessingError.xml"))
	require.NoError(t, err)

	event, err := unmarshalEventXML(data)
	require.NoError(t, err)

	body := formattedBody(event)
	require.Equal(t, uint32(15027), body["error"].(map[string]any)["code"])
	require.Equal(t, "message", body["error"].(map[string]any)["data_item_name"])
}
```

**Step 2: Run test to verify it fails**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestFormattedBody_ProcessingError -v`
Expected: FAIL

**Step 3: Implement ProcessingErrorData fields**

Add to `EventXML` struct in `xml.go`:

```go
type EventXML struct {
	// ... existing fields ...
	RenderErrorCode         uint32 `xml:"ProcessingErrorData>ErrorCode"`
	RenderErrorDataItemName string `xml:"ProcessingErrorData>DataItemName"`
}
```

Add to `formattedBody()`:

```go
if e.RenderErrorCode != 0 || e.RenderErrorDataItemName != "" {
	errMap := map[string]any{}
	if e.RenderErrorCode != 0 {
		errMap["code"] = e.RenderErrorCode
	}
	if e.RenderErrorDataItemName != "" {
		errMap["data_item_name"] = e.RenderErrorDataItemName
	}
	body["error"] = errMap
}
```

**Step 4: Run tests**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestFormattedBody_ProcessingError -v`
Expected: PASS

Also run existing tests for regression:
Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestParse -v`
Expected: All pass (ProcessingErrorData absent in existing fixtures, so fields are zero-valued and not emitted)

**Step 5: Commit**

```
feat: parse ProcessingErrorData for rendering error diagnostics
```

---

### Task 5c: XML Input Sanitization

Strip invalid XML 1.0 characters before parsing event XML.

**Files:**
- Modify: `receiver/windowseventlogreceiver/internal/windows/xml.go`
- Modify: `receiver/windowseventlogreceiver/internal/windows/xml_test.go`

**Step 1: Write failing test**

```go
func TestUnmarshalEventXML_InvalidChars(t *testing.T) {
	// XML with control characters that are invalid in XML 1.0
	raw := `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
		<System>
			<Provider Name="Test"/>
			<EventID>1</EventID>
			<Level>4</Level>
			<Channel>App</Channel>
			<Computer>test` + "\x01\x08" + `</Computer>
			<TimeCreated SystemTime="2024-01-15T10:00:00.000Z"/>
			<EventRecordID>1</EventRecordID>
		</System>
	</Event>`
	event, err := unmarshalEventXML([]byte(raw))
	require.NoError(t, err)
	require.Equal(t, "test", event.Computer) // control chars stripped
}
```

**Step 2: Run test to verify it fails**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestUnmarshalEventXML_InvalidChars -v`
Expected: FAIL — xml.Unmarshal returns an error on invalid chars

**Step 3: Implement sanitizeXML**

Add to `xml.go`:

```go
// sanitizeXML strips characters that are invalid in XML 1.0.
// Valid XML 1.0 chars: #x9 | #xA | #xD | [#x20-#xD7FF] | [#xE000-#xFFFD] | [#x10000-#x10FFFF]
func sanitizeXML(data []byte) []byte {
	return bytes.Map(func(r rune) rune {
		if r == 0x09 || r == 0x0A || r == 0x0D ||
			(r >= 0x20 && r <= 0xD7FF) ||
			(r >= 0xE000 && r <= 0xFFFD) ||
			(r >= 0x10000 && r <= 0x10FFFF) {
			return r
		}
		return -1 // drop invalid character
	}, data)
}
```

Call it in `unmarshalEventXML`. Important: preserve the original raw bytes for
`event.Original` (used by `include_log_record_original`) — sanitization is only
for the XML parser, not for the forensic record:

```go
func unmarshalEventXML(data []byte) (*EventXML, error) {
	sanitized := sanitizeXML(data)
	var event EventXML
	if err := xml.Unmarshal(sanitized, &event); err != nil {
		return nil, err
	}
	event.Original = string(data) // raw bytes, not sanitized
	return &event, nil
}
```

**Step 4: Run tests**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run "TestUnmarshalEventXML_InvalidChars|TestParse" -v`
Expected: All pass

**Step 5: Commit**

```
feat: sanitize invalid XML characters before event parsing
```

---

### Task 6: Locale/Language Config

Add `language` config option and pass it through to publisher metadata opening.

**Files:**
- Modify: `receiver/windowseventlogreceiver/config.go` (already has `Language` field from Task 1)
- Modify: `receiver/windowseventlogreceiver/internal/windows/publisher.go`
- Modify: `receiver/windowseventlogreceiver/internal/windows/publishercache.go`
- Modify: `receiver/windowseventlogreceiver/internal/windows/input.go`

**Step 1: Modify publisher.go to accept locale**

The current `Publisher.Open(provider)` calls `evtOpenPublisherMetadata(0, utf16, nil, 0, 0)`. Change to accept a locale parameter:

```go
func (p *Publisher) Open(provider string, locale uint32) error {
	utf16, err := syscall.UTF16PtrFromString(provider)
	if err != nil {
		return err
	}
	handle, err := evtOpenPublisherMetadata(0, utf16, nil, locale, 0)
	if err != nil {
		return err
	}
	p.handle = handle
	return nil
}
```

**Step 2: Thread locale through publisherCache**

Add `locale uint32` field to `publisherCache`. Pass it when opening publishers:

```go
type publisherCache struct {
	cache  map[string]Publisher
	locale uint32
}

func newPublisherCache(locale uint32) publisherCache {
	return publisherCache{
		cache:  make(map[string]Publisher),
		locale: locale,
	}
}
```

In `get(provider)`:
```go
p.Open(provider, pc.locale)
```

**Step 3: Thread locale from config to Input**

In `input.go`, the `Input` struct gets locale from config:

```go
type Input struct {
	// ...
	publisherCache publisherCache
	// ...
}
```

In `Build()` / constructor:
```go
publisherCache: newPublisherCache(cfg.Language),
```

**Step 4: This is Windows-only code — no cross-platform test needed**

The locale parameter is just threaded through. Verify compilation:
Run: `cd receiver/windowseventlogreceiver && go vet ./...`
Expected: No errors

**Step 5: Commit**

```
feat: add language config for publisher metadata locale
```

---

### Task 7: Exponential Backoff

New file with backoff logic and error classification.

**Files:**
- Create: `receiver/windowseventlogreceiver/internal/windows/backoff.go`
- Create: `receiver/windowseventlogreceiver/internal/windows/backoff_test.go`

**Step 1: Write failing tests**

```go
package windows

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackoff_InitialDelay(t *testing.T) {
	b := newBackoff()
	require.Equal(t, 5*time.Second, b.next())
}

func TestBackoff_Doubles(t *testing.T) {
	b := newBackoff()
	require.Equal(t, 5*time.Second, b.next())
	require.Equal(t, 10*time.Second, b.next())
	require.Equal(t, 20*time.Second, b.next())
	require.Equal(t, 40*time.Second, b.next())
	require.Equal(t, 60*time.Second, b.next()) // capped
	require.Equal(t, 60*time.Second, b.next()) // stays capped
}

func TestBackoff_Reset(t *testing.T) {
	b := newBackoff()
	b.next()
	b.next()
	b.reset()
	require.Equal(t, 5*time.Second, b.next())
}
```

**Step 2: Run test to verify it fails**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestBackoff -v`
Expected: FAIL — `newBackoff` not defined

**Step 3: Implement backoff.go**

```go
package windows

import "time"

const (
	backoffInitial = 5 * time.Second
	backoffMax     = 60 * time.Second
	backoffFactor  = 2
)

type backoff struct {
	current time.Duration
}

func newBackoff() *backoff {
	return &backoff{current: backoffInitial}
}

// next returns the current delay and advances to the next one.
func (b *backoff) next() time.Duration {
	d := b.current
	b.current *= backoffFactor
	if b.current > backoffMax {
		b.current = backoffMax
	}
	return d
}

// reset returns the backoff to its initial delay.
func (b *backoff) reset() {
	b.current = backoffInitial
}
```

**Step 4: Write failing tests for error classification**

```go
func TestIsRecoverableError(t *testing.T) {
	tests := []struct {
		name     string
		code     uint32
		expected bool
	}{
		{"ERROR_INVALID_HANDLE", 6, true},
		{"RPC_S_SERVER_UNAVAILABLE", 1722, true},
		{"RPC_S_CALL_CANCELLED", 1818, true},
		{"ERROR_EVT_QUERY_RESULT_STALE", 15011, true},
		{"ERROR_INVALID_PARAMETER", 87, true},
		{"ERROR_EVT_PUBLISHER_DISABLED", 15037, true},
		{"ERROR_EVT_CHANNEL_NOT_FOUND", 15007, false},
		{"ERROR_ACCESS_DENIED", 5, false},
		{"unknown error", 99999, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, isRecoverableError(tt.code))
		})
	}
}
```

**Step 5: Implement error classification**

```go
// Windows error codes for classification.
const (
	errorInvalidHandle        = 6
	errorInvalidParameter     = 87
	errorAccessDenied         = 5
	rpcServerUnavailable      = 1722
	rpcCallCancelled          = 1818
	evtQueryResultStale       = 15011
	evtChannelNotFound        = 15007
	evtPublisherDisabled      = 15037
)

func isRecoverableError(code uint32) bool {
	switch code {
	case errorInvalidHandle,
		rpcServerUnavailable,
		rpcCallCancelled,
		evtQueryResultStale,
		errorInvalidParameter,
		evtPublisherDisabled:
		return true
	default:
		return false
	}
}
```

Note: Use the actual error code constants from `golang.org/x/sys/windows` where available. Some of these may need to be defined locally since not all are in the standard library. Check `golang.org/x/sys/windows` for `ERROR_INVALID_HANDLE`, `ERROR_ACCESS_DENIED`, etc. and use those if available.

**Step 6: Run all backoff tests**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run "TestBackoff|TestIsRecoverable" -v`
Expected: All pass

**Step 7: Commit**

```
feat: add exponential backoff and error classification
```

---

### Task 8: Transient Subscription-Open Recovery

Modify the polling loop to retry transient open failures and reopen local subscriptions on runtime errors.

**Files:**
- Modify: `receiver/windowseventlogreceiver/internal/windows/input.go`

This task modifies Windows-specific code (the `Start()` and `readBatch()` methods) that can only be fully tested on Windows. The changes are structural:

**Step 1: Modify Start() to always start polling**

Current behavior: if `subscription.Open()` fails with a transient error, no goroutine is started. Change to:

```go
func (i *Input) Start(persister operator.Persister) error {
	// ... context setup, bookmark loading ...

	subscription := NewLocalSubscription()
	if err := subscription.Open(i.startAt, i.channel, i.query, i.bookmark); err != nil {
		if isNonTransientError(err) {
			if !i.ignoreChannelErrors {
				return fmt.Errorf("failed to open subscription: %w", err)
			}
			i.Logger().Warn("Non-transient error opening subscription, not starting", zap.Error(err))
			return nil
		}
		// Transient error: log and start polling anyway — it will retry
		i.Logger().Warn("Transient error opening subscription, will retry with backoff", zap.Error(err))
		i.subscriptionOpen = false
	} else {
		i.subscription = subscription
		i.subscriptionOpen = true
	}

	i.wg.Add(1)
	go i.pollAndRead(ctx)
	return nil
}
```

Add `subscriptionOpen bool` and `backoff *backoff` fields to `Input`.

**Step 2: Modify pollAndRead to handle closed subscription**

```go
func (i *Input) pollAndRead(ctx context.Context) {
	defer i.wg.Done()
	bo := newBackoff()
	for {
		if !i.subscriptionOpen {
			// Try to open subscription
			delay := bo.next()
			i.Logger().Info("Retrying subscription open", zap.Duration("delay", delay))
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			if err := i.subscription.Open(i.startAt, i.channel, i.query, i.bookmark); err != nil {
				i.Logger().Warn("Failed to reopen subscription", zap.Error(err))
				continue
			}
			i.subscriptionOpen = true
			bo.reset()
		}
		// ... existing poll logic ...
	}
}
```

**Step 3: Add local subscription reopen in readBatch**

When `Read()` returns `ERROR_INVALID_HANDLE` or `errSubscriptionHandleNotOpen`:

```go
if errors.Is(err, errSubscriptionHandleNotOpen) || isErrorCode(err, errorInvalidHandle) {
	i.Logger().Warn("Subscription handle invalid, closing and reopening")
	_ = i.subscription.Close()
	i.subscriptionOpen = false
	return false
}
```

This causes the `pollAndRead` loop to retry opening on the next iteration with backoff.

**Step 4: Verify compilation**

Run: `cd receiver/windowseventlogreceiver && go vet ./...`
Expected: No errors

**Step 5: Commit**

```
feat: retry transient subscription-open failures with backoff
```

---

### Task 8a: Strict Bookmark Recovery

Add `EvtSubscribeStrict` flag and deterministic stale bookmark recovery.

**Files:**
- Modify: `receiver/windowseventlogreceiver/internal/windows/subscription.go`
- Modify: `receiver/windowseventlogreceiver/internal/windows/api.go`

This is Windows-only code that modifies the subscription opening logic.

**Step 1: Add EvtSubscribeStrict constant to api.go**

```go
const (
	// ... existing constants ...
	// EvtSubscribeStrict causes EvtSubscribe to return an error if the
	// bookmarked event no longer exists in the log.
	EvtSubscribeStrict uint32 = 0x10000
)
```

Also add error code constants for stale bookmark detection:

```go
const (
	ErrorNotFound                       = 1168
	ErrorEvtQueryResultStale            = 15011
	ErrorEvtQueryResultInvalidPosition  = 15012
)
```

Note: verify the exact values against `golang.org/x/sys/windows` or Windows SDK headers.

**Step 2: Modify createFlags in subscription.go**

```go
func (*Subscription) createFlags(startAt string, bookmark Bookmark) uint32 {
	if bookmark.handle != 0 {
		return EvtSubscribeStartAfterBookmark | EvtSubscribeStrict
	}
	if startAt == "beginning" {
		return EvtSubscribeStartAtOldestRecord
	}
	return EvtSubscribeToFutureEvents
}
```

**Step 3: Add stale bookmark recovery in Open**

Modify `Open()` to catch stale bookmark errors and retry from oldest:

```go
func (s *Subscription) Open(startAt string, channel string, query *string, bookmark Bookmark) error {
	// ... existing signal event creation, UTF-16 conversion ...

	flags := s.createFlags(startAt, bookmark)
	handle, err := evtSubscribe(0, signalEvent, channelPtr, queryPtr, bookmark.handle, 0, 0, flags)
	if err != nil {
		// If bookmark is stale, retry from oldest
		if bookmark.handle != 0 && isStaleBookmarkError(err) {
			handle, err = evtSubscribe(0, signalEvent, channelPtr, queryPtr, 0, 0, 0, EvtSubscribeStartAtOldestRecord)
			if err != nil {
				return fmt.Errorf("failed to subscribe after stale bookmark recovery: %w", err)
			}
			// Fall through to success
		} else {
			return fmt.Errorf("failed to subscribe: %w", err)
		}
	}
	s.handle = handle
	return nil
}

func isStaleBookmarkError(err error) bool {
	// Check for ERROR_NOT_FOUND, ERROR_EVT_QUERY_RESULT_STALE,
	// ERROR_EVT_QUERY_RESULT_INVALID_POSITION
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch uint32(errno) {
		case ErrorNotFound, ErrorEvtQueryResultStale, ErrorEvtQueryResultInvalidPosition:
			return true
		}
	}
	return false
}
```

**Step 4: Verify compilation**

Run: `cd receiver/windowseventlogreceiver && go vet ./...`
Expected: No errors

**Step 5: Commit**

```
feat: add strict bookmark recovery on stale/missing bookmarks
```

---

### Task 9: SID Resolution

New file with SID lookup and LRU cache. The actual `LookupAccountSid` call is Windows-only, but the cache logic is platform-independent and testable.

**Files:**
- Create: `receiver/windowseventlogreceiver/internal/windows/sid.go`
- Create: `receiver/windowseventlogreceiver/internal/windows/sid_test.go`

**Step 1: Write failing tests for the cache**

Design the cache with a pluggable lookup function so it's testable without Windows:

```go
package windows

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSIDCache_Lookup(t *testing.T) {
	lookupCount := 0
	cache := newSIDCache(1024, 5*time.Minute, func(sid string) (*SIDInfo, error) {
		lookupCount++
		return &SIDInfo{
			UserName: "testuser",
			Domain:   "TESTDOMAIN",
			Type:     "User",
		}, nil
	})

	info, err := cache.resolve("S-1-5-21-123")
	require.NoError(t, err)
	require.Equal(t, "testuser", info.UserName)
	require.Equal(t, 1, lookupCount)

	// Second call should hit cache
	info, err = cache.resolve("S-1-5-21-123")
	require.NoError(t, err)
	require.Equal(t, "testuser", info.UserName)
	require.Equal(t, 1, lookupCount) // no new lookup
}

func TestSIDCache_NegativeCache(t *testing.T) {
	lookupCount := 0
	cache := newSIDCache(1024, 5*time.Minute, func(sid string) (*SIDInfo, error) {
		lookupCount++
		return nil, fmt.Errorf("not found")
	})

	info, err := cache.resolve("S-1-5-21-unknown")
	require.Error(t, err)
	require.Nil(t, info)

	// Second call should hit negative cache
	_, _ = cache.resolve("S-1-5-21-unknown")
	require.Equal(t, 1, lookupCount)
}

func TestSIDCache_TTLExpiry(t *testing.T) {
	lookupCount := 0
	cache := newSIDCache(1024, 1*time.Millisecond, func(sid string) (*SIDInfo, error) {
		lookupCount++
		return &SIDInfo{UserName: "user"}, nil
	})

	cache.resolve("S-1-5-21-123")
	time.Sleep(5 * time.Millisecond)
	cache.resolve("S-1-5-21-123")
	require.Equal(t, 2, lookupCount) // TTL expired, looked up again
}

func TestSIDCache_Eviction(t *testing.T) {
	lookupCount := 0
	cache := newSIDCache(2, 5*time.Minute, func(sid string) (*SIDInfo, error) {
		lookupCount++
		return &SIDInfo{UserName: sid}, nil
	})

	cache.resolve("sid1")
	cache.resolve("sid2")
	cache.resolve("sid3") // evicts sid1
	require.Equal(t, 3, lookupCount)

	cache.resolve("sid1") // cache miss, re-lookup
	require.Equal(t, 4, lookupCount)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestSIDCache -v`
Expected: FAIL

**Step 3: Implement sid.go**

```go
package windows

import (
	"fmt"
	"sync"
	"time"
)

// SIDInfo holds resolved SID information.
type SIDInfo struct {
	UserName string
	Domain   string
	Type     string // "User", "Group", "WellKnownGroup", "Computer", etc.
}

// sidLookupFunc is the function signature for SID resolution.
// On Windows this wraps LookupAccountSid; for testing it's injectable.
type sidLookupFunc func(sid string) (*SIDInfo, error)

type sidCacheEntry struct {
	info      *SIDInfo
	err       error
	expiresAt time.Time
}

type sidCache struct {
	mu       sync.Mutex
	entries  map[string]sidCacheEntry
	order    []string // LRU order (newest at end)
	maxSize  int
	ttl      time.Duration
	lookupFn sidLookupFunc
}

func newSIDCache(maxSize int, ttl time.Duration, lookupFn sidLookupFunc) *sidCache {
	return &sidCache{
		entries:  make(map[string]sidCacheEntry, maxSize),
		order:    make([]string, 0, maxSize),
		maxSize:  maxSize,
		ttl:      ttl,
		lookupFn: lookupFn,
	}
}

func (c *sidCache) resolve(sid string) (*SIDInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[sid]; ok && time.Now().Before(entry.expiresAt) {
		// Move to end of LRU order
		c.touch(sid)
		return entry.info, entry.err
	}

	// Cache miss or expired — perform lookup
	info, err := c.lookupFn(sid)
	c.put(sid, sidCacheEntry{
		info:      info,
		err:       err,
		expiresAt: time.Now().Add(c.ttl),
	})
	return info, err
}

func (c *sidCache) put(sid string, entry sidCacheEntry) {
	if _, exists := c.entries[sid]; exists {
		c.entries[sid] = entry
		c.touch(sid)
		return
	}
	if len(c.entries) >= c.maxSize {
		// Evict oldest
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
	c.entries[sid] = entry
	c.order = append(c.order, sid)
}

func (c *sidCache) touch(sid string) {
	for i, s := range c.order {
		if s == sid {
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, sid)
			return
		}
	}
}
```

Then create a Windows-specific file `sid_windows.go` with the actual `LookupAccountSid` call:

```go
//go:build windows

package windows

import (
	"fmt"
	"syscall"
	syswin "golang.org/x/sys/windows"
)

func defaultSIDLookup(sidStr string) (*SIDInfo, error) {
	sid, err := syswin.StringToSid(sidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SID %q: %w", sidStr, err)
	}
	account, domain, accType, err := sid.LookupAccount("")
	if err != nil {
		return nil, fmt.Errorf("lookup failed for %q: %w", sidStr, err)
	}
	return &SIDInfo{
		UserName: account,
		Domain:   domain,
		Type:     sidAccountTypeName(accType),
	}, nil
}

func sidAccountTypeName(t uint32) string {
	switch t {
	case 1: return "User"
	case 2: return "Group"
	case 3: return "Domain"
	case 4: return "Alias"
	case 5: return "WellKnownGroup"
	case 6: return "DeletedAccount"
	case 7: return "Invalid"
	case 8: return "Unknown"
	case 9: return "Computer"
	default: return "Unknown"
	}
}
```

**Step 4: Run tests**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestSIDCache -v`
Expected: All pass

**Step 5: Commit**

```
feat: add SID resolution with LRU cache and TTL
```

---

### Task 10: Parameter Message Expansion

New file that scans for `%%\d+` patterns and resolves them.

**Files:**
- Create: `receiver/windowseventlogreceiver/internal/windows/paramexpand.go`
- Create: `receiver/windowseventlogreceiver/internal/windows/paramexpand_test.go`

**Step 1: Write failing tests**

The expansion logic (regex scanning + replacement) is platform-independent. The actual `EvtFormatMessage` call is Windows-only. Use a pluggable resolver:

```go
package windows

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpandParams_NoParams(t *testing.T) {
	resolver := func(id uint32) (string, bool) { return "", false }
	result := expandParamMessages("No parameters here", resolver)
	require.Equal(t, "No parameters here", result)
}

func TestExpandParams_SingleParam(t *testing.T) {
	resolver := func(id uint32) (string, bool) {
		if id == 1234 {
			return "Success", true
		}
		return "", false
	}
	result := expandParamMessages("Status: %%1234", resolver)
	require.Equal(t, "Status: Success", result)
}

func TestExpandParams_MultipleParams(t *testing.T) {
	resolver := func(id uint32) (string, bool) {
		switch id {
		case 100: return "Read", true
		case 200: return "Write", true
		default: return "", false
		}
	}
	result := expandParamMessages("Permissions: %%100 and %%200", resolver)
	require.Equal(t, "Permissions: Read and Write", result)
}

func TestExpandParams_UnresolvableLeftAsIs(t *testing.T) {
	resolver := func(id uint32) (string, bool) { return "", false }
	result := expandParamMessages("Status: %%9999", resolver)
	require.Equal(t, "Status: %%9999", result)
}

func TestExpandParams_SinglePercent_NotExpanded(t *testing.T) {
	resolver := func(id uint32) (string, bool) { return "X", true }
	result := expandParamMessages("50% complete", resolver)
	require.Equal(t, "50% complete", result)
}
```

**Step 2: Run test to verify it fails**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestExpandParams -v`
Expected: FAIL

**Step 3: Implement paramexpand.go**

```go
package windows

import (
	"regexp"
	"strconv"
)

var paramPattern = regexp.MustCompile(`%%(\d+)`)

// paramResolver looks up a parameter message ID and returns the resolved
// string. Returns (resolved, true) on success or ("", false) if not found.
type paramResolver func(id uint32) (string, bool)

// expandParamMessages scans text for %%NNNN tokens and replaces them using
// the resolver. Unresolvable tokens are left as-is.
func expandParamMessages(text string, resolve paramResolver) string {
	return paramPattern.ReplaceAllStringFunc(text, func(match string) string {
		idStr := match[2:] // strip "%%"
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			return match
		}
		if resolved, ok := resolve(uint32(id)); ok {
			return resolved
		}
		return match
	})
}
```

A Windows-specific wrapper will create a `paramResolver` that calls `EvtFormatMessage` with `EvtFormatMessageId` and caches results. That integration happens in Task 12.

**Step 4: Run tests**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run TestExpandParams -v`
Expected: All pass

**Step 5: Commit**

```
feat: add %%ID parameter message expansion
```

---

### Task 11: Message Template Fallback

New file that caches message templates and renders them with a safe helper.

**Files:**
- Create: `receiver/windowseventlogreceiver/internal/windows/msgtemplate.go`
- Create: `receiver/windowseventlogreceiver/internal/windows/msgtemplate_test.go`

**Step 1: Write failing tests for template conversion**

Test converting Windows `%1`, `%2` format to Go templates and executing safely:

```go
package windows

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertWindowsTemplate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "User %1 logged on from %2", `User {{eventParam . 0}} logged on from {{eventParam . 1}}`},
		{"with format spec", "Size: %1!d! bytes", `Size: {{eventParam . 0}} bytes`},
		{"no params", "System started", "System started"},
		{"adjacent", "%1%2", `{{eventParam . 0}}{{eventParam . 1}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, convertWindowsTemplate(tt.input))
		})
	}
}

func TestExecuteTemplate(t *testing.T) {
	tmpl, err := compileTemplate("test", "User %1 logged on from %2")
	require.NoError(t, err)

	result, err := executeTemplate(tmpl, []string{"admin", "10.0.0.1"})
	require.NoError(t, err)
	require.Equal(t, "User admin logged on from 10.0.0.1", result)
}

func TestExecuteTemplate_MissingParam(t *testing.T) {
	tmpl, err := compileTemplate("test", "User %1 action %2 on %3")
	require.NoError(t, err)

	// Only 1 param provided, %2 and %3 should be preserved as placeholders
	result, err := executeTemplate(tmpl, []string{"admin"})
	require.NoError(t, err)
	require.Equal(t, "User admin action %2 on %3", result)
}

func TestExecuteTemplate_NoParams(t *testing.T) {
	tmpl, err := compileTemplate("test", "System restarted")
	require.NoError(t, err)

	result, err := executeTemplate(tmpl, nil)
	require.NoError(t, err)
	require.Equal(t, "System restarted", result)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run "TestConvertWindowsTemplate|TestExecuteTemplate" -v`
Expected: FAIL

**Step 3: Implement msgtemplate.go**

```go
package windows

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"text/template"
)

// windowsParamPattern matches %N or %N!format! in Windows message templates.
var windowsParamPattern = regexp.MustCompile(`%(\d+)(?:![^!]*!)?`)

// convertWindowsTemplate converts a Windows message template string to Go
// text/template syntax. %1 becomes {{eventParam . 0}}, %2 becomes
// {{eventParam . 1}}, etc. Format specifiers like %1!d! are stripped.
func convertWindowsTemplate(s string) string {
	return windowsParamPattern.ReplaceAllStringFunc(s, func(match string) string {
		sub := windowsParamPattern.FindStringSubmatch(match)
		n, err := strconv.Atoi(sub[1])
		if err != nil || n < 1 {
			return match
		}
		return fmt.Sprintf("{{eventParam . %d}}", n-1) // 1-based to 0-based
	})
}

// templateFuncs provides the eventParam helper.
var templateFuncs = template.FuncMap{
	"eventParam": func(params []string, index int) string {
		if index >= 0 && index < len(params) {
			return params[index]
		}
		// Preserve original placeholder for missing params
		return fmt.Sprintf("%%%d", index+1)
	},
}

// compileTemplate converts a Windows message template to a compiled Go template.
func compileTemplate(name, windowsTemplate string) (*template.Template, error) {
	goTemplate := convertWindowsTemplate(windowsTemplate)
	return template.New(name).Funcs(templateFuncs).Parse(goTemplate)
}

// executeTemplate runs a compiled template with the given parameter values.
func executeTemplate(tmpl *template.Template, params []string) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// templateKey identifies a message template by event ID and version.
type templateKey struct {
	eventID uint32
	version uint8
}

// templateCache stores compiled message templates per provider.
type templateCache struct {
	templates map[templateKey]*template.Template
}

func newTemplateCache() *templateCache {
	return &templateCache{
		templates: make(map[templateKey]*template.Template),
	}
}

func (c *templateCache) get(eventID uint32, version uint8) (*template.Template, bool) {
	t, ok := c.templates[templateKey{eventID, version}]
	return t, ok
}

func (c *templateCache) put(eventID uint32, version uint8, tmpl *template.Template) {
	c.templates[templateKey{eventID, version}] = tmpl
}
```

The Windows-specific code to enumerate provider event metadata (`EvtOpenEventMetadataEnum` / `EvtNextEventMetadata`) and populate the cache will be in a `msgtemplate_windows.go` file, wired in Task 12.

**Step 4: Run tests**

Run: `cd receiver/windowseventlogreceiver && go test ./internal/windows/ -run "TestConvertWindowsTemplate|TestExecuteTemplate" -v`
Expected: All pass

**Step 5: Commit**

```
feat: add message template fallback with safe parameter helper
```

---

### Task 12: Wire Improvements into Event Pipeline

Connect all new capabilities into the event processing flow in `event.go` and `input.go`.

**Files:**
- Modify: `receiver/windowseventlogreceiver/internal/windows/event.go`
- Modify: `receiver/windowseventlogreceiver/internal/windows/input.go`
- Modify: `receiver/windowseventlogreceiver/internal/windows/publishercache.go`
- Create: `receiver/windowseventlogreceiver/internal/windows/msgtemplate_windows.go`
- Create: `receiver/windowseventlogreceiver/internal/windows/paramexpand_windows.go`

This task is primarily Windows-specific integration code. The cross-platform-testable logic was built in Tasks 7-11.

**Step 1: Extend publisherCache entries**

Add template cache and param message cache to each publisher entry:

```go
type publisherEntry struct {
	publisher     Publisher
	templates     *templateCache     // message templates for this provider
	paramMessages map[uint32]string  // %%ID → resolved string cache
}
```

Modify `publisherCache` to store `publisherEntry` instead of just `Publisher`.

**Step 2: Create paramexpand_windows.go**

Windows-specific wrapper that creates a `paramResolver` backed by `EvtFormatMessage`:

```go
//go:build windows

package windows

// newPublisherParamResolver creates a paramResolver that uses EvtFormatMessage
// with EvtFormatMessageId to resolve %%NNNN parameter messages, caching results
// in the publisher entry's paramMessages map.
func newPublisherParamResolver(pub Publisher, cache map[uint32]string) paramResolver {
	return func(id uint32) (string, bool) {
		if s, ok := cache[id]; ok {
			return s, s != "" // empty string = negative cache
		}
		s, err := formatMessageID(pub, id)
		if err != nil {
			cache[id] = "" // negative cache
			return "", false
		}
		cache[id] = s
		return s, true
	}
}
```

Where `formatMessageID` wraps `EvtFormatMessage` with `EvtFormatMessageId` flag. Add this API call to `api.go` if not already present. Note: the OTel code already has `evtFormatMessage` — it just needs to be called with the right flag (`EvtFormatMessageId = 8`).

**Step 3: Create msgtemplate_windows.go**

Windows-specific code to populate the template cache from provider metadata:

```go
//go:build windows

package windows

// loadTemplates enumerates a provider's event metadata and caches compiled
// message templates. Called lazily on first template fallback for a provider.
func loadTemplates(pub Publisher, cache *templateCache) error {
	// 1. Call EvtOpenEventMetadataEnum(pub.handle)
	// 2. Loop: EvtNextEventMetadata(enumHandle)
	//    - For each: get event ID, version from metadata properties
	//    - Call EvtFormatMessage with EvtFormatMessageEvent on the metadata handle
	//    - compileTemplate(name, windowsTemplateString)
	//    - cache.put(eventID, version, compiledTemplate)
	// 3. Close enum handle
	return nil
}
```

Add the required API functions to `api.go`:
- `EvtOpenEventMetadataEnum` — wraps `wevtapi.EvtOpenEventMetadataEnum`
- `EvtNextEventMetadata` — wraps `wevtapi.EvtNextEventMetadata`
- `EvtGetEventMetadataProperty` — wraps `wevtapi.EvtGetEventMetadataProperty` (to get event ID, version from metadata)

These are new syscall bindings that need to be added. Reference the Windows SDK documentation for the function signatures:
- `EvtOpenEventMetadataEnum(PublisherMetadata, Flags)` → handle
- `EvtNextEventMetadata(EventMetadataEnum, Flags)` → handle
- `EvtGetEventMetadataProperty(EventMetadata, PropertyId, Flags, BufferSize, Buffer, BufferUsed)` → bool

**Step 4: Modify event.go — processEventWithRenderingInfo**

Add the fallback chain to the deep rendering path:

```go
func (i *Input) processEventWithRenderingInfo(ctx context.Context, event Event) error {
	publisherName := event.GetPublisherName(i.buffer)

	// Get publisher + entry from cache
	entry := i.publisherCache.getEntry(publisherName)

	// Try deep render
	xmlStr, err := event.RenderDeep(i.buffer, entry.publisher)
	if err != nil {
		// FALLBACK: try message template
		xmlStr, _ = event.RenderSimple(i.buffer)
		eventXML, parseErr := unmarshalEventXML([]byte(xmlStr))
		if parseErr == nil {
			msg := i.renderFromTemplate(entry, eventXML)
			if msg != "" {
				eventXML.Message = msg
			}
		}
		// ... continue with eventXML ...
	}

	// Apply %%ID expansion to message
	if entry.publisher.Valid() {
		resolver := newPublisherParamResolver(entry.publisher, entry.paramMessages)
		eventXML.Message = expandParamMessages(eventXML.Message, resolver)
	}

	// Apply SID resolution
	if i.resolveSIDs && eventXML.Security != nil && eventXML.Security.UserID != "" {
		info, err := i.sidCache.resolve(eventXML.Security.UserID)
		if err == nil && info != nil {
			// Add resolved fields to the formatted body
			// (handled in formattedBody or via extra fields)
		}
	}

	return i.sendEvent(ctx, eventXML)
}
```

**Step 5: Modify input.go — add SID cache and config wiring**

Add to `Input` struct:
```go
resolveSIDs bool
sidCache    *sidCache
```

Initialize in constructor / `Start()`:
```go
if cfg.ResolveSIDs {
	i.sidCache = newSIDCache(cfg.SIDCacheSize, 5*time.Minute, defaultSIDLookup)
}
```

**Step 6: Modify formattedBody to include SID resolution results**

Pass resolved SID info into `formattedBody` or add fields after the fact:

```go
if sidInfo != nil {
	sec := body["security"].(map[string]any)
	sec["user_name"] = sidInfo.UserName
	sec["domain"] = sidInfo.Domain
	sec["user_type"] = sidInfo.Type
}
```

**Step 7: Verify compilation**

Run: `cd receiver/windowseventlogreceiver && go vet ./...`
Expected: No errors

**Step 8: Commit**

```
feat: wire SID resolution, %%ID expansion, and template fallback into event pipeline
```

---

### Task 13: Builder Integration

Register the new receiver in the collector build, replacing the contrib receiver.

**Files:**
- Modify: `builder/builder-config.yaml`
- Modify: `builder/go.mod` (after running `go generate`)

**Step 1: Update builder-config.yaml**

Replace the contrib windowseventlogreceiver with our custom one:

```yaml
# Remove this line:
#  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/windowseventlogreceiver v0.146.0

# Add this line:
  - gomod: github.com/Graylog2/collector-sidecar/receiver/windowseventlogreceiver v0.0.0
    path: ../receiver/windowseventlogreceiver
```

The `path:` directive tells OCB to use the local directory (like the existing sidecar extension).

**Step 2: Run code generation**

Run: `cd /home/bernd/graylog/sidecar && go generate .`

This runs OCB which:
1. Regenerates `builder/components.go` with the new receiver import
2. Regenerates `builder/main.go`
3. Runs `builder/mod/main.go` for customization hooks

**Step 3: Update builder go.mod**

Run: `cd builder && go mod tidy`

This adds the local replace directive and resolves dependencies.

**Step 4: Verify build**

Run: `cd /home/bernd/graylog/sidecar && make v2`

Expected: Build succeeds. On Linux the non-Windows stub is used for our receiver, so it compiles but the receiver returns an error if someone tries to use it on Linux.

**Step 5: Verify the receiver type is registered**

Check that `builder/components.go` now imports our receiver module and registers `windowseventlog` instead of the contrib `windowseventlogreceiver`.

**Step 6: Commit**

```
feat: register windowseventlog receiver in collector build
```

---

## Testing Strategy

### Cross-Platform Tests (run on Linux CI)

These test pure Go logic without Windows API calls:
- `xml_test.go` — XML parsing, UserData, severity mapping, timestamp parsing, audit outcome, ProcessingErrorData
- `security_test.go` — Security message parsing
- `buffer_test.go` — Buffer management
- `backoff_test.go` — Backoff timing and error classification
- `sid_test.go` — SID cache (with mock lookup function)
- `paramexpand_test.go` — `%%ID` regex expansion (with mock resolver)
- `msgtemplate_test.go` — Template conversion and execution
- `config_test.go` — XML query validation
- `publishercache_test.go` — Cache behavior (with mock publishers)

### Windows-Only Tests (run on Windows CI or manually)

These test actual Windows API integration:
- Subscription open/close with strict bookmark recovery
- Event reading from live channels
- Deep rendering with publisher metadata
- SID resolution with real `LookupAccountSid`
- `%%ID` expansion with real `EvtFormatMessage`
- Template loading from real provider metadata
- End-to-end: receiver Start → Read → Process → Stop

### How to Run

```bash
# Cross-platform tests (Linux/macOS/Windows)
cd receiver/windowseventlogreceiver && go test ./... -v

# Windows-only integration tests
cd receiver/windowseventlogreceiver && go test ./... -v -tags integration

# Full collector build
make v2

# Full test suite
make v2test
```
