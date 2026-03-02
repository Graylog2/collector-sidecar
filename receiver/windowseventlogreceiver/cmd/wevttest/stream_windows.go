// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	wel "github.com/Graylog2/collector-sidecar/receiver/windowseventlogreceiver"
)

func streamEvents(ctx context.Context, channels []string, startAt string, f *formatter) error {
	logger := zap.Must(zap.NewDevelopment())

	cons, err := consumer.NewLogs(func(ctx context.Context, ld plog.Logs) error {
		return f.writeLogs(ld)
	})
	if err != nil {
		return fmt.Errorf("creating consumer: %w", err)
	}

	factory := wel.NewFactory()
	var receivers []receiver.Logs

	for _, ch := range channels {
		cfg := factory.CreateDefaultConfig().(*wel.WindowsLogConfig)
		cfg.Channel = ch
		cfg.StartAt = startAt
		cfg.IncludeLogRecordOriginal = true

		settings := receiver.Settings{
			ID: component.MustNewIDWithName("windowseventlog", ch),
			TelemetrySettings: component.TelemetrySettings{
				Logger:         logger.Named(ch),
				MeterProvider:  noop.NewMeterProvider(),
				TracerProvider: tracenoop.NewTracerProvider(),
			},
		}

		r, err := factory.CreateLogs(ctx, settings, cfg, cons)
		if err != nil {
			return fmt.Errorf("creating receiver for channel %q: %w", ch, err)
		}
		receivers = append(receivers, r)
	}

	for i, r := range receivers {
		if err := r.Start(ctx, componentHost{}); err != nil {
			for j := range i {
				receivers[j].Shutdown(context.Background())
			}
			return fmt.Errorf("starting receiver: %w", err)
		}
	}

	logger.Info("streaming events", zap.Strings("channels", channels), zap.String("format", string(f.format)))
	fmt.Fprintf(os.Stderr, "Streaming from %v (press Ctrl+C to stop)...\n", channels)

	<-ctx.Done()

	fmt.Fprintln(os.Stderr, "\nShutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, r := range receivers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.Shutdown(shutdownCtx); err != nil {
				logger.Error("shutdown error", zap.Error(err))
			}
		}()
	}
	wg.Wait()

	return nil
}

// componentHost implements component.Host with a no-op extension map.
type componentHost struct{}

func (componentHost) GetExtensions() map[component.ID]component.Component {
	return nil
}
