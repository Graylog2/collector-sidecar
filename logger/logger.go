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

package logger

import (
	"github.com/Sirupsen/logrus"
	"github.com/natefinch/lumberjack"
	"io"
)

var log = logrus.New()

func Log() *logrus.Logger {
	return log
}

func GetRotatedLog(path string, maxSize int, maxBackups int) io.WriteCloser {
	log.Debugf("Creating rotated log writer for: %s", path)

	writer := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    maxSize, // megabytes
		MaxBackups: maxBackups,
		MaxAge:     0,     // disable time based rotation
		Compress:   false, // disabled by default
	}
	return writer
}
