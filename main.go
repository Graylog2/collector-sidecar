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

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/services"
	"github.com/Graylog2/collector-sidecar/common"

	// importing backend packages to ensure init() is called
	_ "github.com/Graylog2/collector-sidecar/backends/nxlog"
	_ "github.com/Graylog2/collector-sidecar/backends/beats/topbeat"
)

var (
	log = common.Log()
	printVersion *bool
	serviceParam *string
	configurationFile *string
)

func init() {
	var configurationPath string
	if runtime.GOOS == "windows" {
		configurationPath = filepath.Join("C:\\", "Program Files (x86)", "graylog", "collector-sidecar", "collector_sidecar.yml")
	} else {
		configurationPath = filepath.Join("/etc", "graylog", "collector-sidecar", "collector_sidecar.yml")
	}

	serviceParam = flag.String("service", "", "Control the system service")
	configurationFile = flag.String("c", configurationPath, "Configuration file")
	printVersion = flag.Bool("version", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: graylog-collector-sidecar [OPTIONS] -c [CONFIGURATION FILE]\n")
		fmt.Fprintf(os.Stderr, "OPTIONS can be:\n")
		flag.PrintDefaults()
	}

}

func main() {
	var (
		backendParam           = flag.String("backend", "nxlog", "Set the collector backend")
		collectorPathParam     = flag.String("collector-path", "/usr/bin/nxlog", "Path to collector installation")
		collectorConfPathParam = flag.String("collector-conf-path", "/etc/graylog/collector-sidecar/generated/nxlog.conf", "File path to the rendered collector configuration")
	)
	if CommandLineSetup() {
		os.Exit(0)
	}

	expandedCollectorPath := common.ExpandPath(*collectorPathParam)
	expandedCollectorConfPath := common.ExpandPath(*collectorConfPathParam)

	// initialize application context
	context := context.NewContext(
		expandedCollectorPath,
		expandedCollectorConfPath)
	context.LoadConfig(configurationFile)
	context.NewBackend(expandedCollectorPath)

	// setup system service
	serviceConfig := &service.Config{
		Name:        context.ProgramConfig.Name,
		DisplayName: context.ProgramConfig.DisplayName,
		Description: context.ProgramConfig.Description,
	}

	s, err := service.New(context.Program, serviceConfig)
	if err != nil {
		log.Fatal(err)
	}
	if len(*serviceParam) != 0 {
		err := service.Control(s, *serviceParam)
		if err != nil {
			log.Info("Valid service actions:\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}

	backendCreator, err := backends.GetBackend(*backendParam)
	backend := backendCreator(context)

	// set backend related context values
	context.ProgramConfig.Exec = backend.ExecPath()
	context.ProgramConfig.Args = backend.ExecArgs(expandedCollectorConfPath)

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

func CommandLineSetup() bool {
	flag.Parse()

	if *printVersion {
		fmt.Printf("Graylog Collector Sidecar version %s (%s)\n", common.CollectorVersion, runtime.GOARCH)
		return true
	}

	if _, err := os.Stat(*configurationFile); os.IsNotExist(err) {
		log.Fatal("Can not open collector-sidecar configuration " + *configurationFile)
	}

	return false
}

