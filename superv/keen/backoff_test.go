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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackoff_NextDelay(t *testing.T) {
	b := NewBackoff(BackoffConfig{
		InitialInterval:     100 * time.Millisecond,
		MaxInterval:         1 * time.Second,
		Multiplier:          2.0,
		RandomizationFactor: 0, // No jitter for predictable tests
		MaxRetries:          5,
	})

	// First delay should be around InitialInterval
	d1 := b.NextDelay()
	require.Equal(t, 100*time.Millisecond, d1)

	// Second delay should be doubled (multiplier=2)
	d2 := b.NextDelay()
	require.Equal(t, 200*time.Millisecond, d2)

	// Third delay should be doubled again
	d3 := b.NextDelay()
	require.Equal(t, 400*time.Millisecond, d3)

	// Fourth delay
	d4 := b.NextDelay()
	require.Equal(t, 800*time.Millisecond, d4)

	// Fifth delay should be capped at MaxInterval
	d5 := b.NextDelay()
	require.Equal(t, 1*time.Second, d5)

	// Further delays stay at MaxInterval
	d6 := b.NextDelay()
	require.Equal(t, 1*time.Second, d6)
}

func TestBackoff_Reset(t *testing.T) {
	b := NewBackoff(BackoffConfig{
		InitialInterval:     100 * time.Millisecond,
		MaxInterval:         1 * time.Second,
		Multiplier:          2.0,
		RandomizationFactor: 0,
		MaxRetries:          5,
	})

	b.NextDelay()
	b.NextDelay()
	require.Equal(t, 2, b.Attempts())

	b.Reset()
	require.Equal(t, 0, b.Attempts())

	// After reset, should start from InitialInterval again
	d := b.NextDelay()
	require.Equal(t, 100*time.Millisecond, d)
}

func TestBackoff_ShouldRetry(t *testing.T) {
	b := NewBackoff(BackoffConfig{
		InitialInterval:     100 * time.Millisecond,
		RandomizationFactor: 0,
		MaxRetries:          3,
	})

	require.True(t, b.ShouldRetry())
	b.NextDelay()
	require.True(t, b.ShouldRetry())
	b.NextDelay()
	require.True(t, b.ShouldRetry())
	b.NextDelay()
	require.False(t, b.ShouldRetry()) // 3 attempts reached
}

func TestBackoff_UnlimitedRetries(t *testing.T) {
	b := NewBackoff(BackoffConfig{
		InitialInterval:     1 * time.Millisecond,
		MaxInterval:         1 * time.Millisecond,
		RandomizationFactor: 0,
		MaxRetries:          0, // 0 means unlimited
	})

	for range 100 {
		require.True(t, b.ShouldRetry())
		b.NextDelay()
	}
}

func TestBackoff_ResetAfterStableDuration(t *testing.T) {
	b := NewBackoff(BackoffConfig{
		InitialInterval:     100 * time.Millisecond,
		RandomizationFactor: 0,
		MaxRetries:          5,
		StableAfter:         10 * time.Millisecond,
	})

	b.NextDelay()
	b.NextDelay()
	require.Equal(t, 2, b.Attempts())

	// Mark as running
	b.MarkRunning()

	// Not yet stable
	require.False(t, b.IsStable())

	// Wait for stability
	time.Sleep(15 * time.Millisecond)

	// Now should be stable
	require.True(t, b.IsStable())

	// CheckAndResetIfStable should reset
	require.True(t, b.CheckAndResetIfStable())
	require.Equal(t, 0, b.Attempts())
}

func TestBackoff_Jitter(t *testing.T) {
	b := NewBackoff(BackoffConfig{
		InitialInterval:     100 * time.Millisecond,
		MaxInterval:         1 * time.Second,
		Multiplier:          2.0,
		RandomizationFactor: 0.5, // ±50% jitter
		MaxRetries:          10,
	})

	// Collect several delays and verify they have variation
	delays := make([]time.Duration, 5)
	for i := range delays {
		delays[i] = b.NextDelay()
	}

	// With 50% jitter, first delay should be between 50ms and 150ms
	require.GreaterOrEqual(t, delays[0], 50*time.Millisecond)
	require.LessOrEqual(t, delays[0], 150*time.Millisecond)
}

func TestBackoff_MaxRetries(t *testing.T) {
	b := NewBackoff(BackoffConfig{
		MaxRetries: 7,
	})
	require.Equal(t, 7, b.MaxRetries())
}

func TestBackoff_DefaultConfig(t *testing.T) {
	cfg := DefaultBackoffConfig()
	require.Equal(t, 1*time.Second, cfg.InitialInterval)
	require.Equal(t, 30*time.Second, cfg.MaxInterval)
	require.InEpsilon(t, 2.0, cfg.Multiplier, 0.0001)
	require.InEpsilon(t, 0.5, cfg.RandomizationFactor, 0.0001)
	require.Equal(t, 5, cfg.MaxRetries)
	require.Equal(t, 30*time.Second, cfg.StableAfter)
}
