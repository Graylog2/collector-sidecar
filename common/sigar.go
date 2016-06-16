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
	"fmt"
	sigar "github.com/cloudfoundry/gosigar"
)

func GetFileSystemList75() []string {
	fslist := sigar.FileSystemList{}
	fslist.Get()

	result := []string{}
	for _, fs := range fslist.List {
		dir_name := fs.DirName
		usage := sigar.FileSystemUsage{}
		usage.Get(dir_name)

		if usage.UsePercent() >= 75 {
			result = append(result, fmt.Sprintf("%s (%s)",
				dir_name,
				sigar.FormatPercent(usage.UsePercent())))
		}
	}
	return result
}

func GetLoad1() float64 {
	concreteSigar := sigar.ConcreteSigar{}

	avg, err := concreteSigar.GetLoadAverage()
	if err != nil {
		log.Error("Failed to get load average")
		return 0
	}

	return avg.One
}
