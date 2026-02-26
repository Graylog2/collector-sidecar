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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveEnrollmentEndpoint(t *testing.T) {

	tests := []struct {
		input     string
		output    string
		expectErr bool
	}{
		{"http://example.com", "http://example.com/v1/opamp", false},
		{"https://example.com", "https://example.com/v1/opamp", false},
		{"https://example.com:", "https://example.com/v1/opamp", false},
		{"https://example.com:1234", "https://example.com:1234/v1/opamp", false},
		{"https://example.com/", "https://example.com/v1/opamp", false},
		{"https://example.com/different/opamp", "https://example.com/different/opamp/v1/opamp", false},
		{"https://example.com/graylog", "https://example.com/graylog/v1/opamp", false},
		{"https://example.com/subpath/v1/opamp", "https://example.com/subpath/v1/opamp", false},
		{"wss://example.com", "wss://example.com/v1/opamp", false},
		{"ws://example.com", "ws://example.com/v1/opamp", false},
		{"https://example", "https://example/v1/opamp", false},
		{"https://example/", "https://example/v1/opamp", false},
		{"http://10.0.0.1", "http://10.0.0.1/v1/opamp", false},
		{"localhost", "", true},
		{"127.0.0.1", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			output, err := DeriveEnrollmentEndpoint(tt.input)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.output, output)
		})
	}
}

func TestServerBaseURL(t *testing.T) {
	tests := []struct {
		input     string
		output    string
		expectErr bool
	}{
		{"https://opamp.example.com/", "https://opamp.example.com", false},
		{"https://opamp.example.com/v1/opamp", "https://opamp.example.com", false},
		{"https://opamp.example.com/v1/opamp/", "https://opamp.example.com", false},
		{"https://opamp.example.com:8443/v1/opamp", "https://opamp.example.com:8443", false},
		{"https://opamp.example.com/graylog", "https://opamp.example.com/graylog", false},
		{"https://opamp.example.com/graylog/", "https://opamp.example.com/graylog", false},
		{"https://opamp.example.com/graylog/v1/opamp", "https://opamp.example.com/graylog", false},
		{"wss://opamp.example.com/v1/opamp", "wss://opamp.example.com", false},
		{"", "", true},
	}

	for _, test := range tests {
		url, err := ServerBaseURL(test.input)

		assert.Equal(t, test.output, url, "Unexpected output for %q", test.input)

		if test.expectErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}
