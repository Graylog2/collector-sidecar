// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import (
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator/helper"
)

const operatorType = "windows_eventlog_input"

// NewConfig will return an event log config with default values.
func NewConfig() *Config {
	return NewConfigWithID(operatorType)
}

// NewConfigWithID will return an event log config with default values.
func NewConfigWithID(operatorID string) *Config {
	return &Config{
		InputConfig:         helper.NewInputConfig(operatorID, operatorType),
		MaxReads:            100,
		StartAt:             "end",
		PollInterval:        1 * time.Second,
		IgnoreChannelErrors: false,
		ResolveSIDs:         true,
		SIDCacheSize:        1024,
	}
}

// Config is the configuration of a windows event log operator.
type Config struct {
	helper.InputConfig       `mapstructure:",squash"`
	Channel                  string        `mapstructure:"channel"`
	IgnoreChannelErrors      bool          `mapstructure:"ignore_channel_errors,omitempty"`
	MaxReads                 int           `mapstructure:"max_reads,omitempty"`
	StartAt                  string        `mapstructure:"start_at,omitempty"`
	PollInterval             time.Duration `mapstructure:"poll_interval,omitempty"`
	Raw                      bool          `mapstructure:"raw,omitempty"`
	IncludeLogRecordOriginal bool          `mapstructure:"include_log_record_original,omitempty"`
	SuppressRenderingInfo    bool          `mapstructure:"suppress_rendering_info,omitempty"`
	ExcludeProviders         []string      `mapstructure:"exclude_providers,omitempty"`
	Query                    *string       `mapstructure:"query,omitempty"`
	ResolveSIDs              bool          `mapstructure:"resolve_sids,omitempty"`
	SIDCacheSize             int           `mapstructure:"sid_cache_size,omitempty"`
	Language                 uint32        `mapstructure:"language,omitempty"`
}
