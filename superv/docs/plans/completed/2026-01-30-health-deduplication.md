# Health Monitor Deduplication Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deduplicate health status emissions in the health monitor to avoid unnecessary OpAMP `SetHealth()` calls when status hasn't changed.

**Architecture:** Add change detection to `StartPolling()` by comparing each polled status against the previously emitted status. Only emit to the channel when values differ. Track last-sent status separately from last-checked status.

**Tech Stack:** Go, testify for assertions

---

### Task 1: Add HealthStatus.Equal Method

**Files:**
- Modify: `healthmonitor/monitor.go:68-73` (after HealthStatus struct)
- Test: `healthmonitor/monitor_test.go`

**Step 1: Write failing tests for Equal method**

Add to `healthmonitor/monitor_test.go`:

```go
func TestHealthStatus_Equal(t *testing.T) {
	tests := []struct {
		name     string
		a        *HealthStatus
		b        *HealthStatus
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "first nil",
			a:        nil,
			b:        &HealthStatus{Healthy: true},
			expected: false,
		},
		{
			name:     "second nil",
			a:        &HealthStatus{Healthy: true},
			b:        nil,
			expected: false,
		},
		{
			name:     "equal healthy",
			a:        &HealthStatus{Healthy: true, StatusCode: 200, ErrorMessage: ""},
			b:        &HealthStatus{Healthy: true, StatusCode: 200, ErrorMessage: ""},
			expected: true,
		},
		{
			name:     "equal unhealthy",
			a:        &HealthStatus{Healthy: false, StatusCode: 503, ErrorMessage: "Service Unavailable"},
			b:        &HealthStatus{Healthy: false, StatusCode: 503, ErrorMessage: "Service Unavailable"},
			expected: true,
		},
		{
			name:     "different healthy",
			a:        &HealthStatus{Healthy: true, StatusCode: 200, ErrorMessage: ""},
			b:        &HealthStatus{Healthy: false, StatusCode: 200, ErrorMessage: ""},
			expected: false,
		},
		{
			name:     "different status code",
			a:        &HealthStatus{Healthy: false, StatusCode: 500, ErrorMessage: "Internal Server Error"},
			b:        &HealthStatus{Healthy: false, StatusCode: 503, ErrorMessage: "Service Unavailable"},
			expected: false,
		},
		{
			name:     "different error message",
			a:        &HealthStatus{Healthy: false, StatusCode: 0, ErrorMessage: "connection refused"},
			b:        &HealthStatus{Healthy: false, StatusCode: 0, ErrorMessage: "timeout"},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.a.Equal(tc.b)
			assert.Equal(t, tc.expected, result)
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./healthmonitor/... -run TestHealthStatus_Equal -v`
Expected: FAIL - `s.Equal undefined`

**Step 3: Implement Equal method**

Add to `healthmonitor/monitor.go` after line 72 (after HealthStatus struct):

```go
// Equal returns true if two HealthStatus values are equivalent.
func (s *HealthStatus) Equal(other *HealthStatus) bool {
	if s == nil || other == nil {
		return s == other
	}
	return s.Healthy == other.Healthy &&
		s.StatusCode == other.StatusCode &&
		s.ErrorMessage == other.ErrorMessage
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./healthmonitor/... -run TestHealthStatus_Equal -v`
Expected: PASS

---

### Task 2: Add lastSent Field and Accessor Methods

**Files:**
- Modify: `healthmonitor/monitor.go:93-100` (Monitor struct)
- Test: `healthmonitor/monitor_test.go`

**Step 1: Write failing test for LastSent**

Add to `healthmonitor/monitor_test.go`:

```go
func TestHealthMonitor_LastSent_InitiallyNil(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := Config{
		Endpoint: "http://localhost:8080/health",
		Timeout:  5 * time.Second,
		Interval: 1 * time.Second,
	}

	monitor := New(logger, cfg)

	// Initially, LastSent should be nil
	assert.Nil(t, monitor.LastSent())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./healthmonitor/... -run TestHealthMonitor_LastSent_InitiallyNil -v`
Expected: FAIL - `monitor.LastSent undefined`

**Step 3: Add lastSent field and accessor methods**

Modify `healthmonitor/monitor.go`:

In Monitor struct (around line 99), add field:
```go
type Monitor struct {
	logger *zap.Logger
	cfg    Config
	client *http.Client

	mu       sync.RWMutex
	last     *HealthStatus
	lastSent *HealthStatus // last status emitted to channel
}
```

Add after `setLastStatus` method (around line 158):
```go
// LastSent returns the last status sent to the polling channel.
func (m *Monitor) LastSent() *HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastSent
}

// setLastSent updates the last sent status in a thread-safe manner.
func (m *Monitor) setLastSent(status *HealthStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSent = status
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./healthmonitor/... -run TestHealthMonitor_LastSent_InitiallyNil -v`
Expected: PASS

---

### Task 3: Modify StartPolling for Deduplication

**Files:**
- Modify: `healthmonitor/monitor.go:160-205` (StartPolling method)
- Test: `healthmonitor/monitor_test.go`

**Step 1: Write failing test for deduplication**

Add to `healthmonitor/monitor_test.go`:

```go
func TestHealthMonitor_StartPolling_Deduplication(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Server always returns the same healthy status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		Endpoint: server.URL,
		Timeout:  5 * time.Second,
		Interval: 20 * time.Millisecond, // Fast polling
	}

	monitor := New(logger, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := monitor.StartPolling(ctx)

	// Receive initial status (always emitted)
	status := <-ch
	assert.True(t, status.Healthy)

	// Wait enough time for multiple polls
	time.Sleep(100 * time.Millisecond)

	// Cancel to stop polling
	cancel()

	// Count any additional emissions (should be zero since status unchanged)
	extraEmissions := 0
	for range ch {
		extraEmissions++
	}

	// Should have no extra emissions since status never changed
	assert.Equal(t, 0, extraEmissions, "should not emit unchanged status")

	// Verify LastSent was set
	assert.NotNil(t, monitor.LastSent())
	assert.True(t, monitor.LastSent().Healthy)
}

func TestHealthMonitor_StartPolling_EmitsOnChange(t *testing.T) {
	logger := zaptest.NewLogger(t)

	var requestCount int
	// Server alternates between healthy and unhealthy
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount%2 == 1 {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()

	cfg := Config{
		Endpoint: server.URL,
		Timeout:  5 * time.Second,
		Interval: 20 * time.Millisecond,
	}

	monitor := New(logger, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := monitor.StartPolling(ctx)

	// Collect emissions with timeout
	var emissions []*HealthStatus
	timeout := time.After(150 * time.Millisecond)

collection:
	for {
		select {
		case status, ok := <-ch:
			if !ok {
				break collection
			}
			emissions = append(emissions, status)
			if len(emissions) >= 4 {
				cancel()
			}
		case <-timeout:
			cancel()
			break collection
		}
	}

	// Drain remaining
	for range ch {
	}

	// Should have multiple emissions due to status changes
	assert.GreaterOrEqual(t, len(emissions), 2, "should emit on status changes")

	// First should be healthy, second should be unhealthy (or vice versa depending on timing)
	if len(emissions) >= 2 {
		assert.NotEqual(t, emissions[0].Healthy, emissions[1].Healthy, "consecutive emissions should differ")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./healthmonitor/... -run "TestHealthMonitor_StartPolling_Deduplication|TestHealthMonitor_StartPolling_EmitsOnChange" -v`
Expected: At least `TestHealthMonitor_StartPolling_Deduplication` should FAIL (extra emissions > 0)

**Step 3: Modify StartPolling to deduplicate**

Replace `StartPolling` method in `healthmonitor/monitor.go`:

```go
// StartPolling starts background polling of the health endpoint.
// It performs an initial check immediately, then polls every cfg.Interval.
// Sends health status updates to the returned channel only when status changes.
// Stops when the context is cancelled.
func (m *Monitor) StartPolling(ctx context.Context) <-chan *HealthStatus {
	ch := make(chan *HealthStatus)

	go func() {
		defer close(ch)

		// Perform initial check immediately
		status, err := m.CheckHealth(ctx)
		if err != nil {
			m.logger.Warn("initial health check failed", zap.Error(err))
		}
		// Always send initial status
		select {
		case ch <- status:
			m.setLastSent(status)
		case <-ctx.Done():
			m.logger.Debug("context cancelled")
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
				// Only send if status changed
				if !status.Equal(m.LastSent()) {
					select {
					case ch <- status:
						m.setLastSent(status)
					case <-ctx.Done():
						m.logger.Debug("context cancelled")
						return
					}
				}
			}
		}
	}()

	return ch
}
```

**Step 4: Run all tests to verify they pass**

Run: `go test ./healthmonitor/... -v`
Expected: All PASS

---

### Task 4: Update Existing StartPolling Test

**Files:**
- Modify: `healthmonitor/monitor_test.go:235-278` (TestHealthMonitor_StartPolling)

**Step 1: Review existing test**

The existing `TestHealthMonitor_StartPolling` expects multiple emissions for unchanged status. With deduplication, it will only receive the initial emission.

**Step 2: Update the test**

Replace `TestHealthMonitor_StartPolling` in `healthmonitor/monitor_test.go`:

```go
func TestHealthMonitor_StartPolling(t *testing.T) {
	logger := zaptest.NewLogger(t)

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		Endpoint: server.URL,
		Timeout:  5 * time.Second,
		Interval: 50 * time.Millisecond,
	}

	monitor := New(logger, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := monitor.StartPolling(ctx)

	// Receive initial check
	status := <-ch
	assert.True(t, status.Healthy)
	assert.Equal(t, http.StatusOK, status.StatusCode)

	// Wait for a few poll cycles
	time.Sleep(150 * time.Millisecond)

	// Cancel context to stop polling
	cancel()

	// Channel should be closed after context cancellation
	// With deduplication, no additional status should be emitted since status is unchanged
	for range ch {
		// Drain any remaining (should be none with deduplication)
	}

	// Verify we made multiple requests (polling continued even without emissions)
	assert.GreaterOrEqual(t, requestCount, 2, "should have polled multiple times")

	// Verify LastSent was updated
	assert.NotNil(t, monitor.LastSent())
	assert.True(t, monitor.LastSent().Healthy)
}
```

**Step 3: Run all tests**

Run: `go test ./healthmonitor/... -v`
Expected: All PASS

---

### Task 5: Final Verification

**Step 1: Run full test suite**

Run: `go test ./... -v -count=1 2>&1 | tail -20`
Expected: All tests pass

**Step 2: Run go vet**

Run: `go vet ./healthmonitor/...`
Expected: No issues

**Step 3: Verify go fmt**

Run: `gofmt -l healthmonitor/`
Expected: No output (files are formatted)
