//go:build unix

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigDefaultsDirectories(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "/var/lib/graylog-collector/supervisor", cfg.Persistence.Dir)
	assert.Equal(t, "/var/lib/graylog-collector/storage", cfg.Agent.StorageDir)
	assert.Equal(t, "/var/lib/graylog-collector/keys", cfg.Keys.Dir)
	assert.Equal(t, "/var/lib/graylog-collector/packages", cfg.Packages.StorageDir)
}
