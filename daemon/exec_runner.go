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

package daemon

import (
	"errors"
	"github.com/Graylog2/collector-sidecar/helpers"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/flynn-archive/go-shlex"

	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/logger"
)

type ExecRunner struct {
	RunnerCommon
	exec           string
	args           string
	stderr, stdout string
	isRunning      atomic.Value
	isSupervised   atomic.Value
	restartCount   int
	startTime      time.Time
	cmd            *exec.Cmd
	signals        chan string
	terminate      chan error
}

func init() {
	if err := RegisterBackendRunner("exec", NewExecRunner); err != nil {
		log.Fatal(err)
	}
}

func NewExecRunner(backend backends.Backend, context *context.Ctx) Runner {
	r := &ExecRunner{
		RunnerCommon: RunnerCommon{
			name:    backend.Name,
			context: context,
			backend: backend,
		},
		exec:         backend.ExecutablePath,
		args:         backend.ExecuteParameters,
		restartCount: 1,
		signals:      make(chan string),
		stderr:       filepath.Join(context.UserConfig.LogPath, backend.Name+"_stderr.log"),
		stdout:       filepath.Join(context.UserConfig.LogPath, backend.Name+"_stdout.log"),
		terminate:    make(chan error),
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

func (r *ExecRunner) GetBackend() *backends.Backend {
	return &r.backend
}

func (r *ExecRunner) SetBackend(b backends.Backend) {
	r.backend = b
	r.name = b.Name
	r.stderr = filepath.Join(r.context.UserConfig.LogPath, b.Name+"_stderr.log")
	r.stdout = filepath.Join(r.context.UserConfig.LogPath, b.Name+"_stdout.log")
	r.exec = b.ExecutablePath
	r.args = b.ExecuteParameters
	r.restartCount = 1
}

func (r *ExecRunner) ResetRestartCounter() {
	r.restartCount = 1
}

func (r *ExecRunner) ValidateBeforeStart() error {
	err := r.backend.CheckExecutableAgainstAccesslist(r.context)
	if err != nil {
		r.backend.SetStatusLogErrorf(err.Error())
		return err
	}

	_, err = exec.LookPath(r.exec)
	if err != nil {
		return r.backend.SetStatusLogErrorf("Failed to find collector executable %s: %s", r.exec, err)
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
				r.backend.SetStatusLogErrorf("Unable to start collector after 3 tries, giving up!")

				if output := r.readCollectorOutput(); output != "" {
					log.Errorf("[%s] Collector output: %s", r.name, output)
					r.backend.SetVerboseStatus(output)
				}
				r.setSupervised(false)
				continue
			}

			log.Errorf("[%s] Backend finished unexpectedly, trying to restart %d/3.", r.name, r.restartCount)
			r.restartCount += 1
			r.Restart()
		}
	}()
}

func (r *ExecRunner) readCollectorOutput() string {
	output := ""
	for _, file := range []string{r.stderr, r.stdout} {
		out, err := ioutil.ReadFile(file)
		if err == nil {
			output += string(out)
		}
	}
	return output
}

func (r *ExecRunner) start() error {
	if err := r.ValidateBeforeStart(); err != nil {
		return err
	}

	// setup process environment
	var err error
	var quotedArgs []string
	if runtime.GOOS == "windows" {
		quotedArgs = helpers.CommandLineToArgv(r.args)
	} else {
		quotedArgs, err = shlex.Split(r.args)
	}
	if err != nil {
		return err
	}
	r.cmd = exec.Command(r.exec, quotedArgs...)
	r.cmd.Dir = r.daemon.Dir
	r.cmd.Env = append(os.Environ(), r.daemon.Env...)
	Setpgid(r.cmd) // run with a new process group (unix only)

	r.terminate = make(chan error)
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

	KillProcess(r, 5*time.Second)

	if !r.Running() {
		r.backend.SetStatus(backends.StatusStopped, "Stopped", "")
	} else {
		r.backend.SetStatus(backends.StatusError, "Failed to be stopped", "")
	}

	return nil
}

func (r *ExecRunner) Restart() error {
	r.signals <- "restart"
	return nil
}

func (r *ExecRunner) restart() error {
	if r.Running() {
		r.stop()
		limit := 5
		for timeout := 0; r.Running() && timeout < limit; timeout++ {
			log.Debugf("[%s] waiting %ds/%ds for process to finish...", r.Name(), timeout, limit)
			time.Sleep(1 * time.Second)
		}
	}
	if r.Running() {
		// skip the hanging r.cmd.Wait() goroutine
		r.terminate <- errors.New("timeout")
		<-r.terminate // wait for termination
	}

	// wipe collector log files after each try
	os.Truncate(r.stderr, 0)
	os.Truncate(r.stdout, 0)
	err := r.start()
	if err != nil {
		log.Errorf("[%s] got start error: %s", r.Name(), err)
	}

	return nil
}

func (r *ExecRunner) run() {
	log.Infof("[%s] Starting (%s driver)", r.name, r.backend.ServiceType)

	if r.stderr != "" {
		err := common.CreatePathToFile(r.stderr)
		if err != nil {
			r.backend.SetStatusLogErrorf("Failed to create path to collector's stderr log: %s", r.stderr)
		}

		f := logger.GetRotatedLog(r.stderr, r.context.UserConfig.LogRotateMaxFileSize, r.context.UserConfig.LogRotateKeepFiles)
		r.cmd.Stderr = f
	}
	if r.stdout != "" {
		err := common.CreatePathToFile(r.stdout)
		if err != nil {
			r.backend.SetStatusLogErrorf("Failed to create path to collector's stdout log: %s", r.stdout)
		}

		f := logger.GetRotatedLog(r.stdout, r.context.UserConfig.LogRotateMaxFileSize, r.context.UserConfig.LogRotateKeepFiles)
		r.cmd.Stdout = f
	}

	r.backend.SetStatus(backends.StatusRunning, "Running", "")
	err := r.cmd.Start()
	if err != nil {
		r.backend.SetStatusLogErrorf("Failed to start collector: %s", err)
	}

	// wait for process exit in the background. Ensure single cmd.Wait() call
	// to deal with hanging processes, provide a termination channel
	go func() {
		r.setRunning(true)
		go func(terminate chan error) {
			err := r.cmd.Wait()
			terminate <- err
		}(r.terminate)

		err := <-r.terminate
		if err != nil {
			log.Debugf("[%s] Wait() error %s", r.name, err)
		}
		r.setRunning(false)
		r.backend.SetStatus(backends.StatusStopped, "Stopped", "")
		r.terminate <- nil //confirm termination
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
