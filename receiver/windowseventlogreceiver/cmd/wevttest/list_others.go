// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import "errors"

func listChannels() error {
	return errors.New("the list command is only supported on Windows")
}
