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

package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Graylog2/collector-sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/logger"
	"github.com/pborman/uuid"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode"
)

var log = logger.Log()

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
		id = idFromFile(filePath)
	}

	if id != "" && !cfgfile.ValidateConfig() {
		log.Info("Using node-id: ", id)
	}
	return id
}

func idFromFile(filePath string) string {
	err := common.FileExists(filePath)
	if err != nil {
		log.Info("node-id file doesn't exist, generating a new one")
		err = common.CreatePathToFile(filePath)
		if err == nil {
			err = ioutil.WriteFile(filePath, []byte(RandomUuid()), 0644)
			if err != nil {
				log.Error("Can not write node-id file: ", err)
			}
		}
	}

	file, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Error("Can not read node-id file: ", err)
		return ""
	}
	return strings.Trim(string(file), " \n")
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

func NewTrue() *bool {
	b := true
	return &b
}

func NewFalse() *bool {
	b := false
	return &b
}

func IsInList(id string, list []string) bool {
	for _, match := range list {
		if id == match {
			return true
		}
	}
	return false
}

func Sprintf(format string, values ...interface{}) (string, error) {
	matched, err := regexp.MatchString("%[vTsqxX]", format)
	if err != nil {
		return "", err
	}
	if matched {
		return fmt.Sprintf(format, values...), nil
	} else {
		return format, nil
	}
}

type PathMatchResult struct {
	Path      string
	Match     bool
	IsLink    bool
	DoesExist bool
}

func PathMatch(path string, patternList []string) (PathMatchResult, error) {
	result := PathMatchResult{Path: path, DoesExist: true}

	if _, err := os.Lstat(path); err == nil {
		resolvedPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			// error out on broken symlinks, because we cannot resolve
			// their path with EvalSymlinks
			result.DoesExist = false
			return result, err
		} else {
			if resolvedPath != path {
				result.IsLink = true
				result.Path = resolvedPath
			}
		}
	} else {
		result.DoesExist = false
	}

	for _, pattern := range patternList {
		match, err := filepath.Match(pattern, result.Path)
		if err != nil {
			result.Match = false
			return result, err
		}
		if match {
			result.Match = true
			return result, nil
		}
	}
	result.Match = false
	return result, nil
}
