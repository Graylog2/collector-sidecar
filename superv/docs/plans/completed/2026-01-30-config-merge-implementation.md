# Config Merge Hybrid Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add OTel-compatible custom merge function for correct `service.extensions` concatenation while keeping modular API.

**Architecture:** Add `collectorConfigMerge` function that intercepts koanf merge to concatenate and deduplicate `service.extensions` arrays. Integrate into existing `MergeConfigs` and `MergeMultiple` functions via `koanf.WithMergeFunc()`.

**Tech Stack:** Go, koanf (maps, parsers/yaml, providers/rawbytes, v2), testify

**Reference:** Design doc `docs/plans/2026-01-30-config-merge-hybrid-design.md`

---

## Task 1: Add Extension Concatenation Tests

**Files:**
- Modify: `configmerge/merge_test.go`

**Step 1: Write failing test for extension concatenation**

Add to `configmerge/merge_test.go`:

```go
func TestMergeConfigs_ExtensionsConcatenated(t *testing.T) {
	base := []byte(`
service:
  extensions: [health_check]
`)
	override := []byte(`
service:
  extensions: [opamp]
`)

	result, err := MergeConfigs(base, override)
	require.NoError(t, err)

	// Both extensions should be present (concatenated, not overwritten)
	require.Contains(t, string(result), "health_check")
	require.Contains(t, string(result), "opamp")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./configmerge/... -v -run TestMergeConfigs_ExtensionsConcatenated`
Expected: FAIL - `health_check` not found (overwritten by `opamp`)

---

## Task 2: Add Extension Deduplication Test

**Files:**
- Modify: `configmerge/merge_test.go`

**Step 1: Write failing test for extension deduplication**

Add to `configmerge/merge_test.go`:

```go
func TestMergeConfigs_ExtensionsDeduplicated(t *testing.T) {
	base := []byte(`
service:
  extensions: [health_check, opamp]
`)
	override := []byte(`
service:
  extensions: [opamp, zpages]
`)

	result, err := MergeConfigs(base, override)
	require.NoError(t, err)

	// Should have all three, with opamp appearing only once
	resultStr := string(result)
	require.Contains(t, resultStr, "health_check")
	require.Contains(t, resultStr, "opamp")
	require.Contains(t, resultStr, "zpages")

	// Count occurrences of "opamp" - should be exactly 1
	count := 0
	for i := 0; i < len(resultStr)-4; i++ {
		if resultStr[i:i+5] == "opamp" {
			count++
		}
	}
	require.Equal(t, 1, count, "opamp should appear exactly once")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./configmerge/... -v -run TestMergeConfigs_ExtensionsDeduplicated`
Expected: FAIL

---

## Task 3: Add MergeMultiple Extensions Test

**Files:**
- Modify: `configmerge/merge_test.go`

**Step 1: Write failing test for MergeMultiple with extensions**

Add to `configmerge/merge_test.go`:

```go
func TestMergeMultiple_ExtensionsConcatenated(t *testing.T) {
	configs := [][]byte{
		[]byte(`
service:
  extensions: [health_check]
`),
		[]byte(`
service:
  extensions: [opamp]
`),
		[]byte(`
service:
  extensions: [zpages]
`),
	}

	result, err := MergeMultiple(configs...)
	require.NoError(t, err)

	// All three extensions should be present
	require.Contains(t, string(result), "health_check")
	require.Contains(t, string(result), "opamp")
	require.Contains(t, string(result), "zpages")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./configmerge/... -v -run TestMergeMultiple_ExtensionsConcatenated`
Expected: FAIL

---

## Task 4: Add Edge Case Tests

**Files:**
- Modify: `configmerge/merge_test.go`

**Step 1: Write tests for edge cases**

Add to `configmerge/merge_test.go`:

```go
func TestMergeConfigs_NoServiceSection(t *testing.T) {
	base := []byte(`
receivers:
  otlp: {}
`)
	override := []byte(`
exporters:
  debug: {}
`)

	result, err := MergeConfigs(base, override)
	require.NoError(t, err)

	// Should merge normally without service section
	require.Contains(t, string(result), "receivers")
	require.Contains(t, string(result), "exporters")
}

func TestMergeConfigs_OneConfigHasExtensions(t *testing.T) {
	base := []byte(`
receivers:
  otlp: {}
`)
	override := []byte(`
service:
  extensions: [opamp]
`)

	result, err := MergeConfigs(base, override)
	require.NoError(t, err)

	require.Contains(t, string(result), "receivers")
	require.Contains(t, string(result), "opamp")
}

func TestMergeConfigs_EmptyExtensionsList(t *testing.T) {
	base := []byte(`
service:
  extensions: []
`)
	override := []byte(`
service:
  extensions: [opamp]
`)

	result, err := MergeConfigs(base, override)
	require.NoError(t, err)

	require.Contains(t, string(result), "opamp")
}
```

**Step 2: Run tests to verify behavior**

Run: `go test ./configmerge/... -v -run "TestMergeConfigs_NoServiceSection|TestMergeConfigs_OneConfigHasExtensions|TestMergeConfigs_EmptyExtensionsList"`
Expected: Some may pass (no extensions to merge), some may fail

---

## Task 5: Implement deduplicateSlice Helper

**Files:**
- Modify: `configmerge/merge.go`

**Step 1: Add deduplicateSlice function**

Add to `configmerge/merge.go` (before existing functions):

```go
// deduplicateSlice removes duplicates from a slice while preserving order.
func deduplicateSlice(slice []any) []any {
	seen := make(map[any]struct{}, len(slice))
	result := make([]any, 0, len(slice))
	for _, v := range slice {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}
```

**Step 2: Verify it compiles**

Run: `go build ./configmerge/...`
Expected: Success

---

## Task 6: Implement collectorConfigMerge Function

**Files:**
- Modify: `configmerge/merge.go`

**Step 1: Add import for koanf/maps**

Update imports in `configmerge/merge.go`:

```go
import (
	"github.com/knadh/koanf/maps"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
)
```

**Step 2: Add collectorConfigMerge function**

Add to `configmerge/merge.go` (after deduplicateSlice):

```go
// collectorConfigMerge is a custom merge function for koanf that handles
// OTel Collector config semantics. Extension lists in service.extensions
// are concatenated and deduplicated rather than overwritten.
func collectorConfigMerge(src, dest map[string]any) error {
	// Capture extension lists before standard merge overwrites them
	srcExt := maps.Search(src, []string{"service", "extensions"})
	destExt := maps.Search(dest, []string{"service", "extensions"})

	// Standard map merge (this overwrites arrays)
	maps.Merge(src, dest)

	// Restore concatenated, deduplicated extensions
	destSlice, _ := destExt.([]any)
	srcSlice, _ := srcExt.([]any)

	if len(destSlice) > 0 || len(srcSlice) > 0 {
		merged := deduplicateSlice(append(destSlice, srcSlice...))
		if service, ok := dest["service"].(map[string]any); ok {
			service["extensions"] = merged
		}
	}

	return nil
}
```

**Step 3: Verify it compiles**

Run: `go build ./configmerge/...`
Expected: Success

---

## Task 7: Update MergeConfigs to Use Custom Merge

**Files:**
- Modify: `configmerge/merge.go`

**Step 1: Update MergeConfigs function**

Replace the existing `MergeConfigs` function:

```go
// MergeConfigs merges two YAML configurations with collector-aware semantics.
// Extension lists in service.extensions are concatenated and deduplicated.
func MergeConfigs(base, override []byte) ([]byte, error) {
	k := koanf.New("::")

	// Load base config
	if len(base) > 0 {
		if err := k.Load(rawbytes.Provider(base), yaml.Parser()); err != nil {
			return nil, err
		}
	}

	// Merge override config with custom merge function
	if len(override) > 0 {
		if err := k.Load(rawbytes.Provider(override), yaml.Parser(),
			koanf.WithMergeFunc(collectorConfigMerge)); err != nil {
			return nil, err
		}
	}

	// Marshal back to YAML
	return k.Marshal(yaml.Parser())
}
```

**Step 2: Run extension concatenation tests**

Run: `go test ./configmerge/... -v -run "TestMergeConfigs_Extensions"`
Expected: PASS

---

## Task 8: Update MergeMultiple to Use Custom Merge

**Files:**
- Modify: `configmerge/merge.go`

**Step 1: Update MergeMultiple function**

Replace the existing `MergeMultiple` function:

```go
// MergeMultiple merges multiple YAML configurations in order with collector-aware semantics.
// Later configs take precedence over earlier ones.
// Extension lists in service.extensions are concatenated and deduplicated.
func MergeMultiple(configs ...[]byte) ([]byte, error) {
	k := koanf.New("::")

	for i, cfg := range configs {
		if len(cfg) == 0 {
			continue
		}

		var opts []koanf.Option
		if i > 0 {
			// Use custom merge for all configs after the first
			opts = append(opts, koanf.WithMergeFunc(collectorConfigMerge))
		}

		if err := k.Load(rawbytes.Provider(cfg), yaml.Parser(), opts...); err != nil {
			return nil, err
		}
	}

	return k.Marshal(yaml.Parser())
}
```

**Step 2: Run MergeMultiple extension tests**

Run: `go test ./configmerge/... -v -run TestMergeMultiple_ExtensionsConcatenated`
Expected: PASS

---

## Task 9: Run Full Test Suite

**Files:**
- None (verification only)

**Step 1: Run all configmerge tests**

Run: `go test ./configmerge/... -v`
Expected: All tests PASS

**Step 2: Run all project tests**

Run: `go test ./... -v`
Expected: All tests PASS

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Extension concatenation test | merge_test.go |
| 2 | Extension deduplication test | merge_test.go |
| 3 | MergeMultiple extensions test | merge_test.go |
| 4 | Edge case tests | merge_test.go |
| 5 | Implement deduplicateSlice | merge.go |
| 6 | Implement collectorConfigMerge | merge.go |
| 7 | Update MergeConfigs | merge.go |
| 8 | Update MergeMultiple | merge.go |
| 9 | Full test verification | - |
