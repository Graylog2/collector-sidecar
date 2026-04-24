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

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Graylog2/collector-sidecar/superv"
	"go.opentelemetry.io/collector/otelcol"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// maybeSupervisorService checks whether this process was started as a Windows
// service with the "supervisor" argument. If so, it runs the supervisor's
// service handler and returns (true, nil) on clean exit or (true, err) on
// failure. If this is not a supervisor invocation, it returns (false, false).
// If the SCM connection fails (interactive mode), it returns (false, true) so
// the caller skips any further svc.Run calls (StartServiceCtrlDispatcher can
// only be called once per process).
func maybeSupervisorService(_ otelcol.CollectorSettings) (handled bool, triedSCM bool) {
	if len(os.Args) <= 1 || os.Args[1] != "supervisor" {
		return false, false
	}
	err := svc.Run("", superv.NewSvcHandler())
	if errors.Is(err, windows.ERROR_FAILED_SERVICE_CONTROLLER_CONNECT) {
		return false, true
	}
	if err != nil {
		// Service handler failed — treat as handled so the process exits.
		fmt.Fprintf(os.Stderr, "supervisor service error: %v\n", err)
		return true, true
	}
	return true, true
}
