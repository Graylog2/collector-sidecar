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
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/daemon"
	"github.com/Graylog2/collector-sidecar/services"

	// importing backend packages to ensure init() is called
	_ "github.com/Graylog2/collector-sidecar/backends/beats/topbeat"
	_ "github.com/Graylog2/collector-sidecar/backends/nxlog"
	_ "github.com/Graylog2/collector-sidecar/daemon"
)

var (
	log               = common.Log()
	printVersion      *bool
	serviceParam      *string
	configurationFile *string
)

func init() {
	var configurationPath string
	if runtime.GOOS == "windows" {
		configurationPath = filepath.Join("C:\\", "Program Files (x86)", "graylog", "collector-sidecar", "collector_sidecar.yml")
	} else {
		configurationPath = filepath.Join("/etc", "graylog", "collector-sidecar", "collector_sidecar.yml")
	}

	serviceParam = flag.String("service", "", "Control the system service [start stop restart install uninstall]")
	configurationFile = flag.String("c", configurationPath, "Configuration file")
	printVersion = flag.Bool("version", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: graylog-collector-sidecar -c [CONFIGURATION FILE]\n")
		flag.PrintDefaults()
	}

}

func main() {
	if commandLineSetup() {
		return
	}

	// setup system service
	serviceConfig := &service.Config{
		Name:        daemon.Daemon.Name,
		DisplayName: daemon.Daemon.DisplayName,
		Description: daemon.Daemon.Description,
	}

	supervisor := daemon.Daemon.NewSupervisor()
	s, err := service.New(supervisor, serviceConfig)
	if err != nil {
		log.Fatal(err)
	}
	supervisor.BindToService(s)

	if len(*serviceParam) != 0 {
		err := service.Control(s, *serviceParam)
		if err != nil {
			log.Info("Valid service actions:\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}

	// initialize application context
	context := context.NewContext()
	context.LoadConfig(configurationFile)
	backendSetup(context)

	// start main loop
	services.StartPeriodicals(context)
	err = s.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func commandLineSetup() bool {
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

func backendSetup(context *context.Ctx) {
	for _, collector := range context.UserConfig.Backends {
		backendCreator, err := backends.GetCreator(collector.Name)
		if err != nil {
			log.Error("Unsupported collector backend found in configuration: " + collector.Name)
			continue
		}
		backend := backendCreator(context)
		backends.Store.AddBackend(backend)
		if *collector.Enabled == true && backend.ValidatePreconditions() {
			log.Debug("Add collector backend: " + backend.Name())
			daemon.Daemon.AddBackendAsRunner(backend, context)
		}
	}

}
