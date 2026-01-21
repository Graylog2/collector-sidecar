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

package logger

import (
	"io"

	"github.com/docker/go-units"
	"github.com/natefinch/lumberjack"
	"github.com/sirupsen/logrus"
)

var log *logrus.Logger

func init() {
	log = logrus.New()
	// We write to the Collector's zap logger and to a sidecar.log file via hooks. No need to write to stdout.
	log.SetOutput(io.Discard)
}

func Log() *logrus.Logger {
	return log
}

func GetRotatedLog(path string, maxSize int64, maxBackups int) io.WriteCloser {
	writer := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    int(maxSize / units.MiB), // megabytes
		MaxBackups: maxBackups,
		MaxAge:     0,     // disable time based rotation
		Compress:   false, // disabled by default
	}
	log.Debugf("Creating rotated log writer (%d/%d) for: %s", writer.MaxSize, writer.MaxBackups, path)
	return writer
}
