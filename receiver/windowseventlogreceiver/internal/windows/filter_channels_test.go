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

// filter_channels_test.go
package windows

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanonicalizeChannelList(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "no changes needed",
			in:   []string{"Security", "Application"},
			want: []string{"Security", "Application"},
		},
		{
			name: "trims whitespace",
			in:   []string{"  Security ", "\tApplication\n"},
			want: []string{"Security", "Application"},
		},
		{
			name: "removes empty entries",
			in:   []string{"Security", "", "  ", "Application"},
			want: []string{"Security", "Application"},
		},
		{
			name: "deduplicates case-insensitive, first wins",
			in:   []string{"Security", "SECURITY", "security"},
			want: []string{"Security"},
		},
		{
			name: "combined",
			in:   []string{"  Security", "Application", "security ", "", "SYSTEM", "system"},
			want: []string{"Security", "Application", "SYSTEM"},
		},
		{
			name: "nil input returns nil",
			in:   nil,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalizeChannelList(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFilterChannels(t *testing.T) {
	available := map[string]struct{}{
		"security":    {},
		"application": {},
		"system":      {},
	}
	tests := []struct {
		name    string
		wanted  []string
		want    []string
		skipped []string
	}{
		{
			name:    "all exist",
			wanted:  []string{"Security", "Application"},
			want:    []string{"Security", "Application"},
			skipped: nil,
		},
		{
			name:    "some missing",
			wanted:  []string{"Security", "Microsoft-Windows-Sysmon/Operational"},
			want:    []string{"Security"},
			skipped: []string{"Microsoft-Windows-Sysmon/Operational"},
		},
		{
			name:    "none exist",
			wanted:  []string{"Nonexistent"},
			want:    nil,
			skipped: []string{"Nonexistent"},
		},
		{
			name:    "case insensitive match",
			wanted:  []string{"SECURITY", "application"},
			want:    []string{"SECURITY", "application"},
			skipped: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, skipped := filterChannels(tt.wanted, available)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.skipped, skipped)
		})
	}
}

func TestApplyChannelFilter_ListFails(t *testing.T) {
	wanted := []string{"Security"}
	listErr := errors.New("RPC unavailable")

	_, _, err := applyChannelFilter(wanted, func() ([]string, error) {
		return nil, listErr
	})
	assert.ErrorIs(t, err, listErr)
}

func TestApplyChannelFilter_AllMatch(t *testing.T) {
	wanted := []string{"Security", "Application"}

	filtered, skipped, err := applyChannelFilter(wanted, func() ([]string, error) {
		return []string{"Security", "Application", "System"}, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, []string{"Security", "Application"}, filtered)
	assert.Nil(t, skipped)
}

func TestApplyChannelFilter_NoneMatch(t *testing.T) {
	wanted := []string{"Nonexistent"}

	filtered, skipped, err := applyChannelFilter(wanted, func() ([]string, error) {
		return []string{"Security"}, nil
	})
	assert.NoError(t, err)
	assert.Nil(t, filtered)
	assert.Equal(t, []string{"Nonexistent"}, skipped)
}

func TestApplyChannelFilter_PartialMatch(t *testing.T) {
	wanted := []string{"Security", "Microsoft-Windows-Sysmon/Operational"}

	filtered, skipped, err := applyChannelFilter(wanted, func() ([]string, error) {
		return []string{"Security", "Application"}, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, []string{"Security"}, filtered)
	assert.Equal(t, []string{"Microsoft-Windows-Sysmon/Operational"}, skipped)
}
