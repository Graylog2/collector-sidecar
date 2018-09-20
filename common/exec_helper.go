// +build !windows

package common

// Dummy function. Only used on Windows
func CommandLineToArgv(cmd string) []string {
	panic("not implemented on this platform")
	return []string{}
}
