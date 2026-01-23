// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opamp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewServer(t *testing.T) {
	logger := zaptest.NewLogger(t)
	callbacks := &ServerCallbacks{}

	server, err := NewServer(logger, ServerConfig{
		ListenEndpoint: "localhost:0",
	}, callbacks)
	require.NoError(t, err)
	require.NotNil(t, server)
}

func TestServer_StartStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	callbacks := &ServerCallbacks{}

	server, err := NewServer(logger, ServerConfig{
		ListenEndpoint: "localhost:0",
	}, callbacks)
	require.NoError(t, err)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	addr := server.Addr()
	require.NotEmpty(t, addr)

	err = server.Stop(ctx)
	require.NoError(t, err)
}

func TestServerConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       ServerConfig
		expectErr bool
	}{
		{
			name: "valid",
			cfg: ServerConfig{
				ListenEndpoint: "localhost:4320",
			},
			expectErr: false,
		},
		{
			name:      "empty endpoint",
			cfg:       ServerConfig{},
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
