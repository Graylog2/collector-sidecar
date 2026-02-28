// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package windows

import (
	"go.opentelemetry.io/collector/component"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator"
)

func init() {
	operator.Register(operatorType, func() operator.Builder { return NewConfig() })
}

// Build will build a windows event log operator.
func (c *Config) Build(set component.TelemetrySettings) (operator.Operator, error) {
	inputOperator, err := c.InputConfig.Build(set)
	if err != nil {
		return nil, err
	}

	if err := validateConfig(c); err != nil {
		return nil, err
	}

	input := &Input{
		InputOperator:            inputOperator,
		buffer:                   NewBuffer(),
		channel:                  c.Channel,
		ignoreChannelErrors:      c.IgnoreChannelErrors,
		maxReads:                 c.MaxReads,
		currentMaxReads:          c.MaxReads,
		startAt:                  c.StartAt,
		pollInterval:             c.PollInterval,
		raw:                      c.Raw,
		includeLogRecordOriginal: c.IncludeLogRecordOriginal,
		excludeProviders:         excludeProvidersSet(c.ExcludeProviders),
		language:                 c.Language,
		resolveSIDs:              c.ResolveSIDs,
		sidCacheSize:             c.SIDCacheSize,
		query:                    c.Query,
	}

	if c.SuppressRenderingInfo {
		input.processEvent = input.processEventWithoutRenderingInfo
	} else {
		input.processEvent = input.processEventWithRenderingInfo
	}

	return input, nil
}

func excludeProvidersSet(providers []string) map[string]struct{} {
	set := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		set[provider] = struct{}{}
	}
	return set
}
