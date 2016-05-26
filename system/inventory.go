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

package system

import (
	"github.com/Graylog2/collector-sidecar/common"
	"runtime"
)

type Inventory struct {
}

func NewInventory() *Inventory {
	return &Inventory{}
}

func (inv *Inventory) Version() string {
	return common.CollectorVersion
}

func (inv *Inventory) Linux() bool {
	return runtime.GOOS == "linux"
}

func (inv *Inventory) Darwin() bool {
	return runtime.GOOS == "darwin"
}

func (inv *Inventory) Windows() bool {
	return runtime.GOOS == "windows"
}

func (inv *Inventory) LinuxPlatform() string {
	if runtime.GOOS == "linux" {
		return common.LinuxPlatformFamily()
	} else {
		return runtime.GOOS
	}
}
