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

package main

import (
	"flag"
	"fmt"
	"os"
)

const usage = `wevttest — Windows Event Log receiver test tool

Usage:
  wevttest <command> [flags] [args...]

Commands:
  stream    Live-stream events from one or more channels
  list      List all available event log channels

Run 'wevttest <command> -h' for command-specific help.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "stream":
		if err := runStream(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "list":
		if err := runList(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
}

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	fs.Parse(args)
	return listChannels()
}
