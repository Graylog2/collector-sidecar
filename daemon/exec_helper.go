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
	"os/exec"
	"syscall"
	"time"
)

func Setpgid(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func KillProcess(r *ExecRunner, timeout time.Duration) {
	pid := r.cmd.Process.Pid

	if pid == -1 {
		log.Debugf("[%s] Process already released", r.Name())
		return
	}
	if pid == 0 {
		log.Debugf("[%s] Process not initialized", r.Name())
		return
	}

	// Signal the process group (-pid) instead of just the process. Otherwise, forked child processes
	// can keep running and cause cmd.Wait to hang.
	log.Infof("[%s] SIGTERM process group", r.Name())
	err := syscall.Kill(-pid, syscall.SIGTERM)
	if err != nil {
		log.Infof("[%s] Failed to SIGTERM process group %s", r.Name(), err)
	}

	limit := timeout.Milliseconds()
	tick := 100 * time.Millisecond
	for t := tick.Milliseconds(); r.Running() && t < limit; t += tick.Milliseconds() {
		log.Infof("[%s] Waiting for process group to finish (%vms / %vms)", r.Name(), t, limit)
		time.Sleep(tick)
	}

	if r.Running() {
		log.Infof("[%s] SIGKILL process group", r.Name())
		err := syscall.Kill(-pid, syscall.SIGKILL)
		if err != nil {
			log.Debugf("[%s] Failed to SIGKILL process group %s", r.Name(), err)
		}
	}
}
