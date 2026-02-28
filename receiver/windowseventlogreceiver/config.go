// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windowseventlogreceiver

import (
	"go.opentelemetry.io/collector/component"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/adapter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator"

	"github.com/Graylog2/collector-sidecar/receiver/windowseventlogreceiver/internal/windows"
)

// WindowsLogConfig defines configuration for the windowseventlog receiver.
type WindowsLogConfig struct {
	windows.Config     `mapstructure:",squash"`
	adapter.BaseConfig `mapstructure:",squash"`
}

func createDefaultConfig() component.Config {
	return &WindowsLogConfig{
		BaseConfig: adapter.BaseConfig{
			Operators: []operator.Config{},
		},
		Config: *windows.NewConfig(),
	}
}
