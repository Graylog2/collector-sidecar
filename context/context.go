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

package context

import (
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/helpers"
	"github.com/docker/go-units"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/Graylog2/collector-sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/logger"
	"github.com/Graylog2/collector-sidecar/system"
)

var log = logger.Log()

type Ctx struct {
	ServerUrl  *url.URL
	NodeId     string
	NodeName   string
	UserConfig *cfgfile.SidecarConfig
	Inventory  *system.Inventory
}

func NewContext() *Ctx {
	return &Ctx{
		Inventory: system.NewInventory(),
	}
}

func (ctx *Ctx) LoadConfig(path *string) error {
	err := cfgfile.Read(&ctx.UserConfig, *path)
	if err != nil {
		return err
	}

	// Process top-level configuration
	// server_url
	ctx.ServerUrl, err = url.Parse(ctx.UserConfig.ServerUrl)
	if err != nil || ctx.ServerUrl.Scheme == "" || ctx.ServerUrl.Host == "" {
		log.Fatal("Server-url is not valid. Should be like http://127.0.0.1:9000/api/ ", err)
	}
	if ctx.UserConfig.ServerUrl == "" {
		log.Fatalf("Server-url is empty.")
	}

	// api_token
	if ctx.UserConfig.ServerApiToken == "" {
		log.Fatal("No API token was configured.")
	}

	// node_id
	if ctx.UserConfig.NodeId == "" {
		log.Fatal("No node ID was configured.")
	}
	ctx.NodeId = helpers.GetCollectorId(ctx.UserConfig.NodeId)
	if ctx.NodeId == "" {
		log.Fatal("Empty node-id, exiting! Make sure a valid id is configured.")
	}

	// node_name
	if ctx.UserConfig.NodeName == "" {
		log.Info("No node name was configured, falling back to hostname")
		ctx.UserConfig.NodeName, err = helpers.GetHostname()
		if err != nil {
			log.Fatal("No node name configured and not able to obtain hostname as alternative.")
		}
	}
	ctx.NodeName = ctx.UserConfig.NodeName

	// cache_path
	if ctx.UserConfig.CachePath == "" {
		var cachePath string
		if runtime.GOOS == "windows" {
			cachePath = filepath.Join(os.Getenv("SystemDrive")+"\\", "Program Files", "graylog", "sidecar", "cache")
		} else {
			cachePath = filepath.Join("/var", "cache", "graylog-sidecar")
		}
		ctx.UserConfig.CachePath = cachePath
		log.Errorf("No cache directory was configured. Using default: %s", cachePath)
	}

	// log_path
	if ctx.UserConfig.LogPath == "" {
		log.Fatal("No log directory was configured.")
	}

	// collector_validation_timeout
	if ctx.UserConfig.CollectorValidationTimeoutString == "" {
		log.Fatal("No collector validation timeout was configured.")
	}
	ctx.UserConfig.CollectorValidationTimeout, err = time.ParseDuration(ctx.UserConfig.CollectorValidationTimeoutString)
	if err != nil {
		log.Fatal("Cannot parse validation timeout duration: ", err)
	}
	ctx.UserConfig.CollectorShutdownTimeout, err = time.ParseDuration(ctx.UserConfig.CollectorShutdownTimeoutString)
	if err != nil {
		log.Fatal("Cannot parse shutdown timeout duration: ", err)
	}

	// collector_configuration_directory
	if ctx.UserConfig.CollectorConfigurationDirectory == "" {
		log.Fatal("No collector configuration directory was configured.")
	}
	if !filepath.IsAbs(ctx.UserConfig.CollectorConfigurationDirectory) {
		log.Fatal("Collector configuration directory must be an absolute path.")
	}
	err = os.MkdirAll(ctx.UserConfig.CollectorConfigurationDirectory, 0750)
	if err != nil {
		log.Fatal("Failed to create collector configuration directory. ", err)
	}

	// log_rotate_max_file_size
	if ctx.UserConfig.LogRotateMaxFileSizeString == "" {
		log.Fatal("Please set the maximum log rotation size.")
	}
	ctx.UserConfig.LogRotateMaxFileSize, err = units.RAMInBytes(ctx.UserConfig.LogRotateMaxFileSizeString)
	if err != nil {
		log.Fatal("Cannot parse maximum log rotation size: ", err)
	}
	if ctx.UserConfig.LogRotateMaxFileSize < 1*units.MiB {
		log.Fatalf("Maximum log rotation size (%s) needs to be at least 1MiB",
			units.HumanSize(float64(ctx.UserConfig.LogRotateMaxFileSize)))
	}

	// log_rotate_keep_files
	if !(ctx.UserConfig.LogRotateKeepFiles > 0) {
		log.Fatal("Please set the maximum number of logfiles to retain.")
	}

	// list log files
	if len(ctx.UserConfig.ListLogFiles) > 0 {
		for _, dir := range ctx.UserConfig.ListLogFiles {
			if !common.IsDir(dir) {
				log.Fatal("Please provide a list of directories for list_log_files.")
			}
		}
	}

	// update_interval
	if !(ctx.UserConfig.UpdateInterval > 0) {
		log.Fatal("Please set update interval > 0 seconds.")
	}

	// collector binary accesslist
	if ctx.UserConfig.CollectorBinariesAccesslist == nil && ctx.UserConfig.CollectorBinariesWhitelist == nil {
		log.Fatal("`collector_binaries_accesslist` is not set. Explicitly allow to execute all binaries by setting it to an empty list" +
			" or limit the execution by defining proper values.")
	}

	if ctx.UserConfig.CollectorBinariesWhitelist != nil {
		log.Warn("`collector_binaries_whitelist` is deprecated. Migrate your configuration to `collector_binaries_accesslist`.")
		ctx.UserConfig.CollectorBinariesAccesslist = ctx.UserConfig.CollectorBinariesWhitelist
	}

	// windows_drive_range
	driveRangeValid, _ := regexp.MatchString("^[A-Z]*$", ctx.UserConfig.WindowsDriveRange)
	if !driveRangeValid {
		log.Fatal("`windows_drive_range` must only contain valid windows drive letters in the range A-Z or left empty.")
	}

	return nil
}
