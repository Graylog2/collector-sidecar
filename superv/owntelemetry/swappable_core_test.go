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

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestSwappableCore_NilInner_DisablesAllLevels(t *testing.T) {
	sc := newSwappableCore()
	for _, lvl := range []zapcore.Level{
		zapcore.DebugLevel, zapcore.InfoLevel,
		zapcore.WarnLevel, zapcore.ErrorLevel,
	} {
		assert.False(t, sc.Enabled(lvl), "level %s should be disabled when inner is nil", lvl)
	}
}

func TestSwappableCore_WithInner_EnablesMatchingLevels(t *testing.T) {
	inner, _ := observer.New(zapcore.WarnLevel)
	sc := newSwappableCore()
	sc.swap(inner)

	assert.False(t, sc.Enabled(zapcore.InfoLevel))
	assert.True(t, sc.Enabled(zapcore.WarnLevel))
	assert.True(t, sc.Enabled(zapcore.ErrorLevel))
}

func TestSwappableCore_Write_DelegatesToInner(t *testing.T) {
	inner, logs := observer.New(zapcore.InfoLevel)
	sc := newSwappableCore()
	sc.swap(inner)

	logger := zap.New(sc)
	logger.Info("hello", zap.String("key", "val"))

	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	assert.Equal(t, "hello", entry.Message)
	assert.Equal(t, "val", entry.ContextMap()["key"])
}

func TestSwappableCore_Write_NilInner_NoOp(t *testing.T) {
	sc := newSwappableCore()
	logger := zap.New(sc)
	// Must not panic
	logger.Info("ignored")
}

func TestSwappableCore_With_SeesSwap(t *testing.T) {
	inner1, logs1 := observer.New(zapcore.InfoLevel)
	sc := newSwappableCore()
	sc.swap(inner1)

	// Create a derived logger with With() fields
	logger := zap.New(sc).With(zap.String("component", "test"))
	logger.Info("before swap")
	require.Equal(t, 1, logs1.Len())
	assert.Equal(t, "test", logs1.All()[0].ContextMap()["component"])

	// Swap to a new inner core
	inner2, logs2 := observer.New(zapcore.InfoLevel)
	sc.swap(inner2)

	logger.Info("after swap")
	// Old core should not receive the new message
	assert.Equal(t, 1, logs1.Len())
	// New core should receive it with the With() fields
	require.Equal(t, 1, logs2.Len())
	assert.Equal(t, "after swap", logs2.All()[0].Message)
	assert.Equal(t, "test", logs2.All()[0].ContextMap()["component"])
}

func TestSwappableCore_Swap_NilClearsInner(t *testing.T) {
	inner, logs := observer.New(zapcore.InfoLevel)
	sc := newSwappableCore()
	sc.swap(inner)

	logger := zap.New(sc)
	logger.Info("visible")
	require.Equal(t, 1, logs.Len())

	sc.swap(nil)
	logger.Info("invisible")
	assert.Equal(t, 1, logs.Len()) // no new entries
}

func TestSwappableCore_ConcurrentAccess(t *testing.T) {
	sc := newSwappableCore()
	logger := zap.New(sc).With(zap.String("worker", "x"))

	var done atomic.Bool
	go func() {
		for !done.Load() {
			inner, _ := observer.New(zapcore.InfoLevel)
			sc.swap(inner)
		}
	}()

	for range 1000 {
		logger.Info("msg")
	}
	done.Store(true)
}
