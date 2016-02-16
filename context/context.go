package context

import (
	"net/url"

	"github.com/Sirupsen/logrus"
	"github.com/kardianos/service"

	"github.com/Graylog2/sidecar/backends"
	"github.com/Graylog2/sidecar/daemon"
	"github.com/Graylog2/sidecar/util"
)

type Ctx struct {
	ServerUrl   *url.URL
	NodeId      string
	CollectorId string
	Tags	    []string
	Config      *daemon.Config
	Program     *daemon.Program
	Service     service.Service
	Backend     backends.Backend
}

func NewContext(serverUrl string, collectorPath string, nodeId string, collectorId string) *Ctx {
	dc := daemon.NewConfig(collectorPath)
	dp := daemon.NewProgram(dc)

	url, err := url.Parse(serverUrl)
	if err != nil {
		logrus.Fatal("server-url is not valid", err)
	}

	if nodeId == "" {
		logrus.Fatal("please provide a valid node-id")
	}

	return &Ctx{
		ServerUrl:   url,
		NodeId:      nodeId,
		CollectorId: util.GetCollectorId(collectorId),
		Config:      dc,
		Program:     dp,
	}
}
