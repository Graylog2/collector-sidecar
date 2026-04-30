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

//go:build windows

package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWindowsDataPathPrefixIsAbsolute guards against the filepath.Join("C:", ...)
// pitfall: a bare "C:" is a drive letter, so Join produces a drive-relative path
// (e.g. "C:ProgramData\..."). The volume-root form `C:\` is required to get an
// absolute path ("C:\\ProgramData\\..."). Services resolve relative paths
// against their startup working directory, so a drive-relative default would
// silently write state to the wrong location.
func TestWindowsDataPathPrefixIsAbsolute(t *testing.T) {
	assert.True(t, filepath.IsAbs(WindowsDataPathPrefix),
		"windowsDataPathPrefix must be absolute, got %q", WindowsDataPathPrefix)
}

func TestConfigDefaultsDirectories(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, filepath.Join(`C:\`, "ProgramData", "Graylog", "Collector", "supervisor"), cfg.Persistence.Dir)
	assert.Equal(t, filepath.Join(`C:\`, "ProgramData", "Graylog", "Collector", "storage"), cfg.Agent.StorageDir)
	assert.Equal(t, filepath.Join(`C:\`, "ProgramData", "Graylog", "Collector", "keys"), cfg.Keys.Dir)
	assert.Equal(t, filepath.Join(`C:\`, "ProgramData", "Graylog", "Collector", "packages"), cfg.Packages.StorageDir)
	assert.Equal(t, filepath.Join(`C:\`, "ProgramData", "Graylog", "Collector", "supervisor", "logs", "supervisor.log"), cfg.Logging.File)

	// Regression: these must all be absolute paths.
	assert.True(t, filepath.IsAbs(cfg.Persistence.Dir))
	assert.True(t, filepath.IsAbs(cfg.Agent.StorageDir))
	assert.True(t, filepath.IsAbs(cfg.Keys.Dir))
	assert.True(t, filepath.IsAbs(cfg.Packages.StorageDir))
	assert.True(t, filepath.IsAbs(cfg.Logging.File))
}
