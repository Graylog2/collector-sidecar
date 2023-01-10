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
	"github.com/Graylog2/collector-sidecar/logger"
	"path/filepath"

	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"

	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
)

func AddLogHooks(context *context.Ctx, log *logrus.Logger) {
	filesystemHook(context, log)
}

func filesystemHook(context *context.Ctx, log *logrus.Logger) {
	logfile := filepath.Join(context.UserConfig.LogPath, "sidecar.log")
	err := common.CreatePathToFile(logfile)
	if err != nil {
		log.Fatalf("Failed to create directory for log file %s: %s", logfile, err)
	}
	writer := logger.GetRotatedLog(logfile, context.UserConfig.LogRotateMaxFileSize, context.UserConfig.LogRotateKeepFiles)
	log.Hooks.Add(lfshook.NewHook(lfshook.WriterMap{
		logrus.FatalLevel: writer,
		logrus.ErrorLevel: writer,
		logrus.WarnLevel:  writer,
		logrus.InfoLevel:  writer,
		logrus.DebugLevel: writer,
	}, nil))
}
