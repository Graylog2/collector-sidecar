// Copyright (C) 2020 Graylog, Inc.
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

package common

import (
	"fmt"
	"path/filepath"
	"strings"
)

var (
	// buildinfo set by ldflags
	CollectorVersion       string
	CollectorVersionSuffix string
	GitRevision            string

	// base branding set by ldflags
	VendorName  string = "Graylog"
	ProductName string = "Sidecar"

	// computed values from above
	lowerFullName   string
	displayFullName string
	configBasePath  string
	configFilePath  string
	cachePath       string
)

func init() {
	lowerFullName = fmt.Sprintf("%s-%s", strings.ToLower(VendorName), strings.ToLower(ProductName))
	displayFullName = fmt.Sprintf("%s %s", VendorName, ProductName)
	configBasePath = configBasePathPlatform()
	configFilePath = configFilePathPlatform()
	cachePath = cachePathPlatform()
}

func LowerFullName() string {
	return lowerFullName
}

func DisplayFullName() string {
	return displayFullName
}

// ConfigBasePath use for individual paths inside the default config path, e.g. `ConfigBasePath("node-id")` for `"/etc/graylog-sidecar/node-id"`
// call without arguments to just get the base path itself
func ConfigBasePath(elem ...string) string {
	if len(elem) == 0 {
		return configBasePath
	}
	return filepath.Join(append([]string{configBasePath}, elem...)...)
}

func ConfigFilePath() string {
	return configFilePath
}

func CachePath() string {
	return cachePath
}
