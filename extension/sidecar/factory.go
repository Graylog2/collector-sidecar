package sidecar

import (
	"context"

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
	return &sidecarExtension{config: extensionConfig, logger: settings.Logger}, nil
}
