package sidecar

import (
	"fmt"
	"os"
)

type Config struct {
	Path string `mapstructure:"path"`
}

func (cfg *Config) Validate() error {
	if cfg.Path == "" {
		return fmt.Errorf("config.path is required")
	}

	_, err := os.Stat(cfg.Path)
	if err != nil {
		return fmt.Errorf("provided config path %s does not exist can't be read: %w", cfg.Path, err)
	}

	return nil
}
