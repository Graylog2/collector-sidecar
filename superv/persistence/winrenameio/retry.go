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
	"math/rand/v2"
	"time"
)

// retryTimeout is the maximum cumulative time spent retrying transient errors.
// Modeled after Go's cmd/internal/robustio which uses 2 seconds.
const retryTimeout = 2000 * time.Millisecond

// retry calls op repeatedly when isTransient reports the returned error as
// transient. It uses exponential backoff with jitter, giving up after
// retryTimeout has elapsed. On success it returns nil. On a non-transient
// error it returns immediately. If all retries are exhausted, the last
// transient error is returned.
//
// This is modeled after Go's cmd/internal/robustio retry loop which handles
// flaky file operations on Windows caused by antivirus scanners, search
// indexers, or other processes briefly holding file handles.
func retry(op func() error, isTransient func(error) bool) error {
	var (
		bestErr   error
		start     time.Time
		nextSleep = time.Millisecond
	)
	for {
		err := op()
		if err == nil || !isTransient(err) {
			return err
		}

		bestErr = err

		if start.IsZero() {
			start = time.Now()
		} else if time.Since(start)+nextSleep >= retryTimeout {
			break
		}

		time.Sleep(nextSleep)
		nextSleep += time.Duration(rand.Int64N(int64(nextSleep))) //nolint:gosec // Doesn't need crypto/rand
	}

	return bestErr
}
