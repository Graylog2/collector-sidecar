package daemon

import (
	"net/url"

	"github.com/Sirupsen/logrus"
	"github.com/kardianos/service"

	"mariussturm/gxlog/backends/nxlog/configuration"
)

type Ctx struct {
	ServerUrl *url.URL
	NxPath    string
	*Config
	*Program
	service.Service
	*nxlog.NxConfig
}

func Context(serverUrl string, nxPath string) *Ctx {
	dc := NewConfig(nxPath)
	dp := NewProgram(dc)
	nxc := nxlog.NewNxConfig(nxPath)
	url, err := url.Parse(serverUrl)
	if err != nil {
		logrus.Fatal("server-url is not valid", err)
	}

	return &Ctx{
		ServerUrl: url,
		NxPath:    nxPath,
		Config:    dc,
		Program:   dp,
		NxConfig:  nxc,
	}
}
