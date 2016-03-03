// +build darwin linux

package util

import (
	"log/syslog"

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
