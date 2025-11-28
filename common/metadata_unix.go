//go:build !windows

package common

import (
	"path/filepath"
	"strings"
)

var basePath string

func init() {
	basePath = filepath.Join("/etc", strings.ToLower(VendorName), strings.ToLower(ProductName))
}

func configBasePathPlatform() string {
	return basePath
}

func configFilePathPlatform() string {
	return filepath.Join(basePath, "sidecar.yml")
}

func cachePathPlatform() string {
	return filepath.Join("/var", "cache", strings.ToLower(VendorName), strings.ToLower(ProductName))
}
