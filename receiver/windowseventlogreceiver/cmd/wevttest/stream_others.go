// Copyright (C) 2026 Graylog, Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"context"
	"errors"
)

func streamEvents(_ context.Context, _ []string, _ string, _ *formatter) error {
	return errors.New("the stream command is only supported on Windows")
}
