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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveLocalEndpoint(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{
			name: "explicit loopback with port",
			addr: "127.0.0.1:54321",
			want: "ws://127.0.0.1:54321/v1/opamp",
		},
		{
			name: "localhost with port",
			addr: "localhost:54321",
			want: "ws://localhost:54321/v1/opamp",
		},
		{
			name: "wildcard IPv4",
			addr: "0.0.0.0:54321",
			want: "ws://127.0.0.1:54321/v1/opamp",
		},
		{
			name: "wildcard IPv6 bracketed",
			addr: "[::]:54321",
			want: "ws://[::1]:54321/v1/opamp",
		},
		{
			name: "explicit IPv6 loopback",
			addr: "[::1]:54321",
			want: "ws://[::1]:54321/v1/opamp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveLocalEndpoint(tt.addr)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveLocalEndpoint_Error(t *testing.T) {
	_, err := resolveLocalEndpoint(":::54321")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot parse")
}
