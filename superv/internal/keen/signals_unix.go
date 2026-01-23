//go:build !windows

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package keen

import (
	"os"
	"syscall"
)

func sysProcAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func sendShutdownSignal(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}

func sendReloadSignal(process *os.Process) error {
	return process.Signal(syscall.SIGHUP)
}
