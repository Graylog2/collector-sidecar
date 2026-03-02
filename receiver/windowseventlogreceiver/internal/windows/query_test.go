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

package windows

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelListPersistKey(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
	}{
		{
			name: "different order, same key",
			a:    []string{"Application", "Security"},
			b:    []string{"Security", "Application"},
		},
		{
			name: "different casing, same key",
			a:    []string{"Security"},
			b:    []string{"SECURITY"},
		},
		{
			name: "order and casing combined",
			a:    []string{"SYSTEM", "application", "Security"},
			b:    []string{"security", "Application", "system"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, channelListPersistKey(tt.a), channelListPersistKey(tt.b))
		})
	}

	// Verify that different channel sets produce different keys.
	distinct := [][]string{
		{"Security"},
		{"Application"},
		{"Security", "Application"},
		{"Security", "Application", "System"},
	}
	for i := range distinct {
		for j := i + 1; j < len(distinct); j++ {
			assert.NotEqual(t,
				channelListPersistKey(distinct[i]),
				channelListPersistKey(distinct[j]),
				"expected different keys for %v and %v", distinct[i], distinct[j],
			)
		}
	}
}

func TestBuildQueryFromChannels(t *testing.T) {
	tests := []struct {
		name     string
		channels []string
		want     string
	}{
		{
			name:     "single channel",
			channels: []string{"Security"},
			want:     `<QueryList><Query Id="0"><Select Path="Security">*</Select></Query></QueryList>`,
		},
		{
			name:     "multiple channels",
			channels: []string{"Security", "Application"},
			want:     `<QueryList><Query Id="0"><Select Path="Security">*</Select><Select Path="Application">*</Select></Query></QueryList>`,
		},
		{
			name:     "xml-special characters are escaped",
			channels: []string{`Foo&Bar`, `A<B`},
			want:     `<QueryList><Query Id="0"><Select Path="Foo&amp;Bar">*</Select><Select Path="A&lt;B">*</Select></Query></QueryList>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildQueryFromChannels(tt.channels)
			assert.Equal(t, tt.want, got)
		})
	}
}
