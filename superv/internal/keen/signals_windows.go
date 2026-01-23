//go:build windows

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package keen

import (
	"errors"
	"os"
	"syscall"
)

func sysProcAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func sendShutdownSignal(process *os.Process) error {
	// On Windows, we use CTRL_BREAK_EVENT to signal shutdown
	return process.Signal(os.Interrupt)
}

func sendReloadSignal(process *os.Process) error {
	// SIGHUP not available on Windows - caller should restart instead
	return errors.New("SIGHUP not supported on Windows, use restart instead")
}
