package util

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/Sirupsen/logrus"
	"github.com/kardianos/osext"
	"github.com/pborman/uuid"
)

func GetSidecarPath() (string, error) {
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
		err := FileExists(filePath)
		if err != nil {
			logrus.Info("collector-id file doesn't exist, generating a new one")
			CreatePathToFile(filePath)
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

func FileExists(filePath string) error {
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return err
	}
	return nil

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

func CreatePathToFile(filepath string) error {
	dir := path.Dir(filepath)
	_, err := os.Open(dir)
	if err != nil {
		logrus.Info("Trying to create directory for: ", filepath)
		err = os.MkdirAll(dir, 0750)
		if err != nil {
			logrus.Error("Not able to create directory path: ", dir)
			return err
		}
	}
	return nil
}

func SplitCommaList(list string) []string {
	if list == "" {
		return make([]string, 0)
	}
	return strings.Split(list, ",")
}
