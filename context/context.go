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
	"net/url"

	"github.com/kardianos/service"

	"github.com/Graylog2/collector-sidecar/daemon"
	"github.com/Graylog2/collector-sidecar/system"
	"github.com/Graylog2/collector-sidecar/util"
)

var log = util.Log()

type Ctx struct {
	ServerUrl         *url.URL
	NodeId            string
	CollectorId       string
	CollectorPath     string
	CollectorConfPath string
	LogPath           string
	Tags              []string
	Inventory         *system.Inventory
	Config            *daemon.Config
	Program           *daemon.Program
	Service           service.Service
}

func NewContext(serverUrl string, collectorPath string, collectorConfPath string, nodeId string, collectorId string, logPath string) *Ctx {
	dc := daemon.NewConfig(collectorPath, logPath)
	dp := daemon.NewProgram(dc)

	url, err := url.Parse(serverUrl)
	if err != nil {
		log.Fatal("Server-url is not valid", err)
	}

	if nodeId == "" {
		log.Fatal("Please provide a valid node-id")
	}

	return &Ctx{
		ServerUrl:         url,
		NodeId:            nodeId,
		CollectorId:       util.GetCollectorId(collectorId),
		CollectorPath:     collectorPath,
		CollectorConfPath: collectorConfPath,
		LogPath:           logPath,
		Inventory:         system.NewInventory(),
		Config:            dc,
		Program:           dp,
	}
}
