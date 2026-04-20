//go:build windows

package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigDefaultsDirectories(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, filepath.Join("C:", "ProgramData", "Graylog", "Collector", "supervisor"), cfg.Persistence.Dir)
	assert.Equal(t, filepath.Join("C:", "ProgramData", "Graylog", "Collector", "storage"), cfg.Agent.StorageDir)
	assert.Equal(t, filepath.Join("C:", "ProgramData", "Graylog", "Collector", "keys"), cfg.Keys.Dir)
	assert.Equal(t, filepath.Join("C:", "ProgramData", "Graylog", "Collector", "packages"), cfg.Packages.StorageDir)
}
