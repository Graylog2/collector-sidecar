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

	"github.com/Graylog2/collector-sidecar/system"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/cfgfile"
)

var log = common.Log()

type Ctx struct {
	ServerUrl	  *url.URL
	CollectorId       string
	UserConfig 	  *cfgfile.SidecarConfig
	Inventory         *system.Inventory
}

func NewContext() *Ctx {
	return &Ctx{
		Inventory:         system.NewInventory(),
	}
}

func (ctx *Ctx) LoadConfig(path *string) error {
	err := cfgfile.Read(&ctx.UserConfig, *path)
	if err != nil {
		return fmt.Errorf("loading config file error: %v\n", err)
	}

	// Process top-level configuration
	ctx.ServerUrl, err = url.Parse(ctx.UserConfig.ServerUrl)
	if err != nil {
		log.Fatal("Server-url is not valid", err)
	}

	if ctx.UserConfig.CollectorId == "" {
		log.Fatal("No collector ID was configured.")
	}
	ctx.CollectorId = common.GetCollectorId(ctx.UserConfig.CollectorId)

	if ctx.UserConfig.NodeId == "" {
		log.Fatal("Please provide a valid node-id")
	}

	if len(ctx.UserConfig.Tags) == 0 {
		log.Fatal("Please define configuration tags")
	} else {
		log.Info("Fetching configurations tagged by: ", ctx.UserConfig.Tags)
	}

	return nil
}