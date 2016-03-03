package util

import "golang.org/x/sys/windows/registry"

func ExpandPath(path string) string {
	expandedPath, err := registry.ExpandString(path)
	if err != nil {
		return path
	}
	return expandedPath
}
