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

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/supervisor/connection"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server/types"
	"go.uber.org/zap"

	"github.com/Graylog2/collector-sidecar/superv/auth"
	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/configmanager"
	"github.com/Graylog2/collector-sidecar/superv/configmerge"
	"github.com/Graylog2/collector-sidecar/superv/healthmonitor"
	"github.com/Graylog2/collector-sidecar/superv/keen"
	"github.com/Graylog2/collector-sidecar/superv/opamp"
	"github.com/Graylog2/collector-sidecar/superv/ownlogs"
)

const (
	ServiceName = "supervisor"

	// renewalResponseTimeout is how long to wait for a certificate renewal response
	// before treating the request as failed and retrying.
	renewalResponseTimeout = 2 * time.Minute
)

// templateVars holds variables available for template expansion in agent args.
type templateVars struct {
	ConfigPath string
}

// Supervisor coordinates the management of an OpenTelemetry Collector.
type Supervisor struct {
	logger                    *zap.Logger
	agentCfg                  config.AgentConfig
	authCfg                   config.AuthConfig
	localServerCfg            config.LocalServer
	maxHeartbeatInterval      time.Duration
	persistenceDir            string
	instanceUID               string
	collectorVersion          string
	authManager               *auth.Manager
	connectionSettingsManager *connection.SettingsManager
	configManager             *configmanager.Manager
	healthMonitor             *healthmonitor.Monitor
	healthCancel              context.CancelFunc
	healthWg                  sync.WaitGroup
	commander                 *keen.Commander
	opampClient               *opamp.Client
	opampServer               *opamp.Server
	mu                        sync.RWMutex
	localServerDraining       atomic.Bool

	// Supervisor lifecycle
	ctx        context.Context //nolint:containedctx
	cancel     context.CancelFunc
	isRunning  atomic.Bool
	isStarting atomic.Bool

	// Serialized worker for state-mutating operations.
	workQueue chan workFunc
	workWg    sync.WaitGroup

	// Pending enrollment CSR (set during enrollment, cleared after completion)
	pendingCSR []byte

	// Certificate renewal retry state (protected by mu)
	nextRenewalRetry  time.Time
	renewalBackoff    time.Duration
	renewalBackoffCfg config.BackoffConfig

	// createClientFunc creates and starts a new OpAMP client for the given
	// settings. Defaults to createAndStartClient; overridden in tests for
	// interleaving control.
	createClientFunc func(ctx context.Context, settings connection.Settings) (*opamp.Client, error)

	// ownLogsManager manages OTLP export of the supervisor's own logs.
	ownLogsManager     *ownlogs.Manager
	ownLogsPersistence *ownlogs.Persistence
	currentOwnLogs     *ownlogs.Settings
}

// New creates a new Supervisor instance. The instanceUID must be obtained from
// [persistence.LoadOrCreateInstanceUID] before calling New.
func New(logger *zap.Logger, cfg config.Config, instanceUID string) (*Supervisor, error) {
	// Create auth manager
	authMgr := auth.NewManager(logger.Named("auth"), auth.ManagerConfig{
		KeysDir:     cfg.Keys.Dir,
		JWTLifetime: cfg.Server.Auth.JWTLifetime,
		InsecureTLS: cfg.Server.Auth.InsecureTLS,
	})

	// Setup connection settings manager, trying to load persisted settings or falling back to config defaults.
	connSettingsMgr := connection.NewSettingsManager(logger, cfg.Persistence.Dir)

	connSettings, exist, err := connSettingsMgr.TryLoadPersisted()
	if err != nil {
		return nil, fmt.Errorf("failed to load persisted connection settings: %w", err)
	}

	if !exist {
		connSettings = connection.Settings{
			HeartbeatInterval: 30 * time.Second,
			TLS: connection.TLSSettings{
				Insecure:   cfg.IsInsecure(),
				MinVersion: "TLSv1.3",
				MaxVersion: "TLSv1.3",
			},
			UpdatedAt: time.Now().UTC(),
		}

		if authMgr.IsEnrolled() {
			// We should have stored connection settings when enrolled, but in case we don't,
			// use server config as fallback.
			connSettings.Endpoint = cfg.Server.Endpoint
			connSettings.Headers = cfg.Server.Headers
		} else {
			if enrollEndpoint := cfg.Server.Auth.EnrollmentEndpoint; enrollEndpoint != "" {
				endpoint, err := config.DeriveEnrollmentEndpoint(enrollEndpoint)
				if err != nil {
					return nil, fmt.Errorf("failed to derive enrollment endpoint: %w", err)
				}
				connSettings.Endpoint = endpoint
				connSettings.Headers = cfg.Server.Auth.EnrollmentHeaders
			} else if serverEndpoint := cfg.Server.Endpoint; serverEndpoint != "" {
				connSettings.Endpoint = serverEndpoint
				connSettings.Headers = cfg.Server.Headers
			}
		}
	} else {
		// A configured server endpoint should take precedence over stored connections.
		if serverEndpoint := cfg.Server.Endpoint; serverEndpoint != "" {
			connSettings.Endpoint = serverEndpoint
			connSettings.Headers = cfg.Server.Headers
		}
		// TODO: Do we want to override TLS settings from config as well, or only endpoint and headers?
	}

	if connSettings.Endpoint == "" {
		return nil, errors.New("no server endpoint configured and no persisted connection settings found")
	}

	connSettingsMgr.SetCurrent(connSettings)

	// Parse the health monitor URL (e.g. "http://localhost:13133/health") into
	// the host:port and path components that the OTel health_check extension expects.
	healthCheck := configmerge.HealthCheckConfig{}
	if cfg.Agent.Health.Endpoint != "" {
		u, err := url.Parse(cfg.Agent.Health.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("invalid health endpoint URL %q: %w", cfg.Agent.Health.Endpoint, err)
		}
		if u.Host == "" {
			return nil, fmt.Errorf("invalid health endpoint URL %q: must be an absolute URL (e.g. http://localhost:13133/health)", cfg.Agent.Health.Endpoint)
		}
		healthCheck.Endpoint = u.Host
		if u.Path != "" && u.Path != "/" {
			healthCheck.Path = u.Path
		}
	}

	// Initialize config manager
	configMgr := configmanager.New(logger.Named("config"), configmanager.Config{
		ConfigDir:      filepath.Join(cfg.Persistence.Dir, "config"),
		OutputPath:     filepath.Join(cfg.Persistence.Dir, "config", "collector.yaml"),
		LocalOverrides: cfg.Agent.Config.LocalOverrides,
		LocalEndpoint:  cfg.LocalServer.Endpoint,
		InstanceUID:    instanceUID,
		HealthCheck:    healthCheck,
	})

	// Restore the last applied config hash so ApplyRemoteConfig can skip
	// unchanged configs across supervisor restarts, avoiding unnecessary
	// collector restarts and duplicate APPLIED status messages.
	// Only restore when the last status was APPLIED — if the config failed
	// or was rolled back, the server may resend the same hash expecting a
	// retry, so we must not skip it.
	if status, err := configMgr.LoadRemoteConfigStatus(); err != nil {
		logger.Warn("Failed to load persisted config hash", zap.Error(err))
	} else if status != nil &&
		status.GetStatus() == protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED &&
		len(status.GetLastRemoteConfigHash()) > 0 {
		configMgr.SetLastConfigHash(status.GetLastRemoteConfigHash())
		logger.Debug("Restored last config hash from persisted status")
	}

	// Initialize health monitor
	healthMon := healthmonitor.New(logger.Named("health"), healthmonitor.Config{
		Endpoint:           cfg.Agent.Health.Endpoint,
		Timeout:            cfg.Agent.Health.Timeout,
		Interval:           cfg.Agent.Health.Interval,
		StartupGracePeriod: cfg.Agent.Health.StartupGracePeriod,
	})

	s := &Supervisor{
		logger:                    logger,
		agentCfg:                  cfg.Agent,
		authCfg:                   cfg.Server.Auth,
		localServerCfg:            cfg.LocalServer,
		maxHeartbeatInterval:      cfg.Server.MaxHeartbeatInterval,
		persistenceDir:            cfg.Persistence.Dir,
		instanceUID:               instanceUID,
		authManager:               authMgr,
		configManager:             configMgr,
		healthMonitor:             healthMon,
		connectionSettingsManager: connSettingsMgr,
		renewalBackoffCfg:         cfg.Server.Connection.RetryBackoff,
	}
	s.createClientFunc = s.createAndStartClient
	return s, nil
}

// InstanceUID returns the supervisor's unique instance identifier.
func (s *Supervisor) InstanceUID() string {
	return s.instanceUID
}

// expandArgs expands template placeholders in agent args.
func (s *Supervisor) expandArgs(args []string, configPath string) ([]string, error) {
	vars := templateVars{ConfigPath: configPath}
	expanded := make([]string, len(args))

	for i, arg := range args {
		tmpl, err := template.New("arg").Option("missingkey=error").Parse(arg)
		if err != nil {
			return nil, fmt.Errorf("invalid template in arg %d: %w", i, err)
		}
		var buf strings.Builder
		if err := tmpl.Execute(&buf, vars); err != nil {
			return nil, fmt.Errorf("failed to expand arg %d: %w", i, err)
		}
		expanded[i] = buf.String()
	}
	return expanded, nil
}

// buildCollectorEnv builds the environment variables for the collector process.
// It sets the TLS client key and cert paths from the auth manager, then merges
// any user-configured env vars on top (allowing overrides).
func (s *Supervisor) buildCollectorEnv() map[string]string {
	env := map[string]string{
		"GLC_INTERNAL_INSTANCE_UID":         s.instanceUID,
		"GLC_INTERNAL_TLS_CLIENT_KEY_PATH":  s.authManager.GetSigningKeyPath(),
		"GLC_INTERNAL_TLS_CLIENT_CERT_PATH": s.authManager.GetSigningCertPath(),
		"GLC_INTERNAL_PERSISTENCE_DIR":      filepath.Clean(s.persistenceDir),
		"GLC_INTERNAL_STORAGE_PATH":         filepath.Clean(s.agentCfg.StorageDir),
	}
	maps.Copy(env, s.agentCfg.Env)
	return env
}

func (s *Supervisor) shouldAcceptLocalCollectorConnection() bool {
	return !s.localServerDraining.Load()
}

// Start starts the supervisor and begins managing the collector.
func (s *Supervisor) Start(parentCtx context.Context) error {
	if !s.isStarting.CompareAndSwap(false, true) {
		return fmt.Errorf("already starting")
	}
	if s.isRunning.Load() {
		return fmt.Errorf("already running")
	}

	// Initialize the context early so we can rely on it being there.
	s.ctx, s.cancel = context.WithCancel(parentCtx)

	// Lock is released early because:
	// 1. Start/Stop are never concurrent (lifecycle contract) — no race on s.running.
	// 2. createAndStartClient acquires s.mu.RLock (for pendingCSR); holding Lock
	//    would self-deadlock (RWMutex is not reentrant).
	// 3. Cleanup calls opampClient.Stop which may wait for callbacks that take s.mu.

	s.logger.Info("Starting supervisor",
		zap.String("instance_uid", s.instanceUID),
		zap.String("endpoint", s.connectionSettingsManager.GetCurrent().Endpoint),
	)
	s.localServerDraining.Store(false)

	// Initialize authentication
	if err := s.initAuth(s.ctx); err != nil {
		return fmt.Errorf("authentication initialization failed: %w", err)
	}

	// Use the config manager's output path — this is where ApplyRemoteConfig writes
	// the merged effective config that the collector should read.
	configPath := s.configManager.OutputPath()

	// Expand template variables in agent args
	expandedArgs, err := s.expandArgs(s.agentCfg.Args, configPath)
	if err != nil {
		return fmt.Errorf("failed to expand agent args: %w", err)
	}

	// Create commander for agent process management
	cmd, err := keen.New(s.logger, s.persistenceDir, keen.Config{
		Executable:      s.agentCfg.Executable,
		Args:            expandedArgs,
		Env:             s.buildCollectorEnv(),
		PassthroughLogs: s.agentCfg.PassthroughLogs,
	}, keen.NewBackoff(keen.BackoffConfig{
		InitialInterval:     s.agentCfg.Restart.InitialInterval,
		MaxInterval:         s.agentCfg.Restart.MaxInterval,
		Multiplier:          s.agentCfg.Restart.Multiplier,
		RandomizationFactor: s.agentCfg.Restart.RandomizationFactor,
		MaxRetries:          s.agentCfg.Restart.MaxRetries,
		StableAfter:         s.agentCfg.Restart.StableAfter,
	}))
	if err != nil {
		return fmt.Errorf("creating commander: %w", err)
	}
	s.commander = cmd

	// Create local OpAMP server for collector
	serverCallbacks := &opamp.ServerCallbacks{
		OnConnect: func(conn types.Connection) {
			s.logger.Info("Collector connected to local OpAMP server")
		},
		OnDisconnect: func(conn types.Connection) {
			s.logger.Info("Collector disconnected from local OpAMP server")
		},
		OnConnectingFunc: func(_ *http.Request) (bool, int) {
			if !s.shouldAcceptLocalCollectorConnection() {
				return false, http.StatusServiceUnavailable
			}
			return true, 0
		},
	}

	opampServer, err := opamp.NewServer(s.logger, opamp.ServerConfig{
		ListenEndpoint: s.localServerCfg.Endpoint,
	}, serverCallbacks)
	if err != nil {
		return fmt.Errorf("creating local OpAMP server: %w", err)
	}
	s.opampServer = opampServer

	// Start local OpAMP server
	if err := s.opampServer.Start(s.ctx); err != nil {
		return fmt.Errorf("starting local OpAMP server: %w", err)
	}

	// Resolve the runtime-bound local OpAMP endpoint and update the config
	// manager. This replaces the static config value (e.g. "localhost:0") with
	// the actual ws:// URL, used by both EnsureBootstrapConfig and
	// ApplyRemoteConfig for extension injection.
	localEndpoint, err := resolveLocalEndpoint(s.opampServer.Addr())
	if err != nil {
		if stopErr := s.opampServer.Stop(s.ctx); stopErr != nil {
			s.logger.Warn("Failed to stop local OpAMP server", zap.Error(stopErr))
		}
		return fmt.Errorf("failed to resolve local OpAMP endpoint: %w", err)
	}
	s.configManager.SetLocalEndpoint(localEndpoint)

	// Ensure a valid config exists before starting the collector. On first run
	// this writes a minimal bootstrap config; on subsequent runs it re-injects
	// the opamp and health_check extensions to update the local endpoint
	// (which may have changed if the server binds to an ephemeral port).
	if err := s.configManager.EnsureBootstrapConfig(); err != nil {
		if stopErr := s.opampServer.Stop(s.ctx); stopErr != nil {
			s.logger.Warn("Failed to stop local OpAMP server", zap.Error(stopErr))
		}
		return fmt.Errorf("failed to ensure bootstrap config: %w", err)
	}

	// Initialize and start the serialized worker. Must be running before
	// the OpAMP client starts, otherwise early callbacks block on the
	// unbuffered channel with no consumer.
	s.workQueue = make(chan workFunc)
	s.workWg.Add(1)
	go s.runWorker()

	// Create and start OpAMP client for upstream server
	opampClient, err := s.createClientFunc(s.ctx, s.connectionSettingsManager.GetCurrent())
	if err != nil {
		s.cancel()
		s.workWg.Wait()
		if stopErr := s.opampServer.Stop(s.ctx); stopErr != nil {
			s.logger.Warn("Failed to stop local OpAMP server", zap.Error(stopErr))
		}
		return fmt.Errorf("create OpAMP client: %w", err)
	}

	// Publish opampClient under lock for visibility to health goroutine and callbacks.
	s.mu.Lock()
	s.opampClient = opampClient
	s.mu.Unlock()

	// Start the collector agent
	if err := s.commander.Start(s.ctx); err != nil {
		// Nil out opampClient under lock (it was published above) before stopping,
		// so callbacks see nil.
		s.mu.Lock()
		s.opampClient = nil
		s.mu.Unlock()

		s.cancel()
		if err := opampClient.Stop(s.ctx); err != nil {
			s.logger.Warn("Failed to stop local OpAMP client", zap.Error(err))
		}
		s.workWg.Wait()
		if stopErr := opampServer.Stop(s.ctx); stopErr != nil {
			s.logger.Warn("Failed to stop local OpAMP server during cleanup", zap.Error(stopErr))
		}
		return fmt.Errorf("failed to start agent: %w", err)
	}

	// Start health monitoring after the collector is running.
	// The monitor's startup grace period gives the collector time to bind
	// its health endpoint before the first check.
	healthCtx, healthCancel := context.WithCancel(s.ctx)
	s.healthCancel = healthCancel
	healthUpdates := s.healthMonitor.StartPolling(healthCtx)
	s.healthWg.Go(func() {
		// Check immediately at startup; the returned delay schedules the next check
		// based on the certificate's actual renewal time rather than a fixed interval.
		nextCheck := s.checkCertificateRenewal()
		renewalTimer := time.NewTimer(nextCheck)
		defer renewalTimer.Stop()

		for {
			select {
			case status, ok := <-healthUpdates:
				if !ok {
					return
				}
				// Only report health if we're enrolled - during enrollment we want to avoid sending multiple requests
				// using the enrollment token.
				if s.authManager.IsEnrolled() { // TODO: Check how expensive IsEnrolled is and if we can cache it after enrollment
					s.mu.RLock()
					client := s.opampClient
					s.mu.RUnlock()

					if client != nil {
						// TODO: Pass commander as AgentStateProvider once it implements the interface
						// This will enable accurate agent start time reporting per OpAMP spec
						if err := client.SetHealth(status.ToComponentHealth(nil)); err != nil {
							s.logger.Warn("Failed to report health", zap.Error(err))
						}
					}
				}

			case <-renewalTimer.C:
				nextCheck = s.checkCertificateRenewal()
				renewalTimer.Reset(nextCheck)
			}
		}
	})

	s.isRunning.Store(true)
	s.isStarting.Store(false)

	return nil
}

// Stop stops the supervisor and the managed collector.
func (s *Supervisor) Stop(ctx context.Context) error {
	if !s.isRunning.CompareAndSwap(true, false) {
		return nil
	}
	s.localServerDraining.Store(true)

	s.mu.Lock()
	// Snapshot and nil-out to prevent concurrent use after unlock.
	client := s.opampClient
	server := s.opampServer
	s.opampClient = nil
	s.opampServer = nil

	// Cancel worker context while still holding mu. This ensures that
	// reconnectClient (which checks s.ctx.Err() under mu) cannot
	// assign a new client after we've already snapshot s.opampClient.
	s.cancel()
	s.mu.Unlock()

	s.logger.Info("Stopping supervisor")

	// Fields below (healthCancel, commander) are safe to read without mu because
	// Start() and Stop() are never concurrent (see Lifecycle Contract in design doc).
	if s.healthCancel != nil {
		s.healthCancel()
	}
	s.healthWg.Wait()

	if s.commander != nil {
		if err := s.commander.Stop(ctx); err != nil {
			s.logger.Error("Error stopping agent", zap.Error(err))
		}
	}

	server.DisconnectAll()

	if client != nil {
		if err := client.Stop(ctx); err != nil {
			s.logger.Error("Error stopping OpAMP client", zap.Error(err))
		}
	}

	if server != nil {
		if err := server.Stop(ctx); err != nil {
			s.logger.Error("Error stopping OpAMP server", zap.Error(err))
		}
	}

	s.workWg.Wait()

	// Shut down own logs OTLP export last, so shutdown log messages are still exported.
	if s.ownLogsManager != nil {
		if err := s.ownLogsManager.Shutdown(ctx); err != nil {
			s.logger.Error("Error shutting down own logs exporter", zap.Error(err))
		}
	}

	return nil
}

// SetOwnLogs configures the own-logs manager and persistence for OTLP log export.
// Must be called before Start.
func (s *Supervisor) SetOwnLogs(manager *ownlogs.Manager, persistence *ownlogs.Persistence, current *ownlogs.Settings) {
	s.ownLogsManager = manager
	s.ownLogsPersistence = persistence
	if current != nil {
		s.currentOwnLogs = new(*current)
	}
}

// IsRunning returns true if the supervisor is running.
func (s *Supervisor) IsRunning() bool {
	return s.isRunning.Load()
}

func (s *Supervisor) isStopping() bool {
	return s.ctx != nil && s.ctx.Err() != nil
}

func (s *Supervisor) isShutdownCancellation(err error) bool {
	return errors.Is(err, context.Canceled) && s.isStopping()
}
