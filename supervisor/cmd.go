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
	"fmt"
	"os"
	"os/signal"

	"github.com/open-telemetry/opentelemetry-collector-contrib/cmd/opampsupervisor/supervisor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/cmd/opampsupervisor/supervisor/telemetry"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "supervisor",
		Short:             "Start the supervisor",
		Long:              "Start the OTel Collector supervisor process",
		ValidArgs:         nil,
		ValidArgsFunction: nil,
		Args:              nil,
		ArgAliases:        nil,
		RunE:              runSupervisor,
	}
	cmd.Flags().StringP("config", "c", "supervisor.yml", "Path to a supervisor configuration file")
	cmd.Flags().StringP("server-url", "u", "ws://localhost:9000/v1/opamp", "OpAMP server URL")
	cmd.Flags().String("data-dir", "", "Supervisor data dir (default platform-dependent)")
	cmd.Flags().Bool("sidecar", false, "Run Collector with legacy Sidecar extension")
	cmd.Flags().Bool("dev", false, "Enable development profile")
	_ = cmd.Flags().MarkHidden("dev") // Developer-only setting

	return cmd
}

func runSupervisor(cmd *cobra.Command, args []string) error {
	//configFlag, _ := cmd.Flags().GetString("config")

	cfg, err := buildConfig(cmd)
	if err != nil {
		return err
	}

	//cfg, err := config.Load(configFlag)
	//if err != nil {
	//	return fmt.Errorf("failed to load config: %w", err)
	//}

	logger, err := telemetry.NewLogger(cfg.Telemetry.Logs)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sv, err := supervisor.NewSupervisor(ctx, logger.Named("supervisor"), *cfg)
	if err != nil {
		return fmt.Errorf("failed to create supervisor: %w", err)
	}

	err = sv.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start supervisor: %w", err)
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	<-interrupt
	sv.Shutdown()

	return nil
}
