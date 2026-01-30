# Config Merge Hybrid Design

**Date:** 2026-01-30
**Status:** Draft
**Goal:** Adopt OTel's custom merge function for correct `service.extensions` handling while keeping modular API.

## Problem

The current config merge implementation uses koanf's default merge behavior, which **overwrites arrays**. When merging two configs that both define `service.extensions`, the second config's list replaces the first instead of concatenating them.

```yaml
# Config A                    # Config B                    # Result (wrong)
service:                      service:                      service:
  extensions: [health_check]    extensions: [opamp]           extensions: [opamp]  # health_check lost!
```

The OTel opampsupervisor solves this with a custom merge function.

## Solution

Add a custom merge function that:
1. Captures `service.extensions` from both configs before merge
2. Performs standard `maps.Merge`
3. Concatenates and deduplicates the extension lists
4. Restores the merged list

Keep the existing modular API (`MergeConfigs`, `MergeMultiple`, `InjectSettings`, etc.).

## Implementation

### New: `collectorConfigMerge` function

```go
func collectorConfigMerge(src, dest map[string]any) error {
    srcExt := maps.Search(src, []string{"service", "extensions"})
    destExt := maps.Search(dest, []string{"service", "extensions"})

    maps.Merge(src, dest)

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

### New: `deduplicateSlice` helper

```go
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

### Modified: `MergeConfigs`

Use custom merge function for override config:

```go
func MergeConfigs(base, override []byte) ([]byte, error) {
    k := koanf.New("::")

    if len(base) > 0 {
        if err := k.Load(rawbytes.Provider(base), yaml.Parser()); err != nil {
            return nil, err
        }
    }

    if len(override) > 0 {
        if err := k.Load(rawbytes.Provider(override), yaml.Parser(),
            koanf.WithMergeFunc(collectorConfigMerge)); err != nil {
            return nil, err
        }
    }

    return k.Marshal(yaml.Parser())
}
```

### Modified: `MergeMultiple`

Use custom merge function for all configs after the first:

```go
func MergeMultiple(configs ...[]byte) ([]byte, error) {
    k := koanf.New("::")

    for i, cfg := range configs {
        if len(cfg) == 0 {
            continue
        }
        var opts []koanf.Option
        if i > 0 {
            opts = append(opts, koanf.WithMergeFunc(collectorConfigMerge))
        }
        if err := k.Load(rawbytes.Provider(cfg), yaml.Parser(), opts...); err != nil {
            return nil, err
        }
    }

    return k.Marshal(yaml.Parser())
}
```

### Unchanged

- `InjectSettings` - uses `k.Set()`, not merge
- `inject.go` functions - already handle single extension injection correctly

## Files Changed

| File | Change |
|------|--------|
| `configmerge/merge.go` | Add `collectorConfigMerge`, `deduplicateSlice`; update `MergeConfigs`, `MergeMultiple` |
| `configmerge/merge_test.go` | Add tests for extension list merging |

## Test Cases

1. **Basic merge** - two configs, no extensions (unchanged behavior)
2. **Extension concatenation** - Config A has `[health_check]`, Config B has `[opamp]` → result has `[health_check, opamp]`
3. **Extension deduplication** - both configs have `[opamp]` → result has `[opamp]` (not duplicated)
4. **Multiple configs** - three configs with overlapping extensions
5. **Empty extension list** - one config has extensions, other doesn't
6. **No service section** - configs without `service.extensions` at all

## References

- OTel opampsupervisor: `supervisor/supervisor.go:2119-2148`
- koanf maps package: `github.com/knadh/koanf/maps`
