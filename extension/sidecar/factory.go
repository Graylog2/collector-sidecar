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
