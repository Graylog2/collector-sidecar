// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import "time"

const (
	backoffInitial = 5 * time.Second
	backoffMax     = 60 * time.Second
	backoffFactor  = 2
)

type backoff struct {
	current time.Duration
}

func newBackoff() *backoff {
	return &backoff{current: backoffInitial}
}

// next returns the current delay and advances to the next one.
func (b *backoff) next() time.Duration {
	d := b.current
	b.current *= backoffFactor
	if b.current > backoffMax {
		b.current = backoffMax
	}
	return d
}

// reset returns the backoff to its initial delay.
func (b *backoff) reset() {
	b.current = backoffInitial
}

// Windows error codes for classification.
// These are plain uint32 (not syscall.Errno) because this file is cross-platform.
// The typed syscall.Errno variants live in api.go (Windows-only).
const (
	// Recoverable (transient) error codes.
	errorInvalidHandle    = 6
	errorInvalidParameter = 87
	rpcServerUnavailable  = 1722
	rpcCallCancelled      = 1818
	evtQueryResultStale   = 15011
	evtPublisherDisabled  = 15037

	// Non-recoverable error codes (defined for classification).
	evtChannelNotFound = 15007
)

// isRecoverableError checks if a Windows error code is transient/recoverable.
func isRecoverableError(code uint32) bool {
	switch code {
	case errorInvalidHandle,
		rpcServerUnavailable,
		rpcCallCancelled,
		evtQueryResultStale,
		errorInvalidParameter,
		evtPublisherDisabled:
		return true
	default:
		return false
	}
}
