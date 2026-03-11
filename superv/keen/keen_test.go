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
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestCommander_StartStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - need different test binary")
	}

	logger := zaptest.NewLogger(t)
	logsDir := t.TempDir()

	cmd, err := New(logger, logsDir, Config{
		Executable: "/bin/sleep",
		Args:       []string{"60"},
	}, NewBackoff(DefaultBackoffConfig()))
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.start(ctx)
	require.NoError(t, err)
	require.True(t, cmd.IsRunning())
	require.Greater(t, cmd.Pid(), 0)

	err = cmd.Stop(ctx)
	require.NoError(t, err)
	require.False(t, cmd.IsRunning())
}

func TestCommander_StartAlreadyRunning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	logger := zaptest.NewLogger(t)
	logsDir := t.TempDir()

	cmd, err := New(logger, logsDir, Config{
		Executable: "/bin/sleep",
		Args:       []string{"60"},
	}, NewBackoff(DefaultBackoffConfig()))
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.start(ctx)
	require.NoError(t, err)
	defer cmd.Stop(ctx)

	// Second start should be no-op
	err = cmd.start(ctx)
	require.NoError(t, err)
}

func TestCommander_StopNotRunning(t *testing.T) {
	logger := zaptest.NewLogger(t)
	logsDir := t.TempDir()

	cmd, err := New(logger, logsDir, Config{
		Executable: "/bin/sleep",
		Args:       []string{"60"},
	}, NewBackoff(DefaultBackoffConfig()))
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.Stop(ctx)
	require.NoError(t, err)
}

func TestCommander_ExitedChannel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	logger := zaptest.NewLogger(t)
	logsDir := t.TempDir()

	cmd, err := New(logger, logsDir, Config{
		Executable: "/bin/sh",
		Args:       []string{"-c", "exit 0"},
	}, NewBackoff(DefaultBackoffConfig()))
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.start(ctx)
	require.NoError(t, err)

	select {
	case <-cmd.Exited():
		// Expected
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for process to exit")
	}

	require.False(t, cmd.IsRunning())
}

func TestCommander_Restart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	logger := zaptest.NewLogger(t)
	logsDir := t.TempDir()

	cmd, err := New(logger, logsDir, Config{
		Executable: "/bin/sleep",
		Args:       []string{"60"},
	}, NewBackoff(BackoffConfig{MaxRetries: 0})) // Disable restart so we don't have to take care of the async recovery look startup
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.start(ctx)
	require.NoError(t, err)

	pid1 := cmd.Pid()

	err = cmd.Restart(ctx)
	require.NoError(t, err)
	require.True(t, cmd.IsRunning())

	pid2 := cmd.Pid()
	require.NotEqual(t, pid1, pid2, "PID should change after restart")

	cmd.Stop(ctx)
}

func TestCommander_CrashRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - need different test binary")
	}

	logger := zaptest.NewLogger(t)

	// Use a command that exits immediately (simulates crash)
	cmd, err := New(logger, t.TempDir(), Config{
		Executable: "/bin/false", // Always exits with code 1
		Args:       []string{},
	}, NewBackoff(BackoffConfig{ // Configure backoff with short delays for testing
		InitialInterval:     10 * time.Millisecond,
		MaxInterval:         20 * time.Millisecond,
		Multiplier:          2.0,
		RandomizationFactor: 0, // No jitter for predictable tests
		MaxRetries:          3,
	}))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// start with crash recovery enabled
	err = cmd.Start(ctx)
	require.NoError(t, err)

	// Wait for max retries to be exhausted
	select {
	case <-cmd.Done():
		// Expected - process gave up after max retries
	case <-ctx.Done():
		t.Fatal("timeout waiting for crash recovery to exhaust retries")
	}

	require.GreaterOrEqual(t, cmd.CrashCount(), 2)
}

func TestCommander_CrashRecovery_GracefulExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	logger := zaptest.NewLogger(t)

	// Use a command that exits cleanly (exit code 0)
	cmd, err := New(logger, t.TempDir(), Config{
		Executable: "/bin/true", // Always exits with code 0
		Args:       []string{},
	}, NewBackoff(BackoffConfig{
		InitialInterval:     10 * time.Millisecond,
		RandomizationFactor: 0,
		MaxRetries:          5,
	}))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = cmd.Start(ctx)
	require.NoError(t, err)

	// Wait for recovery loop to finish
	select {
	case <-cmd.Done():
		// Expected - process exited cleanly, no restart
	case <-ctx.Done():
		t.Fatal("timeout waiting for clean exit")
	}

	// No crash should be counted for clean exit
	require.Equal(t, 0, cmd.CrashCount())
}

func TestCommander_Stop_DuringAsyncStartup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	logger := zaptest.NewLogger(t)

	// Use a command that takes time to start (sleep simulates a slow startup).
	// With recovery enabled, Start() returns immediately and start() runs
	// asynchronously. Calling Stop() before cmd.Start() populates cmd.Process
	// must not panic.
	cmd, err := New(logger, t.TempDir(), Config{
		Executable: "/bin/sleep",
		Args:       []string{"60"},
	}, NewBackoff(BackoffConfig{
		InitialInterval:     10 * time.Millisecond,
		RandomizationFactor: 0,
		MaxRetries:          1,
	}))
	require.NoError(t, err)

	ctx := context.Background()

	// Simulate the race: set cmd (as start() does) but leave Process nil
	// (as it would be before exec.Cmd.Start() completes).
	cmd.mu.Lock()
	cmd.running.Store(true)
	cmd.cmd = &exec.Cmd{} // Process is nil
	cmd.mu.Unlock()

	// Stop must not panic on nil cmd.Process
	err = cmd.Stop(ctx)
	require.NoError(t, err)
}

func TestCommander_CrashRecovery_StopDuringRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	logger := zaptest.NewLogger(t)

	// Use a command that exits immediately but would retry forever
	cmd, err := New(logger, t.TempDir(), Config{
		Executable: "/bin/false",
		Args:       []string{},
	}, NewBackoff(BackoffConfig{
		InitialInterval:     50 * time.Millisecond,
		MaxInterval:         50 * time.Millisecond,
		RandomizationFactor: 0,
		MaxRetries:          0, // Unlimited retries
	}))
	require.NoError(t, err)

	ctx := context.Background()

	err = cmd.Start(ctx)
	require.NoError(t, err)

	// Wait a bit for some crashes to occur
	time.Sleep(100 * time.Millisecond)

	// Stop should work even during recovery
	err = cmd.Stop(ctx)
	require.NoError(t, err)

	// Wait for recovery loop to finish
	select {
	case <-cmd.Done():
		// Expected
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for recovery loop to stop")
	}
}
