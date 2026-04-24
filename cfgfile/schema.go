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
	CollectorShutdownTimeoutString   string        `config:"collector_shutdown_timeout"`
	CollectorShutdownTimeout         time.Duration // set from CollectorShutdownTimeoutString
	LogRotateMaxFileSizeString       string        `config:"log_rotate_max_file_size"`
	LogRotateMaxFileSize             int64         // set from LogRotateMaxFileSizeString
	LogRotateKeepFiles               int           `config:"log_rotate_keep_files"`
	UpdateInterval                   int           `config:"update_interval"`
	SendStatus                       bool          `config:"send_status"`
	ListLogFiles                     []string      `config:"list_log_files"`
	CollectorBinariesWhitelist       []string      `config:"collector_binaries_whitelist"`
	CollectorBinariesAccesslist      []string      `config:"collector_binaries_accesslist"`
	Tags                             []string      `config:"tags"`
	WindowsDriveRange                string        `config:"windows_drive_range"`
}

func (config *SidecarConfig) InitDefaults() {
	config.ServerUrl = "http://127.0.0.1:9000/api/"
	config.ServerApiToken = ""
	config.TlsSkipVerify = false
	config.CollectorValidationTimeoutString = "1m"
	config.CollectorShutdownTimeoutString = "10s"
	config.LogRotateMaxFileSizeString = "10MiB"
	config.LogRotateKeepFiles = 10
	config.UpdateInterval = 10
	config.SendStatus = true
	config.ListLogFiles = []string{}
	config.Tags = []string{}
	// these unset values are overridden by the platform defaults, the rest are computed or required:
	// NodeId: contains platform dependent path
	// CachePath: contains platform dependent path
	// LogPath: contains platform dependent path
	// CollectorConfigurationDirectory: contains platform dependent path
	// CollectorBinariesAccesslist: contains platform dependent path
	// WindowsDriveRange: windows only

	withPlatformDefaults(config)
}
