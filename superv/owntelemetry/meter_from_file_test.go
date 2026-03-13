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

package owntelemetry

import (
	"testing"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/stretchr/testify/assert"
)

func TestNewMeterProviderFromFile_NoFile(t *testing.T) {
	provider, err := NewMeterProviderFromFile(
		t.TempDir(), "", "",
		nil,
		config.BatchConfig{ExportInterval: 10 * time.Second},
		[]string{"otelcol_exporter_sent_spans"},
	)
	assert.NoError(t, err)
	assert.Nil(t, provider)
}

func TestNewMeterProviderFromFile_EmptyAllowList(t *testing.T) {
	// Even with a valid config file, empty allow-list → nil provider
	dir := t.TempDir()
	p := NewPersistence(dir, "own-metrics.yaml", "", "")
	_ = p.Save(Settings{Endpoint: "http://localhost:4318"})

	provider, err := NewMeterProviderFromFile(
		dir, "", "",
		nil,
		config.BatchConfig{ExportInterval: 10 * time.Second},
		[]string{},
	)
	assert.NoError(t, err)
	assert.Nil(t, provider)
}
