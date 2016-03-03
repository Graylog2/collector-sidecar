package context

import (
	"net/url"

	"github.com/kardianos/service"

	"github.com/Graylog2/sidecar/daemon"
	"github.com/Graylog2/sidecar/system"
	"github.com/Graylog2/sidecar/util"
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
		Inventory: 	   system.NewInventory(),
		Config:            dc,
		Program:           dp,
	}
}
