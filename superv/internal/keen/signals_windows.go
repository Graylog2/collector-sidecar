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
