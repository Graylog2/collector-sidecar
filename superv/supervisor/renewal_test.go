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

package supervisor

import (
	"testing"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/auth"
	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestCheckCertificateRenewal_Scheduling(t *testing.T) {
	renewalInterval := 1 * time.Hour
	fraction := 0.75

	authCfg := config.AuthConfig{
		RenewalInterval: renewalInterval,
		RenewalFraction: fraction,
	}

	t.Run("not enrolled returns fallback interval", func(t *testing.T) {
		s := &Supervisor{
			logger:      zaptest.NewLogger(t),
			authManager: auth.NewManager(zap.NewNop(), auth.ManagerConfig{KeysDir: t.TempDir()}),
			authCfg:     authCfg,
		}

		delay := s.checkCertificateRenewal()
		require.Equal(t, renewalInterval, delay)
	})

	t.Run("fresh long-lived cert returns capped delay", func(t *testing.T) {
		// Cert valid for 365 days from now. Renewal time = 0.75 * 365d = ~273 days away.
		// Should be capped at renewalInterval (1h).
		now := time.Now()
		s := &Supervisor{
			logger:      zaptest.NewLogger(t),
			authManager: newEnrolledAuthManager(t, now.Add(-time.Hour), now.Add(365*24*time.Hour)),
			authCfg:     authCfg,
		}

		delay := s.checkCertificateRenewal()
		require.Equal(t, renewalInterval, delay)
	})

	t.Run("short-lived cert returns time until renewal", func(t *testing.T) {
		// Cert valid for 10 minutes from now. Renewal time = 0.75 * 10m = 7.5m away.
		// Should return ~7.5m, not the 1h fallback.
		now := time.Now()
		s := &Supervisor{
			logger:      zaptest.NewLogger(t),
			authManager: newEnrolledAuthManager(t, now, now.Add(10*time.Minute)),
			authCfg:     authCfg,
		}

		delay := s.checkCertificateRenewal()

		// Renewal time is ~7.5m from now; allow some slack for test execution.
		require.Greater(t, delay, 7*time.Minute)
		require.Less(t, delay, 8*time.Minute)
	})

	t.Run("cert past renewal threshold returns fallback", func(t *testing.T) {
		// Cert issued 24h ago, expires in 1h. Renewal threshold long past.
		now := time.Now()
		s := &Supervisor{
			logger:      zaptest.NewLogger(t),
			authManager: newEnrolledAuthManager(t, now.Add(-24*time.Hour), now.Add(time.Hour)),
			authCfg:     authCfg,
		}

		delay := s.checkCertificateRenewal()
		require.Equal(t, renewalInterval, delay)
	})

	t.Run("pending CSR retry returns time until next retry", func(t *testing.T) {
		now := time.Now()
		s := &Supervisor{
			logger:      zaptest.NewLogger(t),
			authManager: newEnrolledAuthManager(t, now.Add(-time.Hour), now.Add(24*time.Hour)),
			authCfg:     authCfg,
			pendingCSR:  []byte("fake-csr"),
		}
		retryTime := now.Add(30 * time.Second)
		s.nextRenewalRetry = retryTime

		delay := s.checkCertificateRenewal()

		require.Greater(t, delay, 25*time.Second)
		require.Less(t, delay, 31*time.Second)
	})

	t.Run("pending CSR past retry time returns fallback", func(t *testing.T) {
		now := time.Now()
		s := &Supervisor{
			logger:      zaptest.NewLogger(t),
			authManager: newEnrolledAuthManager(t, now.Add(-time.Hour), now.Add(24*time.Hour)),
			authCfg:     authCfg,
			pendingCSR:  []byte("fake-csr"),
		}
		// Retry was due 10 seconds ago.
		s.nextRenewalRetry = now.Add(-10 * time.Second)

		delay := s.checkCertificateRenewal()
		// requestCertificateRenewal will fail (no opampClient) but should still return fallback.
		require.Equal(t, renewalInterval, delay)
	})
}
