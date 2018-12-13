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

package cfgfile

type SidecarConfig struct {
	ServerUrl                       string   `config:"server_url"`
	ServerApiToken                  string   `config:"server_api_token"`
	TlsSkipVerify                   bool     `config:"tls_skip_verify"`
	NodeName                        string   `config:"node_name"`
	NodeId                          string   `config:"node_id"`
	CachePath                       string   `config:"cache_path"`
	LogPath                         string   `config:"log_path"`
	CollectorConfigurationDirectory string   `config:"collector_configuration_directory"`
	LogRotationTime                 int      `config:"log_rotation_time"`
	LogMaxAge                       int      `config:"log_max_age"`
	UpdateInterval                  int      `config:"update_interval"`
	SendStatus                      bool     `config:"send_status"`
	ListLogFiles                    []string `config:"list_log_files"`
	CollectorBinariesWhitelist      []string `config:"collector_binaries_whitelist"`
}

// Default Sidecar configuration
const CommonDefaults = `
server_url: "http://127.0.0.1:9000/api/"
server_api_token: ""
node_id: "file:/etc/graylog/sidecar/node-id"
update_interval: 10
tls_skip_verify: false
send_status: true
list_log_files:
cache_path: "/var/cache/graylog-sidecar"
log_path: "/var/log/graylog-sidecar"
log_rotation_time: 86400
log_max_age: 604800
collector_configuration_directory: "/var/lib/graylog-sidecar/generated"
collector_binaries_whitelist:
  - "/usr/lib/graylog-sidecar/filebeat"
  - "/usr/bin/filebeat"
  - "/usr/bin/packetbeat"
  - "/usr/bin/metricbeat"
  - "/usr/bin/heartbeat"
  - "/usr/bin/auditbeat"
  - "/opt/nxlog/bin/nxlog"
`

// Windows specific options. Gets merged over `CommonDefaults`
const WindowsDefaults = `
node_id: "file:C:\\Program Files\\Graylog\\sidecar\\node-id"
cache_path: "C:\\Program Files\\Graylog\\sidecar\\cache"
log_path: "C:\\Program Files\\Graylog\\sidecar\\logs"
collector_configuration_directory: "C:\\Program Files\\Graylog\\sidecar\\generated"
collector_binaries_whitelist:
  - "C:\\Program Files\\Graylog\\sidecar\\filebeat.exe"
  - "C:\\Program Files\\Graylog\\sidecar\\winlogbeat.exe"
  - "C:\\Program Files\\Filebeat\\filebeat.exe"
  - "C:\\Program Files\\Packetbeat\\packetbeat.exe"
  - "C:\\Program Files\\Metricbeat\\metricbeat.exe"
  - "C:\\Program Files\\Heartbeat\\heartbeat.exe"
  - "C:\\Program Files\\Auditbeat\\auditbeat.exe"
  - "C:\\Program Files (x86)\\nxlog\\nxlog.exe"
`
