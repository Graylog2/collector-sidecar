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

package sidecar

import (
	"context"

	"github.com/Graylog2/collector-sidecar/extension/sidecar/logger/hooks"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

func NewFactory() extension.Factory {
	return extension.NewFactory(
		component.MustNewType("sidecar"),
		createConfig,
		createExtension,
		component.StabilityLevelStable,
	)
}

func createConfig() component.Config {
	return &Config{}
}

func createExtension(_ context.Context, settings extension.Settings, cfg component.Config) (extension.Extension, error) {
	extensionConfig := cfg.(*Config)
	hooks.AddZapHook(log, settings.Logger) // Add zap logger as early as possible
	return &sidecarExtension{config: extensionConfig, logger: settings.Logger}, nil
}
