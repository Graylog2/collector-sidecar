package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/kardianos/service"

	"mariussturm/gxlog/util"
	"runtime"
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

func NewConfig(glServer string, glPort int, nxPath string) *Config {
	rootDir, err := util.GetRootPath()
	if err != nil {
		logrus.Error("Can not access root directory")
	}
	gxlogPath, err := util.GetGxlogPath()
	if err != nil {
		logrus.Error("Can not access application path")
	}

	execPath := nxPath
	if runtime.GOOS == "windows" {
		execPath, err = util.AppendIfDir(nxPath, "nxlog.exe")
	} else {
		execPath, err = util.AppendIfDir(nxPath, "nxlog")
	}
	if err != nil {
			logrus.Error("Failed to auto-complete nxlog path. Please provide full path to binary")
	}

	c := &Config{
		Name:        "gxlog",
		DisplayName: "gxlog",
		Description: "Wrapper service for Graylog controlled nxlog",
		Dir:         rootDir,
		Exec:        execPath,
		Args:        []string{"-f", "-c", filepath.Join(gxlogPath, "nxlog", "nxlog.conf")},
		Env:         []string{},
		Stderr:      filepath.Join(gxlogPath, "log", "gxlog_err.log"),
		Stdout:      filepath.Join(gxlogPath, "log", "gxlog_err.log"),
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
	fullExec, err := exec.LookPath(p.Exec)
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
	p.cmd.Process.Kill()
	return nil
}

func (p *Program) Restart(s service.Service) error {
	logrus.Info("Restarting nxlog")

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
	logrus.Info("Starting nxlog")

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

	p.cmd.Run()
	return
}

func (p *Program) checkConfigurtionFile() error {
	gxlogPath, _ := util.GetGxlogPath()
	cmd := exec.Command(p.Exec, "-v", "-c", filepath.Join(gxlogPath, "nxlog", "nxlog.conf"))
	err := cmd.Run()
	logrus.Infof("Config validation: %v", err)
	return err
}
