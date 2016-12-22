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

package filebeat

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/system"
	"github.com/Graylog2/collector-sidecar/logger"
)

const (
	name   = "filebeat"
	driver = "exec"
)

var (
	log           = logger.Log()
	backendStatus = system.Status{}
)

func init() {
	if err := backends.RegisterBackend(name, New); err != nil {
		log.Fatal(err)
	}
}

func New(context *context.Ctx) backends.Backend {
	return NewCollectorConfig(context)
}

func (fbc *FileBeatConfig) Name() string {
	return name
}

func (fbc *FileBeatConfig) Driver() string {
	return driver
}

func (fbc *FileBeatConfig) ExecPath() string {
	execPath := fbc.Beats.UserConfig.BinaryPath
	if common.FileExists(execPath) != nil {
		log.Fatal("Configured path to collector binary does not exist: " + execPath)
	}

	return execPath
}

func (fbc *FileBeatConfig) ConfigurationPath() string {
	configurationPath := fbc.Beats.UserConfig.ConfigurationPath
	if !common.IsDir(filepath.Dir(configurationPath)) {
		err := common.CreatePathToFile(configurationPath)
		if err != nil {
			log.Fatal("Configured path to collector configuration does not exist: " + configurationPath)
		}
	}

	return configurationPath
}

func (fbc *FileBeatConfig) CachePath() string {
	cachePath := fbc.Beats.Context.UserConfig.CachePath
	if !common.IsDir(filepath.Dir(cachePath)) {
		err := common.CreatePathToFile(cachePath)
		if err != nil {
			log.Fatal("Configured path to cache directory does not exist: " + cachePath)
		}
	}

	return filepath.Join(cachePath, "filebeat", "data")
}

func (fbc *FileBeatConfig) ExecArgs() []string {
	return []string{"-c", fbc.ConfigurationPath()}
}

func (fbc *FileBeatConfig) readVersion() ([]int, error) {
	var version = []int{}
	output, err := exec.Command(fbc.ExecPath(), "-version").CombinedOutput()
	versionString := string(output)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("[%s] Error while fetching Beats collector version: %s", fbc.Name(), versionString))
	}
	re := regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
	versions := re.FindStringSubmatch(versionString)[1:4] // ["1.2.3", "1", "2", "3"]

	for _, value := range versions {
		digit, err := strconv.Atoi(value)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("[%s] Error converting Beats collector version: %s", fbc.Name(), versionString))
		}
		version = append(version, digit)
	}
	return version, nil
}

func (fbc *FileBeatConfig) ValidatePreconditions() bool {
	version, err := fbc.readVersion()
	if err != nil {
		log.Errorf("[%s] Validation failed, skipping backend: %s", fbc.Name(), err)
	}
	if version[0] < 1 || version[0] > 5 {
		log.Errorf("[%s] Unsupported Filebeats version, please install 5.x", fbc.Name())
	}
	fbc.Beats.Version = version
	return true
}

func (fbc *FileBeatConfig) SetStatus(state int, message string) {
	// if error state is already set don't overwrite the message to get the root cause
	if state > backends.StatusRunning &&
		backendStatus.Status > backends.StatusRunning &&
		len(backendStatus.Message) != 0 {
		backendStatus.Set(state, backendStatus.Message)
	} else {
		backendStatus.Set(state, message)
	}
}

func (fbc *FileBeatConfig) Status() system.Status {
	return backendStatus
}
