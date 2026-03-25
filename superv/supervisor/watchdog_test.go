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

package supervisor

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/Graylog2/collector-sidecar/superv/opamp"
	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
)

// newSynctestSupervisor builds a minimal Supervisor with the worker running
// inside the current goroutine's synctest bubble. Must be called from within
// synctest.Test.
func newSynctestSupervisor(t *testing.T) *Supervisor {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	s := &Supervisor{
		logger:                    zaptest.NewLogger(t),
		connectionSettingsManager: connection.NewSettingsManager(zaptest.NewLogger(t), t.TempDir()),
		workQueue:                 make(chan workFunc),
		workCtx:                   ctx,
		workCancel:                cancel,
	}
	s.createClientFunc = func(_ context.Context, _ connection.Settings) (*opamp.Client, error) {
		return newStubClient(t), nil
	}

	s.workWg.Add(1)
	go s.runWorker()

	t.Cleanup(func() {
		cancel()
		s.workWg.Wait()
	})

	return s
}

func TestCheckConnectionWatchdog_NoReconnectWhenFresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSynctestSupervisor(t)
		s.connectionSettingsManager.SetCurrent(connection.Settings{
			Endpoint:          "https://stub.invalid/v1/opamp",
			HeartbeatInterval: 10 * time.Second,
		})
		s.opampClient = newStubClient(t)
		s.lastConnected.Store(time.Now().UnixNano())

		var reconnected atomic.Bool
		s.createClientFunc = func(_ context.Context, _ connection.Settings) (*opamp.Client, error) {
			reconnected.Store(true)
			return newStubClient(t), nil
		}

		s.checkConnectionWatchdog()
		synctest.Wait()

		assert.False(t, reconnected.Load(), "should not reconnect when connection is fresh")
	})
}

func TestCheckConnectionWatchdog_NoReconnectBeforeFirstConnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSynctestSupervisor(t)
		s.connectionSettingsManager.SetCurrent(connection.Settings{
			Endpoint:          "https://stub.invalid/v1/opamp",
			HeartbeatInterval: 10 * time.Second,
		})

		// lastConnected is zero (never connected).
		var reconnected atomic.Bool
		s.createClientFunc = func(_ context.Context, _ connection.Settings) (*opamp.Client, error) {
			reconnected.Store(true)
			return newStubClient(t), nil
		}

		s.checkConnectionWatchdog()
		synctest.Wait()

		assert.False(t, reconnected.Load(), "should not reconnect before first successful connection")
	})
}

func TestCheckConnectionWatchdog_ReconnectsWhenStale(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSynctestSupervisor(t)
		s.connectionSettingsManager.SetCurrent(connection.Settings{
			Endpoint:          "https://stub.invalid/v1/opamp",
			HeartbeatInterval: 10 * time.Second,
		})
		s.opampClient = newStubClient(t)

		// Record connection time, then advance the fake clock past the 3x threshold.
		s.lastConnected.Store(time.Now().UnixNano())
		time.Sleep(31 * time.Second) // > 3 * 10s

		var reconnected atomic.Bool
		s.createClientFunc = func(_ context.Context, _ connection.Settings) (*opamp.Client, error) {
			reconnected.Store(true)
			return newStubClient(t), nil
		}

		s.checkConnectionWatchdog()
		synctest.Wait()

		assert.True(t, reconnected.Load(), "should reconnect when connection is stale")
	})
}

func TestCheckConnectionWatchdog_UsesDefaultHeartbeatWhenZero(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSynctestSupervisor(t)
		s.connectionSettingsManager.SetCurrent(connection.Settings{
			Endpoint:          "https://stub.invalid/v1/opamp",
			HeartbeatInterval: 0, // zero → should use 30s default
		})
		s.opampClient = newStubClient(t)

		// Connected, then advance 10s. With default 30s heartbeat, threshold
		// is 90s, so 10s is well within bounds.
		s.lastConnected.Store(time.Now().UnixNano())
		time.Sleep(10 * time.Second)

		var reconnected atomic.Bool
		s.createClientFunc = func(_ context.Context, _ connection.Settings) (*opamp.Client, error) {
			reconnected.Store(true)
			return newStubClient(t), nil
		}

		s.checkConnectionWatchdog()
		synctest.Wait()

		assert.False(t, reconnected.Load(), "10s stale with 30s heartbeat should not trigger reconnect")
	})
}

func TestCheckConnectionWatchdog_ReconnectUpdatesLastConnected(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSynctestSupervisor(t)
		s.connectionSettingsManager.SetCurrent(connection.Settings{
			Endpoint:          "https://stub.invalid/v1/opamp",
			HeartbeatInterval: 10 * time.Second,
		})
		s.opampClient = newStubClient(t)

		var reconnectCount atomic.Int32
		s.createClientFunc = func(_ context.Context, _ connection.Settings) (*opamp.Client, error) {
			reconnectCount.Add(1)
			return newStubClient(t), nil
		}

		// Record connection time, then make it stale.
		s.lastConnected.Store(time.Now().UnixNano())
		time.Sleep(31 * time.Second)

		// First watchdog check triggers reconnect.
		s.checkConnectionWatchdog()
		synctest.Wait()
		require.Equal(t, int32(1), reconnectCount.Load())

		// Simulate OnConnect updating the timestamp (as the real callback does).
		s.lastConnected.Store(time.Now().UnixNano())

		// Second watchdog check should NOT trigger another reconnect.
		s.checkConnectionWatchdog()
		synctest.Wait()
		assert.Equal(t, int32(1), reconnectCount.Load(), "should not reconnect again after lastConnected is refreshed")
	})
}

func TestCheckConnectionWatchdog_SkipsWebSocket(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSynctestSupervisor(t)
		s.connectionSettingsManager.SetCurrent(connection.Settings{
			Endpoint:          "wss://stub.invalid/v1/opamp",
			HeartbeatInterval: 10 * time.Second,
		})
		s.opampClient = newStubClient(t)

		// Make it look stale — would trigger reconnect for HTTP.
		s.lastConnected.Store(time.Now().UnixNano())
		time.Sleep(31 * time.Second)

		var reconnected atomic.Bool
		s.createClientFunc = func(_ context.Context, _ connection.Settings) (*opamp.Client, error) {
			reconnected.Store(true)
			return newStubClient(t), nil
		}

		s.checkConnectionWatchdog()
		synctest.Wait()

		assert.False(t, reconnected.Load(), "watchdog should not fire for WebSocket transport")
	})
}
