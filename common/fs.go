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
	"os"
	"path"
	"path/filepath"

	"github.com/Graylog2/collector-sidecar/logger"
)

var log = logger.Log()

func FileExists(filePath string) error {
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return err
	}
	return nil

}

func IsDir(filePath string) bool {
	fi, err := os.Stat(filePath)
	if err != nil {
		log.Error(err)
		return false
	}
	if fi.Mode().IsDir() {
		return true
	}
	return false

}

func CreatePathToFile(filepath string) error {
	dir := path.Dir(filepath)
	_, err := os.Open(dir)
	if err != nil {
		log.Info("Trying to create directory for: ", filepath)
		err = os.MkdirAll(dir, 0750)
		if err != nil {
			log.Error("Not able to create directory path: ", dir)
			return err
		}
	}
	return nil
}

func ListFiles(paths []string) []File {
	list := []File{}

	filter := func(path string, file os.FileInfo, err error) error {
		if err == nil {
			list = append(list,
				File{Path: path,
					ModTime: file.ModTime(),
					Size:    file.Size(),
					IsDir:   file.IsDir()})
			return nil
		} else {
			log.Errorf("Can not get file list for %s: %v", path, err)
			// Make sure to return SkipDir here so the walk will
			// continue!
			return filepath.SkipDir
		}
	}

	for _, path := range paths {
		if !IsDir(path) {
			continue
		}

		err := filepath.Walk(path, filter)
		if err != nil {
			log.Errorf("Error listing files for %s: %v", path, err)
		}
	}

	return list
}
