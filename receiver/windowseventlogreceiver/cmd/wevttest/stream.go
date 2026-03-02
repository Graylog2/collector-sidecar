// Copyright (C) 2026 Graylog, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
)

var defaultChannels = []string{"Application", "System", "Security"}

func runStream(args []string) error {
	fs := flag.NewFlagSet("stream", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: wevttest stream [flags] [channel...]\n\nLive-stream events from Windows Event Log channels.\n\nDefault channels: Application, System, Security\n\nFlags:\n")
		fs.PrintDefaults()
	}

	formatStr := fs.String("format", "json", "output format: json, xml, otel")
	startAt := fs.String("start-at", "end", "start position: beginning, end")
	output := fs.String("output", "", "write output to file (uses compact JSON when format is json)")
	fs.Parse(args)

	format, err := validFormat(*formatStr)
	if err != nil {
		return err
	}

	if *startAt != "beginning" && *startAt != "end" {
		return fmt.Errorf("invalid --start-at value %q (valid: beginning, end)", *startAt)
	}

	channels := fs.Args()
	if len(channels) == 0 {
		channels = defaultChannels
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	w := os.Stdout
	ndjson := false
	if *output != "" {
		file, err := os.Create(*output)
		if err != nil {
			return fmt.Errorf("open output file: %w", err)
		}
		defer file.Close()
		w = file
		ndjson = format == formatJSON
	}

	f := &formatter{format: format, w: w, ndjson: ndjson}
	return streamEvents(ctx, channels, *startAt, f)
}
