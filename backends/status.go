package backends

import (
	"fmt"
	"github.com/Graylog2/collector-sidecar/system"
)

const (
	StatusRunning int = 0
	StatusUnknown int = 1
	StatusError   int = 2
)

func (b *Backend) SetStatus(state int, message string) {
	b.backendStatus.Set(state, message)
}

func (b *Backend) Status() system.Status {
	return b.backendStatus
}

func SetStatusLogErrorf(name string, format string, args ...interface{}) error {
	Store.backends[name].SetStatus(StatusError, fmt.Sprintf(format, args...))
	log.Errorf(fmt.Sprintf("[%s] ", name)+format, args...)
	return fmt.Errorf(format, args)
}