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

package context

import (
	"fmt"
	"net/url"
	"runtime"
	"path/filepath"
	"os"

	"github.com/Graylog2/collector-sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/system"
	"github.com/Graylog2/collector-sidecar/logger"
)

var log = logger.Log()

type Ctx struct {
	ServerUrl   *url.URL
	NodeId 		string
	NodeName    string
	UserConfig  *cfgfile.SidecarConfig
	Inventory   *system.Inventory
}

func NewContext() *Ctx {
	return &Ctx{
		Inventory: system.NewInventory(),
	}
}

func (ctx *Ctx) LoadConfig(path *string) error {
	err := cfgfile.Read(&ctx.UserConfig, *path)
	if err != nil {
		return fmt.Errorf("loading config file error: %v\n", err)
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

	// node_id
	if ctx.UserConfig.NodeId == "" {
		log.Fatal("No node ID was configured.")
	}
	ctx.NodeId = common.GetCollectorId(ctx.UserConfig.NodeId)

	// node_name
	if ctx.UserConfig.NodeName == "" {
		log.Info("No node name was configured, falling back to hostname")
		ctx.UserConfig.NodeName, err = common.GetHostname()
		if err != nil {
			log.Fatal("No node name configured and not able to obtain hostname as alternative.")
		}
	}
        ctx.NodeName = ctx.UserConfig.NodeName

	// cache_path
	if ctx.UserConfig.CachePath == "" {
		var cachePath string
		if runtime.GOOS == "windows" {
			cachePath = filepath.Join(os.Getenv("SystemDrive")+"\\", "Program Files", "graylog", "collector-sidecar", "cache")
		} else {
			cachePath = filepath.Join("/var", "cache", "graylog", "collector-sidecar")
		}
		ctx.UserConfig.CachePath = cachePath
		log.Errorf("No cache directory was configured. Using default: %s", cachePath)
	}

	// log_path
	if ctx.UserConfig.LogPath == "" {
		log.Fatal("No log directory was configured.")
	}

	// log_rotation_time
	if !(ctx.UserConfig.LogRotationTime > 0) {
		log.Fatal("Please set the log rotation time > 0 seconds.")
	}

	// log_max_age
	if !(ctx.UserConfig.LogMaxAge > 0) {
		log.Fatal("Please set the maximum age of log file rotation > 0 seconds.")
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

	return nil
}
