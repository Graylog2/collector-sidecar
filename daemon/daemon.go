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

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kardianos/service"

	"github.com/Graylog2/collector-sidecar/common"
)

var log = common.Log()

type Config struct {
	Name, DisplayName, Description string

	Dir  string
	Exec string
	Args []string
	Env  []string

	Stderr, Stdout string
}

type Program struct {
	exit    chan struct{}
	Running bool
	service service.Service
	*Config
	cmd *exec.Cmd
}

func NewConfig(collectorPath string, logPath string) *Config {
	rootDir, err := common.GetRootPath()
	if err != nil {
		log.Error("Can not access root directory")
	}

	c := &Config{
		Name:        "collector-sidecar",
		DisplayName: "Graylog collector sidecar",
		Description: "Wrapper service for Graylog controlled collector",
		Dir:         rootDir,
		Env:         []string{},
		Stderr:      filepath.Join(logPath, "collector_stderr.log"),
		Stdout:      filepath.Join(logPath, "collector_stdout.log"),
	}

	return c
}

func NewProgram(config *Config) *Program {
	p := &Program{
		exit:   make(chan struct{}),
		Config: config,
	}
	return p
}

func (p *Program) BindToService(s service.Service) {
	p.service = s
}

func (p *Program) Start(s service.Service) error {
	absPath, _ := filepath.Abs(p.Exec)
	fullExec, err := exec.LookPath(absPath)
	if err != nil {
		return fmt.Errorf("Failed to find collector executable %q: %v", p.Exec, err)
	}

	p.cmd = exec.Command(fullExec, p.Args...)
	p.cmd.Dir = p.Dir
	p.cmd.Env = append(os.Environ(), p.Env...)

	go p.run()
	return nil
}

func (p *Program) Stop(s service.Service) error {
	log.Info("Stopping collector")
	close(p.exit)
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
	return nil
}

func (p *Program) Restart(s service.Service) error {
	p.Stop(s)
	time.Sleep(3 * time.Second)
	p.exit = make(chan struct{})
	p.Start(s)

	return nil
}

func (p *Program) run() {
	log.Info("Starting collector")

	if p.Stderr != "" {
		err := common.CreatePathToFile(p.Stderr)
		f, err := os.OpenFile(p.Stderr, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0777)
		if err != nil {
			log.Warningf("Failed to open std err %q: %v", p.Stderr, err)
			return
		}
		defer f.Close()
		p.cmd.Stderr = f
	}
	if p.Stdout != "" {
		err := common.CreatePathToFile(p.Stderr)
		f, err := os.OpenFile(p.Stdout, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0777)
		if err != nil {
			log.Warningf("Failed to open std out %q: %v", p.Stdout, err)
			return
		}
		defer f.Close()
		p.cmd.Stdout = f
	}

	p.Running = true
	startTime := time.Now()
	p.cmd.Run()

	if time.Since(startTime) < 3*time.Second {
		log.Error("Collector exits immediately, this should not happen! Please check your collector configuration!")
	}

	return
}
