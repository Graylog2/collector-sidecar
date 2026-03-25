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

package keen

import (
	"cmp"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
)

// BackoffConfig configures the backoff behavior.
type BackoffConfig struct {
	// InitialInterval is the first delay duration. Default: 1s.
	InitialInterval time.Duration

	// MaxInterval is the maximum delay duration. Default: 30s.
	MaxInterval time.Duration

	// Multiplier is the factor by which the delay increases. Default: 2.0.
	Multiplier float64

	// RandomizationFactor adds jitter to delays. 0.5 means ±50%. Default: 0.5.
	RandomizationFactor float64

	// MaxRetries is the maximum number of retry attempts. 0 means unlimited.
	MaxRetries int

	// StableAfter is the duration after which a running process is considered
	// stable and the backoff counter should be reset. 0 disables stability tracking.
	StableAfter time.Duration
}

// DefaultBackoffConfig returns sensible defaults for process restart backoff.
func DefaultBackoffConfig() BackoffConfig {
	return BackoffConfig{
		InitialInterval:     1 * time.Second,
		MaxInterval:         30 * time.Second,
		Multiplier:          2.0,
		RandomizationFactor: 0.5,
		MaxRetries:          5,
		StableAfter:         30 * time.Second,
	}
}

// Backoff tracks restart attempts and calculates delays using exponential backoff.
type Backoff struct {
	cfg       BackoffConfig
	exp       *backoff.ExponentialBackOff
	attempts  int
	startTime time.Time
	mu        sync.Mutex
}

// NewBackoff creates a new backoff tracker.
func NewBackoff(cfg BackoffConfig) *Backoff {
	defaults := DefaultBackoffConfig()

	// Apply defaults for zero values.
	// RandomizationFactor of 0 is valid (no jitter), so don't default it.
	cfg.InitialInterval = cmp.Or(cfg.InitialInterval, defaults.InitialInterval)
	cfg.MaxInterval = cmp.Or(cfg.MaxInterval, defaults.MaxInterval)
	cfg.Multiplier = cmp.Or(cfg.Multiplier, defaults.Multiplier)
	cfg.StableAfter = cmp.Or(cfg.StableAfter, defaults.StableAfter)

	exp := backoff.NewExponentialBackOff()
	exp.InitialInterval = cfg.InitialInterval
	exp.MaxInterval = cfg.MaxInterval
	exp.Multiplier = cfg.Multiplier
	exp.RandomizationFactor = cfg.RandomizationFactor

	return &Backoff{
		cfg: cfg,
		exp: exp,
	}
}

// NextDelay returns the next backoff delay and increments the attempt counter.
func (b *Backoff) NextDelay() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	delay := b.exp.NextBackOff()
	b.attempts++

	return delay
}

// ShouldRetry returns true if another retry attempt is allowed.
func (b *Backoff) ShouldRetry() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cfg.MaxRetries == 0 {
		return true // Unlimited retries
	}
	return b.attempts < b.cfg.MaxRetries
}

// Attempts returns the current number of attempts.
func (b *Backoff) Attempts() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.attempts
}

// Reset resets the backoff counter and delay calculation to initial state.
func (b *Backoff) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.attempts = 0
	b.startTime = time.Time{}
	b.exp.Reset()
}

// MarkRunning marks the process as running, starting the stability timer.
func (b *Backoff) MarkRunning() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.startTime = time.Now()
}

// IsStable returns true if the process has been running long enough to be
// considered stable (and backoff should be reset).
func (b *Backoff) IsStable() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.startTime.IsZero() || b.cfg.StableAfter == 0 {
		return false
	}
	return time.Since(b.startTime) >= b.cfg.StableAfter
}

// CheckAndResetIfStable checks if stable and resets if so. Returns true if reset occurred.
// This operation is atomic to prevent race conditions.
func (b *Backoff) CheckAndResetIfStable() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.startTime.IsZero() || b.cfg.StableAfter == 0 {
		return false
	}
	if time.Since(b.startTime) >= b.cfg.StableAfter {
		b.attempts = 0
		b.startTime = time.Time{}
		b.exp.Reset()
		return true
	}
	return false
}

// MaxRetries returns the configured maximum number of retries.
func (b *Backoff) MaxRetries() int {
	return b.cfg.MaxRetries
}
