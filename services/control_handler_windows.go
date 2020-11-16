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


package services

import (
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/daemon"
	"github.com/kardianos/service"
)

func ControlHandler(action string) {
	switch action {
	case "install":
	case "uninstall":
		cleanupInstalledServices()
	case "start":
	case "stop":
	case "restart":
	case "status":
	default:
		log.Fatalf("Valid service actions: %s", service.ControlAction)
	}
}

func cleanupInstalledServices() {
	// Cleans all services starting with "graylog-collector-"
	daemon.CleanOldServices([]*backends.Backend{})
}
