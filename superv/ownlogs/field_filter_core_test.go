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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func newFilteredLogger(dropFields []string) (*zap.Logger, *observer.ObservedLogs) {
	inner, logs := observer.New(zapcore.InfoLevel)
	core := &FieldFilterCore{
		Core:       inner,
		DropFields: dropFields,
	}
	return zap.New(core), logs
}

func TestFieldFilterCore_DropsMatchingField(t *testing.T) {
	logger, logs := newFilteredLogger([]string{"resource"})

	logger.With(zap.String("resource", "should-be-dropped"), zap.String("keep", "yes")).
		Info("test")

	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	assert.Equal(t, "yes", entry.ContextMap()["keep"])
	assert.NotContains(t, entry.ContextMap(), "resource")
}

func TestFieldFilterCore_KeepsNonMatchingFields(t *testing.T) {
	logger, logs := newFilteredLogger([]string{"resource"})

	logger.With(zap.String("component", "health_check"), zap.Int("status", 200)).
		Info("ok")

	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	assert.Equal(t, "health_check", entry.ContextMap()["component"])
	assert.Equal(t, int64(200), entry.ContextMap()["status"])
}

func TestFieldFilterCore_DropsMultipleFields(t *testing.T) {
	logger, logs := newFilteredLogger([]string{"resource", "secret"})

	logger.With(
		zap.String("resource", "dropped"),
		zap.String("secret", "dropped"),
		zap.String("visible", "kept"),
	).Info("test")

	require.Equal(t, 1, logs.Len())
	ctx := logs.All()[0].ContextMap()
	assert.NotContains(t, ctx, "resource")
	assert.NotContains(t, ctx, "secret")
	assert.Equal(t, "kept", ctx["visible"])
}

func TestFieldFilterCore_NoDropFields_PassesThrough(t *testing.T) {
	logger, logs := newFilteredLogger(nil)

	logger.With(zap.String("resource", "kept")).Info("test")

	require.Equal(t, 1, logs.Len())
	assert.Equal(t, "kept", logs.All()[0].ContextMap()["resource"])
}

func TestFieldFilterCore_ChainedWith_FiltersAtEachLevel(t *testing.T) {
	logger, logs := newFilteredLogger([]string{"resource"})

	// First With adds a dropped field, second With adds another dropped field.
	child := logger.With(zap.String("resource", "dropped1"), zap.String("a", "1"))
	grandchild := child.With(zap.String("resource", "dropped2"), zap.String("b", "2"))
	grandchild.Info("test")

	require.Equal(t, 1, logs.Len())
	ctx := logs.All()[0].ContextMap()
	assert.NotContains(t, ctx, "resource")
	assert.Equal(t, "1", ctx["a"])
	assert.Equal(t, "2", ctx["b"])
}

func TestFieldFilterCore_RespectsLogLevel(t *testing.T) {
	inner, logs := observer.New(zapcore.WarnLevel)
	core := &FieldFilterCore{
		Core:       inner,
		DropFields: []string{"resource"},
	}
	logger := zap.New(core)

	logger.Info("below threshold")
	logger.Warn("at threshold")

	require.Equal(t, 1, logs.Len())
	assert.Equal(t, "at threshold", logs.All()[0].Message)
}

func TestFieldFilterCore_InlineFields_NotFiltered(t *testing.T) {
	// Fields passed directly to the log call (not via With) are not filtered,
	// because they go through Write(), not With().
	logger, logs := newFilteredLogger([]string{"resource"})

	logger.Info("test", zap.String("resource", "inline"))

	require.Equal(t, 1, logs.Len())
	assert.Equal(t, "inline", logs.All()[0].ContextMap()["resource"])
}
