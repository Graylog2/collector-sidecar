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
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

// AgentState represents the current state of the managed agent.
type AgentState int

const (
	// AgentStateUnknown indicates the agent state is unknown.
	AgentStateUnknown AgentState = iota
	// AgentStateStarting indicates the agent is starting up.
	AgentStateStarting
	// AgentStateRunning indicates the agent is running normally.
	AgentStateRunning
	// AgentStateStopping indicates the agent is shutting down.
	AgentStateStopping
	// AgentStateStopped indicates the agent has stopped.
	AgentStateStopped
	// AgentStateFailed indicates the agent has failed.
	AgentStateFailed
)

// AgentStateProvider provides information about the managed agent's state.
// This interface allows the health monitor to report accurate agent information
// without being tightly coupled to specific agent management implementations.
type AgentStateProvider interface {
	// StartTime returns the time when the agent process started.
	// Returns zero time if the agent has not started or start time is unknown.
	StartTime() time.Time

	// State returns the current state of the agent.
	State() AgentState
}

// Config holds configuration for the health monitor.
type Config struct {
	Endpoint           string
	Timeout            time.Duration
	Interval           time.Duration
	StartupGracePeriod time.Duration
}

// HealthStatus represents the health state of the collector.
type HealthStatus struct {
	Healthy      bool
	StatusCode   int
	ErrorMessage string
}

// Equal returns true if two HealthStatus values are equivalent.
func (s *HealthStatus) Equal(other *HealthStatus) bool {
	if s == nil || other == nil {
		return s == other
	}
	return s.Healthy == other.Healthy &&
		s.StatusCode == other.StatusCode &&
		s.ErrorMessage == other.ErrorMessage
}

// ToComponentHealth converts the health status to OpAMP ComponentHealth format.
// agentProvider is optional - if nil, start time will be zero.
func (s *HealthStatus) ToComponentHealth(agentProvider AgentStateProvider) *protobufs.ComponentHealth {
	var startTimeUnixNano uint64
	if agentProvider != nil {
		startTime := agentProvider.StartTime()
		if !startTime.IsZero() {
			startTimeUnixNano = uint64(startTime.UnixNano())
		}
	}

	return &protobufs.ComponentHealth{
		Healthy:           s.Healthy,
		LastError:         s.ErrorMessage,
		StartTimeUnixNano: startTimeUnixNano,
	}
}

// Monitor polls the collector's health endpoint and reports status.
type Monitor struct {
	logger *zap.Logger
	cfg    Config
	client *http.Client

	mu       sync.RWMutex
	last     *HealthStatus
	lastSent *HealthStatus // last status emitted to channel
}

// New creates a new health monitor with an HTTP client using the configured timeout.
func New(logger *zap.Logger, cfg Config) *Monitor {
	if cfg.Interval <= 0 {
		cfg.Interval = 10 * time.Second
	}
	return &Monitor{
		logger: logger,
		cfg:    cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// CheckHealth performs a single health check via HTTP GET to the configured endpoint.
// Returns HealthStatus with Healthy=true if the status code is 2xx.
// On connection error, returns an error and HealthStatus with Healthy=false.
// Updates the internal last status with the result.
func (m *Monitor) CheckHealth(ctx context.Context) (*HealthStatus, error) {
	status := &HealthStatus{}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.cfg.Endpoint, nil)
	if err != nil {
		status.ErrorMessage = err.Error()
		m.setLastStatus(status)
		return status, err
	}

	resp, err := m.client.Do(req)
	if err != nil {
		status.ErrorMessage = err.Error()
		m.setLastStatus(status)
		return status, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	status.StatusCode = resp.StatusCode
	status.Healthy = resp.StatusCode >= 200 && resp.StatusCode < 300

	if !status.Healthy {
		status.ErrorMessage = http.StatusText(resp.StatusCode)
	}

	m.setLastStatus(status)
	return status, nil
}

// LastStatus returns the last health status.
func (m *Monitor) LastStatus() *HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.last
}

// setLastStatus updates the last status in a thread-safe manner.
func (m *Monitor) setLastStatus(status *HealthStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.last = status
}

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

// StartPolling starts background polling of the health endpoint.
// It performs an initial check immediately, then polls every cfg.Interval.
// Sends health status updates to the returned channel only when status changes.
// Stops when the context is cancelled.
func (m *Monitor) StartPolling(ctx context.Context) <-chan *HealthStatus {
	ch := make(chan *HealthStatus)

	go func() {
		defer close(ch)

		// Wait for the startup grace period before the first health check,
		// giving the collector time to bind its health endpoint.
		if m.cfg.StartupGracePeriod > 0 {
			m.logger.Debug("Waiting for startup grace period", zap.Duration("duration", m.cfg.StartupGracePeriod))
			timer := time.NewTimer(m.cfg.StartupGracePeriod)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
				m.logger.Debug("Context cancelled during startup grace period")
				return
			}
		}

		// Perform initial check
		status, err := m.CheckHealth(ctx)
		if err != nil {
			m.logger.Debug("Initial health check failed", zap.Error(err))
		}
		// Always send initial status
		select {
		case ch <- status:
			m.setLastSent(status)
		case <-ctx.Done():
			m.logger.Debug("Context cancelled")
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
					m.logger.Debug("Health check failed", zap.Error(err))
				}
				// Only send if status changed
				if !status.Equal(m.LastSent()) {
					select {
					case ch <- status:
						m.setLastSent(status)
					case <-ctx.Done():
						m.logger.Debug("Context cancelled")
						return
					}
				}
			}
		}
	}()

	return ch
}
