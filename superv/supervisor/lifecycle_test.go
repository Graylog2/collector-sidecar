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
	"crypto/tls"
	"sync"
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/Graylog2/collector-sidecar/superv/internal/testserver"
	"github.com/Graylog2/collector-sidecar/superv/opamp"
	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
)

// newStubClient creates a minimal *opamp.Client that is valid but never started.
// Stop() on an unstarted client is a no-op, which is safe for tests that
// only exercise the lock protocol around reconnectClient / Stop.
func newStubClient(t *testing.T) *opamp.Client {
	t.Helper()
	client, err := opamp.NewClient(zaptest.NewLogger(t), opamp.ClientConfig{
		Endpoint:    "ws://stub.invalid/v1/opamp",
		InstanceUID: "00000000-0000-0000-0000-000000000001",
	}, &opamp.Callbacks{})
	require.NoError(t, err)
	return client
}

// newTestSupervisor builds a minimal Supervisor with the worker running and
// createClientFunc ready to override. It does NOT call Start() — only the
// fields needed for reconnectClient and Stop are initialised.
func newTestSupervisor(t *testing.T) *Supervisor {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	s := &Supervisor{
		logger:                    zaptest.NewLogger(t),
		connectionSettingsManager: connection.NewSettingsManager(zaptest.NewLogger(t), t.TempDir()),
		workQueue:                 make(chan workFunc),
		ctx:                       ctx,
		cancel:                    cancel,
	}
	s.connectionSettingsManager.SetCurrent(connection.Settings{
		Endpoint: "ws://stub.invalid/v1/opamp",
	})
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

// TestReconnectClient_StopDuringReconnect verifies that Stop() completing its
// critical section while reconnectClient is blocked inside createClientFunc
// causes reconnectClient to discard the new client.
func TestReconnectClient_StopDuringReconnect(t *testing.T) {
	s := newTestSupervisor(t)
	s.opampClient = newStubClient(t)
	s.isRunning.Store(true)

	entered := make(chan struct{})
	barrier := make(chan struct{})

	s.createClientFunc = func(_ context.Context, _ connection.Settings) (*opamp.Client, error) {
		close(entered)
		<-barrier
		return newStubClient(t), nil
	}

	// Enqueue reconnectClient on the worker.
	settings := s.connectionSettingsManager.GetCurrent()
	var reconnectErr error
	var reconnectDone sync.WaitGroup
	reconnectDone.Add(1)
	ok := s.enqueueWork(context.Background(), func(ctx context.Context) {
		defer reconnectDone.Done()
		reconnectErr = s.reconnectClient(ctx, settings)
	})
	require.True(t, ok)

	// Wait until the factory is entered (createClientFunc is running).
	<-entered

	// Stop() in a goroutine — it will cancel s.ctx and nil out opampClient.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	var stopErr error
	var stopDone sync.WaitGroup
	stopDone.Go(func() {
		stopErr = s.Stop(stopCtx)
	})

	// Wait for s.ctx to be cancelled, confirming Stop's critical section ran.
	<-s.ctx.Done()

	// Release the barrier — reconnectClient should see s.ctx cancelled.
	close(barrier)

	// Wait for both goroutines.
	reconnectDone.Wait()
	stopDone.Wait()

	require.NoError(t, stopErr)
	assert.ErrorContains(t, reconnectErr, "supervisor stopped")

	s.mu.RLock()
	assert.Nil(t, s.opampClient)
	s.mu.RUnlock()
}

// TestReconnectClient_DuringStartupWindow verifies that reconnectClient
// succeeds when called before s.running is set to true (the startup window
// between worker start and the end of Start).
func TestReconnectClient_DuringStartupWindow(t *testing.T) {
	s := newTestSupervisor(t)
	// running is false — simulating the startup window.

	settings := s.connectionSettingsManager.GetCurrent()
	err := s.reconnectClient(context.Background(), settings)

	require.NoError(t, err)

	s.mu.RLock()
	assert.NotNil(t, s.opampClient)
	s.mu.RUnlock()
}

// TestReconnectClient_StopCompletesBeforeAssign verifies that when Stop()
// fully completes its critical section before reconnectClient checks s.ctx,
// the new client is discarded.
func TestReconnectClient_StopCompletesBeforeAssign(t *testing.T) {
	s := newTestSupervisor(t)
	s.opampClient = newStubClient(t)
	s.isRunning.Store(true)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	s.createClientFunc = func(_ context.Context, _ connection.Settings) (*opamp.Client, error) {
		// Call Stop synchronously inside the factory. This means by the time
		// reconnectClient checks s.ctx, Stop has already cancelled it.
		err := s.Stop(stopCtx)
		require.NoError(t, err)
		return newStubClient(t), nil
	}

	// Call reconnectClient directly (not via worker, since Stop drains it).
	settings := s.connectionSettingsManager.GetCurrent()
	err := s.reconnectClient(context.Background(), settings)

	assert.ErrorContains(t, err, "supervisor stopped")

	s.mu.RLock()
	assert.Nil(t, s.opampClient)
	s.mu.RUnlock()
}

// TestEnqueueWork_WorkerStopped verifies that enqueueWork returns false
// promptly once the serialized worker context has been cancelled.
func TestEnqueueWork_WorkerStopped(t *testing.T) {
	s := newTestSupervisor(t)

	s.cancel()
	<-s.ctx.Done()

	ok := s.enqueueWork(context.Background(), func(context.Context) {
		t.Fatal("work item should not run after worker shutdown")
	})

	assert.False(t, ok)
}

// TestIntegration_StopDuringConnectionSettingsUpdate exercises the Stop-during-
// reconnect interleaving with a real OpAMP client talking to a real testserver.
// Unlike the unit tests (which use stub clients where Stop is a no-op), this
// test verifies that a real WebSocket close in reconnectClient does not
// interfere with the lock protocol.
func TestIntegration_StopDuringConnectionSettingsUpdate(t *testing.T) {
	const instanceUID = "00000000-0000-0000-0000-000000000042"
	// Hex-encoded 16-byte UUID, as stored by testserver.
	const instanceUIDHex = "00000000000000000000000000000042"

	// 1. Start testserver with event recorder.
	server, err := testserver.New()
	require.NoError(t, err)
	server.RequireAuth = false

	recorder := testserver.NewTestRecorder()
	server.Logger = recorder

	serverURL := server.Start()
	// Registered first → runs last (LIFO). Must outlive the client cleanup.
	t.Cleanup(server.Stop)

	logger := zaptest.NewLogger(t)

	// 2. Create and start a real OpAMP client using HTTP polling.
	httpURL := serverURL + "/v1/opamp"
	initialClient, err := opamp.NewClient(logger, opamp.ClientConfig{
		Endpoint:    httpURL,
		InstanceUID: instanceUID,
		TLSConfig:   &tls.Config{InsecureSkipVerify: true},
	}, &opamp.Callbacks{})
	require.NoError(t, err)
	require.NoError(t, initialClient.SetAgentDescription(&protobufs.AgentDescription{
		IdentifyingAttributes: []*protobufs.KeyValue{{
			Key:   "service.name",
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: "test"}},
		}},
	}))
	require.NoError(t, initialClient.Start(context.Background()))
	// Registered second → runs before server.Stop (LIFO). Best-effort:
	// stop the client if it wasn't already stopped by reconnectClient
	// during the test. Prevents goroutine leaks on early assertion failure.
	// Short timeout: in the happy path the client is already stopped and
	// the opamp-go HTTP client blocks on a redundant Stop(); in the failure
	// path 1 s is enough for a clean shutdown.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		initialClient.Stop(cleanupCtx)
	})

	// 3. Wait for the agent to appear on the testserver.
	_, err = recorder.WaitForKind(testserver.EventAgentConnect, 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, server.GetAgent(instanceUIDHex))

	// 4. Build a minimal supervisor around the real client.
	ctx, cancel := context.WithCancel(context.Background())
	s := &Supervisor{
		logger:                    logger,
		connectionSettingsManager: connection.NewSettingsManager(logger, t.TempDir()),
		workQueue:                 make(chan workFunc),
		ctx:                       ctx,
		cancel:                    cancel,
		opampClient:               initialClient,
	}
	s.isRunning.Store(true)
	s.connectionSettingsManager.SetCurrent(connection.Settings{
		Endpoint: httpURL,
		TLS:      connection.TLSSettings{Insecure: true},
	})
	s.workWg.Add(1)
	go s.runWorker()
	t.Cleanup(func() {
		cancel()
		s.workWg.Wait()
	})

	// 5. Override createClientFunc: signal "entered", block on barrier,
	//    then return a stub (it will be discarded by the cancelled-ctx check).
	entered := make(chan struct{})
	barrier := make(chan struct{})
	s.createClientFunc = func(_ context.Context, _ connection.Settings) (*opamp.Client, error) {
		close(entered)
		<-barrier
		return newStubClient(t), nil
	}

	// 6. Enqueue reconnectClient on the worker.
	settings := s.connectionSettingsManager.GetCurrent()
	var reconnectErr error
	var reconnectDone sync.WaitGroup
	reconnectDone.Add(1)
	ok := s.enqueueWork(context.Background(), func(wCtx context.Context) {
		defer reconnectDone.Done()
		reconnectErr = s.reconnectClient(wCtx, settings)
	})
	require.True(t, ok)

	// 7. Wait until the factory is entered — the old (real) client has been
	//    stopped by reconnectClient at this point.
	<-entered

	// 8. Stop() in a goroutine.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	var stopErr error
	var stopDone sync.WaitGroup
	stopDone.Go(func() {
		stopErr = s.Stop(stopCtx)
	})

	// 9. Wait for s.ctx to be cancelled (Stop's critical section completed).
	<-s.ctx.Done()

	// 10. Release the barrier — reconnectClient will see s.ctx cancelled.
	close(barrier)

	// 11. Wait for both goroutines.
	reconnectDone.Wait()
	stopDone.Wait()

	require.NoError(t, stopErr)
	assert.ErrorContains(t, reconnectErr, "supervisor stopped")

	s.mu.RLock()
	assert.Nil(t, s.opampClient)
	s.mu.RUnlock()
}
