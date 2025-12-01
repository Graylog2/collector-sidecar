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

package cfgfile

import (
	"fmt"

	"github.com/Graylog2/collector-sidecar/common"
)

func withPlatformDefaults(config *SidecarConfig) {
	config.NodeId = fmt.Sprintf("file:%s", common.ConfigBasePath("node-id"))
	config.CachePath = common.ConfigBasePath("cache")
	config.LogPath = common.ConfigBasePath("logs")
	config.CollectorConfigurationDirectory = common.ConfigBasePath("generated")
	config.CollectorBinariesAccesslist = []string{
		common.ConfigBasePath("filebeat.exe"),
		common.ConfigBasePath("winlogbeat.exe"),
		"C:\\Program Files\\Filebeat\\filebeat.exe",
		"C:\\Program Files\\Packetbeat\\packetbeat.exe",
		"C:\\Program Files\\Metricbeat\\metricbeat.exe",
		"C:\\Program Files\\Heartbeat\\heartbeat.exe",
		"C:\\Program Files\\Auditbeat\\auditbeat.exe",
		"C:\\Program Files (x86)\\nxlog\\nxlog.exe",
		"C:\\Program Files\\nxlog\\nxlog.exe",
	}
	config.WindowsDriveRange = "CDEFGHIJKLMNOPQRSTUVWXYZ"
}
