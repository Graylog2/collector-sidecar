// Copyright (C)  2026 Graylog, Inc.
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
//
// SPDX-License-Identifier: SSPL-1.0

package keen

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Config holds the configuration for the commander.
type Config struct {
	Executable      string
	Args            []string
	Env             map[string]string
	PassthroughLogs bool
}

// Commander manages the lifecycle of an agent process.
type Commander struct {
	logger  *zap.Logger
	logsDir string
	cfg     Config
	cmd     *exec.Cmd
	running atomic.Bool
	doneCh  chan struct{}
	exitCh  chan struct{}
}

// New creates a new Commander instance.
func New(logger *zap.Logger, logsDir string, cfg Config) (*Commander, error) {
	return &Commander{
		logger:  logger,
		logsDir: logsDir,
		cfg:     cfg,
		doneCh:  make(chan struct{}, 1),
		exitCh:  make(chan struct{}, 1),
	}, nil
}

// Start starts the agent process.
func (c *Commander) Start(ctx context.Context) error {
	if !c.running.CompareAndSwap(false, true) {
		return nil // Already running
	}

	// Drain channels from previous runs
	select {
	case <-c.doneCh:
	default:
	}
	select {
	case <-c.exitCh:
	default:
	}

	c.logger.Debug("Starting agent", zap.String("executable", c.cfg.Executable))

	c.cmd = exec.CommandContext(ctx, c.cfg.Executable, c.cfg.Args...)
	c.cmd.Env = c.buildEnv()
	c.cmd.SysProcAttr = sysProcAttrs()

	if c.cfg.PassthroughLogs {
		return c.startWithPassthroughLogging()
	}
	return c.startNormal()
}

func (c *Commander) buildEnv() []string {
	if c.cfg.Env == nil {
		return nil
	}
	env := os.Environ()
	for k, v := range c.cfg.Env {
		env = append(env, k+"="+v)
	}
	return env
}

func (c *Commander) startNormal() error {
	logFilePath := filepath.Join(c.logsDir, "agent.log")
	logFile, err := os.Create(logFilePath)
	if err != nil {
		c.running.Store(false)
		return fmt.Errorf("cannot create log file %s: %w", logFilePath, err)
	}

	c.cmd.Stdout = logFile
	c.cmd.Stderr = logFile

	if err := c.cmd.Start(); err != nil {
		logFile.Close()
		c.running.Store(false)
		return fmt.Errorf("failed to start agent: %w", err)
	}

	c.logger.Debug("Agent process started", zap.Int("pid", c.cmd.Process.Pid))

	go func() {
		defer logFile.Close()
		c.watch()
	}()

	return nil
}

func (c *Commander) startWithPassthroughLogging() error {
	stdoutPipe, err := c.cmd.StdoutPipe()
	if err != nil {
		c.running.Store(false)
		return fmt.Errorf("stdout pipe: %w", err)
	}

	stderrPipe, err := c.cmd.StderrPipe()
	if err != nil {
		c.running.Store(false)
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := c.cmd.Start(); err != nil {
		c.running.Store(false)
		return fmt.Errorf("failed to start agent: %w", err)
	}

	agentLogger := c.logger.Named("agent")

	go c.pipeOutput(stdoutPipe, agentLogger, false)
	go c.pipeOutput(stderrPipe, agentLogger, true)

	c.logger.Debug("Agent process started", zap.Int("pid", c.cmd.Process.Pid))

	go c.watch()

	return nil
}

func (c *Commander) pipeOutput(pipe io.ReadCloser, logger *zap.Logger, isStderr bool) {
	reader := bufio.NewReader(pipe)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF && !errors.Is(err, os.ErrClosed) {
				c.logger.Error("Error reading agent output", zap.Error(err))
			}
			if line != "" {
				line = strings.TrimRight(line, "\r\n")
				if isStderr {
					logger.Error(line)
				} else {
					logger.Info(line)
				}
			}
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if isStderr {
			logger.Error(line)
		} else {
			logger.Info(line)
		}
	}
}

func (c *Commander) watch() {
	err := c.cmd.Wait()

	var exitError *exec.ExitError
	if err != nil && !errors.As(err, &exitError) {
		c.logger.Error("Error watching agent process", zap.Error(err))
	}

	c.running.Store(false)

	select {
	case c.doneCh <- struct{}{}:
	default:
	}
	select {
	case c.exitCh <- struct{}{}:
	default:
	}
}

// Stop stops the agent process gracefully.
func (c *Commander) Stop(ctx context.Context) error {
	if !c.running.Load() {
		return nil
	}

	pid := c.cmd.Process.Pid
	c.logger.Debug("Stopping agent process", zap.Int("pid", pid))

	if err := sendShutdownSignal(c.cmd.Process); err != nil {
		return err
	}

	// Wait with timeout for graceful shutdown
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	go func() {
		<-waitCtx.Done()
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			c.logger.Debug("Agent not responding to SIGTERM, sending SIGKILL", zap.Int("pid", pid))
			c.cmd.Process.Kill()
		}
	}()

	select {
	case <-c.doneCh:
		// Process exited
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// Restart restarts the agent process.
func (c *Commander) Restart(ctx context.Context) error {
	c.logger.Debug("Restarting agent")
	if err := c.Stop(ctx); err != nil {
		return err
	}
	return c.Start(ctx)
}

// ReloadConfig sends SIGHUP to the agent to reload configuration.
func (c *Commander) ReloadConfig() error {
	if c.cmd == nil || c.cmd.Process == nil {
		return errors.New("agent process is not running")
	}
	return sendReloadSignal(c.cmd.Process)
}

// Exited returns a channel that signals when the agent process exits.
func (c *Commander) Exited() <-chan struct{} {
	return c.exitCh
}

// Pid returns the agent process PID, or 0 if not running.
func (c *Commander) Pid() int {
	if c.cmd == nil || c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

// ExitCode returns the agent process exit code, or 0 if not exited.
func (c *Commander) ExitCode() int {
	if c.cmd == nil || c.cmd.ProcessState == nil {
		return 0
	}
	return c.cmd.ProcessState.ExitCode()
}

// IsRunning returns true if the agent process is running.
func (c *Commander) IsRunning() bool {
	return c.running.Load()
}
