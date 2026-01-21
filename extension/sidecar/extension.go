package sidecar

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Graylog2/collector-sidecar/extension/sidecar/cfg"
	"github.com/Graylog2/collector-sidecar/extension/sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/extension/sidecar/daemon"
	"github.com/Graylog2/collector-sidecar/extension/sidecar/logger/hooks"
	"github.com/Graylog2/collector-sidecar/extension/sidecar/services"
	"github.com/kardianos/service"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

var _ extension.Extension = (*sidecarExtension)(nil)

// SidecarExtension is an OpenTelemetry Collector extension that integrates Graylog Config functionality.
type sidecarExtension struct {
	config *Config
	svc    service.Service
}

func (sce *sidecarExtension) Start(ctx context.Context, host component.Host) error {
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
		return fmt.Errorf("unsupported operating system: %w", err)
	}
	sce.svc = s
	distributor.BindToService(s)

	// TODO
	//if len(*serviceParam) != 0 {
	//	services.ControlHandler(*serviceParam)
	//	err := service.Control(s, *serviceParam)
	//	if err != nil {
	//		log.Fatalf("Failed service action: %v", err)
	//	}
	//	return
	//}

	configurationFile := sce.config.Path

	if configurationFile == "" {
		if runtime.GOOS == "windows" {
			configurationFile = filepath.Join(os.Getenv("SystemDrive")+"\\", "Program Files", "graylog", "sidecar", "sidecar.yml")
		} else {
			configurationFile = filepath.Join("/etc", "graylog", "sidecar", "sidecar.yml")
		}
	}

	// initialize application context
	config := cfg.NewConfig()
	err = config.LoadConfig(&configurationFile)
	if err != nil {
		return fmt.Errorf("loading configuration file: %w", err)
	} else {
		// Persist path for later reloads
		cfgfile.SetConfigPath(configurationFile)
	}
	// TODO
	//if cfgfile.ValidateConfig() {
	//	// if ctx.LoadConfig didn't fail already print message and exit
	//	fmt.Println("ExtensionConfig OK")
	//	return
	//}

	// setup logging
	if *debug {
		log.Level = logrus.DebugLevel
	} else {
		log.Level = logrus.InfoLevel
	}
	hooks.AddLogHooks(config, log)

	// start main loop
	services.StartPeriodicals(config)
	if err = s.Start(); err != nil {
		return fmt.Errorf("starting sidecar service: %w", err)
	}
	return nil
}

func (sce *sidecarExtension) Shutdown(ctx context.Context) error {
	if sce.svc != nil {
		if err := sce.svc.Stop(); err != nil {
			return fmt.Errorf("stopping sidecar service: %w", err)
		}
	}
	return nil
}
