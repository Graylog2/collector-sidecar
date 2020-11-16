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
