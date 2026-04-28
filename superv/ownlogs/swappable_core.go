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

package ownlogs

import (
	"fmt"
	"sync/atomic"

	"go.uber.org/zap/zapcore"
)

// swappableCore is a zapcore.Core that atomically delegates to a replaceable
// inner core. When the inner is nil, the core acts as a no-op (Enabled returns
// false for all levels, Write is a no-op).
//
// With() returns a derivative that shares the same atomic pointer, so all
// derived loggers see swaps. Stored With-fields are re-applied on each Write
// call to ensure the current inner core receives them.
type swappableCore struct {
	inner  *atomic.Pointer[zapcore.Core]
	fields []zapcore.Field
}

var _ zapcore.Core = (*swappableCore)(nil)

func newSwappableCore() *swappableCore {
	return &swappableCore{
		inner: &atomic.Pointer[zapcore.Core]{},
	}
}

// swap atomically replaces the inner core. Pass nil to disable.
func (s *swappableCore) swap(core zapcore.Core) {
	if core == nil {
		s.inner.Store(nil)
	} else {
		s.inner.Store(&core)
	}
}

func (s *swappableCore) loadInner() zapcore.Core {
	p := s.inner.Load()
	if p == nil {
		return nil
	}
	return *p
}

func (s *swappableCore) Enabled(level zapcore.Level) bool {
	inner := s.loadInner()
	if inner == nil {
		return false
	}
	return inner.Enabled(level)
}

func (s *swappableCore) With(fields []zapcore.Field) zapcore.Core {
	combined := make([]zapcore.Field, 0, len(s.fields)+len(fields))
	combined = append(combined, s.fields...)
	combined = append(combined, fields...)
	return &swappableCore{
		inner:  s.inner,
		fields: combined,
	}
}

func (s *swappableCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	inner := s.loadInner()
	if inner == nil {
		return ce
	}
	if inner.Enabled(ent.Level) {
		return ce.AddCore(ent, s)
	}
	return ce
}

func (s *swappableCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	inner := s.loadInner()
	if inner == nil {
		return nil
	}
	if len(s.fields) > 0 {
		inner = inner.With(s.fields)
	}
	if err := inner.Write(ent, fields); err != nil {
		return fmt.Errorf("writing log entry: %w", err)
	}
	return nil
}

func (s *swappableCore) Sync() error {
	inner := s.loadInner()
	if inner == nil {
		return nil
	}
	if err := inner.Sync(); err != nil {
		return fmt.Errorf("syncing log core: %w", err)
	}
	return nil
}
