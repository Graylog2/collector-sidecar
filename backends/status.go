// Copyright (C) 2020 Graylog, Inc.
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

package backends

import (
	"fmt"
	"github.com/Graylog2/collector-sidecar/system"
)

const (
	StatusRunning int = 0
	StatusUnknown int = 1
	StatusError   int = 2
	StatusStopped int = 3
)

func (b *Backend) SetStatus(state int, message string, verbose string) {
	b.backendStatus.Set(state, message, verbose)
}

func (b *Backend) SetVerboseStatus(verbose string) {
	b.backendStatus.VerboseMessage = verbose
}

func (b *Backend) SetStatusLogErrorf(format string, args ...interface{}) error {
	b.SetStatus(StatusError, fmt.Sprintf(format, args...), "")
	log.Errorf(fmt.Sprintf("[%s] ", b.Name)+format, args...)
	return fmt.Errorf(format, args)
}

func (b *Backend) Status() system.VerboseStatus {
	return b.backendStatus
}
