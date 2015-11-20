package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kardianos/osext"
	"github.com/kardianos/service"
	"github.com/rakyll/globalconf"

	"mariussturm/gxlog/nxconfig"
)

type Config struct {
	Name, DisplayName, Description string

	Dir  string
	Exec string
	Args []string
	Env  []string

	Stderr, Stdout string
}

type program struct {
	exit    chan struct{}
	service service.Service

	*Config

	cmd *exec.Cmd
}

type Properties map[string]string

var logger service.Logger

func (p *program) Start(s service.Service) error {
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

func (p *program) run() {
	logger.Info("Starting ", p.DisplayName)
	defer func() {
		if service.Interactive() {
			p.Stop(p.service)
		} else {
			p.service.Stop()
		}
	}()

	if p.Stderr != "" {
		f, err := os.OpenFile(p.Stderr, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0777)
		if err != nil {
			logger.Warningf("Failed to open std err %q: %v", p.Stderr, err)
			return
		}
		defer f.Close()
		p.cmd.Stderr = f
	}
	if p.Stdout != "" {
		f, err := os.OpenFile(p.Stdout, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0777)
		if err != nil {
			logger.Warningf("Failed to open std out %q: %v", p.Stdout, err)
			return
		}
		defer f.Close()
		p.cmd.Stdout = f
	}

	err := p.cmd.Run()
	if err != nil {
		logger.Warningf("Error running: %v", err)
	}

	return
}

func (p *program) Stop(s service.Service) error {
	close(p.exit)
	logger.Info("Stopping ", p.DisplayName)
	if p.cmd.ProcessState.Exited() == false {
		p.cmd.Process.Kill()
	}
	if service.Interactive() {
		os.Exit(0)
	}
	return nil
}

func getGxlogPath() (string, error) {
	fullexecpath, err := osext.Executable()
	if err != nil {
		return "", err
	}

	dir, _ := filepath.Split(fullexecpath)
	return dir, nil
}

func main() {
	gxlogPath, _ := getGxlogPath()
	conf, _ := globalconf.NewWithOptions(&globalconf.Options{
	    Filename:  filepath.Join(gxlogPath, "gxlog.ini"),
	    EnvPrefix: "GXLOG_",
	})
	//conf, _ := globalconf.New("gxlog")

	var (
		svcFlag  = flag.String("service", "", "Control the system service.")
		nxPath   = flag.String("nxpath", "", "Path to nxlog installation")
		glServer = flag.String("glserver", "", "Graylog server IP")
		glPort   = flag.String("glport", "", "Graylog server GELF port")
	)
	conf.ParseAll()

	nxc := nxconfig.NewNxConfig(*glServer, *glPort, *nxPath)
	nxc.FetchFromServer(*glServer)
	//fmt.Print("nxlog configuration: ", nxc.Render().String)
	nxc.RenderToFile(filepath.Join(gxlogPath, "nxlog", "nxlog.conf"))

	config := &Config{
		Name:        "gxlog",
		DisplayName: "gxlog",
		Description: "Wrapper service for Graylog controlled nxlog",
		Dir:         "C:",
		Exec:        filepath.Join(*nxPath, "nxlog.exe"),
		Args:        []string{"-f", "-c", filepath.Join(gxlogPath, "nxlog", "nxlog.conf")},
		Env:         []string{},
		Stderr:      filepath.Join(gxlogPath, "log", "gxlog_err.log"),
		Stdout:      filepath.Join(gxlogPath, "log", "gxlog_err.log"),
	}

	svcConfig := &service.Config{
		Name:        config.Name,
		DisplayName: config.DisplayName,
		Description: config.Description,
	}

	prg := &program{
		exit:   make(chan struct{}),
		Config: config,
	}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	prg.service = s

	errs := make(chan error, 5)
	logger, err = s.Logger(errs)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			err := <-errs
			if err != nil {
				log.Print(err)
			}
		}
	}()

	go func() {
		for {
			nxc.FetchFromServer(*glServer)
			needsRestart, _ := nxc.RenderToFile(filepath.Join(gxlogPath, "nxlog", "nxlog.conf"))
			if (needsRestart) {
				log.Printf("Config updated!")
			}
			time.Sleep((10)*time.Second)
		}
	}()
	
	if len(*svcFlag) != 0 {
		err := service.Control(s, *svcFlag)
		if err != nil {
			log.Printf("Valid actions: %q\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}
	err = s.Run()
	if err != nil {
		logger.Error(err)
	}
}
