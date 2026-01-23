// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package version

var (
	version = "0.1.0-dev"
	commit  = "unknown"
)

func Version() string {
	return version + " (" + commit + ")"
}
