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
	"errors"
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

type ExecRunner struct {
	RunnerCommon
	exec           string
	args           []string
	stderr, stdout string
	isRunning      bool
	isSupervised   bool
	restartCount   int
	startTime      time.Time
	cmd            *exec.Cmd
	service        service.Service
	signals        chan string
}

func init() {
	if err := RegisterBackendRunner("exec", NewExecRunner); err != nil {
		log.Fatal(err)
	}
}

func NewExecRunner(backend backends.Backend, context *context.Ctx) Runner {
	r := &ExecRunner{
		RunnerCommon: RunnerCommon{
			name:    backend.Name(),
			context: context,
			backend: backend,
		},
		exec:         backend.ExecPath(),
		args:         backend.ExecArgs(),
		isRunning:    false,
		restartCount: 1,
		isSupervised: false,
		signals:      make(chan string),
		stderr:       filepath.Join(context.UserConfig.LogPath, backend.Name()+"_stderr.log"),
		stdout:       filepath.Join(context.UserConfig.LogPath, backend.Name()+"_stdout.log"),
	}

	r.signalProcessor()
	return r
}

func (r *ExecRunner) Name() string {
	return r.name
}

func (r *ExecRunner) Running() bool {
	return r.isRunning
}

func (r *ExecRunner) SetDaemon(d *DaemonConfig) {
	r.daemon = d
}

func (r *ExecRunner) BindToService(s service.Service) {
	r.service = s
}

func (r *ExecRunner) GetService() service.Service {
	return r.service
}

func (r *ExecRunner) ValidateBeforeStart() error {
	_, err := exec.LookPath(r.exec)
	if err != nil {
		return backends.SetStatusLogErrorf(r.name, "Failed to find collector executable %q: %v", r.exec, err)
	}
	if r.isRunning {
		return errors.New("Failed to start collector, it's already running")
	}
	return nil
}

func (r *ExecRunner) StartSupervisor() {
	if r.isSupervised == true {
		log.Debugf("[%s] Won't start second supervisor", r.Name())
		return
	}

	r.isSupervised = true
	r.restartCount = 1
	go func() {
		for {
			// blocks till process exits
			r.cmd.Wait()

			// ignore regular shutdown
			if !r.isRunning {
				time.Sleep(300 * time.Millisecond)
				continue
			}
			// After 60 seconds we can reset the restart counter
			if time.Since(r.startTime) > 60*time.Second {
				r.restartCount = 1
			}
			// don't continue to restart after 3 tries, exit supervisor and wait for a configuration update
			if r.restartCount > 3 {
				backends.SetStatusLogErrorf(r.name, "Unable to start collector after 3 tries, giving up!")
				r.cmd.Wait()
				r.isRunning = false
				break
			}

			log.Errorf("[%s] Backend crashed, trying to restart %d/3", r.name, r.restartCount)
			r.restartCount += 1
			r.Restart()

		}
		r.isSupervised = false
	}()
}

func (r *ExecRunner) Start() error {
	r.signals <- "start"
	return nil
}

func (r *ExecRunner) start() error {
	if err := r.ValidateBeforeStart(); err != nil {
		log.Errorf("[%s] %s", r.Name(), err)
		return err
	}

	// setup process environment
	r.cmd = exec.Command(r.exec, r.args...)
	r.cmd.Dir = r.daemon.Dir
	r.cmd.Env = append(os.Environ(), r.daemon.Env...)

	// start the actual process and don't block
	r.startTime = time.Now()
	r.run()

	// keep the process alive
	r.StartSupervisor()
	r.isRunning = true

	return nil
}

func (r *ExecRunner) Stop() error {
	r.signals <- "stop"
	return nil
}

func (r *ExecRunner) stop() error {
	log.Infof("[%s] Stopping", r.name)

	// deactivate supervisor
	r.isRunning = false

	// give the chance to cleanup resources
	if r.cmd.Process != nil {
		r.cmd.Process.Signal(syscall.SIGHUP)
	}
	time.Sleep(2 * time.Second)

	// in doubt kill the process
	if r.cmd.Process != nil {
		r.cmd.Process.Kill()
	}

	// in case the process is still running we can only wait and prevent a second process to spawn
	r.cmd.Wait()

	return nil
}

func (r *ExecRunner) Restart() error {
	r.Stop()
	r.Start()

	return nil
}

func (r *ExecRunner) run() {
	log.Infof("[%s] Starting (%s driver)", r.name, r.backend.Driver())

	if r.stderr != "" {
		err := common.CreatePathToFile(r.stderr)
		if err != nil {
			backends.SetStatusLogErrorf(r.name, "Failed to create path to collector's stderr log: %s", r.stderr)
		}

		f := common.GetRotatedLog(r.stderr, r.context.UserConfig.LogRotationTime, r.context.UserConfig.LogMaxAge)
		defer f.Close()
		r.cmd.Stderr = f
	}
	if r.stdout != "" {
		err := common.CreatePathToFile(r.stdout)
		if err != nil {
			backends.SetStatusLogErrorf(r.name, "Failed to create path to collector's stdout log: %s", r.stdout)
		}

		f := common.GetRotatedLog(r.stderr, r.context.UserConfig.LogRotationTime, r.context.UserConfig.LogMaxAge)
		defer f.Close()
		r.cmd.Stdout = f
	}

	r.backend.SetStatus(backends.StatusRunning, "Running")
	err := r.cmd.Start()
	if err != nil {
		backends.SetStatusLogErrorf(r.name, "Failed to start collector: %s", err)
	}
}

// process signals sequentially to prevent race conditions with the supervisor
func (r *ExecRunner) signalProcessor () {
	go func() {
		for {
			switch <- r.signals {
			case "stop":
				r.stop()
			case "start":
				r.start()
			}
		}
	}()
}