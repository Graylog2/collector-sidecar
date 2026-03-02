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

package windows

import (
	"fmt"
	"strings"
)

// canonicalizeChannelList trims whitespace from each entry, removes empty
// entries, and deduplicates case-insensitively (first occurrence wins).
func canonicalizeChannelList(channels []string) []string {
	if channels == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(channels))
	result := make([]string, 0, len(channels))
	for _, ch := range channels {
		ch = strings.TrimSpace(ch)
		if ch == "" {
			continue
		}
		key := strings.ToLower(ch)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, ch)
	}
	return result
}

// filterChannels returns the subset of wanted channels that exist in the
// available set (case-insensitive lookup). The available map keys must be
// lowercase. Returns the filtered list preserving original casing, and
// the list of channels that were skipped.
func filterChannels(wanted []string, available map[string]struct{}) (matched, skipped []string) {
	for _, ch := range wanted {
		if _, ok := available[strings.ToLower(ch)]; ok {
			matched = append(matched, ch)
		} else {
			skipped = append(skipped, ch)
		}
	}
	return matched, skipped
}

// applyChannelFilter enumerates available channels using listFn, filters
// wanted against them, and returns the filtered list and skipped list.
// Returns (nil, nil, err) if listFn fails.
// Returns (nil, skipped, nil) if no channels match (caller decides policy).
func applyChannelFilter(wanted []string, listFn func() ([]string, error)) ([]string, []string, error) {
	available, err := listFn()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enumerate available channels: %w", err)
	}

	availableSet := make(map[string]struct{}, len(available))
	for _, ch := range available {
		availableSet[strings.ToLower(ch)] = struct{}{}
	}

	filtered, skipped := filterChannels(wanted, availableSet)
	return filtered, skipped, nil
}
