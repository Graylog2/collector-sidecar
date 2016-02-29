package context

import (
	"net/url"

	"github.com/kardianos/service"

	"github.com/Graylog2/sidecar/backends"
	"github.com/Graylog2/sidecar/daemon"
	"github.com/Graylog2/sidecar/util"
)

var log = util.Log()

type Ctx struct {
	ServerUrl         *url.URL
	NodeId            string
	CollectorId       string
	CollectorConfPath string
	Tags              []string
	Config            *daemon.Config
	Program           *daemon.Program
	Service           service.Service
	Backend           backends.Backend
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
		CollectorConfPath: collectorConfPath,
		Config:            dc,
		Program:           dp,
	}
}
