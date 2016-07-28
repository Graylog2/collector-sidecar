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
	"syscall"
	"time"

	"github.com/kardianos/service"

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
)

var Daemon *DaemonConfig
var log = common.Log()

type DaemonConfig struct {
	Name        string
	DisplayName string
	Description string

	Dir string
	Env []string

	Runner map[string]*Runner
}

type Runner struct {
	Name           string
	Exec           string
	Args           []string
	Stderr, Stdout string
	Running        bool
	RestartCount   int
	StartTime      time.Time
	Backend        backends.Backend
	Context        *context.Ctx
	Daemon         *DaemonConfig
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
		Runner:      map[string]*Runner{},
	}

	return dc
}

func (dc *DaemonConfig) NewRunner(backend backends.Backend, context *context.Ctx) *Runner {
	r := &Runner{
		Running:      false,
		Context:      context,
		Backend:      backend,
		Name:         backend.Name(),
		Exec:         backend.ExecPath(),
		Args:         backend.ExecArgs(),
		RestartCount: 1,
		Stderr:       filepath.Join(context.UserConfig.LogPath, backend.Name()+"_stderr.log"),
		Stdout:       filepath.Join(context.UserConfig.LogPath, backend.Name()+"_stdout.log"),
		Daemon:       dc,
		exit:         make(chan struct{}),
	}

	return r
}

func (dc *DaemonConfig) AddBackendAsRunner(backend backends.Backend, context *context.Ctx) {
	dc.Runner[backend.Name()] = dc.NewRunner(backend, context)
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
		msg := "Failed to find collector executable"
		r.Backend.SetStatus(backends.StatusError, msg)
		return fmt.Errorf("[%s] %s %q: %v", r.Name, msg, r.Exec, err)
	}

	r.RestartCount = 1
	go func() {
		for {
			r.cmd = exec.Command(fullExec, r.Args...)
			r.cmd.Dir = r.Daemon.Dir
			r.cmd.Env = append(os.Environ(), r.Daemon.Env...)
			r.StartTime = time.Now()
			r.run()

			// A backend should stay alive longer than 3 seconds
			if time.Since(r.StartTime) < 3*time.Second {
				msg := "Collector exits immediately, this should not happen! Please check your collector configuration!"
				r.Backend.SetStatus(backends.StatusError, msg)
				log.Errorf("[%s] %s", r.Name, msg)
			}
			// After 60 seconds we can reset the restart counter
			if time.Since(r.StartTime) > 60*time.Second {
				r.RestartCount = 0
			}
			if r.RestartCount <= 3 && r.Running {
				log.Errorf("[%s] Backend crashed, trying to restart %d/3", r.Name, r.RestartCount)
				time.Sleep(5 * time.Second)
				r.RestartCount += 1
				continue
				// giving up
			} else if r.RestartCount > 3 {
				msg := "Collector failed to start after 3 tries!"
				r.Backend.SetStatus(backends.StatusError, msg)
				log.Errorf("[%s] %s", r.Name, msg)
			}

			r.Running = false
			break
		}
	}()
	return nil
}

func (r *Runner) Stop(s service.Service) error {
	log.Infof("[%s] Stopping", r.Name)

	// deactivate supervisor
	r.Running = false

	// give the chance to cleanup resources
	r.cmd.Process.Signal(syscall.SIGHUP)
	time.Sleep(2 * time.Second)

	close(r.exit)
	if r.cmd.Process != nil {
		r.cmd.Process.Kill()
	}
	return nil
}

func (r *Runner) Restart(s service.Service) error {
	r.Stop(s)
	time.Sleep(2 * time.Second)
	r.exit = make(chan struct{})
	r.Start(s)

	return nil
}

func (r *Runner) run() {
	log.Infof("[%s] Starting", r.Name)

	if r.Stderr != "" {
		err := common.CreatePathToFile(r.Stderr)
		if err != nil {
			msg := "Failed to create path to collector's stderr log"
			r.Backend.SetStatus(backends.StatusError, msg)
			log.Errorf("[%s] %s: %s", r.Name, msg, r.Stderr)
		}

		f := common.GetRotatedLog(r.Stderr, r.Context.UserConfig.LogRotationTime, r.Context.UserConfig.LogMaxAge)
		defer f.Close()
		r.cmd.Stderr = f
	}
	if r.Stdout != "" {
		err := common.CreatePathToFile(r.Stdout)
		if err != nil {
			msg := "Failed to create path to collector's stdout log"
			r.Backend.SetStatus(backends.StatusError, msg)
			log.Errorf("[%s] %s: %s", r.Name, msg, r.Stdout)
		}

		f := common.GetRotatedLog(r.Stderr, r.Context.UserConfig.LogRotationTime, r.Context.UserConfig.LogMaxAge)
		defer f.Close()
		r.cmd.Stdout = f
	}

	r.Running = true
	r.Backend.SetStatus(backends.StatusRunning, "Running")
	r.cmd.Run()

	return
}
