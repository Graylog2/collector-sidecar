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

import "time"

type SidecarConfig struct {
	ServerUrl                        string        `config:"server_url"`
	ServerApiToken                   string        `config:"server_api_token"`
	TlsSkipVerify                    bool          `config:"tls_skip_verify"`
	NodeName                         string        `config:"node_name"`
	NodeId                           string        `config:"node_id"`
	CachePath                        string        `config:"cache_path"`
	LogPath                          string        `config:"log_path"`
	CollectorValidationTimeoutString string        `config:"collector_validation_timeout"`
	CollectorValidationTimeout       time.Duration // set from CollectorValidationTimeoutString
	CollectorConfigurationDirectory  string        `config:"collector_configuration_directory"`
	LogRotateMaxFileSizeString       string        `config:"log_rotate_max_file_size"`
	LogRotateMaxFileSize             int64         // set from LogRotateMaxFileSizeString
	LogRotateKeepFiles               int           `config:"log_rotate_keep_files"`
	UpdateInterval                   int           `config:"update_interval"`
	SendStatus                       bool          `config:"send_status"`
	ListLogFiles                     []string      `config:"list_log_files"`
	CollectorBinariesWhitelist       []string      `config:"collector_binaries_whitelist"`
	CollectorBinariesAccesslist      []string      `config:"collector_binaries_accesslist"`
}

// Default Sidecar configuration
const CommonDefaults = `
server_url: "http://127.0.0.1:9000/api/"
server_api_token: ""
node_id: "file:/etc/graylog/sidecar/node-id"
update_interval: 10
tls_skip_verify: false
send_status: true
list_log_files: []
cache_path: "/var/cache/graylog-sidecar"
log_path: "/var/log/graylog-sidecar"
log_rotate_max_file_size: "10MiB"
log_rotate_keep_files: 10
collector_validation_timeout: "1m"
collector_configuration_directory: "/var/lib/graylog-sidecar/generated"
collector_binaries_accesslist:
  - "/usr/bin/filebeat"
  - "/usr/bin/packetbeat"
  - "/usr/bin/metricbeat"
  - "/usr/bin/heartbeat"
  - "/usr/bin/auditbeat"
  - "/usr/bin/journalbeat"
  - "/usr/share/filebeat/bin/filebeat"
  - "/usr/share/packetbeat/bin/packetbeat"
  - "/usr/share/metricbeat/bin/metricbeat"
  - "/usr/share/heartbeat/bin/heartbeat"
  - "/usr/share/auditbeat/bin/auditbeat"
  - "/usr/share/journalbeat/bin/journalbeat"
  - "/usr/bin/nxlog"
  - "/opt/nxlog/bin/nxlog"
`

// Windows specific options. Gets merged over `CommonDefaults`
const WindowsDefaults = `
node_id: "file:C:\\Program Files\\Graylog\\sidecar\\node-id"
cache_path: "C:\\Program Files\\Graylog\\sidecar\\cache"
log_path: "C:\\Program Files\\Graylog\\sidecar\\logs"
collector_configuration_directory: "C:\\Program Files\\Graylog\\sidecar\\generated"
collector_binaries_accesslist:
  - "C:\\Program Files\\Graylog\\sidecar\\filebeat.exe"
  - "C:\\Program Files\\Graylog\\sidecar\\winlogbeat.exe"
  - "C:\\Program Files\\Filebeat\\filebeat.exe"
  - "C:\\Program Files\\Packetbeat\\packetbeat.exe"
  - "C:\\Program Files\\Metricbeat\\metricbeat.exe"
  - "C:\\Program Files\\Heartbeat\\heartbeat.exe"
  - "C:\\Program Files\\Auditbeat\\auditbeat.exe"
  - "C:\\Program Files (x86)\\nxlog\\nxlog.exe"
`
