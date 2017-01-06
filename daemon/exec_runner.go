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
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/logger"
)

type ExecRunner struct {
	RunnerCommon
	exec           string
	args           []string
	stderr, stdout string
	isRunning      atomic.Value
	isSupervised   atomic.Value
	restartCount   int
	startTime      time.Time
	cmd            *exec.Cmd
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
		restartCount: 1,
		signals:      make(chan string),
		stderr:       filepath.Join(context.UserConfig.LogPath, backend.Name()+"_stderr.log"),
		stdout:       filepath.Join(context.UserConfig.LogPath, backend.Name()+"_stdout.log"),
	}

	// set default state
	r.setRunning(false)
	r.setSupervised(false)

	r.signalProcessor()
	r.startSupervisor()

	return r
}

func (r *ExecRunner) Name() string {
	return r.name
}

func (r *ExecRunner) Running() bool {
	return r.isRunning.Load().(bool)
}

func (r *ExecRunner) setRunning(state bool) {
	r.isRunning.Store(state)
}

func (r *ExecRunner) Supervised() bool {
	return r.isSupervised.Load().(bool)
}

func (r *ExecRunner) setSupervised(state bool) {
	r.isSupervised.Store(state)
}

func (r *ExecRunner) SetDaemon(d *DaemonConfig) {
	r.daemon = d
}

func (r *ExecRunner) ValidateBeforeStart() error {
	_, err := exec.LookPath(r.exec)
	if err != nil {
		return backends.SetStatusLogErrorf(r.name, "Failed to find collector executable %q: %v", r.exec, err)
	}
	if r.Running() {
		return errors.New("Failed to start collector, it's already running")
	}
	return nil
}

func (r *ExecRunner) startSupervisor() {
	r.restartCount = 1
	go func() {
		for {
			// prevent cpu lock
			time.Sleep(1 * time.Second)

			// ignore regular shutdown
			if !r.Supervised() {
				continue
			}

			// check if process exited
			if r.Running() {
				continue
			}

			// after 60 seconds we can reset the restart counter
			if time.Since(r.startTime) > 60*time.Second {
				r.restartCount = 1
			}
			// don't continue to restart after 3 tries, stop the supervisor and wait for a configuration update
			// or manual restart
			if r.restartCount > 3 {
				backends.SetStatusLogErrorf(r.name, "Unable to start collector after 3 tries, giving up!")
				r.setSupervised(false)
				continue
			}

			log.Errorf("[%s] Backend finished unexpectedly, trying to restart %d/3.", r.name, r.restartCount)
			r.restartCount += 1
			r.Restart()
		}
	}()
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

	r.setSupervised(true)
	return nil
}

func (r *ExecRunner) Shutdown() error {
	r.signals <- "shutdown"
	return nil
}

func (r *ExecRunner) stop() error {
	// deactivate supervisor
	r.setSupervised(false)

	// if the command hasn't been started yet or doesn't run anymore, just return
	if r.cmd == nil || r.cmd.Process == nil {
		return nil
	}

	log.Infof("[%s] Stopping", r.name)

	// give the chance to cleanup resources
	if r.cmd.Process != nil && runtime.GOOS != "windows"{
		r.cmd.Process.Signal(syscall.SIGHUP)
	}
	time.Sleep(2 * time.Second)

	// in doubt kill the process
	if r.cmd.Process != nil {
		log.Debugf("[%s] SIGHUP ignored, killing process", r.Name())
		err := r.cmd.Process.Kill()
		if err != nil {
			log.Debugf("[%s] Failed to kill process %s", r.Name(), err)
		}
	}

	return nil
}

func (r *ExecRunner) Restart() error {
	r.signals <- "restart"
	return nil
}

func (r *ExecRunner) restart() error {
	r.stop()
	for timeout := 0; r.Running() || timeout >= 5; timeout++ {
		log.Debugf("[%s] waiting for process to finish...", r.Name())
		time.Sleep(1 * time.Second)
	}
	r.start()

	return nil
}

func (r *ExecRunner) run() {
	log.Infof("[%s] Starting (%s driver)", r.name, r.backend.Driver())

	if r.stderr != "" {
		err := common.CreatePathToFile(r.stderr)
		if err != nil {
			backends.SetStatusLogErrorf(r.name, "Failed to create path to collector's stderr log: %s", r.stderr)
		}

		f := logger.GetRotatedLog(r.stderr, r.context.UserConfig.LogRotationTime, r.context.UserConfig.LogMaxAge)
		defer f.Close()
		r.cmd.Stderr = f
	}
	if r.stdout != "" {
		err := common.CreatePathToFile(r.stdout)
		if err != nil {
			backends.SetStatusLogErrorf(r.name, "Failed to create path to collector's stdout log: %s", r.stdout)
		}

		f := logger.GetRotatedLog(r.stderr, r.context.UserConfig.LogRotationTime, r.context.UserConfig.LogMaxAge)
		defer f.Close()
		r.cmd.Stdout = f
	}

	r.backend.SetStatus(backends.StatusRunning, "Running")
	err := r.cmd.Start()
	if err != nil {
		backends.SetStatusLogErrorf(r.name, "Failed to start collector: %s", err)
	}

	// wait for process exit in the background. Ensure single cmd.Wait() call
	go func() {
		r.setRunning(true)
		r.cmd.Wait()
		r.setRunning(false)
	}()
}

// process signals sequentially to prevent race conditions with the supervisor
func (r *ExecRunner) signalProcessor() {
	go func() {
		seq := 0
		for {
			cmd := <-r.signals
			seq++
			log.Debugf("[signal-processor] (seq=%d) handling cmd: %v", seq, cmd)
			switch cmd {
			case "restart":
				r.restart()
			case "shutdown":
				r.stop()
			}
			log.Debugf("[signal-processor] (seq=%d) cmd done: %v", seq, cmd)
		}
	}()
}
