// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package windowseventlogreceiver

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/receiver"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/adapter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator"
)

func newFactoryAdapter() receiver.Factory {
	return adapter.NewFactory(receiverType{}, component.StabilityLevelAlpha)
}

type receiverType struct{}

func (receiverType) Type() component.Type {
	return component.MustNewType(typeStr)
}

func (receiverType) CreateDefaultConfig() component.Config {
	return createDefaultConfig()
}

func (receiverType) BaseConfig(cfg component.Config) adapter.BaseConfig {
	return cfg.(*WindowsLogConfig).BaseConfig
}

func (receiverType) InputConfig(cfg component.Config) operator.Config {
	return operator.NewConfig(&cfg.(*WindowsLogConfig).Config)
}
