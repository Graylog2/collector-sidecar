// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package windows

import "errors"

// ListChannels is only supported on Windows.
func ListChannels() ([]string, error) {
	return nil, errors.New("channel enumeration is only supported on Windows")
}
