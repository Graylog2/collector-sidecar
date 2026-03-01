// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package windows

import (
	"errors"
	"fmt"
	"sort"
	"syscall"
	"unsafe"
)

// evtOpenChannelEnum opens a handle to enumerate event log channels.
var evtOpenChannelEnum = func(session uintptr, flags uint32) (uintptr, error) {
	handle, _, err := openChannelEnumProc.Call(session, uintptr(flags))
	if !errors.Is(err, ErrorSuccess) {
		return 0, err
	}
	return handle, nil
}

// evtNextChannelPath retrieves the next channel path from the enumerator.
// Returns the channel path, or ("", ErrorNoMoreItems) when done.
var evtNextChannelPath = func(channelEnum uintptr, channelPathBufferSize uint32, channelPathBuffer *uint16, channelPathBufferUsed *uint32) error {
	_, _, err := nextChannelPathProc.Call(
		channelEnum,
		uintptr(channelPathBufferSize),
		uintptr(unsafe.Pointer(channelPathBuffer)),
		uintptr(unsafe.Pointer(channelPathBufferUsed)),
	)
	if !errors.Is(err, ErrorSuccess) {
		return err
	}
	return nil
}

// ListChannels enumerates all available Windows Event Log channels
// and returns them sorted alphabetically.
func ListChannels() ([]string, error) {
	handle, err := evtOpenChannelEnum(0, 0)
	if err != nil {
		return nil, fmt.Errorf("EvtOpenChannelEnum: %w", err)
	}
	defer evtClose(handle)

	var channels []string
	buf := make([]uint16, 512)

	for {
		var used uint32
		err := evtNextChannelPath(handle, uint32(len(buf)), &buf[0], &used)
		if errors.Is(err, ErrorNoMoreItems) {
			break
		}
		if errors.Is(err, ErrorInsufficientBuffer) {
			buf = make([]uint16, used)
			err = evtNextChannelPath(handle, uint32(len(buf)), &buf[0], &used)
			if err != nil {
				return nil, fmt.Errorf("EvtNextChannelPath: %w", err)
			}
		} else if err != nil {
			return nil, fmt.Errorf("EvtNextChannelPath: %w", err)
		}
		channels = append(channels, syscall.UTF16ToString(buf[:used]))
	}

	sort.Strings(channels)
	return channels, nil
}
