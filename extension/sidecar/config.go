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

package sidecar

import (
	"fmt"
	"os"
)

type Config struct {
	Path string `mapstructure:"path"`
}

func (cfg *Config) Validate() error {
	if cfg.Path == "" {
		return fmt.Errorf("config.path is required")
	}

	_, err := os.Stat(cfg.Path)
	if err != nil {
		return fmt.Errorf("provided config path %s does not exist can't be read: %w", cfg.Path, err)
	}

	return nil
}
