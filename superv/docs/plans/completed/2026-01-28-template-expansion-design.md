# Template Expansion for Agent Args

## Overview

The supervisor needs to expand template placeholders in agent args before passing them to the commander. This allows configuration like:

```yaml
agent:
  args: ["--config", "{{.ConfigPath}}"]
```

## Design

### Template Variables

Minimal set for now (extend when needed):

| Variable | Description |
|----------|-------------|
| `ConfigPath` | Path to the effective collector config written by the supervisor |

### Implementation Location

Template expansion happens in the **supervisor** package, not in keen or config:
- Supervisor knows the config path (it writes the effective config)
- Keeps keen focused purely on process management
- Config package doesn't know runtime values

### Implementation

```go
// In supervisor/supervisor.go

type templateVars struct {
    ConfigPath string
}

func (s *Supervisor) expandArgs(args []string, configPath string) ([]string, error) {
    vars := templateVars{ConfigPath: configPath}
    expanded := make([]string, len(args))

    for i, arg := range args {
        tmpl, err := template.New("arg").Option("missingkey=error").Parse(arg)
        if err != nil {
            return nil, fmt.Errorf("invalid template in arg %d: %w", i, err)
        }
        var buf strings.Builder
        if err := tmpl.Execute(&buf, vars); err != nil {
            return nil, fmt.Errorf("failed to expand arg %d: %w", i, err)
        }
        expanded[i] = buf.String()
    }
    return expanded, nil
}
```

### Integration

In `Supervisor.Start()`:

1. Determine the effective config path: `filepath.Join(s.cfg.Persistence.Dir, "effective.yaml")`
2. Call `expandArgs()` with the config path
3. Pass expanded args to `keen.New()`

### Error Handling

| Error Case | Behavior |
|------------|----------|
| Invalid template syntax | Fail at startup with clear error message |
| Unknown variable | Fail at startup (using `missingkey=error` option) |

### Test Cases

1. Args without templates pass through unchanged
2. `{{.ConfigPath}}` expands correctly
3. Invalid template syntax returns error
4. Unknown variable returns error

## Future Extensions

Additional variables can be added to `templateVars` as needed:
- `InstanceUID`
- `DataDir`
- `OpAMPEndpoint`
