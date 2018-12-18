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
	ServerUrl                       string    `config:"server_url"`
	ServerApiToken                  string    `config:"server_api_token"`
	TlsSkipVerify                   bool      `config:"tls_skip_verify"`
	NodeName                        string    `config:"node_name"`
	NodeId                          string    `config:"node_id"`
	CachePath                       string    `config:"cache_path"`
	LogPath                         string    `config:"log_path"`
	CollectorConfigurationDirectory string    `config:"collector_configuration_directory"`
	LogRotateEveryMb                int       `config:"log_rotate_every_mb"`
	LogRotateKeepFiles              int       `config:"log_rotate_keep_files"`
	UpdateInterval                  int       `config:"update_interval"`
	SendStatus                      bool      `config:"send_status"`
	ListLogFiles                    []string  `config:"list_log_files"`
	CollectorBinariesWhitelist      *[]string `config:"collector_binaries_whitelist"`
}
