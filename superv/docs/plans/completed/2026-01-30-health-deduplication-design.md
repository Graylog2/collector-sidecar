# Health Monitor Deduplication Design

## Context

The OpAMP spec states that apart from `instance_uid`, `sequence_num`, and `capabilities`, only changed values should be sent in `AgentToServer` messages. While the opamp-go library handles message-level deduplication for some fields, it does not deduplicate `Health` updates - every `SetHealth()` call results in the value being included in the next message.

Our health monitor polls the collector's health endpoint at a configured interval and sends every result to the supervisor, which then calls `SetHealth()`. This causes unnecessary network traffic when health status hasn't changed.

## Decision

Implement deduplication in the health monitor's `StartPolling` method. Compare each polled status against the previously emitted status and only send to the channel when values change.

## Design

### Change Detection Logic

A `HealthStatus` is considered changed when any of these fields differ:
- `Healthy` (bool)
- `StatusCode` (int)
- `ErrorMessage` (string)

### New Types and Methods

```go
// Equal method on HealthStatus
func (s *HealthStatus) Equal(other *HealthStatus) bool {
    if s == nil || other == nil {
        return s == other
    }
    return s.Healthy == other.Healthy &&
           s.StatusCode == other.StatusCode &&
           s.ErrorMessage == other.ErrorMessage
}
```

### Monitor Struct Changes

Add a new field to track the last emitted status:

```go
type Monitor struct {
    // ... existing fields ...

    mu       sync.RWMutex
    last     *HealthStatus   // most recent check result
    lastSent *HealthStatus   // last status emitted to channel (NEW)
}
```

Add accessor methods:

```go
func (m *Monitor) LastSent() *HealthStatus {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.lastSent
}

func (m *Monitor) setLastSent(status *HealthStatus) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.lastSent = status
}
```

### Modified StartPolling

```go
func (m *Monitor) StartPolling(ctx context.Context) <-chan *HealthStatus {
    ch := make(chan *HealthStatus)

    go func() {
        defer close(ch)

        // Initial check - always emit
        status, err := m.CheckHealth(ctx)
        if err != nil {
            m.logger.Warn("initial health check failed", zap.Error(err))
        }
        select {
        case ch <- status:
            m.setLastSent(status)
        case <-ctx.Done():
            return
        }

        ticker := time.NewTicker(m.cfg.Interval)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                status, err := m.CheckHealth(ctx)
                if err != nil {
                    m.logger.Debug("health check failed", zap.Error(err))
                }
                // Only emit if changed
                if !status.Equal(m.LastSent()) {
                    select {
                    case ch <- status:
                        m.setLastSent(status)
                    case <-ctx.Done():
                        return
                    }
                }
            }
        }
    }()

    return ch
}
```

## Edge Cases

1. **First check always emits**: `lastSent` is nil initially, `Equal(nil)` returns false
2. **Error message changes**: Different error messages trigger emission
3. **Recovery detection**: Healthy/unhealthy transitions always emit
4. **Nil status handling**: `Equal` method handles nil safely

## Test Cases

- Unchanged healthy status: no emission after initial
- Unchanged unhealthy status: no emission after initial
- Healthy to unhealthy: emits
- Unhealthy to healthy: emits
- Different error messages: emits
- Same error, different status code: emits

## Files Changed

- `healthmonitor/monitor.go`: Add `Equal` method, `lastSent` field, accessor methods, modify `StartPolling`
- `healthmonitor/monitor_test.go`: Add deduplication test cases

## No Changes Required

- `CheckHealth()`: Continues updating `last` on every call
- `LastStatus()`: Returns most recent check result
- `ToComponentHealth()`: Unchanged
- Supervisor: Receives fewer updates transparently
