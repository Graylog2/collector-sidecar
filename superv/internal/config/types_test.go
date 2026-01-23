// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigStructExists(t *testing.T) {
	cfg := Config{}
	require.NotNil(t, &cfg)
}

func TestServerConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	require.NotEmpty(t, cfg.Server.Endpoint)
}

func TestAgentConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	require.Greater(t, cfg.Agent.ConfigApplyTimeout, time.Duration(0))
	require.Greater(t, cfg.Agent.BootstrapTimeout, time.Duration(0))
}
