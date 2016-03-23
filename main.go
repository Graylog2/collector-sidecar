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

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kardianos/service"
	"github.com/rakyll/globalconf"

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/services"
	"github.com/Graylog2/collector-sidecar/common"

	// importing backend packages to ensure init() is called
	_ "github.com/Graylog2/collector-sidecar/backends/nxlog"
	_ "github.com/Graylog2/collector-sidecar/backends/beats/topbeat"
)

var log = common.Log()

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: graylog-collector-sidecar [OPTIONS] [CONFIGURATION FILE]\n")
		if runtime.GOOS == "windows" {
			fmt.Fprintf(os.Stderr, "Default configuration path is C:\\\\Program Files (x86)\\graylog\\collector-sidecar\\collector_sidecar.ini\n")
		} else {
			fmt.Fprintf(os.Stderr, "Default configuration path is /etc/graylog/collector-sidecar/collector_sidecar.ini\n")
		}
		fmt.Fprintf(os.Stderr, "OPTIONS can be:\n")
		flag.PrintDefaults()
	}
	var (
		svcFlagParam           = flag.String("service", "", "Control the system service")
		backendParam           = flag.String("backend", "nxlog", "Set the collector backend")
		collectorPathParam     = flag.String("collector-path", "/usr/bin/nxlog", "Path to collector installation")
		collectorConfPathParam = flag.String("collector-conf-path", "/etc/graylog/collector-sidecar/generated/nxlog.conf", "File path to the rendered collector configuration")
		serverUrlParam         = flag.String("server-url", "http://127.0.0.1:12900", "Graylog server URL")
		nodeIdParam            = flag.String("node-id", "graylog-collector-sidecar", "Collector identification string")
		collectorIdParam       = flag.String("collector-id", "file:/etc/graylog/collector-sidecar/collector-id", "UUID used for collector registration")
		tagsParam              = flag.String("tags", "", "Comma separated tag list")
		logPathParam           = flag.String("log-path", "/var/log/graylog/collector-sidecar", "Directory for collector output logs")
	)

	flag.Parse() // dummy parse to access flag.Arg(n)
	sidecarConfigurationFile := flag.Arg(0)
	if sidecarConfigurationFile == "" {
		if runtime.GOOS == "windows" {
			sidecarConfigurationFile = filepath.Join("C:\\", "Program Files (x86)", "graylog", "collector-sidecar", "collector_sidecar.ini")
		} else {
			sidecarConfigurationFile = filepath.Join("/etc", "graylog", "collector-sidecar", "collector_sidecar.ini")
		}
	}
	if _, err := os.Stat(sidecarConfigurationFile); os.IsNotExist(err) {
		log.Error("Can not open collector-sidecar configuration " + sidecarConfigurationFile)
		sidecarConfigurationFile = ""
	}

	// parse .ini file or use command line switches
	conf, _ := globalconf.NewWithOptions(&globalconf.Options{
		Filename:  sidecarConfigurationFile,
		EnvPrefix: "COLLECTOR_SIDECAR_",
	})
	conf.ParseAll()

	expandedCollectorPath := common.ExpandPath(*collectorPathParam)
	expandedCollectorConfPath := common.ExpandPath(*collectorConfPathParam)
	expandedCollectorId := common.ExpandPath(*collectorIdParam)
	expandedLogPath := common.ExpandPath(*logPathParam)

	if common.IsDir(expandedCollectorConfPath) {
		log.Fatal("Please provide the full path to the configuration file to render.")
	}

	// initialize application context
	context := context.NewContext(*serverUrlParam,
		expandedCollectorPath,
		expandedCollectorConfPath,
		*nodeIdParam,
		expandedCollectorId,
		expandedLogPath)

	if context.CollectorId == "" {
		log.Fatal("No collector ID was configured, exiting!")
	}

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
	context.Tags = common.SplitCommaList(*tagsParam)
	if len(context.Tags) != 0 {
		log.Info("Fetching configurations tagged by: ", context.Tags)
	}

	backendCreator, err := backends.GetBackend(*backendParam)
	backend := backendCreator(context)

	// set backend related context values
	context.Config.Exec = backend.ExecPath()
	context.Config.Args = backend.ExecArgs(expandedCollectorConfPath)

	// bind service to context
	context.Program.BindToService(s)
	context.Service = s

	// start main loop
	backend.ValidatePreconditions()
	services.StartPeriodicals(context, backend)
	err = s.Run()
	if err != nil {
		log.Fatal(err)
	}
}
