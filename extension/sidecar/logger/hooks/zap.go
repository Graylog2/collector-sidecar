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

package hooks

import (
	"github.com/sirupsen/logrus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ZapHook is a logrus hook that forwards log entries to a zap.Logger.
type ZapHook struct {
	zapLogger *zap.Logger
}

// NewZapHook creates a new ZapHook that forwards logrus entries to the given zap.Logger.
func NewZapHook(zapLogger *zap.Logger) *ZapHook {
	return &ZapHook{
		// Disable stacktraces for Sidecar errors
		zapLogger: zapLogger.WithOptions(zap.AddStacktrace(zap.ErrorLevel + 1)),
	}
}

// Levels returns all log levels that this hook should be fired for.
func (h *ZapHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire is called when a log event occurs. It forwards the logrus entry to the zap logger.
func (h *ZapHook) Fire(entry *logrus.Entry) error {
	fields := make([]zap.Field, 0, len(entry.Data))
	for key, value := range entry.Data {
		fields = append(fields, zap.Any(key, value))
	}

	zapLevel := logrusToZapLevel(entry.Level)

	if checkedEntry := h.zapLogger.Check(zapLevel, entry.Message); checkedEntry != nil {
		checkedEntry.Write(fields...)
	}

	return nil
}

// logrusToZapLevel converts a logrus log level to the equivalent zap log level.
func logrusToZapLevel(level logrus.Level) zapcore.Level {
	switch level {
	case logrus.ErrorLevel:
		return zapcore.ErrorLevel
	case logrus.PanicLevel, logrus.FatalLevel:
		// We don't want to exit the process here, so map Fatal and Panic to Error
		return zapcore.ErrorLevel
	case logrus.WarnLevel:
		return zapcore.WarnLevel
	case logrus.InfoLevel:
		return zapcore.InfoLevel
	case logrus.DebugLevel:
		return zapcore.DebugLevel
	case logrus.TraceLevel:
		return zapcore.DebugLevel // zap doesn't have trace, map to debug
	default:
		return zapcore.InfoLevel
	}
}
