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
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/configmanager"
	"github.com/Graylog2/collector-sidecar/superv/keen"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCreateOpAMPCallbacks_OnRemoteConfig_DoesNotRollbackOnShutdownCancellation(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "collector.yaml")
	require.NoError(t, os.WriteFile(outputPath, []byte("receivers:\n  nop:\n"), 0o600))

	mgr := configmanager.New(logger.Named("config"), configmanager.Config{
		ConfigDir:     tmpDir,
		OutputPath:    outputPath,
		LocalEndpoint: "ws://localhost:4320/v1/opamp",
		InstanceUID:   "test-instance",
	})

	executable := "/bin/sleep"
	args := []string{"30"}
	if runtime.GOOS == "windows" {
		executable = "powershell"
		args = []string{"-NoProfile", "-Command", "Start-Sleep -Seconds 30"}
	}

	cmd, err := keen.New(logger, t.TempDir(), keen.Config{
		Executable: executable,
		Args:       args,
	}, keen.NewBackoff(keen.BackoffConfig{}))
	require.NoError(t, err)

	s := &Supervisor{
		logger:        logger,
		configManager: mgr,
		commander:     cmd,
		agentCfg: config.AgentConfig{
			ConfigApplyTimeout: 5 * time.Second,
			Health: config.HealthConfig{
				StartupGracePeriod: time.Minute,
			},
		},
		ctx: ctx,
	}

	callbackCtx, callbackCancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)

	go func() {
		done <- s.handleRemoteConfig(callbackCtx, &protobufs.AgentRemoteConfig{
			Config: &protobufs.AgentConfigMap{
				ConfigMap: map[string]*protobufs.AgentConfigFile{
					"collector.yaml": {
						Body: []byte(`receivers:
  otlp:
    protocols:
      grpc:

exporters:
  debug:

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
`),
						ContentType: "text/yaml",
					},
				},
			},
			ConfigHash: []byte("hash-1"),
		})
	}()

	require.Eventually(t, func() bool {
		return observed.FilterMessage("Waiting for startup grace period before health polling").Len() == 1
	}, time.Second, 10*time.Millisecond)

	cancel()
	callbackCancel()

	require.False(t, <-done)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	require.NoError(t, cmd.Stop(stopCtx))

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	require.Contains(t, string(content), "otlp")
	require.NotContains(t, string(content), "receivers:\n  nop:\n")
	require.FileExists(t, outputPath+".prev")

	require.Equal(t, 1, observed.FilterMessage("Supervisor shutdown interrupted config apply while awaiting collector health").Len())
	require.Zero(t, observed.FilterMessage("Collector unhealthy after restart").Len())
	require.Zero(t, observed.FilterMessage("Rolled back to previous config").Len())
}
