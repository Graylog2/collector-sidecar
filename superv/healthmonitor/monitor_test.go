// Copyright (C)  2026 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.
//
// SPDX-License-Identifier: SSPL-1.0

package healthmonitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestHealthMonitor_CheckHealth_Healthy(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a mock server that returns 200 OK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer server.Close()

	cfg := Config{
		Endpoint: server.URL,
		Timeout:  5 * time.Second,
		Interval: 1 * time.Second,
	}

	monitor := New(logger, cfg)

	status, err := monitor.CheckHealth(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
	assert.Equal(t, http.StatusOK, status.StatusCode)
	assert.Empty(t, status.ErrorMessage)

	// Verify LastStatus was updated
	last := monitor.LastStatus()
	assert.Equal(t, status, last)
}

func TestHealthMonitor_CheckHealth_Unhealthy(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a mock server that returns 503 Service Unavailable
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"unhealthy"}`))
	}))
	defer server.Close()

	cfg := Config{
		Endpoint: server.URL,
		Timeout:  5 * time.Second,
		Interval: 1 * time.Second,
	}

	monitor := New(logger, cfg)

	status, err := monitor.CheckHealth(context.Background())
	require.NoError(t, err) // No error returned, but status is unhealthy
	assert.False(t, status.Healthy)
	assert.Equal(t, http.StatusServiceUnavailable, status.StatusCode)
	assert.Equal(t, "Service Unavailable", status.ErrorMessage)

	// Verify LastStatus was updated
	last := monitor.LastStatus()
	assert.Equal(t, status, last)
}

func TestHealthMonitor_CheckHealth_Unreachable(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Use an endpoint that doesn't exist
	cfg := Config{
		Endpoint: "http://127.0.0.1:59999/health", // Non-existent port
		Timeout:  100 * time.Millisecond,
		Interval: 1 * time.Second,
	}

	monitor := New(logger, cfg)

	status, err := monitor.CheckHealth(context.Background())
	require.Error(t, err)
	assert.False(t, status.Healthy)
	assert.Equal(t, 0, status.StatusCode)
	assert.NotEmpty(t, status.ErrorMessage)

	// Verify LastStatus was updated even on error
	last := monitor.LastStatus()
	assert.Equal(t, status, last)
}

// mockAgentStateProvider implements AgentStateProvider for tests.
type mockAgentStateProvider struct {
	startTime time.Time
	state     AgentState
}

func (m *mockAgentStateProvider) StartTime() time.Time {
	return m.startTime
}

func (m *mockAgentStateProvider) State() AgentState {
	return m.state
}

func TestHealthMonitor_ToComponentHealth(t *testing.T) {
	now := time.Now()
	agentStartTime := now.Add(-time.Hour)

	tests := []struct {
		name          string
		status        *HealthStatus
		agentProvider AgentStateProvider
		expected      struct {
			healthy       bool
			lastError     string
			startTimeNano uint64
		}
	}{
		{
			name: "healthy status with agent provider",
			status: &HealthStatus{
				Healthy:      true,
				StatusCode:   200,
				ErrorMessage: "",
			},
			agentProvider: &mockAgentStateProvider{
				startTime: agentStartTime,
				state:     AgentStateRunning,
			},
			expected: struct {
				healthy       bool
				lastError     string
				startTimeNano uint64
			}{
				healthy:       true,
				lastError:     "",
				startTimeNano: uint64(agentStartTime.UnixNano()),
			},
		},
		{
			name: "unhealthy status with agent provider",
			status: &HealthStatus{
				Healthy:      false,
				StatusCode:   503,
				ErrorMessage: "Service Unavailable",
			},
			agentProvider: &mockAgentStateProvider{
				startTime: agentStartTime,
				state:     AgentStateFailed,
			},
			expected: struct {
				healthy       bool
				lastError     string
				startTimeNano uint64
			}{
				healthy:       false,
				lastError:     "Service Unavailable",
				startTimeNano: uint64(agentStartTime.UnixNano()),
			},
		},
		{
			name: "healthy status with nil agent provider",
			status: &HealthStatus{
				Healthy:      true,
				StatusCode:   200,
				ErrorMessage: "",
			},
			agentProvider: nil,
			expected: struct {
				healthy       bool
				lastError     string
				startTimeNano uint64
			}{
				healthy:       true,
				lastError:     "",
				startTimeNano: 0,
			},
		},
		{
			name: "agent provider with zero start time",
			status: &HealthStatus{
				Healthy:      true,
				StatusCode:   200,
				ErrorMessage: "",
			},
			agentProvider: &mockAgentStateProvider{
				startTime: time.Time{}, // zero time
				state:     AgentStateStarting,
			},
			expected: struct {
				healthy       bool
				lastError     string
				startTimeNano uint64
			}{
				healthy:       true,
				lastError:     "",
				startTimeNano: 0,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			health := tc.status.ToComponentHealth(tc.agentProvider)

			assert.Equal(t, tc.expected.healthy, health.Healthy)
			assert.Equal(t, tc.expected.lastError, health.LastError)
			assert.Equal(t, tc.expected.startTimeNano, health.StartTimeUnixNano)
		})
	}
}

func TestHealthMonitor_StartPolling(t *testing.T) {
	logger := zaptest.NewLogger(t)

	var requestCount int
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
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

	// Verify LastSent was updated after initial emission
	assert.NotNil(t, monitor.LastSent())
	assert.True(t, monitor.LastSent().Healthy)

	// Wait for multiple polls to occur (but no emissions since status unchanged)
	time.Sleep(150 * time.Millisecond)

	// Cancel context to stop polling
	cancel()

	// Channel should be closed after context cancellation
	// Drain any remaining status updates
	for range ch {
		// Just drain the channel
	}

	// Verify we made at least 2 requests (initial + 1 poll)
	// even though only 1 emission occurred due to deduplication
	mu.Lock()
	count := requestCount
	mu.Unlock()
	assert.GreaterOrEqual(t, count, 2)
}

func TestHealthMonitor_StartPolling_ContextCancelled(t *testing.T) {
	logger := zaptest.NewLogger(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		Endpoint: server.URL,
		Timeout:  5 * time.Second,
		Interval: 1 * time.Second, // Long interval
	}

	monitor := New(logger, cfg)

	ctx, cancel := context.WithCancel(context.Background())

	ch := monitor.StartPolling(ctx)

	// Receive initial check
	status := <-ch
	assert.True(t, status.Healthy)

	// Cancel immediately
	cancel()

	// Channel should be closed
	_, ok := <-ch
	assert.False(t, ok, "channel should be closed after context cancellation")
}

func TestHealthMonitor_LastStatus_InitiallyNil(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := Config{
		Endpoint: "http://localhost:8080/health",
		Timeout:  5 * time.Second,
		Interval: 1 * time.Second,
	}

	monitor := New(logger, cfg)

	// Initially, LastStatus should be nil
	assert.Nil(t, monitor.LastStatus())
}

func TestNew(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := Config{
		Endpoint: "http://localhost:13133/health",
		Timeout:  10 * time.Second,
		Interval: 30 * time.Second,
	}

	monitor := New(logger, cfg)

	assert.NotNil(t, monitor)
	assert.Equal(t, logger, monitor.logger)
	assert.Equal(t, cfg, monitor.cfg)
	assert.NotNil(t, monitor.client)
	assert.Equal(t, cfg.Timeout, monitor.client.Timeout)
	assert.Nil(t, monitor.last)
}

func TestHealthMonitor_CheckHealth_2xxStatusCodes(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Test various 2xx status codes
	statusCodes := []int{200, 201, 202, 204}

	for _, code := range statusCodes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer server.Close()

			cfg := Config{
				Endpoint: server.URL,
				Timeout:  5 * time.Second,
				Interval: 1 * time.Second,
			}

			monitor := New(logger, cfg)

			status, err := monitor.CheckHealth(context.Background())
			require.NoError(t, err)
			assert.True(t, status.Healthy)
			assert.Equal(t, code, status.StatusCode)
		})
	}
}

func TestHealthMonitor_CheckHealth_Non2xxStatusCodes(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Test various non-2xx status codes
	statusCodes := []int{400, 401, 403, 404, 500, 502, 503}

	for _, code := range statusCodes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer server.Close()

			cfg := Config{
				Endpoint: server.URL,
				Timeout:  5 * time.Second,
				Interval: 1 * time.Second,
			}

			monitor := New(logger, cfg)

			status, err := monitor.CheckHealth(context.Background())
			require.NoError(t, err)
			assert.False(t, status.Healthy)
			assert.Equal(t, code, status.StatusCode)
			assert.NotEmpty(t, status.ErrorMessage)
		})
	}
}

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

	// Count emissions received during polling (use non-blocking receive with timeout)
	// We wait for multiple poll cycles but should receive no additional emissions
	// since status is unchanged
	extraEmissions := 0
	timeout := time.After(100 * time.Millisecond)
loop:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				break loop
			}
			extraEmissions++
		case <-timeout:
			break loop
		}
	}

	// Should have no extra emissions since status never changed
	assert.Equal(t, 0, extraEmissions, "should not emit unchanged status")

	// Verify LastSent was set
	assert.NotNil(t, monitor.LastSent())
	assert.True(t, monitor.LastSent().Healthy)

	// Clean up
	cancel()
	for range ch {
	}
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
