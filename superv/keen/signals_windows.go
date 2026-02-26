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

//go:build windows

package keen

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

func sysProcAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func sendShutdownSignal(process *os.Process) error {
	// os.Process.Signal(os.Interrupt) is not supported on Windows.
	// Since we create the process with CREATE_NEW_PROCESS_GROUP,
	// we send CTRL_BREAK_EVENT to the process group to trigger
	// graceful shutdown.
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(process.Pid))
}

func sendReloadSignal(process *os.Process) error {
	// SIGHUP not available on Windows - caller should restart instead
	return errors.New("SIGHUP not supported on Windows, use restart instead")
}
