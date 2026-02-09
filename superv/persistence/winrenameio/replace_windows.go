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

package winrenameio

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// ReplaceFile atomically replaces the destination file with the source file.
//
// It calls MoveFileExW with MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH.
// MOVEFILE_WRITE_THROUGH improves durability but does not strictly guarantee it
// end-to-end (see package documentation for details).
//
// On transient Windows errors (ERROR_ACCESS_DENIED, ERROR_SHARING_VIOLATION)
// caused by antivirus scanners, search indexers, or other processes briefly
// holding file handles, the call is retried with exponential backoff for up to
// 2 seconds. This matches the retry strategy used by Go's own
// cmd/internal/robustio package.
//
// Both source and destination must be on the same volume. Cross-volume moves
// fail with an error because MOVEFILE_COPY_ALLOWED is not set.
func ReplaceFile(source, destination string) error {
	src, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return &os.LinkError{Op: "replace", Old: source, New: destination, Err: err}
	}
	dest, err := syscall.UTF16PtrFromString(destination)
	if err != nil {
		return &os.LinkError{Op: "replace", Old: source, New: destination, Err: err}
	}

	op := func() error {
		return windows.MoveFileEx(src, dest, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
	}

	if err := retry(op, isTransientWindows); err != nil {
		return &os.LinkError{Op: "replace", Old: source, New: destination, Err: err}
	}
	return nil
}

// isTransientWindows reports whether err is a transient Windows error that may
// resolve by retrying after a short delay. These errors are typically caused by
// antivirus scanners, search indexers, or backup software briefly holding
// handles on the file being replaced.
func isTransientWindows(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case windows.ERROR_ACCESS_DENIED, windows.ERROR_SHARING_VIOLATION:
			return true
		}
	}
	return false
}
