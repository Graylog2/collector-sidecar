// This file is part of Graylog.
//
// Graylog is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Graylog is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Graylog.  If not, see <http://www.gnu.org/licenses/>.

package common

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"bytes"
	"fmt"
	"github.com/pborman/uuid"
	"net"
	"github.com/Graylog2/collector-sidecar/cfgfile"
)

func GetRootPath() (string, error) {
	return filepath.Abs("/")
}

func GetSystemName() string {
	os := runtime.GOOS
	osRunes := []rune(os)
	osRunes[0] = unicode.ToUpper(osRunes[0])
	return string(osRunes)
}

func GetHostname() (string, error) {
	return os.Hostname()
}

func GetHostIP() string {
	defaultIp := "0.0.0.0"
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return defaultIp
	}

	// try to find IPv4 address first
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
			//if ipnet.IP.To16() != nil {
			//	return ipnet.IP.String()
			//}
		}
	}
	// if nothing found try IPv6 address
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To16() != nil {
				return ipnet.IP.String()
			}
		}
	}
	// in doubt return default address
	return defaultIp
}

func GetCollectorId(collectorId string) string {
	id := collectorId
	if strings.HasPrefix(collectorId, "file:") {
		filePath := strings.SplitAfterN(collectorId, ":", 2)[1]
		err := FileExists(filePath)
		if err != nil {
			log.Info("collector-id file doesn't exist, generating a new one")
			CreatePathToFile(filePath)
			ioutil.WriteFile(filePath, []byte(RandomUuid()), 0644)
		}
		file, err := ioutil.ReadFile(filePath)
		if err != nil {
			log.Fatal("Can not read collector-id file: ", err)
		}
		id = strings.Trim(string(file), " \n")
	}

	if id != "" && !cfgfile.ValidateConfig() {
		log.Info("Using collector-id: ", id)
	}
	return id
}

func RandomUuid() string {
	return uuid.NewRandom().String()
}

func ConvertLineBreak(data []byte) []byte {
	if runtime.GOOS == "windows" {
		return bytes.Replace(data, []byte("\n"), []byte("\r\n"), -1)
	} else {
		return data
	}

}

func EnsureLineBreak(data string) string {
	var result = strings.TrimSuffix(data, "\r\n")
	result = strings.TrimSuffix(data, "\n")
	return result + "\n"
}

func EncloseWith(data string, sep string) string {
	if len(data) == 0 {
		return ""
	}

	format := []string{"%s"}
	if !strings.HasPrefix(data, sep) {
		format = append([]string{sep}, format...)
	}
	if !strings.HasSuffix(data, sep) {
		format = append(format, sep)
	}
	return fmt.Sprintf(strings.Join(format, ""), data)
}

func Inspect(object interface{}) string {
	jsonBytes, _ := json.Marshal(object)
	return string(jsonBytes)
}
