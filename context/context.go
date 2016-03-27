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

	"github.com/kardianos/service"

	"github.com/Graylog2/collector-sidecar/daemon"
	"github.com/Graylog2/collector-sidecar/system"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/cfgfile"
)

var log = common.Log()

type Ctx struct {
	ServerUrl	  *url.URL
	CollectorId       string
	CollectorPath     string
	CollectorConfPath string
	Inventory         *system.Inventory
	Program           *daemon.Program
	ProgramConfig     *daemon.Config
	Config 		  *cfgfile.SidecarConfig
	Service           service.Service
}

func NewContext(collectorPath string, collectorConfPath string) *Ctx {
	return &Ctx{
		CollectorPath:     collectorPath,
		CollectorConfPath: collectorConfPath,
		Inventory:         system.NewInventory(),
	}
}

func (ctx *Ctx)LoadConfig(path *string) error {
	err := cfgfile.Read(&ctx.Config, *path)
	if err != nil {
		return fmt.Errorf("loading config file error: %v\n", err)
	}

	ctx.ServerUrl, err = url.Parse(ctx.Config.ServerUrl)
	if err != nil {
		log.Fatal("Server-url is not valid", err)
	}

	if ctx.Config.NodeId == "" {
		log.Fatal("Please provide a valid node-id")
	}

	if ctx.Config.CollectorId == "" {
		log.Fatal("No collector ID was configured.")
	}
	ctx.CollectorId = common.GetCollectorId(ctx.Config.CollectorId)

	if len(ctx.Config.Tags) != 0 {
		log.Info("Fetching configurations tagged by: ", ctx.Config.Tags)
	}

	return nil
}

func (ctx *Ctx)NewBackend(collectorPath string) {
	//if common.IsDir(ctx.Config.*.ConfigurationPath) {
	//	log.Fatal("Please provide the full path to the configuration file to render.")
	//}

	if collectorPath != "" && ctx.Config.LogPath != "" {
		ctx.ProgramConfig = daemon.NewConfig(collectorPath, ctx.Config.LogPath)
		ctx.Program = daemon.NewProgram(ctx.ProgramConfig)
	} else {
		log.Fatal("Incomplete backend configuration, provide at least binary_path and log_path")
	}
}