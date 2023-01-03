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

//go:build !windows
// +build !windows

package daemon

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func Setpgid(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
func KillProcess(r *ExecRunner, proc *os.Process) {
	log.Debugf("[%s] PID SIGHUP ignored, sending SIGHUP to process group", r.Name())
	err := syscall.Kill(-proc.Pid, syscall.SIGHUP)
	if err != nil {
		log.Debugf("[%s] Failed to HUP process group %s", r.Name(), err)
	}
	time.Sleep(2 * time.Second)
	if r.Running() {
		err := syscall.Kill(-proc.Pid, syscall.SIGKILL)
		if err != nil {
			log.Debugf("[%s] Failed to kill process group %s", r.Name(), err)
		}
	}
}
