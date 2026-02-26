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

package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

const DefaultOpAMPPath = "/v1/opamp"

// DeriveEnrollmentEndpoint derives the OpAMP endpoint from the enrollment URL.
func DeriveEnrollmentEndpoint(enrollmentURL string) (string, error) {
	u, err := url.Parse(enrollmentURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse enrollment URL: %w", err)
	}

	scheme, host, port, path := u.Scheme, u.Hostname(), u.Port(), u.Path

	if scheme == "" && host == "" {
		return "", errors.New("invalid enrollment URL: missing scheme and host")
	}
	if path == "" || path == "/" {
		path = DefaultOpAMPPath
	} else if !strings.HasSuffix(path, DefaultOpAMPPath) {
		path = strings.TrimSuffix(path, "/") + DefaultOpAMPPath
	}

	endpoint := &url.URL{
		Scheme: scheme,
		Host:   strings.TrimSuffix(net.JoinHostPort(host, port), ":"),
		Path:   path,
	}
	return endpoint.String(), nil
}

// ServerBaseURL extracts the base URL (scheme + host) from an enrollment URL.
func ServerBaseURL(enrollmentURL string) (string, error) {
	if enrollmentURL == "" {
		return "", errors.New("enrollment URL cannot be empty")
	}

	u, err := url.Parse(enrollmentURL)
	if err != nil {
		return "", fmt.Errorf("invalid enrollment URL: %w", err)
	}

	path := strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), DefaultOpAMPPath)
	if path == "" || path == "/" {
		return fmt.Sprintf("%s://%s", u.Scheme, u.Host), nil
	}
	return fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, path), nil
}
