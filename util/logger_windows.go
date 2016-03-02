package util

import (
	"github.com/Sirupsen/logrus"
)

var log = logrus.New()

func Log() *logrus.Logger {
	return log
}
