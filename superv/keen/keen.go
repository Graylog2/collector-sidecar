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
	"cmp"
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

	"github.com/DeRuina/timberjack"
	"go.uber.org/zap"
)

// LogRotationConfig holds configuration for agent log file rotation.
type LogRotationConfig struct {
	MaxSize    int // Maximum size in megabytes before rotation (default: 100)
	MaxBackups int // Maximum number of rotated log files to keep (default: 5)
	MaxAge     int // Maximum age in days to retain old log files (default: 30)
}

// Config holds the configuration for the commander.
type Config struct {
	Executable      string
	Args            []string
	Env             map[string]string
	PassthroughLogs bool
	LogRotation     LogRotationConfig
}

// Commander manages the lifecycle of an agent process.
type Commander struct {
	logger  *zap.Logger
	logsDir string
	cfg     Config
	mu      sync.Mutex // protects cmd, recoveryDone, stopRecovery
	cmd     *exec.Cmd
	running atomic.Bool
	doneCh  chan struct{}
	exitCh  chan struct{}

	// Log rotation writer (persists across restarts)
	logWriter *timberjack.Logger

	// Crash recovery fields
	backoff      *Backoff
	crashCount   int
	crashMu      sync.Mutex
	recoveryDone chan struct{}
	stopRecovery context.CancelFunc
}

// New creates a new Commander instance.
func New(logger *zap.Logger, logsDir string, cfg Config, backoff *Backoff) (*Commander, error) {
	if backoff == nil {
		return nil, errors.New("backoff is required")
	}
	logsDir = filepath.Join(logsDir, "logs")
	if err := os.MkdirAll(logsDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}
	return &Commander{
		logger:  logger,
		logsDir: logsDir,
		cfg:     cfg,
		doneCh:  make(chan struct{}, 1),
		exitCh:  make(chan struct{}, 1),
		backoff: backoff,
	}, nil
}

// Start starts the agent process.
// If the first start succeeds, subsequent crash-recovery restarts run in a background
// goroutine; use [Commander.Done] to detect when recovery gives up.
func (c *Commander) Start(ctx context.Context) error {
	// Start the process synchronously first so the caller can observe
	// immediate failures (e.g., missing executable).
	if err := c.start(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	c.recoveryDone = make(chan struct{})
	recoveryCtx, cancel := context.WithCancel(ctx)
	c.stopRecovery = cancel
	c.mu.Unlock()

	go c.recoveryLoop(recoveryCtx)

	return nil
}

func (c *Commander) start(ctx context.Context) error {
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

	cmd := exec.CommandContext(ctx, c.cfg.Executable, c.cfg.Args...)
	cmd.Env = c.buildEnv()
	cmd.SysProcAttr = sysProcAttrs()

	c.mu.Lock()
	c.cmd = cmd
	c.mu.Unlock()

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
	c.mu.Lock()
	if c.logWriter == nil {
		rot := c.cfg.LogRotation
		c.logWriter = &timberjack.Logger{
			Filename:   filepath.Join(c.logsDir, "agent.log"),
			MaxSize:    cmp.Or(rot.MaxSize, 100),
			MaxBackups: cmp.Or(rot.MaxBackups, 5),
			MaxAge:     cmp.Or(rot.MaxAge, 30),
			LocalTime:  true,
		}
	}
	c.mu.Unlock()

	c.cmd.Stdout = c.logWriter
	c.cmd.Stderr = c.logWriter

	if err := c.cmd.Start(); err != nil {
		c.running.Store(false)
		return fmt.Errorf("failed to start agent: %w", err)
	}

	c.logger.Debug("Agent process started", zap.Int("pid", c.cmd.Process.Pid))

	go c.watch()

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
	defer c.closeLogWriter()

	// Cancel recovery loop if running
	c.mu.Lock()
	stopRecovery := c.stopRecovery
	cmd := c.cmd
	c.mu.Unlock()

	if stopRecovery != nil {
		stopRecovery()
	}

	if !c.running.Load() {
		return nil
	}

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid
	c.logger.Debug("Stopping agent process", zap.Int("pid", pid))

	if err := sendShutdownSignal(cmd.Process); err != nil {
		if errors.Is(err, os.ErrProcessDone) || !c.running.Load() {
			// Process already exited, nothing to do.
			return nil
		}
		return fmt.Errorf("failed to send shutdown signal: %w", err)
	}

	// Wait with timeout for graceful shutdown
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	go func() {
		<-waitCtx.Done()
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			c.logger.Debug("Agent not responding to SIGTERM, sending SIGKILL", zap.Int("pid", pid))
			cmd.Process.Kill()
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

func (c *Commander) closeLogWriter() {
	c.mu.Lock()
	lw := c.logWriter
	c.logWriter = nil
	c.mu.Unlock()
	if lw != nil {
		if err := lw.Close(); err != nil {
			c.logger.Warn("Failed to close log writer", zap.Error(err))
		}
	}
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
	c.mu.Lock()
	cmd := c.cmd
	c.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return errors.New("agent process is not running")
	}
	return sendReloadSignal(cmd.Process)
}

// Exited returns a channel that signals when the agent process exits.
// When crash recovery is enabled (MaxRetries >= 1), the recovery loop consumes
// exit events. Use [Commander.Done] instead to detect when the recovery loop completes.
func (c *Commander) Exited() <-chan struct{} {
	return c.exitCh
}

// Pid returns the agent process PID, or 0 if not running.
func (c *Commander) Pid() int {
	c.mu.Lock()
	cmd := c.cmd
	c.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return 0
	}
	return cmd.Process.Pid
}

// ExitCode returns the agent process exit code, or 0 if not exited.
func (c *Commander) ExitCode() int {
	c.mu.Lock()
	cmd := c.cmd
	c.mu.Unlock()

	if cmd == nil || cmd.ProcessState == nil {
		return 0
	}
	return cmd.ProcessState.ExitCode()
}

// IsRunning returns true if the agent process is running.
func (c *Commander) IsRunning() bool {
	return c.running.Load()
}

// recoveryLoop runs the crash recovery loop.
// The first start is done synchronously in [Commander.Start], so this loop
// begins by waiting for the already-running process to exit.
func (c *Commander) recoveryLoop(ctx context.Context) {
	defer close(c.recoveryDone)

	// The first iteration waits for the process started by Start().
	// Subsequent iterations start the process themselves.
	firstIteration := true

	for {
		if !firstIteration {
			// Start the process
			if err := c.start(ctx); err != nil {
				c.logger.Error("Failed to start agent", zap.Error(err))
				if !c.handleCrash(ctx) {
					return
				}
				continue
			}
		}
		firstIteration = false

		// Mark as running for stability tracking
		c.backoff.MarkRunning()

		// Wait for process to exit
		select {
		case <-ctx.Done():
			// Context cancelled, stop the process and exit
			c.logger.Debug("Recovery loop context cancelled")
			stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := c.Stop(stopCtx); err != nil {
				c.logger.Warn("Failed to stop agent during recovery shutdown", zap.Error(err))
			}
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
	c.mu.Lock()
	rd := c.recoveryDone
	c.mu.Unlock()

	if rd != nil {
		return rd
	}
	// Return exitCh if not using recovery
	return c.exitCh
}
