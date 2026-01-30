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

	// Receive at least one more poll
	status = <-ch
	assert.True(t, status.Healthy)

	// Cancel context to stop polling
	cancel()

	// Channel should be closed after context cancellation
	// Drain any remaining status updates
	for range ch {
		// Just drain the channel
	}

	// Verify we made at least 2 requests (initial + 1 poll)
	assert.GreaterOrEqual(t, requestCount, 2)
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
