package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/kardianos/service"
	"github.com/rakyll/globalconf"

	"github.com/Graylog2/sidecar/backends"
	"github.com/Graylog2/sidecar/context"
	"github.com/Graylog2/sidecar/services"
	"github.com/Graylog2/sidecar/system"
	"github.com/Graylog2/sidecar/util"

	// importing backend packages to ensure init() is called
	_ "github.com/Graylog2/sidecar/backends/nxlog"
	"runtime"
)

func main() {
	sidecarConfigurationFile := ""
	if runtime.GOOS == "windows" {
		sidecarConfigurationFile = filepath.Join("%APPDATA%", "sidecar.ini")
	} else {
		sidecarConfigurationFile = filepath.Join("/etc", "sidecar", "sidecar.ini")
	}
	if _, err := os.Stat(sidecarConfigurationFile); os.IsNotExist(err) {
		logrus.Error("Can not open sidecar configuration " + sidecarConfigurationFile)
		sidecarConfigurationFile = ""
	}

	// parse .ini file or use command line switches
	conf, _ := globalconf.NewWithOptions(&globalconf.Options{
		Filename:  sidecarConfigurationFile,
		EnvPrefix: "SIDECAR_",
	})

	var (
		svcFlag           = flag.String("service", "", "Control the system service")
		collectorPath     = flag.String("collector-path", "/usr/bin/nxlog", "Path to collector installation")
		collectorConfPath = flag.String("collector-conf-path", "/tmp/nxlog.conf", "File path to the rendered collector configuration")
		serverUrl         = flag.String("server-url", "http://127.0.0.1:12900", "Graylog server URL")
		nodeId            = flag.String("node-id", "graylog-sidecar", "Collector identification string")
		collectorId       = flag.String("collector-id", "", "UUID used for collector registration")
		tags              = flag.String("tags", "", "Comma separated tag list")
		logPath           = flag.String("log-path", "/var/log/sidecar", "Path to log directory")
	)
	conf.ParseAll()

	// initialize application context
	context := context.NewContext(*serverUrl, *collectorPath, *collectorConfPath, *nodeId, *collectorId, *logPath)

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
	if len(*svcFlag) != 0 {
		err := service.Control(s, *svcFlag)
		if err != nil {
			logrus.Info("Valid service actions:\n", service.ControlAction)
			logrus.Fatal(err)
		}
		return
	}

	// configure context
	context.Tags = util.SplitCommaList(*tags)
	if len(context.Tags) != 0 {
		logrus.Info("Fetching configuration tagged by: ", context.Tags)
	}

	nxlog, err := backends.GetBackend("nxlog")
	if err != nil {
		logrus.Fatal("Can not find backend, exiting.")
	}
	context.Backend = nxlog(*collectorPath)

	// set backend related context values
	context.Config.Exec = context.Backend.ExecPath()
	context.Config.Args = context.Backend.ExecArgs(*collectorConfPath)

	// expose system inventory to backend
	context.Backend.SetInventory(system.NewInventory())

	// bind service to context
	context.Program.BindToService(s)
	context.Service = s

	if context.CollectorId == "" {
		logrus.Fatal("No collector ID was configured, exiting!")
	}

	// start main loop
	services.StartPeriodicals(context)
	err = s.Run()
	if err != nil {
		logrus.Fatal(err)
	}
}
