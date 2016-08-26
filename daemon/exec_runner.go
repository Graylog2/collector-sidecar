package daemon

import (
	"fmt"
	"syscall"
	"time"
	"os"
	"os/exec"
	"path/filepath"

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
	restartCount   int
	startTime      time.Time
	cmd            *exec.Cmd
	service        service.Service
	exit           chan struct{}
}

func init() {
	if err := RegisterBackendRunner("exec", NewExecRunner); err != nil {
		log.Fatal(err)
	}
}

func NewExecRunner(backend backends.Backend, context *context.Ctx) Runner {
	r := &ExecRunner{
		RunnerCommon: RunnerCommon{
			name: backend.Name(),
		 	context: context,
		 	backend:      backend,
		},
		exec:         backend.ExecPath(),
		args:         backend.ExecArgs(),
		isRunning:    false,
		restartCount: 1,
		stderr:       filepath.Join(context.UserConfig.LogPath, backend.Name()+"_stderr.log"),
		stdout:       filepath.Join(context.UserConfig.LogPath, backend.Name()+"_stdout.log"),
		exit:         make(chan struct{}),
	}

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
		msg := "Failed to find collector executable"
		r.backend.SetStatus(backends.StatusError, msg)
		return fmt.Errorf("[%s] %s %q: %v", r.name, msg, r.exec, err)
	}
	return err
}

func (r *ExecRunner) Start(s service.Service) error {
	if err := r.ValidateBeforeStart(); err != nil {
		log.Error(err.Error())
		return err
	}

	r.restartCount = 1
	go func() {
		for {
			r.cmd = exec.Command(r.exec, r.args...)
			r.cmd.Dir = r.daemon.Dir
			r.cmd.Env = append(os.Environ(), r.daemon.Env...)
			r.startTime = time.Now()
			r.run()

			// A backend should stay alive longer than 3 seconds
			if time.Since(r.startTime) < 3*time.Second {
				msg := "Collector exits immediately, this should not happen! Please check your collector configuration!"
				r.backend.SetStatus(backends.StatusError, msg)
				log.Errorf("[%s] %s", r.name, msg)
			}
			// After 60 seconds we can reset the restart counter
			if time.Since(r.startTime) > 60*time.Second {
				r.restartCount = 0
			}
			if r.restartCount <= 3 && r.isRunning {
				log.Errorf("[%s] Backend crashed, trying to restart %d/3", r.name, r.restartCount)
				time.Sleep(5 * time.Second)
				r.restartCount += 1
				continue
				// giving up
			} else if r.restartCount > 3 {
				msg := "Collector failed to start after 3 tries!"
				r.backend.SetStatus(backends.StatusError, msg)
				log.Errorf("[%s] %s", r.name, msg)
			}

			r.isRunning = false
			break
		}
	}()
	return nil
}

func (r *ExecRunner) Stop(s service.Service) error {
	log.Infof("[%s] Stopping", r.name)

	// deactivate supervisor
	r.isRunning = false

	// give the chance to cleanup resources
	if r.cmd.Process != nil {
		r.cmd.Process.Signal(syscall.SIGHUP)
	}
	time.Sleep(2 * time.Second)

	close(r.exit)
	if r.cmd.Process != nil {
		r.cmd.Process.Kill()
	}
	return nil
}

func (r *ExecRunner) Restart(s service.Service) error {
	r.Stop(s)
	time.Sleep(2 * time.Second)
	r.exit = make(chan struct{})
	r.Start(s)

	return nil
}

func (r *ExecRunner) run() {
	log.Infof("[%s] Starting with %s driver", r.name, r.backend.Driver())

	if r.stderr != "" {
		err := common.CreatePathToFile(r.stderr)
		if err != nil {
			msg := "Failed to create path to collector's stderr log"
			r.backend.SetStatus(backends.StatusError, msg)
			log.Errorf("[%s] %s: %s", r.name, msg, r.stderr)
		}

		f := common.GetRotatedLog(r.stderr, r.context.UserConfig.LogRotationTime, r.context.UserConfig.LogMaxAge)
		defer f.Close()
		r.cmd.Stderr = f
	}
	if r.stdout != "" {
		err := common.CreatePathToFile(r.stdout)
		if err != nil {
			msg := "Failed to create path to collector's stdout log"
			r.backend.SetStatus(backends.StatusError, msg)
			log.Errorf("[%s] %s: %s", r.name, msg, r.stdout)
		}

		f := common.GetRotatedLog(r.stderr, r.context.UserConfig.LogRotationTime, r.context.UserConfig.LogMaxAge)
		defer f.Close()
		r.cmd.Stdout = f
	}

	r.isRunning = true
	r.backend.SetStatus(backends.StatusRunning, "Running")
	r.cmd.Run()

	return
}
