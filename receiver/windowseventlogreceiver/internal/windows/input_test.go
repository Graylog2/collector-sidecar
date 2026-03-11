// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package windows

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension/xextension/storage"
	"go.uber.org/zap/zaptest"
	"golang.org/x/sys/windows"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator"
)

type testPersister struct {
	get    func(context.Context, string) ([]byte, error)
	set    func(context.Context, string, []byte) error
	delete func(context.Context, string) error
	batch  func(context.Context, ...*storage.Operation) error
}

func (p testPersister) Get(ctx context.Context, key string) ([]byte, error) {
	if p.get != nil {
		return p.get(ctx, key)
	}
	return nil, nil
}

func (p testPersister) Set(ctx context.Context, key string, value []byte) error {
	if p.set != nil {
		return p.set(ctx, key, value)
	}
	return nil
}

func (p testPersister) Delete(ctx context.Context, key string) error {
	if p.delete != nil {
		return p.delete(ctx, key)
	}
	return nil
}

func (p testPersister) Batch(ctx context.Context, ops ...*storage.Operation) error {
	if p.batch != nil {
		return p.batch(ctx, ops...)
	}
	return nil
}

var _ operator.Persister = testPersister{}

func TestInputStartClosesBookmarkOnChannelFilterError(t *testing.T) {
	input := newTestInput(t)
	input.listChannels = func() ([]string, error) {
		return nil, syscall.Errno(rpcServerUnavailable)
	}

	closedHandles := stubBookmarkLifecycle(t, 42)

	err := input.Start(testPersister{
		get: func(context.Context, string) ([]byte, error) {
			return []byte("<Bookmark/>"), nil
		},
	})

	require.ErrorIs(t, err, syscall.Errno(rpcServerUnavailable))
	require.Equal(t, []uintptr{42}, closedHandles())
	require.Zero(t, input.bookmark.handle)
}

func TestInputStartClosesBookmarkOnSubscriptionOpenError(t *testing.T) {
	input := newTestInput(t)
	input.listChannels = func() ([]string, error) {
		return []string{"Security"}, nil
	}

	closedHandles := stubBookmarkLifecycle(t, 99)

	oldSubscribe := evtSubscribe
	evtSubscribe = func(uintptr, windows.Handle, *uint16, *uint16, uintptr, uintptr, uintptr, uint32) (uintptr, error) {
		return 0, syscall.Errno(evtChannelNotFound)
	}
	t.Cleanup(func() {
		evtSubscribe = oldSubscribe
	})

	err := input.Start(testPersister{
		get: func(context.Context, string) ([]byte, error) {
			return []byte("<Bookmark/>"), nil
		},
	})

	require.ErrorContains(t, err, "failed to open local subscription")
	require.Equal(t, []uintptr{99}, closedHandles())
	require.Zero(t, input.bookmark.handle)
}

func TestInputStartTransientSubscriptionErrorDefersBookmarkCloseToStop(t *testing.T) {
	input := newTestInput(t)
	input.listChannels = func() ([]string, error) {
		return []string{"Security"}, nil
	}

	closedHandles := stubBookmarkLifecycle(t, 7)

	oldSubscribe := evtSubscribe
	evtSubscribe = func(uintptr, windows.Handle, *uint16, *uint16, uintptr, uintptr, uintptr, uint32) (uintptr, error) {
		return 0, syscall.Errno(errorInvalidHandle)
	}
	t.Cleanup(func() {
		evtSubscribe = oldSubscribe
	})

	err := input.Start(testPersister{
		get: func(context.Context, string) ([]byte, error) {
			return []byte("<Bookmark/>"), nil
		},
	})

	require.NoError(t, err)
	require.False(t, input.subscriptionOpen)
	require.Empty(t, closedHandles())

	require.NoError(t, input.Stop())
	require.Equal(t, []uintptr{7}, closedHandles())
}

func TestInputStopWithoutBookmarkIsSafe(t *testing.T) {
	input := &Input{}

	require.NotPanics(t, func() {
		require.NoError(t, input.Stop())
	})
}

func newTestInput(t *testing.T) *Input {
	t.Helper()

	cfg := NewConfig()
	cfg.Channel = "Security"
	cfg.PollInterval = time.Hour

	op, err := cfg.Build(component.TelemetrySettings{
		Logger: zaptest.NewLogger(t),
	})
	require.NoError(t, err)

	input, ok := op.(*Input)
	require.True(t, ok)
	return input
}

func stubBookmarkLifecycle(t *testing.T, handle uintptr) func() []uintptr {
	t.Helper()

	var closed []uintptr

	oldCreateBookmark := evtCreateBookmark
	oldClose := evtClose

	evtCreateBookmark = func(*uint16) (uintptr, error) {
		return handle, nil
	}
	evtClose = func(handle uintptr) error {
		closed = append(closed, handle)
		return nil
	}

	t.Cleanup(func() {
		evtCreateBookmark = oldCreateBookmark
		evtClose = oldClose
	})

	return func() []uintptr {
		return append([]uintptr(nil), closed...)
	}
}
