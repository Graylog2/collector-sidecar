package main

import (
	"flag"
	"os"
	"runtime"
	"path/filepath"

	"github.com/kardianos/service"
	"github.com/rakyll/globalconf"

	"github.com/Graylog2/sidecar/backends"
	"github.com/Graylog2/sidecar/context"
	"github.com/Graylog2/sidecar/services"
	"github.com/Graylog2/sidecar/system"
	"github.com/Graylog2/sidecar/util"

	// importing backend packages to ensure init() is called
	_ "github.com/Graylog2/sidecar/backends/nxlog"
)

var log = util.Log()

func main() {
	sidecarConfigurationFile := ""
	if runtime.GOOS == "windows" {
		sidecarConfigurationFile = filepath.Join(os.Getenv("APPDATA"), "sidecar", "sidecar.ini")
	} else {
		sidecarConfigurationFile = filepath.Join("/etc", "sidecar", "sidecar.ini")
	}
	if _, err := os.Stat(sidecarConfigurationFile); os.IsNotExist(err) {
		log. Error("Can not open sidecar configuration " + sidecarConfigurationFile)
		sidecarConfigurationFile = ""
	}

	// parse .ini file or use command line switches
	conf, _ := globalconf.NewWithOptions(&globalconf.Options{
		Filename:  sidecarConfigurationFile,
		EnvPrefix: "SIDECAR_",
	})

	var (
		svcFlagParam           = flag.String("service", "", "Control the system service")
		backendParam 	       = flag.String("backend", "nxlog", "Set the collector backend")
		collectorPathParam     = flag.String("collector-path", "/usr/bin/nxlog", "Path to collector installation")
		collectorConfPathParam = flag.String("collector-conf-path", "/etc/sidecar/generated/nxlog.conf", "File path to the rendered collector configuration")
		serverUrlParam         = flag.String("server-url", "http://127.0.0.1:12900", "Graylog server URL")
		nodeIdParam            = flag.String("node-id", "graylog-sidecar", "Collector identification string")
		collectorIdParam       = flag.String("collector-id", "file:/etc/sidecar/collector-id", "UUID used for collector registration")
		tagsParam              = flag.String("tags", "", "Comma separated tag list")
		logPathParam           = flag.String("log-path", "/var/log/sidecar", "Directory for collector output logs")
	)
	conf.ParseAll()

	expandedCollectorPath := util.ExpandPath(*collectorPathParam)
	expandedCollectorConfPath := util.ExpandPath(*collectorConfPathParam)
	expandedCollectorId := util.ExpandPath(*collectorIdParam)
	expandedLogPath := util.ExpandPath(*logPathParam)

	// initialize application context
	context := context.NewContext(*serverUrlParam,
				      expandedCollectorPath,
				      expandedCollectorConfPath,
				      *nodeIdParam,
				      expandedCollectorId,
				      expandedLogPath)

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
	if len(*svcFlagParam) != 0 {
		err := service.Control(s, *svcFlagParam)
		if err != nil {
			log.Info("Valid service actions:\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}

	// configure context
	context.Tags = util.SplitCommaList(*tagsParam)
	if len(context.Tags) != 0 {
		log.Info("Fetching configuration tagged by: ", context.Tags)
	}

	nxlog, err := backends.GetBackend(*backendParam)
	if err != nil {
		log.Fatal("Can not find backend, exiting.")
	}
	context.Backend = nxlog(expandedCollectorPath, context.CollectorId)

	if util.IsDir(expandedCollectorConfPath) {
		log.Fatal("Please provide full path to configuration file to render.")
	}

	// set backend related context values
	context.Config.Exec = context.Backend.ExecPath()
	context.Config.Args = context.Backend.ExecArgs(expandedCollectorConfPath)

	// expose system inventory to backend
	context.Backend.SetInventory(system.NewInventory())

	// bind service to context
	context.Program.BindToService(s)
	context.Service = s

	if context.CollectorId == "" {
		log.Fatal("No collector ID was configured, exiting!")
	}

	// start main loop
	services.StartPeriodicals(context)
	err = s.Run()
	if err != nil {
		log.Fatal(err)
	}
}
