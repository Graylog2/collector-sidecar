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

package winrenameio

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errTransient = errors.New("transient")
var errPermanent = errors.New("permanent")

func alwaysTransient(err error) bool { return errors.Is(err, errTransient) }

func TestRetry_SucceedsImmediately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		err := retry(func() error {
			calls++
			return nil
		}, alwaysTransient)

		require.NoError(t, err)
		assert.Equal(t, 1, calls)
	})
}

func TestRetry_SucceedsAfterTransientErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		err := retry(func() error {
			calls++
			if calls < 3 {
				return errTransient
			}
			return nil
		}, alwaysTransient)

		require.NoError(t, err)
		assert.Equal(t, 3, calls)
	})
}

func TestRetry_NonTransientErrorReturnsImmediately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		err := retry(func() error {
			calls++
			return errPermanent
		}, alwaysTransient)

		require.ErrorIs(t, err, errPermanent)
		assert.Equal(t, 1, calls)
	})
}

func TestRetry_GivesUpAfterTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		calls := 0
		err := retry(func() error {
			calls++
			return errTransient
		}, alwaysTransient)

		elapsed := time.Since(start)

		require.ErrorIs(t, err, errTransient)
		assert.GreaterOrEqual(t, calls, 2, "should have retried at least once")
		// The retry loop stops when the next sleep would exceed retryTimeout.
		// With a fake clock we can verify the elapsed time is close to (but
		// may be slightly less than) retryTimeout.
		assert.Greater(t, elapsed, retryTimeout/2, "should have spent significant time retrying")
		assert.LessOrEqual(t, elapsed, retryTimeout, "should not exceed the timeout")
	})
}

func TestRetry_ReturnsLastTransientError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		errFirst := errors.New("first transient")
		errSecond := errors.New("second transient")
		isTransient := func(err error) bool {
			return errors.Is(err, errFirst) || errors.Is(err, errSecond)
		}

		calls := 0
		err := retry(func() error {
			calls++
			if calls == 1 {
				return errFirst
			}
			return errSecond
		}, isTransient)

		// Should get the most recent transient error, not the first.
		require.ErrorIs(t, err, errSecond)
	})
}

func TestRetry_TransientThenPermanentReturnsPermanent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		err := retry(func() error {
			calls++
			if calls == 1 {
				return errTransient
			}
			return errPermanent
		}, alwaysTransient)

		require.ErrorIs(t, err, errPermanent)
		assert.Equal(t, 2, calls)
	})
}
