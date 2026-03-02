# Channel List Filtering Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `channel_list` config field that accepts multiple channel names, filters out non-existent channels at startup, and builds a structured XML query from the remaining channels.

**Architecture:** New `channel_list` field on `Config`. At `Input.Start()`, call `ListChannels()` to enumerate available channels, intersect with the configured list, build a `<QueryList>` XML query, and pass it to the existing `Subscription.Open()` query path. The single-subscription model is unchanged.

**Tech Stack:** Go, Windows Event Log API (`EvtOpenChannelEnum`), XML generation via `encoding/xml`

---

### Task 1: Add `buildQueryFromChannels` function + test

**Files:**
- Create: `internal/windows/query.go`
- Create: `internal/windows/query_test.go`

**Step 1: Write the failing test**

```go
// query_test.go
package windows

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildQueryFromChannels(t *testing.T) {
	tests := []struct {
		name     string
		channels []string
		want     string
	}{
		{
			name:     "single channel",
			channels: []string{"Security"},
			want:     `<QueryList><Query Id="0"><Select Path="Security">*</Select></Query></QueryList>`,
		},
		{
			name:     "multiple channels",
			channels: []string{"Security", "Application"},
			want:     `<QueryList><Query Id="0"><Select Path="Security">*</Select><Select Path="Application">*</Select></Query></QueryList>`,
		},
		{
			name:     "xml-special characters are escaped",
			channels: []string{`Foo&Bar`, `A<B`},
			want:     `<QueryList><Query Id="0"><Select Path="Foo&amp;Bar">*</Select><Select Path="A&lt;B">*</Select></Query></QueryList>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildQueryFromChannels(tt.channels)
			assert.Equal(t, tt.want, got)
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/bernd/graylog/sidecar && go test ./receiver/windowseventlogreceiver/internal/windows/ -run TestBuildQueryFromChannels -v`
Expected: FAIL — `buildQueryFromChannels` undefined

**Step 3: Write implementation**

```go
// query.go
package windows

import (
	"encoding/xml"
	"strings"
)

// queryList represents a Windows Event Log structured query.
type queryList struct {
	XMLName xml.Name     `xml:"QueryList"`
	Query   queryElement `xml:"Query"`
}

type queryElement struct {
	ID      string          `xml:"Id,attr"`
	Selects []selectElement `xml:"Select"`
}

type selectElement struct {
	Path  string `xml:"Path,attr"`
	Value string `xml:",chardata"`
}

// buildQueryFromChannels builds a structured XML query that subscribes
// to all the given channels with a wildcard selector.
func buildQueryFromChannels(channels []string) string {
	q := queryList{
		Query: queryElement{
			ID:      "0",
			Selects: make([]selectElement, len(channels)),
		},
	}
	for i, ch := range channels {
		q.Query.Selects[i] = selectElement{Path: ch, Value: "*"}
	}
	var b strings.Builder
	enc := xml.NewEncoder(&b)
	_ = enc.Encode(q)
	return b.String()
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/bernd/graylog/sidecar && go test ./receiver/windowseventlogreceiver/internal/windows/ -run TestBuildQueryFromChannels -v`
Expected: PASS

**Step 5: Run `make fmt`**

Run: `cd /home/bernd/graylog/sidecar && make fmt`

**Step 6: Commit**

```
feat: add buildQueryFromChannels for channel_list support
```

---

### Task 2: Add `canonicalizeChannelList` and `filterChannels` functions + tests

`canonicalizeChannelList` trims whitespace, removes empty entries, and deduplicates (case-insensitive, first occurrence wins). `filterChannels` takes a list of desired channels and a set of available channels, returns the ones that exist. Both are platform-independent (testable on Linux).

**Files:**
- Create: `internal/windows/filter_channels.go`
- Create: `internal/windows/filter_channels_test.go`

**Step 1: Write the failing tests**

```go
// filter_channels_test.go
package windows

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanonicalizeChannelList(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "no changes needed",
			in:   []string{"Security", "Application"},
			want: []string{"Security", "Application"},
		},
		{
			name: "trims whitespace",
			in:   []string{"  Security ", "\tApplication\n"},
			want: []string{"Security", "Application"},
		},
		{
			name: "removes empty entries",
			in:   []string{"Security", "", "  ", "Application"},
			want: []string{"Security", "Application"},
		},
		{
			name: "deduplicates case-insensitive, first wins",
			in:   []string{"Security", "SECURITY", "security"},
			want: []string{"Security"},
		},
		{
			name: "combined",
			in:   []string{"  Security", "Application", "security ", "", "SYSTEM", "system"},
			want: []string{"Security", "Application", "SYSTEM"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalizeChannelList(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFilterChannels(t *testing.T) {
	available := map[string]struct{}{
		"security":    {},
		"application": {},
		"system":      {},
	}
	tests := []struct {
		name     string
		wanted   []string
		want     []string
		skipped  []string
	}{
		{
			name:    "all exist",
			wanted:  []string{"Security", "Application"},
			want:    []string{"Security", "Application"},
			skipped: nil,
		},
		{
			name:    "some missing",
			wanted:  []string{"Security", "Microsoft-Windows-Sysmon/Operational"},
			want:    []string{"Security"},
			skipped: []string{"Microsoft-Windows-Sysmon/Operational"},
		},
		{
			name:    "none exist",
			wanted:  []string{"Nonexistent"},
			want:    nil,
			skipped: []string{"Nonexistent"},
		},
		{
			name:    "case insensitive match",
			wanted:  []string{"SECURITY", "application"},
			want:    []string{"SECURITY", "application"},
			skipped: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, skipped := filterChannels(tt.wanted, available)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.skipped, skipped)
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/bernd/graylog/sidecar && go test ./receiver/windowseventlogreceiver/internal/windows/ -run "TestCanonicalizeChannelList|TestFilterChannels" -v`
Expected: FAIL — undefined functions

**Step 3: Write implementation**

```go
// filter_channels.go
package windows

import "strings"

// canonicalizeChannelList trims whitespace from each entry, removes empty
// entries, and deduplicates case-insensitively (first occurrence wins).
func canonicalizeChannelList(channels []string) []string {
	seen := make(map[string]struct{}, len(channels))
	result := make([]string, 0, len(channels))
	for _, ch := range channels {
		ch = strings.TrimSpace(ch)
		if ch == "" {
			continue
		}
		key := strings.ToLower(ch)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, ch)
	}
	return result
}

// filterChannels returns the subset of wanted channels that exist in the
// available set (case-insensitive lookup). The available map keys must be
// lowercase. Returns the filtered list preserving original casing, and
// the list of channels that were skipped.
func filterChannels(wanted []string, available map[string]struct{}) (matched, skipped []string) {
	for _, ch := range wanted {
		if _, ok := available[strings.ToLower(ch)]; ok {
			matched = append(matched, ch)
		} else {
			skipped = append(skipped, ch)
		}
	}
	return matched, skipped
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/bernd/graylog/sidecar && go test ./receiver/windowseventlogreceiver/internal/windows/ -run "TestCanonicalizeChannelList|TestFilterChannels" -v`
Expected: PASS

**Step 5: Run `make fmt`**

Run: `cd /home/bernd/graylog/sidecar && make fmt`

**Step 6: Commit**

```
feat: add canonicalizeChannelList and filterChannels for channel_list support
```

---

### Task 3: Add `ChannelList` to Config and update validation

**Files:**
- Modify: `internal/windows/config_all.go:33-48` — add `ChannelList` field
- Modify: `internal/windows/config_validate.go:12-37` — update validation for three-way mutual exclusivity
- Modify: `internal/windows/config_test.go:30-89` — add test cases for `channel_list`

**Step 1: Write the failing tests**

Add these cases to the existing `TestValidateConfig` table in `config_test.go`:

```go
{
	name: "valid channel_list",
	cfg:  Config{ChannelList: []string{"Security", "Application"}, MaxReads: 100, StartAt: "end"},
},
{
	name:    "channel and channel_list",
	cfg:     Config{Channel: "Security", ChannelList: []string{"Application"}, MaxReads: 100, StartAt: "end"},
	wantErr: "only one of `channel`, `channel_list`, or `query` may be set",
},
{
	name:    "channel_list and query",
	cfg:     Config{ChannelList: []string{"Security"}, Query: &validQuery, MaxReads: 100, StartAt: "end"},
	wantErr: "only one of `channel`, `channel_list`, or `query` may be set",
},
{
	name:    "empty channel_list",
	cfg:     Config{ChannelList: []string{}, MaxReads: 100, StartAt: "end"},
	wantErr: "either `channel`, `channel_list`, or `query` must be set",
},
{
	name:    "whitespace-only channel_list rejected after canonicalization",
	cfg:     Config{ChannelList: []string{"   ", "", "\t"}, MaxReads: 100, StartAt: "end"},
	wantErr: "either `channel`, `channel_list`, or `query` must be set",
},
{
	name:    "channel_list with duplicates canonicalized",
	cfg:     Config{ChannelList: []string{"Security", "security", "SECURITY"}, MaxReads: 100, StartAt: "end"},
},
```

**Step 2: Run tests to verify failures**

Run: `cd /home/bernd/graylog/sidecar && go test ./receiver/windowseventlogreceiver/internal/windows/ -run TestValidateConfig -v`
Expected: compilation errors (ChannelList field not defined) or test failures

**Step 3: Add `ChannelList` field to Config**

In `config_all.go`, add after the `Channel` field:

```go
ChannelList             []string      `mapstructure:"channel_list,omitempty"`
```

**Step 4: Update `validateConfig`**

Replace the validation function in `config_validate.go`. Note: canonicalization runs in-place *before* counting sources, so whitespace-only entries are rejected at config time:

```go
func validateConfig(c *Config) error {
	// Canonicalize channel_list in-place before validation so that
	// whitespace-only or duplicate entries don't pass validation
	// only to produce an empty list at runtime.
	c.ChannelList = canonicalizeChannelList(c.ChannelList)

	sources := 0
	if c.Channel != "" {
		sources++
	}
	if len(c.ChannelList) > 0 {
		sources++
	}
	if c.Query != nil {
		sources++
	}

	if sources == 0 {
		return errors.New("either `channel`, `channel_list`, or `query` must be set")
	}
	if sources > 1 {
		return errors.New("only one of `channel`, `channel_list`, or `query` may be set")
	}
	if c.Query != nil && *c.Query == "" {
		return errors.New("the `query` field must not be empty when set")
	}
	if c.MaxReads < 1 {
		return errors.New("the `max_reads` field must be greater than zero")
	}
	if c.StartAt != "end" && c.StartAt != "beginning" {
		return errors.New("the `start_at` field must be set to `beginning` or `end`")
	}
	if c.SIDCacheSize < 0 {
		return errors.New("the `sid_cache_size` field must not be negative")
	}
	if c.Query != nil {
		if err := validateQueryXML(*c.Query); err != nil {
			return err
		}
	}
	return nil
}
```

**Step 5: Update existing test cases**

The "neither channel nor query" test error message changes to `"either \`channel\`, \`channel_list\`, or \`query\` must be set"`. The "both channel and query" test error changes to `"only one of \`channel\`, \`channel_list\`, or \`query\` may be set"`. Update these in the test table.

**Step 6: Run tests**

Run: `cd /home/bernd/graylog/sidecar && go test ./receiver/windowseventlogreceiver/internal/windows/ -run TestValidateConfig -v`
Expected: PASS

Also update `TestNewConfig_Defaults` to assert `ChannelList` is nil:

```go
require.Nil(t, cfg.ChannelList)
```

**Step 7: Run `make fmt`**

Run: `cd /home/bernd/graylog/sidecar && make fmt`

**Step 8: Commit**

```
feat: add channel_list config field with mutual exclusivity validation
```

---

### Task 4: Update `Input` struct and `Config.Build` to use `channelList`

**Files:**
- Modify: `internal/windows/input.go:25-50` — replace `channel string` with `channelList []string`
- Modify: `internal/windows/config_windows.go:18-54` — normalize `channel` → `channelList`, set on Input

**Step 1: Update `Input` struct**

In `input.go`, replace:
```go
channel                  string
```
with:
```go
channelList              []string
```

**Step 2: Update `Config.Build`**

In `config_windows.go`, normalize `channel` to `channelList` and pass it:

```go
func (c *Config) Build(set component.TelemetrySettings) (operator.Operator, error) {
	inputOperator, err := c.InputConfig.Build(set)
	if err != nil {
		return nil, err
	}

	if err := validateConfig(c); err != nil {
		return nil, err
	}

	// Normalize single channel to channel_list.
	// Note: channel_list is already canonicalized by validateConfig().
	channelList := c.ChannelList
	if c.Channel != "" {
		channelList = []string{c.Channel}
	}

	input := &Input{
		InputOperator:            inputOperator,
		buffer:                   NewBuffer(),
		channelList:              channelList,
		ignoreChannelErrors:      c.IgnoreChannelErrors,
		maxReads:                 c.MaxReads,
		currentMaxReads:          c.MaxReads,
		startAt:                  c.StartAt,
		pollInterval:             c.PollInterval,
		raw:                      c.Raw,
		includeLogRecordOriginal: c.IncludeLogRecordOriginal,
		excludeProviders:         excludeProvidersSet(c.ExcludeProviders),
		language:                 c.Language,
		resolveSIDs:              c.ResolveSIDs,
		sidCacheSize:             c.SIDCacheSize,
		query:                    c.Query,
	}

	if c.SuppressRenderingInfo {
		input.processEvent = input.processEventWithoutRenderingInfo
	} else {
		input.processEvent = input.processEventWithRenderingInfo
	}

	return input, nil
}
```

**Step 3: Update `getPersistKey`**

In `input.go`, update `getPersistKey` to use `channelList` with deterministic normalization (lowercase, sort, join):

```go
func (i *Input) getPersistKey() string {
	if i.query != nil {
		return *i.query
	}

	normalized := make([]string, len(i.channelList))
	for idx, ch := range i.channelList {
		normalized[idx] = strings.ToLower(ch)
	}
	sort.Strings(normalized)
	return strings.Join(normalized, "\n")
}
```

Add `"sort"` and `"strings"` to the imports. Note: `channelList` is already canonicalized (deduped) by `Config.Build`, so lowercasing + sorting here is sufficient for a stable key.

**Step 3b: Add persist key stability tests**

Add to `internal/windows/config_test.go` (or a new `persist_key_test.go`):

```go
func TestGetPersistKey_ChannelList(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
	}{
		{
			name: "different order, same key",
			a:    []string{"Application", "Security"},
			b:    []string{"Security", "Application"},
		},
		{
			name: "different casing, same key",
			a:    []string{"Security"},
			b:    []string{"SECURITY"},
		},
		{
			name: "order and casing combined",
			a:    []string{"SYSTEM", "application", "Security"},
			b:    []string{"security", "Application", "system"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ia := &Input{channelList: tt.a}
			ib := &Input{channelList: tt.b}
			assert.Equal(t, ia.getPersistKey(), ib.getPersistKey())
		})
	}
}
```

Note: this test references `Input` directly, so it must be in a file with `//go:build windows` or use a build-tag-free subset. If `Input` is only defined in `input.go` (Windows-only), move this test to a `persist_key_test.go` with `//go:build windows` tag, or extract `getPersistKey` logic into a standalone function testable on all platforms. Prefer the latter:

```go
// In query.go (platform-independent):
func channelListPersistKey(channels []string) string {
	normalized := make([]string, len(channels))
	for idx, ch := range channels {
		normalized[idx] = strings.ToLower(ch)
	}
	sort.Strings(normalized)
	return strings.Join(normalized, "\n")
}
```

Then `Input.getPersistKey()` calls `channelListPersistKey(i.channelList)`, and the test calls `channelListPersistKey` directly.

**Step 4: Update `Start` and `pollAndRead` — pass empty channel + query**

Now that `channelList` is the source of truth, `Start` and `pollAndRead` must pass an empty `channel` and the generated query to `Subscription.Open`. For now (before the filtering logic in Task 5), just build the query directly from `channelList`:

In `Start()`, replace:
```go
if err := subscription.Open(i.startAt, i.channel, i.query, i.bookmark); err != nil {
```
with:
```go
query := i.effectiveQuery()
if err := subscription.Open(i.startAt, "", query, i.bookmark); err != nil {
```

Add a helper:
```go
func (i *Input) effectiveQuery() *string {
	if i.query != nil {
		return i.query
	}
	q := buildQueryFromChannels(i.channelList)
	return &q
}
```

Apply the same change to the `pollAndRead` retry call (line 156).

**Step 5: Verify compilation**

Run: `cd /home/bernd/graylog/sidecar && go build ./receiver/windowseventlogreceiver/...`
Note: This will only compile on Windows. On Linux, verify the non-Windows files compile:
Run: `cd /home/bernd/graylog/sidecar && go vet ./receiver/windowseventlogreceiver/internal/windows/`

**Step 6: Run `make fmt`**

Run: `cd /home/bernd/graylog/sidecar && make fmt`

**Step 7: Commit**

```
refactor: replace Input.channel with channelList, build query from list
```

---

### Task 5: Add channel filtering to `Input.Start`

**Files:**
- Modify: `internal/windows/input.go:71-117` — add filtering logic before subscription open

**Step 1: Add `listChannels` function field to Input for testability**

In the `Input` struct, add:
```go
listChannels             func() ([]string, error)
```

In `config_windows.go` `Build`, set the default:
```go
listChannels:             ListChannels,
```

**Step 2: Add filtering in `Start`**

In `Input.Start()`, after the bookmark/publisher setup and before `subscription.Open`, add the channel filtering logic. If `ListChannels()` fails, startup fails immediately — falling back to an unfiltered list would silently subscribe to non-existent channels, defeating the purpose.

```go
// Filter channel_list to only channels that exist on this machine.
if i.query == nil {
	available, err := i.listChannels()
	if err != nil {
		return fmt.Errorf("failed to enumerate available channels: %w", err)
	}

	availableSet := make(map[string]struct{}, len(available))
	for _, ch := range available {
		availableSet[strings.ToLower(ch)] = struct{}{}
	}

	filtered, skipped := filterChannels(i.channelList, availableSet)
	for _, ch := range skipped {
		i.Logger().Warn("Configured channel not found on this machine, skipping", zap.String("channel", ch))
	}

	if len(filtered) == 0 {
		if i.ignoreChannelErrors {
			i.Logger().Warn("No configured channels found on this machine, not starting")
			return nil
		}
		return fmt.Errorf("none of the configured channels exist on this machine: %v", i.channelList)
	}

	i.channelList = filtered
}
```

**Step 3: Add acceptance tests for startup filtering**

Since `Input.Start` depends on Windows APIs (persister, subscription, etc.), test the filtering logic via a dedicated unit test that exercises the filtering path in isolation. Create `internal/windows/filter_channels_test.go` (append to existing file):

```go
func TestStartupFiltering_ListChannelsFails(t *testing.T) {
	// Simulate ListChannels() failure — should propagate as startup error
	wanted := []string{"Security"}
	listErr := errors.New("RPC unavailable")

	_, err := applyChannelFilter(wanted, func() ([]string, error) {
		return nil, listErr
	})
	assert.ErrorIs(t, err, listErr)
}

func TestStartupFiltering_EmptyResult_IgnoreTrue(t *testing.T) {
	wanted := []string{"Nonexistent"}
	available := func() ([]string, error) {
		return []string{"Security"}, nil
	}

	filtered, err := applyChannelFilter(wanted, available)
	assert.NoError(t, err)
	assert.Nil(t, filtered)
}

func TestStartupFiltering_EmptyResult_IgnoreFalse(t *testing.T) {
	wanted := []string{"Nonexistent"}
	available := func() ([]string, error) {
		return []string{"Security"}, nil
	}

	_, err := applyChannelFilterStrict(wanted, available)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "none of the configured channels exist")
}
```

To make this testable without full `Input.Start`, extract the filtering logic into a standalone function:

```go
// In filter_channels.go:

// applyChannelFilter enumerates available channels using listFn, filters
// wanted against them, and returns the filtered list. Returns (nil, nil)
// if no channels match (caller decides whether to fail or idle).
// Returns (nil, err) if listFn fails.
func applyChannelFilter(wanted []string, listFn func() ([]string, error)) ([]string, []string, error) {
	available, err := listFn()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enumerate available channels: %w", err)
	}

	availableSet := make(map[string]struct{}, len(available))
	for _, ch := range available {
		availableSet[strings.ToLower(ch)] = struct{}{}
	}

	filtered, skipped := filterChannels(wanted, availableSet)
	return filtered, skipped, nil
}
```

Then `Input.Start` calls `applyChannelFilter(i.channelList, i.listChannels)` and handles the empty-result policy.

**Step 4: Verify compilation**

Run: `cd /home/bernd/graylog/sidecar && go vet ./receiver/windowseventlogreceiver/internal/windows/`

**Step 5: Run tests**

Run: `cd /home/bernd/graylog/sidecar && go test ./receiver/windowseventlogreceiver/internal/windows/ -run "TestStartupFiltering|TestGetPersistKey" -v`
Expected: PASS

**Step 6: Run `make fmt`**

Run: `cd /home/bernd/graylog/sidecar && make fmt`

**Step 7: Commit**

```
feat: filter channel_list against available channels at startup
```

---

### Task 6: Full test pass and cleanup

**Files:**
- All test files in `internal/windows/`

**Step 1: Run all tests**

Run: `cd /home/bernd/graylog/sidecar && go test ./receiver/windowseventlogreceiver/... -v`

**Step 2: Run go vet**

Run: `cd /home/bernd/graylog/sidecar && go vet ./receiver/windowseventlogreceiver/...`

**Step 3: Run `make fmt`**

Run: `cd /home/bernd/graylog/sidecar && make fmt`

**Step 4: Fix any issues found**

**Step 5: Commit any fixes**

```
chore: fix lint and test issues from channel_list feature
```
