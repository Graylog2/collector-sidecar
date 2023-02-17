// Copyright (C) 2020 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kardianos/service"
	"github.com/sirupsen/logrus"

	"github.com/Graylog2/collector-sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/daemon"
	"github.com/Graylog2/collector-sidecar/logger"
	"github.com/Graylog2/collector-sidecar/logger/hooks"
	"github.com/Graylog2/collector-sidecar/services"

	// importing backend packages to ensure init() is called
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
		configurationPath = filepath.Join(os.Getenv("SystemDrive")+"\\", "Program Files", "graylog", "sidecar", "sidecar.yml")
	} else {
		configurationPath = filepath.Join("/etc", "graylog", "sidecar", "sidecar.yml")
	}

	serviceParam = flag.String("service", "", "Control the system service [start stop restart install uninstall]")
	configurationFile = flag.String("c", configurationPath, "Configuration file")
	printVersion = flag.Bool("version", false, "Print version and exit")
	debug = flag.Bool("debug", false, "Set log level to debug")

	flag.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: graylog-sidecar -c [CONFIGURATION FILE]\n")
		flag.PrintDefaults()
	}

}

func main() {
	err := commandLineSetup()
	if err != nil {
		log.Fatalln(err)
	}

	// setup system service
	serviceConfig := &service.Config{
		Name:        daemon.Daemon.Name,
		DisplayName: daemon.Daemon.DisplayName,
		Description: daemon.Daemon.Description,
		Option:      services.ServiceOptions(),
	}

	distributor := daemon.Daemon.NewDistributor()
	s, err := service.New(distributor, serviceConfig)
	if err != nil {
		log.Fatalf("Operating system is not supported: %v", err)
	}
	distributor.BindToService(s)

	if len(*serviceParam) != 0 {
		services.ControlHandler(*serviceParam)
		err := service.Control(s, *serviceParam)
		if err != nil {
			log.Fatalf("Failed service action: %v", err)
		}
		return
	}

	// initialize application context
	ctx := context.NewContext()
	err = ctx.LoadConfig(configurationFile)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
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
		return errors.New("Unable to open configuration file: " + *configurationFile)
	}

	return nil
}
