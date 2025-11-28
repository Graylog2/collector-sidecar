package common

import (
	"os"
	"path/filepath"
	"strings"
)

var basePath string

func init() {
	basePath = filepath.Join(os.Getenv("SystemDrive")+"\\", "Program Files", strings.ToLower(VendorName), strings.ToLower(ProductName))
}

func configBasePathPlatform() string {
	return basePath
}

func configFilePathPlatform() string {
	return filepath.Join(basePath, "sidecar.yml")
}

func cachePathPlatform() string {
	return filepath.Join(basePath, "cache")
}
