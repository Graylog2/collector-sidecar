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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/healthmonitor"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSupervisor_NonIdentifyingAttributes_WithCollectorVersion(t *testing.T) {
	s := &Supervisor{
		collectorVersion: "2.0.0-alpha.0",
	}

	attrs := s.nonIdentifyingAttributes("test-host")

	attrMap := make(map[string]string)
	for _, kv := range attrs {
		attrMap[kv.Key] = kv.Value.GetStringValue()
	}

	require.Equal(t, "test-host", attrMap["host.name"])
	require.NotEmpty(t, attrMap["service.version"])
	require.NotEmpty(t, attrMap["os.type"])
	require.NotEmpty(t, attrMap["host.arch"])
	require.Equal(t, "2.0.0-alpha.0", attrMap["collector.version"])
}

func TestSupervisor_NonIdentifyingAttributes_WithoutCollectorVersion(t *testing.T) {
	s := &Supervisor{}

	attrs := s.nonIdentifyingAttributes("test-host")

	attrMap := make(map[string]string)
	for _, kv := range attrs {
		attrMap[kv.Key] = kv.Value.GetStringValue()
	}

	require.Equal(t, "test-host", attrMap["host.name"])
	require.NotEmpty(t, attrMap["service.version"])
	_, hasCollectorVersion := attrMap["collector.version"]
	require.False(t, hasCollectorVersion, "collector.version should not be present when empty")
}

func TestSupervisor_InitialComponentHealth_DefaultHealthyWithoutMonitor(t *testing.T) {
	supervisor := &Supervisor{}

	health := supervisor.initialComponentHealth()
	require.True(t, health.Healthy)
	require.Empty(t, health.LastError)
}

func TestSupervisor_InitialComponentHealth_DefaultHealthyWithoutSample(t *testing.T) {
	monitor := healthmonitor.New(zap.NewNop(), healthmonitor.Config{
		Endpoint: "http://localhost:13133/health",
		Timeout:  time.Second,
		Interval: time.Second,
	})
	supervisor := &Supervisor{healthMonitor: monitor}

	health := supervisor.initialComponentHealth()
	require.True(t, health.Healthy)
	require.Empty(t, health.LastError)
}

func TestSupervisor_InitialComponentHealth_UsesLatestMonitorSample(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	monitor := healthmonitor.New(zap.NewNop(), healthmonitor.Config{
		Endpoint: server.URL,
		Timeout:  time.Second,
		Interval: time.Second,
	})

	_, err := monitor.CheckHealth(context.Background())
	require.NoError(t, err)

	supervisor := &Supervisor{healthMonitor: monitor}
	health := supervisor.initialComponentHealth()

	require.False(t, health.Healthy)
	require.Equal(t, "Service Unavailable", health.LastError)
}
