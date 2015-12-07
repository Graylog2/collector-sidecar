package daemon

import (
	"mariussturm/gxlog/backends/nxlog/configuration"

	"github.com/kardianos/service"
)

type Ctx struct {
	GlServer string
	GlPort   int
	NxPath   string
	*Config
	*Program
	service.Service
	*nxlog.NxConfig
}

func Context(glServer string, glPort int, nxPath string) *Ctx {
	dc := NewConfig(glServer, glPort, nxPath)
	dp := NewProgram(dc)
	nxc := nxlog.NewNxConfig(glServer, glPort, nxPath)

	return &Ctx{
		GlServer: glServer,
		GlPort:   glPort,
		NxPath:   nxPath,
		Config:   dc,
		Program:  dp,
		NxConfig: nxc,
	}
}
