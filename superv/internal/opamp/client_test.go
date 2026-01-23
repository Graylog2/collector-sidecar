// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opamp

import (
	"context"
	"errors"
	"testing"

	"github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewClient(t *testing.T) {
	logger := zaptest.NewLogger(t)
	callbacks := &Callbacks{}

	client, err := NewClient(logger, ClientConfig{
		Endpoint:    "ws://localhost:4320/v1/opamp",
		InstanceUID: "test-instance-uid",
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
				InstanceUID: "test-uid",
			},
			expectErr: false,
		},
		{
			name: "valid wss",
			cfg: ClientConfig{
				Endpoint:    "wss://opamp.example.com/v1/opamp",
				InstanceUID: "test-uid",
			},
			expectErr: false,
		},
		{
			name: "missing endpoint",
			cfg: ClientConfig{
				InstanceUID: "test-uid",
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
		ReportsStatus:          true,
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
		ReportsStatus:                  true,
		AcceptsRemoteConfig:            true,
		ReportsEffectiveConfig:         true,
		AcceptsPackages:                true,
		ReportsPackageStatuses:         true,
		ReportsOwnTraces:               true,
		ReportsOwnMetrics:              true,
		ReportsOwnLogs:                 true,
		AcceptsOpAMPConnectionSettings: true,
		AcceptsRestartCommand:          true,
		ReportsHealth:                  true,
		ReportsRemoteConfig:            true,
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
}

func TestCapabilitiesToProto_NoCapabilities(t *testing.T) {
	caps := Capabilities{}
	proto := caps.ToProto()
	require.Equal(t, protobufs.AgentCapabilities(0), proto)
}

func TestNewClient_ValidationFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)
	callbacks := &Callbacks{}

	// Missing endpoint
	client, err := NewClient(logger, ClientConfig{
		InstanceUID: "test-uid",
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
	require.Contains(t, err.Error(), "instance UID is required")
}

func TestClient_MethodsWithoutStart(t *testing.T) {
	logger := zaptest.NewLogger(t)
	callbacks := &Callbacks{}

	client, err := NewClient(logger, ClientConfig{
		Endpoint:    "ws://localhost:4320/v1/opamp",
		InstanceUID: "test-uid",
	}, callbacks)
	require.NoError(t, err)

	// SetAgentDescription can be called before Start (stores for later)
	err = client.SetAgentDescription(&protobufs.AgentDescription{})
	require.NoError(t, err)

	// SetHealth can be called before Start (stores for later)
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
	ctx := context.Background()

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
	ctx := context.Background()

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
	config, err := typesCallbacks.GetEffectiveConfig(context.Background())
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
	typesCallbacks.SaveRemoteConfigStatus(context.Background(), status)
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
	callbacks.onMessage(context.Background(), &types.MessageData{
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

	err := typesCallbacks.OnOpampConnectionSettings(context.Background(), settings)
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
	err := typesCallbacks.OnOpampConnectionSettings(context.Background(), &protobufs.OpAMPConnectionSettings{})
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
	tc.OnMessage(context.Background(), &types.MessageData{
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
	err := tc.OnCommand(context.Background(), &protobufs.ServerToAgentCommand{})
	require.NoError(t, err)
	require.True(t, commandCalled)
}

func TestParseInstanceUID_ValidUUID(t *testing.T) {
	// Standard UUID format
	uid := "550e8400-e29b-41d4-a716-446655440000"
	result := parseInstanceUID(uid)

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

func TestParseInstanceUID_InvalidUUID(t *testing.T) {
	// Non-UUID string should fallback to copying bytes
	uid := "not-a-valid-uuid"
	result := parseInstanceUID(uid)

	// Should copy raw bytes as fallback
	require.Len(t, result, 16)
	require.Equal(t, byte('n'), result[0])
	require.Equal(t, byte('o'), result[1])
}
