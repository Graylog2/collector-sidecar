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

package opamp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/protobuf/proto"
)

func TestNewClient(t *testing.T) {
	logger := zaptest.NewLogger(t)
	callbacks := &Callbacks{}

	client, err := NewClient(logger, ClientConfig{
		Endpoint:    "ws://localhost:4320/v1/opamp",
		InstanceUID: "550e8400-e29b-41d4-a716-446655440000",
	}, callbacks)
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestClientConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       ClientConfig
		expectErr bool
	}{
		{
			name: "valid websocket",
			cfg: ClientConfig{
				Endpoint:    "ws://localhost:4320/v1/opamp",
				InstanceUID: "550e8400-e29b-41d4-a716-446655440000",
			},
			expectErr: false,
		},
		{
			name: "valid wss",
			cfg: ClientConfig{
				Endpoint:    "wss://opamp.example.com/v1/opamp",
				InstanceUID: "550e8400-e29b-41d4-a716-446655440000",
			},
			expectErr: false,
		},
		{
			name: "missing endpoint",
			cfg: ClientConfig{
				InstanceUID: "550e8400-e29b-41d4-a716-446655440000",
			},
			expectErr: true,
		},
		{
			name: "missing instance UID",
			cfg: ClientConfig{
				Endpoint: "ws://localhost:4320/v1/opamp",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCapabilitiesToProto(t *testing.T) {
	caps := Capabilities{
		AcceptsRemoteConfig:    true,
		ReportsEffectiveConfig: true,
		ReportsHealth:          true,
	}

	proto := caps.ToProto()
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_AcceptsRemoteConfig != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsEffectiveConfig != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsHealth != 0)
}

func TestCapabilitiesToProto_AllCapabilities(t *testing.T) {
	caps := Capabilities{
		AcceptsRemoteConfig:             true,
		ReportsEffectiveConfig:          true,
		AcceptsPackages:                 true,
		ReportsPackageStatuses:          true,
		ReportsOwnTraces:                true,
		ReportsOwnMetrics:               true,
		ReportsOwnLogs:                  true,
		AcceptsOpAMPConnectionSettings:  true,
		AcceptsRestartCommand: true,
		ReportsHealth:                   true,
		ReportsRemoteConfig:             true,
		ReportsHeartbeat:                true,
		ReportsAvailableComponents:      true,
	}

	proto := caps.ToProto()
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_AcceptsRemoteConfig != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsEffectiveConfig != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_AcceptsPackages != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsPackageStatuses != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsOwnTraces != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsOwnMetrics != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsOwnLogs != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_AcceptsOpAMPConnectionSettings != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_AcceptsRestartCommand != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsHealth != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsRemoteConfig != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsHeartbeat != 0)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsAvailableComponents != 0)
}

func TestCapabilitiesToProto_NoCapabilities(t *testing.T) {
	caps := Capabilities{}
	proto := caps.ToProto()
	// ReportsStatus is always set per OpAMP spec, even with empty capabilities
	require.Equal(t, protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus, proto)
}

func TestCapabilitiesToProto_AlwaysIncludesReportsStatus(t *testing.T) {
	// Empty capabilities should still have ReportsStatus
	caps := Capabilities{}
	proto := caps.ToProto()

	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus != 0,
		"ReportsStatus must always be set")
}

func TestCapabilitiesToProto_ReportsStatusAlwaysSet(t *testing.T) {
	// Even with other capabilities, ReportsStatus should be present
	caps := Capabilities{
		AcceptsRemoteConfig: true,
		ReportsHealth:       true,
	}
	proto := caps.ToProto()

	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus != 0,
		"ReportsStatus must always be set regardless of other capabilities")
}

func TestNewClient_ValidationFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)
	callbacks := &Callbacks{}

	// Missing endpoint
	client, err := NewClient(logger, ClientConfig{
		InstanceUID: "550e8400-e29b-41d4-a716-446655440000",
	}, callbacks)
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "endpoint is required")

	// Missing instance UID
	client, err = NewClient(logger, ClientConfig{
		Endpoint: "ws://localhost:4320/v1/opamp",
	}, callbacks)
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "instance_uid is required")
}

func TestClient_MethodsWithoutStart(t *testing.T) {
	logger := zaptest.NewLogger(t)
	callbacks := &Callbacks{}

	client, err := NewClient(logger, ClientConfig{
		Endpoint:    "ws://localhost:4320/v1/opamp",
		InstanceUID: "550e8400-e29b-41d4-a716-446655440000",
	}, callbacks)
	require.NoError(t, err)

	// SetAgentDescription can be called before Start — opamp-go stores it internally
	err = client.SetAgentDescription(&protobufs.AgentDescription{
		IdentifyingAttributes: []*protobufs.KeyValue{
			{Key: "service.name", Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: "test"}}},
		},
	})
	require.NoError(t, err)

	// SetHealth can be called before Start — opamp-go stores it internally
	err = client.SetHealth(&protobufs.ComponentHealth{})
	require.NoError(t, err)

	err = client.UpdateEffectiveConfig(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "client not started")

	err = client.SetRemoteConfigStatus(&protobufs.RemoteConfigStatus{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "client not started")

	// Stop should not error when not started
	err = client.Stop(t.Context())
	require.NoError(t, err)
}

func TestClient_SetEffectiveConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	callbacks := &Callbacks{}

	client, err := NewClient(logger, ClientConfig{
		Endpoint:    "ws://localhost:4320/v1/opamp",
		InstanceUID: "550e8400-e29b-41d4-a716-446655440000",
	}, callbacks)
	require.NoError(t, err)

	// Before start, should store for later
	config := map[string]*protobufs.AgentConfigFile{
		"collector.yaml": {
			Body: []byte("test: config"),
		},
	}
	err = client.SetEffectiveConfig(t.Context(), config)
	require.NoError(t, err)

	// Verify the config was stored internally
	require.NotNil(t, client.effectiveConfig)
	require.NotNil(t, client.effectiveConfig.ConfigMap)
	require.NotNil(t, client.effectiveConfig.ConfigMap.ConfigMap)
	require.Contains(t, client.effectiveConfig.ConfigMap.ConfigMap, "collector.yaml")
	require.Equal(t, []byte("test: config"), client.effectiveConfig.ConfigMap.ConfigMap["collector.yaml"].Body)
}

func TestClient_SetEffectiveConfig_RespectsContext(t *testing.T) {
	// Create a cancelled context
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	logger := zaptest.NewLogger(t)
	c, err := NewClient(logger, ClientConfig{
		Endpoint:    "ws://localhost:4320/v1/opamp",
		InstanceUID: "550e8400-e29b-41d4-a716-446655440000",
	}, &Callbacks{})
	require.NoError(t, err)

	config := map[string]*protobufs.AgentConfigFile{
		"test.yaml": {Body: []byte("test: config")},
	}

	// Before Start(), stores config but doesn't call UpdateEffectiveConfig
	err = c.SetEffectiveConfig(ctx, config)
	require.NoError(t, err)
}

func TestCallbacks_ToTypesCallbacks(t *testing.T) {
	var connectCalled bool
	var connectFailedErr error
	var errorReceived *protobufs.ServerErrorResponse
	var commandReceived *protobufs.ServerToAgentCommand

	callbacks := &Callbacks{
		OnConnect: func(ctx context.Context) {
			connectCalled = true
		},
		OnConnectFailed: func(ctx context.Context, err error) {
			connectFailedErr = err
		},
		OnError: func(ctx context.Context, err *protobufs.ServerErrorResponse) {
			errorReceived = err
		},
		OnCommand: func(ctx context.Context, command *protobufs.ServerToAgentCommand) error {
			commandReceived = command
			return nil
		},
	}

	typesCallbacks := callbacks.ToTypesCallbacks()
	ctx := t.Context()

	// Test OnConnect
	typesCallbacks.OnConnect(ctx)
	require.True(t, connectCalled)

	// Test OnConnectFailed
	testErr := errors.New("connection failed")
	typesCallbacks.OnConnectFailed(ctx, testErr)
	require.Equal(t, testErr, connectFailedErr)

	// Test OnError
	serverErr := &protobufs.ServerErrorResponse{ErrorMessage: "test error"}
	typesCallbacks.OnError(ctx, serverErr)
	require.Equal(t, serverErr, errorReceived)

	// Test OnCommand
	cmd := &protobufs.ServerToAgentCommand{}
	err := typesCallbacks.OnCommand(ctx, cmd)
	require.NoError(t, err)
	require.Equal(t, cmd, commandReceived)

	// Test GetEffectiveConfig (nil handler should return nil, nil)
	config, err := typesCallbacks.GetEffectiveConfig(ctx)
	require.NoError(t, err)
	require.Nil(t, config)
}

func TestCallbacks_NilHandlers(t *testing.T) {
	// Test that nil handlers don't panic
	callbacks := &Callbacks{}
	typesCallbacks := callbacks.ToTypesCallbacks()
	ctx := t.Context()

	// These should not panic
	typesCallbacks.OnConnect(ctx)
	typesCallbacks.OnConnectFailed(ctx, errors.New("test"))
	typesCallbacks.OnError(ctx, &protobufs.ServerErrorResponse{})
	typesCallbacks.SaveRemoteConfigStatus(ctx, &protobufs.RemoteConfigStatus{})

	err := typesCallbacks.OnOpampConnectionSettings(ctx, &protobufs.OpAMPConnectionSettings{})
	require.NoError(t, err)

	err = typesCallbacks.OnCommand(ctx, &protobufs.ServerToAgentCommand{})
	require.NoError(t, err)

	config, err := typesCallbacks.GetEffectiveConfig(ctx)
	require.NoError(t, err)
	require.Nil(t, config)
}

func TestCallbacks_GetEffectiveConfig(t *testing.T) {
	expectedConfig := &protobufs.EffectiveConfig{
		ConfigMap: &protobufs.AgentConfigMap{},
	}

	callbacks := &Callbacks{
		GetEffectiveConfig: func(ctx context.Context) (*protobufs.EffectiveConfig, error) {
			return expectedConfig, nil
		},
	}

	typesCallbacks := callbacks.ToTypesCallbacks()
	config, err := typesCallbacks.GetEffectiveConfig(t.Context())
	require.NoError(t, err)
	require.Equal(t, expectedConfig, config)
}

func TestCallbacks_SaveRemoteConfigStatus(t *testing.T) {
	var savedStatus *protobufs.RemoteConfigStatus

	callbacks := &Callbacks{
		SaveRemoteConfigStatus: func(ctx context.Context, status *protobufs.RemoteConfigStatus) {
			savedStatus = status
		},
	}

	typesCallbacks := callbacks.ToTypesCallbacks()
	status := &protobufs.RemoteConfigStatus{
		LastRemoteConfigHash: []byte("test-hash"),
	}
	typesCallbacks.SaveRemoteConfigStatus(t.Context(), status)
	require.Equal(t, status, savedStatus)
}

func TestCallbacks_OnMessage_RemoteConfig(t *testing.T) {
	var remoteConfigCalled bool

	callbacks := &Callbacks{
		OnRemoteConfig: func(ctx context.Context, config *protobufs.AgentRemoteConfig) bool {
			remoteConfigCalled = true
			return true
		},
	}

	typesCallbacks := callbacks.ToTypesCallbacks()

	// Verify the OnMessage callback is wired up correctly
	require.NotNil(t, typesCallbacks.OnMessage)

	// Call OnMessage directly with a MessageData containing remote config
	// This tests the internal routing in onMessage
	callbacks.onMessage(t.Context(), &types.MessageData{
		RemoteConfig: &protobufs.AgentRemoteConfig{
			ConfigHash: []byte("test-hash"),
		},
	})
	require.True(t, remoteConfigCalled)
}

func TestCallbacks_OnOpampConnectionSettings(t *testing.T) {
	var receivedSettings *protobufs.OpAMPConnectionSettings

	callbacks := &Callbacks{
		OnOpampConnectionSettings: func(ctx context.Context, settings *protobufs.OpAMPConnectionSettings) error {
			receivedSettings = settings
			return nil
		},
	}

	typesCallbacks := callbacks.ToTypesCallbacks()
	settings := &protobufs.OpAMPConnectionSettings{
		DestinationEndpoint: "ws://new-server:4320",
	}

	err := typesCallbacks.OnOpampConnectionSettings(t.Context(), settings)
	require.NoError(t, err)
	require.Equal(t, settings, receivedSettings)
}

func TestCallbacks_OnOpampConnectionSettings_Error(t *testing.T) {
	expectedErr := errors.New("connection settings rejected")

	callbacks := &Callbacks{
		OnOpampConnectionSettings: func(ctx context.Context, settings *protobufs.OpAMPConnectionSettings) error {
			return expectedErr
		},
	}

	typesCallbacks := callbacks.ToTypesCallbacks()
	err := typesCallbacks.OnOpampConnectionSettings(t.Context(), &protobufs.OpAMPConnectionSettings{})
	require.Equal(t, expectedErr, err)
}

func TestCallbacks_OnMessage_Packages(t *testing.T) {
	var packagesCalled bool
	callbacks := &Callbacks{
		OnPackagesAvailable: func(ctx context.Context, packages *protobufs.PackagesAvailable) bool {
			packagesCalled = true
			return true
		},
	}
	tc := callbacks.ToTypesCallbacks()
	tc.OnMessage(t.Context(), &types.MessageData{
		PackagesAvailable: &protobufs.PackagesAvailable{},
	})
	require.True(t, packagesCalled)
}

func TestCallbacks_OnMessage_Command(t *testing.T) {
	var commandCalled bool
	callbacks := &Callbacks{
		OnCommand: func(ctx context.Context, command *protobufs.ServerToAgentCommand) error {
			commandCalled = true
			return nil
		},
	}
	tc := callbacks.ToTypesCallbacks()
	err := tc.OnCommand(t.Context(), &protobufs.ServerToAgentCommand{})
	require.NoError(t, err)
	require.True(t, commandCalled)
}

func TestCallbacks_OnMessage_CustomMessage(t *testing.T) {
	var customMessageCalled bool
	var receivedMessage *protobufs.CustomMessage
	callbacks := &Callbacks{
		OnCustomMessage: func(ctx context.Context, customMessage *protobufs.CustomMessage) {
			customMessageCalled = true
			receivedMessage = customMessage
		},
	}
	tc := callbacks.ToTypesCallbacks()
	expectedMessage := &protobufs.CustomMessage{
		Capability: "test.capability",
		Type:       "test.type",
		Data:       []byte("test data"),
	}
	tc.OnMessage(t.Context(), &types.MessageData{
		CustomMessage: expectedMessage,
	})
	require.True(t, customMessageCalled)
	require.Equal(t, expectedMessage, receivedMessage)
}

func TestParseInstanceUID_ValidUUID(t *testing.T) {
	// Standard UUID format
	uid := "550e8400-e29b-41d4-a716-446655440000"
	result, err := parseInstanceUID(uid)
	require.NoError(t, err)

	// The result should be 16 bytes (binary UUID representation)
	require.Len(t, result, 16)

	// Verify the bytes match the expected UUID binary format
	// 550e8400-e29b-41d4-a716-446655440000 in binary
	expected := types.InstanceUid{
		0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4,
		0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00,
	}
	require.Equal(t, expected, result)
}

func TestParseInstanceUID_RejectsInvalidUUID(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"short string", "abc"},
		{"invalid format", "not-a-uuid-at-all"},
		{"wrong length hex", "0123456789abcdef"}, // 16 chars but not valid UUID
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseInstanceUID(tt.input)
			require.Error(t, err, "should reject invalid UUID: %s", tt.input)
		})
	}
}

func TestParseInstanceUID_AcceptsValidUUID(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"UUID v4", "550e8400-e29b-41d4-a716-446655440000"},
		{"UUID v7", "01902a9e-8b3c-7def-8a12-123456789abc"},
		{"uppercase", "550E8400-E29B-41D4-A716-446655440000"},
		{"no dashes", "550e8400e29b41d4a716446655440000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uid, err := parseInstanceUID(tt.input)
			require.NoError(t, err)
			require.Len(t, uid, 16, "UID must be exactly 16 bytes")
		})
	}
}

func TestClientConfig_Validate_RejectsInvalidInstanceUID(t *testing.T) {
	cfg := ClientConfig{
		Endpoint:    "ws://localhost:4320/v1/opamp",
		InstanceUID: "not-a-valid-uuid",
	}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "instance_uid")
}

func TestCapabilitiesToProto_ReportsHeartbeat(t *testing.T) {
	caps := Capabilities{
		ReportsHeartbeat: true,
	}
	proto := caps.ToProto()

	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsHeartbeat != 0,
		"ReportsHeartbeat capability should be set")
}

func TestClient_HeaderFunc_InvokedPerHTTPRequest(t *testing.T) {
	// Track how many times HeaderFunc is called.
	var callCount atomic.Int32

	// HTTP test server returns a minimal valid ServerToAgent protobuf response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Dynamic-Token") != "" {
			callCount.Add(1)
		}

		resp := &protobufs.ServerToAgent{}
		data, _ := proto.Marshal(resp)
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Write(data)
	}))
	defer srv.Close()

	logger := zaptest.NewLogger(t)
	client, err := NewClient(logger, ClientConfig{
		Endpoint:    srv.URL,
		InstanceUID: "550e8400-e29b-41d4-a716-446655440000",
		HeaderFunc: func(h http.Header) http.Header {
			h.Set("X-Dynamic-Token", "fresh-value")
			return h
		},
		HeartbeatInterval: 100 * time.Millisecond, // Fast polling for test
		Capabilities: Capabilities{
			ReportsHeartbeat: true,
		},
	}, &Callbacks{})
	require.NoError(t, err)

	err = client.SetAgentDescription(&protobufs.AgentDescription{
		IdentifyingAttributes: []*protobufs.KeyValue{
			{Key: "service.name", Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: "test"}}},
		},
	})
	require.NoError(t, err)

	err = client.Start(t.Context())
	require.NoError(t, err)

	// Wait for at least two poll cycles to prove HeaderFunc is called per request.
	require.Eventually(t, func() bool {
		return callCount.Load() >= 2
	}, 5*time.Second, 50*time.Millisecond,
		"HeaderFunc should be invoked on each HTTP poll request")

	require.NoError(t, client.Stop(t.Context()))
}

func TestClient_HeartbeatInterval_FromConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := ClientConfig{
		Endpoint:          "ws://localhost:4320/v1/opamp",
		InstanceUID:       "550e8400-e29b-41d4-a716-446655440000",
		HeartbeatInterval: 45 * time.Second,
		Capabilities: Capabilities{
			ReportsHeartbeat: true,
		},
	}

	client, err := NewClient(logger, cfg, nil)
	require.NoError(t, err)
	require.Equal(t, 45*time.Second, client.cfg.HeartbeatInterval)
}

func TestCapabilitiesToProto_ReportsAvailableComponents(t *testing.T) {
	caps := Capabilities{
		ReportsAvailableComponents: true,
	}

	proto := caps.ToProto()

	// ReportsStatus is always included (mandatory)
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus != 0,
		"ReportsStatus should always be set")
	require.True(t, proto&protobufs.AgentCapabilities_AgentCapabilities_ReportsAvailableComponents != 0,
		"ReportsAvailableComponents should be set")
}

func TestClient_SetAvailableComponents_BeforeStart(t *testing.T) {
	logger := zaptest.NewLogger(t)

	client, err := NewClient(logger, ClientConfig{
		Endpoint:    "ws://localhost:4320/v1/opamp",
		InstanceUID: "550e8400-e29b-41d4-a716-446655440000",
	}, &Callbacks{})
	require.NoError(t, err)

	// Set components before start should not error — opamp-go stores them internally
	components := &protobufs.AvailableComponents{
		Components: map[string]*protobufs.ComponentDetails{
			"receiver/otlp": {},
		},
		Hash: []byte("test-hash"),
	}
	err = client.SetAvailableComponents(components)
	require.NoError(t, err)
}
