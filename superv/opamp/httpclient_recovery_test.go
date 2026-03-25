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

package opamp_test

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/client"
	"github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// testHTTPClientRecovery starts a test server returning rejectCode for the
// first rejectCount requests, then 200 OK. It returns whether the client
// recovered (OnConnect fired) within timeout.
func testHTTPClientRecovery(t *testing.T, rejectCode, rejectCount int, timeout time.Duration) (recovered bool, totalRequests int64) {
	t.Helper()

	var requestCount atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/opamp", func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)
		t.Logf("  request #%d at %v", n, time.Now().Format("15:04:05.000"))

		if n <= int64(rejectCount) {
			w.WriteHeader(rejectCode)
			return
		}

		resp := &protobufs.ServerToAgent{}
		data, err := proto.Marshal(resp)
		if err != nil {
			t.Errorf("marshal: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	srv := &http.Server{Addr: "127.0.0.1:0", Handler: mux}
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", srv.Addr)
	require.NoError(t, err)
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })

	endpoint := "http://" + ln.Addr().String() + "/v1/opamp"

	connected := make(chan struct{}, 1)

	c := client.NewHTTP(nil)
	caps := protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus
	require.NoError(t, c.SetCapabilities(&caps))
	require.NoError(t, c.SetAgentDescription(&protobufs.AgentDescription{
		IdentifyingAttributes: []*protobufs.KeyValue{
			{Key: "service.name", Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: "test"}}},
		},
	}))

	hb := 100 * time.Millisecond
	settings := types.StartSettings{
		OpAMPServerURL:    endpoint,
		InstanceUid:       types.InstanceUid{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		HeartbeatInterval: &hb,
		Callbacks: types.Callbacks{
			OnConnect: func(_ context.Context) {
				select {
				case connected <- struct{}{}:
				default:
				}
			},
			OnConnectFailed: func(_ context.Context, err error) {
				t.Logf("  OnConnectFailed: %v", err)
			},
			OnMessage: func(_ context.Context, msg *types.MessageData) {},
		},
	}

	require.NoError(t, c.Start(t.Context(), settings))
	t.Cleanup(func() { c.Stop(t.Context()) })

	select {
	case <-connected:
		return true, requestCount.Load()
	case <-time.After(timeout):
		return false, requestCount.Load()
	}
}

// TestHTTPClientRecoveryAfterNonRetryableError documents that the opamp-go
// HTTP client's polling heartbeat does NOT generate new requests after the
// server returns a non-retryable status code (anything other than 200, 429,
// 503). The client is permanently dead after a single such response.
//
// This is a known upstream bug. The supervisor works around it with a
// connection watchdog (see supervisor.checkConnectionWatchdog).
//
// If this test starts passing after an opamp-go upgrade, the watchdog is
// still harmless but the upstream bug is fixed.
func TestHTTPClientRecoveryAfterNonRetryableError(t *testing.T) {
	t.Run("405_MethodNotAllowed", func(t *testing.T) {
		recovered, n := testHTTPClientRecovery(t, http.StatusMethodNotAllowed, 1, 2*time.Second)
		t.Logf("recovered=%v requests=%d", recovered, n)
		if !recovered {
			t.Skipf("known opamp-go bug: client dead after 405 (%d requests made)", n)
		}
	})

	t.Run("500_InternalServerError", func(t *testing.T) {
		recovered, n := testHTTPClientRecovery(t, http.StatusInternalServerError, 1, 2*time.Second)
		t.Logf("recovered=%v requests=%d", recovered, n)
		if !recovered {
			t.Skipf("known opamp-go bug: client dead after 500 (%d requests made)", n)
		}
	})

	// Control: 503 IS retried by opamp-go, so the client should recover.
	t.Run("503_ServiceUnavailable_recovers", func(t *testing.T) {
		recovered, n := testHTTPClientRecovery(t, http.StatusServiceUnavailable, 2, 5*time.Second)
		t.Logf("recovered=%v requests=%d", recovered, n)
		if !recovered {
			t.Fatalf("client should recover from 503; %d requests made", n)
		}
	})
}
