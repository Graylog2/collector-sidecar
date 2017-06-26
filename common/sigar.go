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
	"math"
	"os"
	"runtime"

	sigar "github.com/elastic/gosigar"
)

var (
	cpu = &CPU{LastCpuTimes: &CpuTimes{}}
)

type CPU struct {
	LastCpuTimes *CpuTimes
}

type CpuTimes struct {
	sigar.Cpu
	UserPercent    float64
	SystemPercent  float64
	IdlePercent    float64
	IOwaitPercent  float64
	IrqPercent     float64
	NicePercent    float64
	SoftIrqPercent float64
	StealPercent   float64
}

func GetCpuTimes() (*CpuTimes, error) {

	cpu := sigar.Cpu{}
	err := cpu.Get()
	if err != nil {
		return nil, err
	}

	return &CpuTimes{Cpu: cpu}, nil
}

func GetCpuPercentage(last *CpuTimes, current *CpuTimes) *CpuTimes {
	if last != nil && current != nil {
		all_delta := current.Cpu.Total() - last.Cpu.Total()

		if all_delta == 0 {
			// first inquiry
			return current
		}

		calculate := func(field2 uint64, field1 uint64) float64 {
			perc := 0.0
			delta := int64(field2 - field1)
			perc = float64(delta) / float64(all_delta)
			return round(perc, .5, 4)
		}

		current.UserPercent = calculate(current.Cpu.User, last.Cpu.User)
		current.SystemPercent = calculate(current.Cpu.Sys, last.Cpu.Sys)
		current.IdlePercent = calculate(current.Cpu.Idle, last.Cpu.Idle)
		current.IOwaitPercent = calculate(current.Cpu.Wait, last.Cpu.Wait)
		current.IrqPercent = calculate(current.Cpu.Irq, last.Cpu.Irq)
		current.NicePercent = calculate(current.Cpu.Nice, last.Cpu.Nice)
		current.SoftIrqPercent = calculate(current.Cpu.SoftIrq, last.Cpu.SoftIrq)
		current.StealPercent = calculate(current.Cpu.Stolen, last.Cpu.Stolen)
	}

	return current
}

func (cpu *CPU) AddCpuPercentage(t2 *CpuTimes) {
	cpu.LastCpuTimes = GetCpuPercentage(cpu.LastCpuTimes, t2)
}

func GetCpuIdle() float64 {
	cpuStat, err := GetCpuTimes()
	if err != nil {
		return -1
	}

	cpu.AddCpuPercentage(cpuStat)
	return cpu.LastCpuTimes.IdlePercent * 100
}

func GetFileSystemList75() []string {
	result := []string{}
	volumes := []sigar.FileSystem{}

	if runtime.GOOS == "windows" {
		volumes = getWindowsDrives()
	} else {
		fslist := sigar.FileSystemList{}
		fslist.Get()
		volumes = fslist.List
	}

	for _, volume := range volumes {
		dirName := volume.DirName
		usage := sigar.FileSystemUsage{}
		usage.Get(dirName)

		if usage.UsePercent() >= 75 {
			result = append(result, fmt.Sprintf("%s (%s)",
				dirName,
				sigar.FormatPercent(usage.UsePercent())))
		}
	}
	return result
}

func GetLoad1() float64 {
	concreteSigar := sigar.ConcreteSigar{}

	avg, err := concreteSigar.GetLoadAverage()
	if err != nil {
		log.Debug("Failed to get load average")
		return -1
	}

	return avg.One
}

func getWindowsDrives() (drives []sigar.FileSystem) {
	for _, drive := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		dirName := string(drive) + ":\\"
		dirHandle, err := os.Open(dirName)
		defer dirHandle.Close()
		if err == nil {
			fs := sigar.FileSystem{DirName: dirName}
			drives = append(drives, fs)
		}
	}
	return
}

func round(val float64, roundOn float64, places int) (newVal float64) {
	var round float64
	pow := math.Pow(10, float64(places))
	digit := pow * val
	_, div := math.Modf(digit)
	if div >= roundOn {
		round = math.Ceil(digit)
	} else {
		round = math.Floor(digit)
	}
	newVal = round / pow
	return
}
