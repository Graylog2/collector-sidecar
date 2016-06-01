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

// +build darwin linux

package common

import (
	"time"
	"log/syslog"

	"github.com/lestrrat/go-file-rotatelogs"

	"github.com/Sirupsen/logrus"
	"github.com/Sirupsen/logrus/hooks/syslog"
)

var log = logrus.New()

func init() {
	// initialize logging
	hook, err := logrus_syslog.NewSyslogHook("", "", syslog.LOG_INFO, "")

	if err == nil {
		log.Hooks.Add(hook)
	}
}

func Log() *logrus.Logger {
	return log
}

func GetRotatedLog(path string, rotation_time int, max_age int) *rotatelogs.RotateLogs {
	log.Debugf("Creating rotated log writer for: %s", path + ".%Y%m%d%H%M")

	writer := rotatelogs.NewRotateLogs(
		path + ".%Y%m%d%H%M",
	)
	writer.LinkName = path
	writer.RotationTime = time.Duration(rotation_time) * time.Second
	writer.MaxAge = time.Duration(max_age) * time.Second
	writer.Offset = 0

	return writer
}
