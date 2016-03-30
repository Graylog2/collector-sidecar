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
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/context"
)

var Daemon *DaemonConfig
var log = common.Log()

type DaemonConfig struct {
	Name        string
	DisplayName string
	Description string

	Dir         string
	Env         []string

	Runner       map[string]*Runner
}

type Runner struct {
	Name 	       string
	Exec           string
	Args           []string
	Stderr, Stdout string
	Running        bool
	Daemon 	       *DaemonConfig
	cmd            *exec.Cmd
	service        service.Service
	exit           chan struct{}
}

func init() {
	Daemon = NewConfig()
}

func NewConfig() *DaemonConfig {
	rootDir, err := common.GetRootPath()
	if err != nil {
		log.Error("Can not access root directory")
	}

	dc := &DaemonConfig{
		Name:        "collector-sidecar",
		DisplayName: "Graylog collector sidecar",
		Description: "Wrapper service for Graylog controlled collector",
		Dir:         rootDir,
		Env:         []string{},
		Runner:	     map[string]*Runner{},
	}

	return dc
}

func (dc *DaemonConfig) NewRunner(backend backends.Backend, logPath string) *Runner {
	r := &Runner{
		Running: false,
		Name:    backend.Name(),
		Exec:	 backend.ExecPath(),
		Args:	 backend.ExecArgs(),
		Stderr:  filepath.Join(logPath, backend.Name() + "_stderr.log"),
		Stdout:  filepath.Join(logPath, backend.Name() + "_stdout.log"),
		Daemon:  dc,
		exit:    make(chan struct{}),
	}

	return r
}

func (dc *DaemonConfig) AddBackendAsRunner(backend backends.Backend, context *context.Ctx) {
	dc.Runner[backend.Name()] = dc.NewRunner(backend, context.UserConfig.LogPath)
}

func (r *Runner) BindToService(s service.Service) {
	r.service = s
}

func (r *Runner) GetService() service.Service {
	return r.service
}

func (r *Runner) Start(s service.Service) error {
	absPath, _ := filepath.Abs(r.Exec)
	fullExec, err := exec.LookPath(absPath)
	if err != nil {
		return fmt.Errorf("[%s] Failed to find collector executable %q: %v", r.Name, r.Exec, err)
	}

	r.cmd = exec.Command(fullExec, r.Args...)
	r.cmd.Dir = r.Daemon.Dir
	r.cmd.Env = append(os.Environ(), r.Daemon.Env...)

	go r.run()
	return nil
}

func (r *Runner) Stop(s service.Service) error {
	log.Infof("[%s] Stopping", r.Name)
	close(r.exit)
	if r.cmd.Process != nil {
		r.cmd.Process.Kill()
	}
	return nil
}

func (r *Runner) Restart(s service.Service) error {
	r.Stop(s)
	time.Sleep(3 * time.Second)
	r.exit = make(chan struct{})
	r.Start(s)

	return nil
}

func (r *Runner) run() {
	log.Infof("[%s] Starting", r.Name)

	if r.Stderr != "" {
		err := common.CreatePathToFile(r.Stderr)
		f, err := os.OpenFile(r.Stderr, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0777)
		if err != nil {
			log.Warningf("[%s] Failed to open std err %q: %v", r.Name, r.Stderr, err)
			return
		}
		defer f.Close()
		r.cmd.Stderr = f
	}
	if r.Stdout != "" {
		err := common.CreatePathToFile(r.Stderr)
		f, err := os.OpenFile(r.Stdout, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0777)
		if err != nil {
			log.Warningf("[%s] Failed to open std out %q: %v", r.Name, r.Stdout, err)
			return
		}
		defer f.Close()
		r.cmd.Stdout = f
	}

	r.Running = true
	startTime := time.Now()
	r.cmd.Run()

	if time.Since(startTime) < 3*time.Second {
		log.Errorf("[%s] Collector exits immediately, this should not happen! Please check your collector configuration!", r.Name)
	}

	return
}
