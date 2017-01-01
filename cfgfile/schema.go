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

import (
	"errors"
)

type SidecarConfig struct {
	ServerUrl       string   `config:"server_url"`
	TlsSkipVerify   bool     `config:"tls_skip_verify"`
	NodeId          string   `config:"node_id"`
	CollectorId     string   `config:"collector_id"`
	Tags            []string `config:"tags"`
	CachePath       string   `config:"cache_path"`
	LogPath         string   `config:"log_path"`
	LogRotationTime int      `config:"log_rotation_time"`
	LogMaxAge       int      `config:"log_max_age"`
	UpdateInterval  int      `config:"update_interval"`
	SendStatus      bool     `config:"send_status"`
	ListLogFiles    []string `config:"list_log_files"`
	Backends        []SidecarBackend
}

type SidecarBackend struct {
	Name              string `config:"name"`
	Enabled           *bool  `config:"enabled"`
	BinaryPath        string `config:"binary_path"`
	ConfigurationPath string `config:"configuration_path"`
	RunPath           string `config:"run_path"`
}

func (sc *SidecarConfig) GetBackendIndexByName(name string) (int, error) {
	index := -1
	for i, backend := range sc.Backends {
		if backend.Name == name {
			index = i
		}
	}
	if index < 0 {
		return index, errors.New("Can not find configuration for: " + name)
	}
	return index, nil
}
