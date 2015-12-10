package util

import (
	"os"
	"path/filepath"

	"github.com/kardianos/osext"
	"github.com/Sirupsen/logrus"
	"runtime"
	"unicode"
)

func GetGxlogPath() (string, error) {
	fullexecpath, err := osext.Executable()
	if err != nil {
		return "", err
	}

	dir, _ := filepath.Split(fullexecpath)
	return dir, nil
}

func GetRootPath() (string, error) {
	return filepath.Abs("/")
}

func GetSystemName() (string) {
	os := runtime.GOOS
	osRunes := []rune(os)
	osRunes[0] = unicode.ToUpper(osRunes[0])
	return string(osRunes)
}

func AppendIfDir(dir string, appendix string) (string, error) {
	file, err := os.Open(dir)
	if err != nil {
		logrus.Error("Can not access ", dir)
		return dir, err
	}

	fileInfo, err := file.Stat();
	switch {
	case err != nil:
		return "", err
	case fileInfo.IsDir():
		appended := filepath.Join(dir, appendix)
		return appended, nil
	default:
		return dir, nil
	}
}
