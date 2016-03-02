// +build darwin linux

package util

import "os"

func ExpandPath(path string) string {
	return os.ExpandEnv(path)
}
