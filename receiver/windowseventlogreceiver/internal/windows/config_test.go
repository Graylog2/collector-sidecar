// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateQueryXML_Valid(t *testing.T) {
	q := `<QueryList><Query><Select Path="Security">*</Select></Query></QueryList>`
	err := validateQueryXML(q)
	require.NoError(t, err)
}

func TestValidateQueryXML_Malformed(t *testing.T) {
	q := `<QueryList><Query><Select Path="Security">*</Select></Query>` // missing closing tag
	err := validateQueryXML(q)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid xml_query")
}

func TestValidateQueryXML_Empty(t *testing.T) {
	err := validateQueryXML("")
	require.Error(t, err) // empty string is invalid XML
}

func TestValidateConfig(t *testing.T) {
	validQuery := `<QueryList><Query><Select Path="Security">*</Select></Query></QueryList>`
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "valid channel",
			cfg:  Config{Channel: "Application", MaxReads: 100, StartAt: "end"},
		},
		{
			name: "valid query",
			cfg:  Config{Query: &validQuery, MaxReads: 100, StartAt: "end"},
		},
		{
			name:    "neither channel nor query",
			cfg:     Config{MaxReads: 100, StartAt: "end"},
			wantErr: "either `channel`, `channel_list`, or `query` must be set",
		},
		{
			name:    "both channel and query",
			cfg:     Config{Channel: "Application", Query: &validQuery, MaxReads: 100, StartAt: "end"},
			wantErr: "only one of `channel`, `channel_list`, or `query` may be set",
		},
		{
			name:    "max_reads zero",
			cfg:     Config{Channel: "Application", MaxReads: 0, StartAt: "end"},
			wantErr: "the `max_reads` field must be greater than zero",
		},
		{
			name:    "invalid start_at",
			cfg:     Config{Channel: "Application", MaxReads: 100, StartAt: "middle"},
			wantErr: "the `start_at` field must be set to `beginning` or `end`",
		},
		{
			name: "start_at beginning",
			cfg:  Config{Channel: "Application", MaxReads: 100, StartAt: "beginning"},
		},
		{
			name:    "empty query string",
			cfg:     Config{Query: new(""), MaxReads: 100, StartAt: "end"},
			wantErr: "the `query` field must not be empty when set",
		},
		{
			name:    "negative sid_cache_size",
			cfg:     Config{Channel: "Application", MaxReads: 100, StartAt: "end", SIDCacheSize: -1},
			wantErr: "the `sid_cache_size` field must not be negative",
		},
		{
			name: "valid channel_list",
			cfg:  Config{ChannelList: []string{"Security", "Application"}, MaxReads: 100, StartAt: "end"},
		},
		{
			name:    "channel and channel_list",
			cfg:     Config{Channel: "Security", ChannelList: []string{"Application"}, MaxReads: 100, StartAt: "end"},
			wantErr: "only one of `channel`, `channel_list`, or `query` may be set",
		},
		{
			name:    "channel_list and query",
			cfg:     Config{ChannelList: []string{"Security"}, Query: &validQuery, MaxReads: 100, StartAt: "end"},
			wantErr: "only one of `channel`, `channel_list`, or `query` may be set",
		},
		{
			name:    "empty channel_list",
			cfg:     Config{ChannelList: []string{}, MaxReads: 100, StartAt: "end"},
			wantErr: "either `channel`, `channel_list`, or `query` must be set",
		},
		{
			name:    "whitespace-only channel_list rejected after canonicalization",
			cfg:     Config{ChannelList: []string{"   ", "", "\t"}, MaxReads: 100, StartAt: "end"},
			wantErr: "either `channel`, `channel_list`, or `query` must be set",
		},
		{
			name: "channel_list with duplicates canonicalized",
			cfg:  Config{ChannelList: []string{"Security", "security", "SECURITY"}, MaxReads: 100, StartAt: "end"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(&tt.cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

//go:fix inline
func ptrString(s string) *string { return new(s) }

func TestNewConfig_Defaults(t *testing.T) {
	cfg := NewConfig()
	require.Equal(t, 100, cfg.MaxReads)
	require.Equal(t, "end", cfg.StartAt)
	require.True(t, cfg.ResolveSIDs)
	require.Equal(t, 1024, cfg.SIDCacheSize)
	require.Equal(t, "", cfg.Channel)
	require.Nil(t, cfg.Query)
	require.Nil(t, cfg.ChannelList)
	require.False(t, cfg.Raw)
}
