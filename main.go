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
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kardianos/service"
	"github.com/Sirupsen/logrus"

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/daemon"
	"github.com/Graylog2/collector-sidecar/logger"
	"github.com/Graylog2/collector-sidecar/logger/hooks"
	"github.com/Graylog2/collector-sidecar/services"

	// importing backend packages to ensure init() is called
	_ "github.com/Graylog2/collector-sidecar/backends/beats/filebeat"
	_ "github.com/Graylog2/collector-sidecar/backends/beats/winlogbeat"
	_ "github.com/Graylog2/collector-sidecar/backends/nxlog"
	_ "github.com/Graylog2/collector-sidecar/daemon"
)

var (
	log               = logger.Log()
	printVersion      *bool
	debug             *bool
	serviceParam      *string
	configurationFile *string
)

func init() {
	var configurationPath string
	if runtime.GOOS == "windows" {
		configurationPath = filepath.Join(os.Getenv("SystemDrive")+"\\", "Program Files", "graylog", "collector-sidecar", "collector_sidecar.yml")
	} else {
		configurationPath = filepath.Join("/etc", "graylog", "collector-sidecar", "collector_sidecar.yml")
	}

	serviceParam = flag.String("service", "", "Control the system service [start stop restart install uninstall]")
	configurationFile = flag.String("c", configurationPath, "Configuration file")
	printVersion = flag.Bool("version", false, "Print version and exit")
	debug = flag.Bool("debug", false, "Set log level to debug")

	flag.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: graylog-collector-sidecar -c [CONFIGURATION FILE]\n")
		flag.PrintDefaults()
	}

}

func main() {
	err := commandLineSetup()
	if err != nil {
		fmt.Println(err)
		return
	}

	// setup system service
	serviceConfig := &service.Config{
		Name:        daemon.Daemon.Name,
		DisplayName: daemon.Daemon.DisplayName,
		Description: daemon.Daemon.Description,
	}

	distributor := daemon.Daemon.NewDistributor()
	s, err := service.New(distributor, serviceConfig)
	if err != nil {
		fmt.Printf("Operating system is not supported: %v", err)
		return
	}
	distributor.BindToService(s)

	if len(*serviceParam) != 0 {
		services.ControlHandler(*serviceParam)
		err := service.Control(s, *serviceParam)
		if err != nil {
			fmt.Printf("Failed service action: %v", err)
		}
		return
	}

	// initialize application context
	ctx := context.NewContext()
	err = ctx.LoadConfig(configurationFile)
	if err != nil {
		fmt.Println("Loading configuration file failed.")
		return
	} else {
		// Persist path for later reloads
		cfgfile.SetConfigPath(*configurationFile)
	}
	if cfgfile.ValidateConfig() {
		// if ctx.LoadConfig didn't fail already print message and exit
		fmt.Println("Config OK")
		return
	}

	// setup logging
	if *debug {
		log.Level = logrus.DebugLevel
	} else {
		log.Level = logrus.InfoLevel
	}
	hooks.AddLogHooks(ctx, log)

	// initialize backends
	backendSetup(ctx)

	// start main loop
	services.StartPeriodicals(ctx)
	err = s.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func commandLineSetup() error {
	flag.Parse()

	if *printVersion {
		fmt.Printf("Graylog Collector Sidecar version %s%s (%s) [%s/%s]\n",
			common.CollectorVersion,
			common.CollectorVersionSuffix,
			common.GitRevision,
			runtime.Version(),
			runtime.GOARCH)
		os.Exit(0)
	}

	if _, err := os.Stat(*configurationFile); os.IsNotExist(err) {
		return errors.New("Can not open collector-sidecar configuration " + *configurationFile)
	}

	return nil
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
			daemon.Daemon.AddBackend(backend, context)
		}
	}

}
