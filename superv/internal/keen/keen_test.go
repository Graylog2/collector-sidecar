// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package keen

import (
	"context"
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
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.Start(ctx)
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
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.Start(ctx)
	require.NoError(t, err)
	defer cmd.Stop(ctx)

	// Second start should be no-op
	err = cmd.Start(ctx)
	require.NoError(t, err)
}

func TestCommander_StopNotRunning(t *testing.T) {
	logger := zaptest.NewLogger(t)
	logsDir := t.TempDir()

	cmd, err := New(logger, logsDir, Config{
		Executable: "/bin/sleep",
		Args:       []string{"60"},
	})
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
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.Start(ctx)
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
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = cmd.Start(ctx)
	require.NoError(t, err)

	pid1 := cmd.Pid()

	err = cmd.Restart(ctx)
	require.NoError(t, err)
	require.True(t, cmd.IsRunning())

	pid2 := cmd.Pid()
	require.NotEqual(t, pid1, pid2, "PID should change after restart")

	cmd.Stop(ctx)
}
