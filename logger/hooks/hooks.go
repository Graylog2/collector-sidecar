// This file is part of Graylog.
//
// Graylog is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Graylog is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Graylog.  If not, see <http://www.gnu.org/licenses/>.

package hooks

import (
	"path/filepath"

	"github.com/rifflock/lfshook"
	"github.com/Sirupsen/logrus"

	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/logger"
	"github.com/Graylog2/collector-sidecar/common"
)

func AddLogHooks(context *context.Ctx, log *logrus.Logger) {
	filesystemHook(context, log)
}

func filesystemHook(context *context.Ctx, log *logrus.Logger) {
	logfile := filepath.Join(context.UserConfig.LogPath, "collector_sidecar.log")
	err := common.CreatePathToFile(logfile)
	if err != nil {
		log.Fatalf("Failed to create directory for log file %s: %s", logfile, err)
	}
	writer := logger.GetRotatedLog(logfile, context.UserConfig.LogRotationTime, context.UserConfig.LogMaxAge)
	log.Hooks.Add(lfshook.NewHook(lfshook.WriterMap{
		logrus.FatalLevel: writer,
		logrus.ErrorLevel: writer,
		logrus.WarnLevel: writer,
		logrus.InfoLevel: writer,
		logrus.DebugLevel: writer,
	}))
}
