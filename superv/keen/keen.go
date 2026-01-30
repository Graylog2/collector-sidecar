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
	"sync"
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

	// Crash recovery fields
	backoff      *Backoff
	crashCount   int
	crashMu      sync.Mutex
	recoveryDone chan struct{}
	stopRecovery context.CancelFunc
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
	// Cancel recovery loop if running
	if c.stopRecovery != nil {
		c.stopRecovery()
	}

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

// SetBackoff configures the backoff behavior for crash recovery.
func (c *Commander) SetBackoff(cfg BackoffConfig) {
	c.backoff = NewBackoff(cfg)
}

// StartWithRecovery starts the agent process with automatic crash recovery.
// It will restart the process on non-zero exit codes according to the
// configured backoff policy.
func (c *Commander) StartWithRecovery(ctx context.Context) error {
	if c.backoff == nil {
		c.backoff = NewBackoff(DefaultBackoffConfig())
	}

	c.recoveryDone = make(chan struct{})

	// Create a cancellable context for the recovery loop
	recoveryCtx, cancel := context.WithCancel(ctx)
	c.stopRecovery = cancel

	go c.recoveryLoop(recoveryCtx)

	return nil
}

// recoveryLoop runs the crash recovery loop.
func (c *Commander) recoveryLoop(ctx context.Context) {
	defer close(c.recoveryDone)

	for {
		// Start the process
		if err := c.Start(ctx); err != nil {
			c.logger.Error("Failed to start agent", zap.Error(err))
			if !c.handleCrash(ctx) {
				return
			}
			continue
		}

		// Mark as running for stability tracking
		c.backoff.MarkRunning()

		// Wait for process to exit
		select {
		case <-ctx.Done():
			// Context cancelled, stop the process and exit
			c.logger.Debug("Recovery loop context cancelled")
			stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			c.Stop(stopCtx)
			cancel()
			return

		case <-c.exitCh:
			// Process exited, check if we should restart
			exitCode := c.ExitCode()

			// Check if process was stable before crash
			c.backoff.CheckAndResetIfStable()

			if exitCode == 0 {
				// Clean exit, don't restart
				c.logger.Debug("Agent exited cleanly", zap.Int("exit_code", exitCode))
				return
			}

			// Non-zero exit, handle as crash
			c.logger.Warn("Agent crashed",
				zap.Int("exit_code", exitCode),
				zap.Int("crash_count", c.CrashCount()+1),
			)

			if !c.handleCrash(ctx) {
				return
			}
		}
	}
}

// handleCrash handles a process crash. Returns true if recovery should continue.
func (c *Commander) handleCrash(ctx context.Context) bool {
	c.crashMu.Lock()
	c.crashCount++
	c.crashMu.Unlock()

	if !c.backoff.ShouldRetry() {
		c.logger.Error("Max crash recovery attempts reached, giving up",
			zap.Int("crash_count", c.CrashCount()),
			zap.Int("max_retries", c.backoff.MaxRetries()),
		)
		return false
	}

	delay := c.backoff.NextDelay()
	c.logger.Info("Waiting before restart",
		zap.Duration("delay", delay),
		zap.Int("attempt", c.backoff.Attempts()),
	)

	select {
	case <-ctx.Done():
		c.logger.Debug("Recovery cancelled during backoff wait")
		return false
	case <-time.After(delay):
		return true
	}
}

// CrashCount returns the number of times the process has crashed.
func (c *Commander) CrashCount() int {
	c.crashMu.Lock()
	defer c.crashMu.Unlock()
	return c.crashCount
}

// Done returns a channel that signals when the recovery loop has completed.
// This happens when max retries are exhausted, the process exits cleanly,
// or the recovery is stopped.
func (c *Commander) Done() <-chan struct{} {
	if c.recoveryDone != nil {
		return c.recoveryDone
	}
	// Return exitCh if not using recovery
	return c.exitCh
}
