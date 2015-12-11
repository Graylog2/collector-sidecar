package util

import (
	"os"
	"path/filepath"

	"github.com/Graylog2/nxlog-sidecar/vendor/code.google.com/p/go-uuid/uuid"
	"github.com/Sirupsen/logrus"
	"github.com/kardianos/osext"
	"io/ioutil"
	"runtime"
	"strings"
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

func GetSystemName() string {
	os := runtime.GOOS
	osRunes := []rune(os)
	osRunes[0] = unicode.ToUpper(osRunes[0])
	return string(osRunes)
}

func GetCollectorId(collectorId string) string {
	id := collectorId
	if strings.HasPrefix(collectorId, "file:") {
		filePath := strings.SplitAfter(collectorId, ":")[1]
		_, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			logrus.Info("collector-id file doesn't exist, generating a new one")
			ioutil.WriteFile(filePath, []byte(RandomUuid()), 0644)
		}
		file, err := ioutil.ReadFile(filePath)
		if err != nil {
			logrus.Fatal("Can not read collector-id file: ", err)
		}
		id = string(file)
	}

	logrus.Info("Using collector-id: ", id)
	if id == "" {
		logrus.Fatal("Couldn't find any collector-id")
	}
	return id
}

func RandomUuid() string {
	return uuid.NewRandom().String()
}

func AppendIfDir(dir string, appendix string) (string, error) {
	file, err := os.Open(dir)
	if err != nil {
		logrus.Error("Can not access ", dir)
		return dir, err
	}

	fileInfo, err := file.Stat()
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
