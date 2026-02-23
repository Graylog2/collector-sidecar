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
	"fmt"
	"net"
	"net/netip"

	"github.com/Graylog2/collector-sidecar/superv/config"
)

// resolveLocalEndpoint converts a net.Addr.String() result (e.g. "0.0.0.0:54321")
// into a dialable WebSocket URL for the collector's OpAMP extension.
//
// Unspecified addresses are replaced with family-aware loopback:
// 0.0.0.0 → 127.0.0.1, [::] → [::1].
//
// Returns an error if the address cannot be parsed.
func resolveLocalEndpoint(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("cannot parse local server address %q: %w", addr, err)
	}

	// Replace unspecified addresses with family-aware loopback.
	if ip, err := netip.ParseAddr(host); err == nil && ip.IsUnspecified() {
		if ip.Is4() {
			host = "127.0.0.1"
		} else {
			host = "::1"
		}
	}

	return fmt.Sprintf("ws://%s%s", net.JoinHostPort(host, port), config.DefaultOpAMPPath), nil
}
