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

package owntelemetry

import "slices"

import "go.uber.org/zap/zapcore"

// FieldFilterCore wraps a zapcore.Core and drops fields with specific keys
// before forwarding them. This prevents the OTel Collector's internal
// telemetry "resource" field from leaking into log record attributes.
type FieldFilterCore struct {
	zapcore.Core
	DropFields []string
}

func (c *FieldFilterCore) With(fields []zapcore.Field) zapcore.Core {
	filtered := make([]zapcore.Field, 0, len(fields))
	for _, f := range fields {
		if !slices.Contains(c.DropFields, f.Key) {
			filtered = append(filtered, f)
		}
	}
	return &FieldFilterCore{
		Core:       c.Core.With(filtered),
		DropFields: c.DropFields,
	}
}

func (c *FieldFilterCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}
