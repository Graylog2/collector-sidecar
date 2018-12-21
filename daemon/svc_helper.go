// +build !windows

package daemon

import (
	"github.com/Graylog2/collector-sidecar/backends"
)

// Dummy function. Only used on Windows
func CleanOldServices(assignedBackends []*backends.Backend) {
}
