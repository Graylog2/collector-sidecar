package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/kardianos/service"

	"github.com/Graylog2/nxlog-sidecar/util"
)

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
	service service.Service
	*Config
	cmd *exec.Cmd
}

func NewConfig(collectorPath string) *Config {
	rootDir, err := util.GetRootPath()
	if err != nil {
		logrus.Error("Can not access root directory")
	}
	sidecarPath, err := util.GetSidecarPath()
	if err != nil {
		logrus.Error("Can not access filepath to sidecar")
	}

	execPath := collectorPath
	if runtime.GOOS == "windows" {
		execPath, err = util.AppendIfDir(collectorPath, "nxlog.exe")
	} else {
		execPath, err = util.AppendIfDir(collectorPath, "nxlog")
	}
	if err != nil {
		logrus.Error("Failed to auto-complete nxlog path. Please provide full path to binary")
	}

	c := &Config{
		Name:        "sidecar",
		DisplayName: "sidecar",
		Description: "Wrapper service for Graylog controlled collector",
		Dir:         rootDir,
		Exec:        execPath,
		Args:        []string{"-f", "-c", filepath.Join(sidecarPath, "nxlog", "nxlog.conf")},
		Env:         []string{},
		Stderr:      filepath.Join(sidecarPath, "log", "sidecar.log"),
		Stdout:      filepath.Join(sidecarPath, "log", "sidecar.log"),
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
		return fmt.Errorf("Failed to find nxlog executable %q: %v", p.Exec, err)
	}

	p.cmd = exec.Command(fullExec, p.Args...)
	p.cmd.Dir = p.Dir
	p.cmd.Env = append(os.Environ(), p.Env...)

	go p.run()
	return nil
}

func (p *Program) Stop(s service.Service) error {
	logrus.Info("Stopping nxlog")
	close(p.exit)
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
	return nil
}

func (p *Program) Restart(s service.Service) error {
	for p.checkConfigurtionFile() != nil {
		logrus.Info("Configuration file for nxlog is not valid, waiting for update...")
		time.Sleep(30 * time.Second)
	}

	p.Stop(s)
	time.Sleep(3 * time.Second)
	p.exit = make(chan struct{})
	p.Start(s)

	return nil
}

func (p *Program) run() {
	logrus.Info("Starting collector")

	if p.Stderr != "" {
		f, err := os.OpenFile(p.Stderr, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0777)
		if err != nil {
			logrus.Warningf("Failed to open std err %q: %v", p.Stderr, err)
			return
		}
		defer f.Close()
		p.cmd.Stderr = f
	}
	if p.Stdout != "" {
		f, err := os.OpenFile(p.Stdout, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0777)
		if err != nil {
			logrus.Warningf("Failed to open std out %q: %v", p.Stdout, err)
			return
		}
		defer f.Close()
		p.cmd.Stdout = f
	}

	startTime := time.Now()
	p.cmd.Run()

	if time.Since(startTime) < 3*time.Second {
		logrus.Error("collector exits immediately, this should not happen! Please check your collector configuration!")
	}

	return
}

func (p *Program) checkConfigurtionFile() error {
	sidecarPath, _ := util.GetSidecarPath()
	cmd := exec.Command(p.Exec, "-v", "-c", filepath.Join(sidecarPath, "nxlog", "nxlog.conf"))
	err := cmd.Run()
	if err != nil {
		logrus.Error("Error during configuration validation: ", err)
	}
	return err
}
