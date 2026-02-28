// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windowseventlogreceiver

import (
	"go.opentelemetry.io/collector/receiver"
)

const typeStr = "windowseventlog"

// NewFactory creates a factory for the windowseventlog receiver.
func NewFactory() receiver.Factory {
	return newFactoryAdapter()
}
