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

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	assert.Empty(t, cfg.Server.Endpoint)
	assert.Equal(t, "auto", cfg.Server.Transport)

	assert.Greater(t, cfg.Server.Connection.RetryBackoff.Initial, time.Duration(0))
	assert.Greater(t, cfg.Server.Connection.RetryBackoff.Max, 1*time.Minute)
	assert.Greater(t, cfg.Server.Connection.RetryBackoff.Multiplier, 0.0)

	assert.GreaterOrEqual(t, cfg.Server.Auth.JWTLifetime, time.Minute)
	assert.False(t, cfg.Server.Auth.InsecureTLS)
	assert.Empty(t, cfg.Server.Auth.EnrollmentURL)

	assert.Equal(t, "localhost:0", cfg.LocalServer.Endpoint)

	assert.Greater(t, cfg.Agent.ConfigApplyTimeout, time.Duration(0))
	assert.Greater(t, cfg.Agent.BootstrapTimeout, time.Duration(0))

	assert.Greater(t, cfg.Agent.Health.Timeout, time.Duration(0))
	assert.Greater(t, cfg.Agent.Health.Interval, time.Duration(0))

	assert.Equal(t, 0, cfg.Agent.Restart.MaxRetries) // Unlimited retries by default
	assert.Greater(t, cfg.Agent.Restart.InitialInterval, time.Duration(0))
	assert.Greater(t, cfg.Agent.Restart.MaxInterval, time.Duration(0))
	assert.Greater(t, cfg.Agent.Restart.Multiplier, 0.0)
	assert.Greater(t, cfg.Agent.Restart.RandomizationFactor, 0.0)
	assert.Greater(t, cfg.Agent.Restart.StableAfter, time.Duration(0))

	assert.Greater(t, cfg.Agent.Shutdown.GracefulTimeout, 5*time.Second)

	assert.False(t, cfg.Agent.Sidecar.Enabled)
	assert.True(t, cfg.Agent.Sidecar.Autodetect)

	assert.Equal(t, "/var/lib/graylog-collector/supervisor", cfg.Persistence.Dir)
	assert.Equal(t, "/var/lib/graylog-collector/supervisor/keys", cfg.Keys.Dir)
	assert.Equal(t, "/var/lib/graylog-collector/supervisor/packages", cfg.Packages.StorageDir)
}

func TestConfigInsecure(t *testing.T) {
	cfg := DefaultConfig()

	assert.False(t, cfg.IsInsecure())

	cfg.SetInsecure()

	assert.True(t, cfg.IsInsecure())

	t.Run("IsInsecure", func(t *testing.T) {
		cfg := DefaultConfig()

		assert.False(t, cfg.IsInsecure())

		cfg.Server.Auth.InsecureTLS = true
		cfg.Server.TLS.Insecure = true

		assert.True(t, cfg.IsInsecure())

		cfg.Server.Auth.InsecureTLS = true
		cfg.Server.TLS.Insecure = false

		assert.True(t, cfg.IsInsecure())

		cfg.Server.Auth.InsecureTLS = false
		cfg.Server.TLS.Insecure = true

		assert.True(t, cfg.IsInsecure())

		cfg.Server.Auth.InsecureTLS = false
		cfg.Server.TLS.Insecure = false

		assert.False(t, cfg.IsInsecure())
	})
}
