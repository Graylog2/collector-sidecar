// Copyright (C)  2026 Graylog, Inc.
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
//
// SPDX-License-Identifier: SSPL-1.0

package supervisor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/open-telemetry/opentelemetry-collector-contrib/cmd/opampsupervisor/supervisor/config"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"
	"go.uber.org/zap/zapcore"
)

const exampleConfig = `---
server_url: "ws://localhost:9000/ws"
data_dir: "./data/supervisor"
dev: false
`

type Config struct {
	URL         string `koanf:"server_url"`
	Development bool   `koanf:"dev"`
	Sidecar     bool   `koanf:"sidecar"`
	DataDir     string `koanf:"data_dir"`
}

const envPrefix = "GLC_"

var envOpts = env.Opt{
	Prefix: envPrefix,
	TransformFunc: func(k, v string) (string, any) {
		return strings.ToLower(strings.TrimPrefix(k, envPrefix)), v
	},
}

func parseConfig(cmd *cobra.Command) (*Config, error) {
	k := koanf.New("::")

	if err := k.Load(env.Provider("::", envOpts), nil); err != nil {
		return nil, err
	}
	// Command line flags should overwrite env values.
	if err := k.Load(posflag.ProviderWithValue(cmd.Flags(), "::", k, func(key string, value string) (string, interface{}) {
		return strings.ReplaceAll(key, "-", "_"), value
	}), nil); err != nil {
		return nil, err
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func buildConfig(cmd *cobra.Command) (*config.Supervisor, error) {
	cfg, err := parseConfig(cmd)
	if err != nil {
		return nil, err
	}

	agentExecutable, err := filepath.Abs(os.Args[0])
	if err != nil {
		return nil, fmt.Errorf("couldn't get absolute path for %s", os.Args[0])
	}

	supervisorCfg := config.Supervisor{
		Server: config.OpAMPServer{
			Endpoint: cfg.URL,
			Headers:  nil,
			TLS:      configtls.ClientConfig{},
		},
		Agent: config.Agent{
			Executable:              agentExecutable,
			OrphanDetectionInterval: 5 * time.Second,
			Description: config.AgentDescription{
				IdentifyingAttributes:    nil,
				NonIdentifyingAttributes: nil,
			},
			ConfigApplyTimeout: 5 * time.Second,
			BootstrapTimeout:   3 * time.Second,
			OpAMPServerPort:    0,
			// The supervisor will otherwise write an ever-growing agent.log file into the storage dir. (no rotation/trunction)
			PassthroughLogs:    true,
			UseHUPConfigReload: runtime.GOOS != "windows", // HUP reload is not supported on Windows
			ConfigFiles: []string{
				string(config.SpecialConfigFileOwnTelemetry),
				string(config.SpecialConfigFileOpAMPExtension),
				string(config.SpecialConfigFileRemoteConfig),
			},
			Arguments: nil,
			Env:       nil,
		},
		Capabilities: config.Capabilities{
			//AcceptsRemoteConfig:            false,
			AcceptsRemoteConfig:            true,
			AcceptsRestartCommand:          false,
			AcceptsOpAMPConnectionSettings: false,
			ReportsEffectiveConfig:         true,
			//ReportsOwnMetrics:              true,
			ReportsOwnMetrics: false,
			ReportsOwnLogs:    false,
			ReportsOwnTraces:  false,
			ReportsHealth:     true,
			//ReportsRemoteConfig:        false,
			ReportsRemoteConfig: true,
			//ReportsAvailableComponents: false,
			ReportsAvailableComponents: true,
			ReportsHeartbeat:           true,
		},
		Storage: config.Storage{
			Directory: "", // Set below
		},
		Telemetry: config.Telemetry{
			Logs: config.Logs{
				Level:            zapcore.InfoLevel,
				OutputPaths:      []string{"stdout"},
				ErrorOutputPaths: []string{"stderr"},
				Processors:       nil,
			},
			Traces:   otelconftelemetry.TracesConfig{},
			Resource: nil,
		},
		HealthCheck: config.HealthCheck{
			ServerConfig: confighttp.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  "",
					Transport: confignet.TransportTypeTCP,
					DialerConfig: confignet.DialerConfig{
						Timeout: 0,
					},
				},
				TLS:                   configoptional.Optional[configtls.ServerConfig]{},
				CORS:                  configoptional.Optional[confighttp.CORSConfig]{},
				Auth:                  configoptional.Optional[confighttp.AuthConfig]{},
				MaxRequestBodySize:    0,
				IncludeMetadata:       false,
				ResponseHeaders:       nil,
				CompressionAlgorithms: nil,
				ReadTimeout:           0,
				ReadHeaderTimeout:     0,
				WriteTimeout:          0,
				IdleTimeout:           0,
				Middlewares:           nil,
				KeepAlivesEnabled:     false,
			},
		},
	}

	if cfg.DataDir == "" {
		// TODO: Branding!
		switch runtime.GOOS {
		case "windows":
			if cfg.Development {
				supervisorCfg.Storage.Directory = "./data/supervisor"
			} else {
				// Windows default is "%ProgramData%\Graylog-Collector\Supervisor"
				// If the ProgramData environment variable is not set,
				// it falls back to C:\ProgramData
				programDataDir := os.Getenv("ProgramData")
				if programDataDir == "" {
					programDataDir = `C:\ProgramData`
				}

				supervisorCfg.Storage.Directory = filepath.Join(programDataDir, "graylog", "collector", "supervisor")
			}
		default:
			if cfg.Development {
				supervisorCfg.Storage.Directory = "./data/supervisor"
			} else {
				supervisorCfg.Storage.Directory = "/var/lib/graylog/collector/supervisor"
			}
		}
	} else {
		supervisorCfg.Storage.Directory = cfg.DataDir
	}

	sidecarExtensionPath := filepath.Join(supervisorCfg.Storage.Directory, "sidecar-extension.yaml")
	if cfg.Sidecar {
		if err := writeSidecarExtensionConfig(sidecarExtensionPath, "sidecar.yml"); err != nil {
			return nil, err
		}
		supervisorCfg.Agent.ConfigFiles = append(supervisorCfg.Agent.ConfigFiles, sidecarExtensionPath)
	} else {
		if err := os.Remove(sidecarExtensionPath); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("couldn't remove unused Sidecar extension config file: %w", err)
			}
		}
	}

	commonConfigPath := filepath.Join(supervisorCfg.Storage.Directory, "common-config.yaml")
	if err := writeCommonConfig(commonConfigPath); err != nil {
		return nil, err
	}
	supervisorCfg.Agent.ConfigFiles = append(supervisorCfg.Agent.ConfigFiles, commonConfigPath)

	return &supervisorCfg, nil
}

func writeConfigFile(data string, path string) error {
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		return fmt.Errorf("couldn't write config file %q: %w", path, err)
	}
	return nil
}

func writeCommonConfig(commonConfigPath string) error {
	// Common collector config:
	// - We don't want a Prometheus listener on localhost:8888
	// TODO: Make configurable? Do we need it to report collector metrics upstream? Can we use a unix socket?
	content := fmt.Sprintf(`---
service:
  telemetry:
    metrics:
      level: "none"
`)

	if err := writeConfigFile(content, commonConfigPath); err != nil {
		return err
	}
	return nil
}

func writeSidecarExtensionConfig(sidecarExtensionPath string, sidecarConfigPath string) error {
	content := fmt.Sprintf(`---
extensions:
  sidecar:
    path: "%s"

service:
  extensions:
    - "sidecar"
`, sidecarConfigPath)

	if err := writeConfigFile(content, sidecarExtensionPath); err != nil {
		return err
	}
	return nil
}
