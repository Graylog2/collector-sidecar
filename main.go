package main

import (
	"flag"
	"log"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/kardianos/service"
	"github.com/rakyll/globalconf"

	"github.com/Graylog2/nxlog-sidecar/context"
	"github.com/Graylog2/nxlog-sidecar/services"
	"github.com/Graylog2/nxlog-sidecar/util"
)

func main() {
	gxlogPath, err := util.GetGxlogPath()
	if err != nil {
		logrus.Fatal("Can not find path to Gxlog installation.")
	}

	// parse .ini file or use command line switches
	conf, _ := globalconf.NewWithOptions(&globalconf.Options{
		Filename:  filepath.Join(gxlogPath, "gxlog.ini"),
		EnvPrefix: "GXLOG_",
	})

	var (
		svcFlag     = flag.String("service", "", "Control the system service.")
		nxlogPath   = flag.String("nxlog-path", "", "Path to nxlog installation")
		serverUrl   = flag.String("server-url", "", "Graylog server URL")
		nodeId      = flag.String("node-id", "graylog-collector", "Collector identification string")
		collectorId = flag.String("collector-id", "", "UUID used for collector registration")
	)
	conf.ParseAll()

	// initilaize application context
	context := context.NewContext(*serverUrl, *nxlogPath, *nodeId, *collectorId)

	// setup system service
	serviceConfig := &service.Config{
		Name:        context.Config.Name,
		DisplayName: context.Config.DisplayName,
		Description: context.Config.Description,
	}

	s, err := service.New(context.Program, serviceConfig)
	if err != nil {
		log.Fatal(err)
	}
	context.Program.BindToService(s)
	context.Service = s

	if len(*svcFlag) != 0 {
		err := service.Control(s, *svcFlag)
		if err != nil {
			logrus.Info("Valid service actions: %q\n", service.ControlAction)
			logrus.Fatal(err)
		}
		return
	}

	// start main loop
	services.StartPeriodicals(context)
	err = s.Run()
	if err != nil {
		logrus.Fatal(err)
	}
}
