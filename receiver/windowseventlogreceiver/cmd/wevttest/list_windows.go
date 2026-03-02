// Copyright (C) 2026 Graylog, Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"fmt"

	"github.com/Graylog2/collector-sidecar/receiver/windowseventlogreceiver/internal/windows"
)

func listChannels() error {
	channels, err := windows.ListChannels()
	if err != nil {
		return err
	}
	for _, ch := range channels {
		fmt.Println(ch)
	}
	return nil
}
