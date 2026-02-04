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

// Note: Testing lock behavior (that Broadcast doesn't hold the lock while sending)
// is impractical in unit tests as it requires simulating slow network connections.
// The implementation uses a snapshot pattern (copy connections under lock, then
// iterate without lock) which is verified by code review rather than testing.
