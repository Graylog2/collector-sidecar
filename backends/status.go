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

func (b *Backend)SetStatusLogErrorf(format string, args ...interface{}) error {
	b.SetStatus(StatusError, fmt.Sprintf(format, args...))
	log.Errorf(fmt.Sprintf("[%s] ", b.Name)+format, args...)
	return fmt.Errorf(format, args)
}

func (b *Backend) Status() system.Status {
	return b.backendStatus
}

