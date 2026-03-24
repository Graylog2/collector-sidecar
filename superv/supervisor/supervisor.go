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
	"crypto/tls"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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
	"github.com/Graylog2/collector-sidecar/superv/components"
	"github.com/Graylog2/collector-sidecar/superv/config"
	"github.com/Graylog2/collector-sidecar/superv/configmanager"
	"github.com/Graylog2/collector-sidecar/superv/configmerge"
	"github.com/Graylog2/collector-sidecar/superv/healthmonitor"
	"github.com/Graylog2/collector-sidecar/superv/keen"
	"github.com/Graylog2/collector-sidecar/superv/opamp"
	"github.com/Graylog2/collector-sidecar/superv/ownlogs"
	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"github.com/Graylog2/collector-sidecar/superv/version"
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
	running                   bool
	localServerDraining       atomic.Bool

	// Serialized worker for state-mutating operations.
	workQueue  chan workFunc
	workCtx    context.Context
	workCancel context.CancelFunc
	workWg     sync.WaitGroup

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
		"GLC_INTERNAL_PERSISTENCE_DIR":      s.persistenceDir,
	}
	maps.Copy(env, s.agentCfg.Env)
	return env
}

func (s *Supervisor) shouldAcceptLocalCollectorConnection() bool {
	return !s.localServerDraining.Load()
}

// Start starts the supervisor and begins managing the collector.
func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

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
	if err := s.initAuth(ctx); err != nil {
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
		return err
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
		return err
	}
	s.opampServer = opampServer

	// Start local OpAMP server
	if err := s.opampServer.Start(ctx); err != nil {
		return err
	}

	// Resolve the runtime-bound local OpAMP endpoint and update the config
	// manager. This replaces the static config value (e.g. "localhost:0") with
	// the actual ws:// URL, used by both EnsureBootstrapConfig and
	// ApplyRemoteConfig for extension injection.
	localEndpoint, err := resolveLocalEndpoint(s.opampServer.Addr())
	if err != nil {
		if stopErr := s.opampServer.Stop(ctx); stopErr != nil {
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
		if stopErr := s.opampServer.Stop(ctx); stopErr != nil {
			s.logger.Warn("Failed to stop local OpAMP server", zap.Error(stopErr))
		}
		return fmt.Errorf("failed to ensure bootstrap config: %w", err)
	}

	// Initialize and start the serialized worker. Must be running before
	// the OpAMP client starts, otherwise early callbacks block on the
	// unbuffered channel with no consumer.
	s.workQueue = make(chan workFunc)
	s.workCtx, s.workCancel = context.WithCancel(ctx)
	s.workWg.Add(1)
	go s.runWorker()

	// Create and start OpAMP client for upstream server
	opampClient, err := s.createClientFunc(ctx, s.connectionSettingsManager.GetCurrent())
	if err != nil {
		s.workCancel()
		s.workWg.Wait()
		if stopErr := s.opampServer.Stop(ctx); stopErr != nil {
			s.logger.Warn("Failed to stop local OpAMP server", zap.Error(stopErr))
		}
		return fmt.Errorf("create OpAMP client: %w", err)
	}

	// Publish opampClient under lock for visibility to health goroutine and callbacks.
	s.mu.Lock()
	s.opampClient = opampClient
	s.mu.Unlock()

	// Start the collector agent
	if err := s.commander.Start(ctx); err != nil {
		// Nil out opampClient under lock (it was published above) before stopping,
		// so callbacks see nil.
		s.mu.Lock()
		s.opampClient = nil
		s.mu.Unlock()

		s.workCancel()
		opampClient.Stop(ctx)
		s.workWg.Wait()
		if stopErr := opampServer.Stop(ctx); stopErr != nil {
			s.logger.Warn("Failed to stop local OpAMP server during cleanup", zap.Error(stopErr))
		}
		return fmt.Errorf("failed to start agent: %w", err)
	}

	// Start health monitoring after the collector is running.
	// The monitor's startup grace period gives the collector time to bind
	// its health endpoint before the first check.
	healthCtx, healthCancel := context.WithCancel(ctx)
	s.healthCancel = healthCancel
	healthUpdates := s.healthMonitor.StartPolling(healthCtx)
	s.healthWg.Go(func() {
		renewalInterval := s.authCfg.RenewalInterval // validated > 0 by config
		// Check immediately at startup, then on the ticker interval.
		s.checkCertificateRenewal()

		renewalTicker := time.NewTicker(renewalInterval)
		defer renewalTicker.Stop()

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

			case <-renewalTicker.C:
				s.checkCertificateRenewal()
			}
		}
	})

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	return nil
}

// Stop stops the supervisor and the managed collector.
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.localServerDraining.Store(true)

	// Snapshot and nil-out to prevent concurrent use after unlock.
	client := s.opampClient
	server := s.opampServer
	s.opampClient = nil
	s.opampServer = nil

	// Cancel worker context while still holding mu. This ensures that
	// reconnectClient (which checks workCtx.Err() under mu) cannot
	// assign a new client after we've already snapshot s.opampClient.
	s.workCancel()
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
		settingsCopy := *current
		s.currentOwnLogs = &settingsCopy
	}
}

// IsRunning returns true if the supervisor is running.
func (s *Supervisor) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// createAgentDescription creates the initial agent description for OpAMP.
func (s *Supervisor) createAgentDescription() *protobufs.AgentDescription {
	hostname, _ := os.Hostname()

	return &protobufs.AgentDescription{
		IdentifyingAttributes: []*protobufs.KeyValue{
			{
				Key:   "service.name",
				Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: ServiceName}},
			},
			{
				Key:   "service.instance.id",
				Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: s.instanceUID}},
			},
		},
		NonIdentifyingAttributes: s.nonIdentifyingAttributes(hostname),
	}
}

// nonIdentifyingAttributes builds the list of non-identifying attributes for the agent description.
func (s *Supervisor) nonIdentifyingAttributes(hostname string) []*protobufs.KeyValue {
	attrs := []*protobufs.KeyValue{
		{
			Key:   "service.version",
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: version.Version()}},
		},
		{
			Key:   "host.name",
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: hostname}},
		},
		{
			Key:   "os.type",
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: runtime.GOOS}},
		},
		{
			Key:   "host.arch",
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: runtime.GOARCH}},
		},
	}

	if s.collectorVersion != "" {
		attrs = append(attrs, &protobufs.KeyValue{
			Key:   "collector.version",
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: s.collectorVersion}},
		})
	}

	return attrs
}

// discoverComponents discovers available components from the collector.
// This is a best-effort operation - failures are logged but don't prevent startup.
// Returns the protobuf representation and the collector's build version (if available).
func (s *Supervisor) discoverComponents(ctx context.Context) (*protobufs.AvailableComponents, string) {
	cfg := components.DiscoverConfig{
		Executable: s.agentCfg.Executable,
		Timeout:    10 * time.Second,
	}

	discovered, err := components.Discover(ctx, cfg)
	if err != nil {
		s.logger.Warn("Failed to discover components", zap.Error(err))
		return nil, ""
	}

	s.logger.Info("Discovered available components",
		zap.Int("receivers", len(discovered.Receivers)),
		zap.Int("processors", len(discovered.Processors)),
		zap.Int("exporters", len(discovered.Exporters)),
		zap.Int("extensions", len(discovered.Extensions)),
	)

	return discovered.ToProto(), discovered.BuildInfo.Version
}

// createAndStartClient creates a new OpAMP client with current config, sets it up, and starts it.
// This is used when reconnecting with new or restored connection settings.
func (s *Supervisor) createAndStartClient(ctx context.Context, settings connection.Settings) (*opamp.Client, error) {
	headers, headerFunc := s.buildAuthHeaders(settings)

	minVersion, maxVersion, err := settings.TLS.ToTLSMinMaxVersion()
	if err != nil {
		return nil, fmt.Errorf("invalid TLS settings: %w", err)
	}

	client, err := opamp.NewClient(s.logger, opamp.ClientConfig{
		Endpoint:          settings.Endpoint,
		InstanceUID:       s.instanceUID,
		Headers:           headers,
		HeaderFunc:        headerFunc,
		HeartbeatInterval: settings.HeartbeatInterval,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: settings.TLS.Insecure,
			MinVersion:         minVersion,
			MaxVersion:         maxVersion,
		},
		Capabilities: opamp.Capabilities{
			AcceptsRemoteConfig:            true,
			ReportsEffectiveConfig:         true,
			ReportsRemoteConfig:            true,
			ReportsHealth:                  true,
			AcceptsOpAMPConnectionSettings: true,
			// ReportsConnectionSettingsStatus is disabled because opamp-go schedules APPLIED immediately after the callback
			// returns, but the actual reconnect happens asynchronously on the worker. The sender's in-flight status POST races
			// with client.Stop(), producing a spurious "context canceled" error. Enable once opamp-go exposes a public
			// SetConnectionSettingsStatus API so we can report the outcome after the async reconnect completes.
			ReportsConnectionSettingsStatus: false,
			AcceptsRestartCommand:           true,
			ReportsOwnLogs:                  s.ownLogsManager != nil,
			ReportsHeartbeat:                true,
			ReportsAvailableComponents:      true,
		},
	}, s.createOpAMPCallbacks())
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	// Discover and set available components first, so the collector version
	// is available for the agent description below.
	if err := s.setClientAvailableComponents(ctx, client); err != nil {
		return nil, fmt.Errorf("set available components: %w", err)
	}

	if err := client.SetAgentDescription(s.createAgentDescription()); err != nil {
		return nil, fmt.Errorf("set agent description: %w", err)
	}

	if err := client.SetHealth(s.initialComponentHealth()); err != nil {
		return nil, fmt.Errorf("set health: %w", err)
	}

	// Restore persisted remote config status so the server knows our
	// last config state after a supervisor restart.
	if status, err := s.configManager.LoadRemoteConfigStatus(); err != nil {
		s.logger.Warn("Failed to load persisted remote config status, starting with UNSET", zap.Error(err))
	} else if status != nil {
		client.SetInitialRemoteConfigStatus(status)
	}

	// Read pendingCSR under RLock — handleEnrollmentCertificate (phase 1, on opamp-go
	// goroutine) clears it under Lock concurrently with the worker calling this method.
	s.mu.RLock()
	csr := s.pendingCSR
	s.mu.RUnlock()

	if len(csr) > 0 {
		s.logger.Info("Sending CSR for enrollment via OpAMP")
		if err := client.RequestConnectionSettings(csr); err != nil {
			return nil, fmt.Errorf("request connection settings: %w", err)
		}
	}

	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("start client: %w", err)
	}

	return client, nil
}

// initialComponentHealth returns the latest known collector health to seed a new
// OpAMP client. During first-time enrollment, health polling may observe failures
// before enrollment completes; seeding reconnects from monitor state prevents that
// latest status from being lost due to health deduplication.
func (s *Supervisor) initialComponentHealth() *protobufs.ComponentHealth {
	if s.healthMonitor == nil {
		return &protobufs.ComponentHealth{Healthy: true}
	}

	status := s.healthMonitor.LastStatus()
	if status == nil {
		return &protobufs.ComponentHealth{Healthy: true}
	}

	return status.ToComponentHealth(nil)
}

// setClientAvailableComponents discovers and sets available components on the given client.
// It also stores the discovered collector version for use in agent descriptions.
func (s *Supervisor) setClientAvailableComponents(ctx context.Context, client *opamp.Client) error {
	availableComponents, collectorVersion := s.discoverComponents(ctx)
	if availableComponents == nil {
		// Use empty components if discovery fails (still need valid hash for opamp-go)
		availableComponents = (&components.Components{}).ToProto()
	}
	s.collectorVersion = collectorVersion
	return client.SetAvailableComponents(availableComponents)
}

// initAuth initializes authentication by loading credentials or preparing enrollment.
// If enrollment is needed, this prepares the CSR which will be sent via OpAMP.
func (s *Supervisor) initAuth(ctx context.Context) error {
	if s.authManager.IsEnrolled() {
		s.logger.Debug("Loading existing credentials")
		if err := s.authManager.LoadCredentials(); err != nil {
			return fmt.Errorf("failed to load credentials: %w", err)
		}

		s.logger.Info("Credentials loaded",
			zap.String("cert_fingerprint", s.authManager.CertFingerprint()),
		)
		return nil
	}

	// Need to enroll - prepare the CSR
	if s.authCfg.EnrollmentEndpoint == "" {
		return fmt.Errorf("not enrolled and no enrollment URL configured")
	}
	if s.authCfg.EnrollmentToken == "" {
		return fmt.Errorf("not enrolled and no enrollment token configured")
	}

	s.logger.Info("Preparing enrollment")
	result, err := s.authManager.PrepareEnrollment(ctx, s.authCfg.EnrollmentEndpoint, s.authCfg.EnrollmentToken, s.instanceUID)
	if err != nil {
		return fmt.Errorf("enrollment preparation failed: %w", err)
	}

	// Store CSR to send via OpAMP after connection is established
	s.pendingCSR = result.CSRPEM

	s.logger.Info("Enrollment prepared, CSR ready for submission via OpAMP",
		zap.String("tenant_id", result.TenantID),
	)

	return nil
}

func toHTTPHeaders(headers map[string]string) http.Header {
	h := make(http.Header)
	for k, v := range headers {
		h.Set(k, v)
	}
	return h
}

// buildAuthHeaders creates HTTP headers and an optional HeaderFunc for authentication.
// During enrollment, the enrollment JWT is set as a static Authorization header.
// After enrollment, a HeaderFunc is returned that generates a fresh JWT for each request,
// ensuring the token doesn't expire during long-running connections.
func (s *Supervisor) buildAuthHeaders(settings connection.Settings) (http.Header, func(http.Header) http.Header) {
	headers := toHTTPHeaders(settings.Headers)

	// If we're not enrolled yet, use the enrollment JWT as a static header.
	if !s.authManager.IsEnrolled() {
		if jwt := s.authManager.EnrollmentJWT(); jwt != "" {
			headers.Set("Authorization", "Bearer "+jwt)
		}
		return headers, nil
	}

	// When enrolled, generate a fresh JWT before each HTTP request so the
	// token never expires during long-running connections.
	headerFunc := func(h http.Header) http.Header {
		authHeader, err := s.authManager.GetAuthorizationHeader()
		if err != nil {
			s.logger.Error("Failed to generate JWT for OpAMP request", zap.Error(err))
			return h
		}
		h.Set("Authorization", authHeader)
		return h
	}

	return headers, headerFunc
}

func (s *Supervisor) reconnectClient(ctx context.Context, settings connection.Settings) error {
	s.mu.Lock()
	client := s.opampClient
	s.opampClient = nil // Nil immediately so concurrent readers see nil, not stopped client.
	s.mu.Unlock()

	// If client is nil, skip the stop step. This happens during rollback when a previous
	// reconnect failed after nilling s.opampClient but before assigning a new client.
	if client != nil {
		s.logger.Info("Stopping OpAMP client for connection settings update")
		if err := client.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop client: %w", err)
		}
	}

	s.logger.Info("Starting OpAMP client with new connection settings",
		zap.String("endpoint", settings.Endpoint))
	newClient, err := s.createClientFunc(ctx, settings)
	if err != nil {
		return fmt.Errorf("apply connection settings: %w", err)
	}

	// Check workCtx under mu before assigning. Stop() cancels workCtx while
	// holding mu, so this check-and-assign is atomic with respect to shutdown.
	// We use workCtx instead of s.running because s.running is only set at the
	// end of Start(), but reconnects can happen during the startup window.
	s.mu.Lock()
	if s.workCtx.Err() != nil {
		s.mu.Unlock()
		_ = newClient.Stop(ctx)
		return fmt.Errorf("supervisor stopped during reconnect")
	}
	s.opampClient = newClient
	s.mu.Unlock()

	return nil
}

// pendingReconnect holds state between prepareConnectionSettings (phase 1)
// and applyConnectionSettings (phase 2).
type pendingReconnect struct {
	newSettings connection.Settings
	oldSettings connection.Settings
	stagedFile  persistence.StagedFile
}

// prepareConnectionSettings validates and stages new connection settings.
// Runs synchronously in the OnOpampConnectionSettings callback so the return
// value drives opamp-go's ConnectionSettingsStatus reporting.
func (s *Supervisor) prepareConnectionSettings(
	ctx context.Context,
	settings *protobufs.OpAMPConnectionSettings,
) (*pendingReconnect, error) {
	if settings == nil {
		return nil, nil
	}

	newlyEnrolled, err := s.handleCertificateResponse(settings)
	if err != nil {
		return nil, fmt.Errorf("enrollment certificate handling failed: %w", err)
	}

	newSettings, changed := s.connectionSettingsManager.SettingsChanged(settings)
	if !changed && !newlyEnrolled {
		return nil, nil
	}

	stagedFile, err := s.connectionSettingsManager.StageNext(newSettings)
	if err != nil {
		return nil, fmt.Errorf("failed to persist new settings: %w", err)
	}

	oldSettings := s.connectionSettingsManager.GetCurrent()
	return &pendingReconnect{
		newSettings: newSettings,
		oldSettings: oldSettings,
		stagedFile:  stagedFile,
	}, nil
}

// applyConnectionSettings reconnects the OpAMP client with new settings.
// Runs on the serialized worker goroutine. On failure, rolls back to old settings.
func (s *Supervisor) applyConnectionSettings(ctx context.Context, pending *pendingReconnect) {
	if err := s.reconnectClient(ctx, pending.newSettings); err != nil {
		s.logger.Error("Failed to connect with new settings, rolling back", zap.Error(err))
		if cleanupErr := pending.stagedFile.Cleanup(); cleanupErr != nil {
			s.logger.Error("Failed to clean up staged settings file", zap.Error(cleanupErr))
		}
		if rollbackErr := s.reconnectClient(ctx, pending.oldSettings); rollbackErr != nil {
			s.logger.Error("Rollback also failed", zap.Error(rollbackErr))
		}
		return
	}

	if err := pending.stagedFile.Commit(); err != nil {
		s.logger.Error("Failed to commit staged settings file", zap.Error(err))

		// Reconnect to old settings since persisting the new settings failed.
		if reconnectErr := s.reconnectClient(ctx, pending.oldSettings); reconnectErr != nil {
			s.logger.Error("Rollback after persistence error failed", zap.Error(reconnectErr))

			// If rollback fails, try new settings again for runtime consistency.
			if recoverErr := s.reconnectClient(ctx, pending.newSettings); recoverErr != nil {
				s.logger.Error("Recovery with new settings also failed", zap.Error(recoverErr))
			} else {
				s.connectionSettingsManager.SetCurrent(pending.newSettings)
				if persistErr := s.connectionSettingsManager.Persist(pending.newSettings); persistErr != nil {
					s.logger.Error("Recovery succeeded but persisting new settings still failed", zap.Error(persistErr))
				}
			}
		}
		return
	}

	s.logger.Info("Connection settings applied successfully")
}

// handleCertificateResponse handles certificate from enrollment or renewal response.
func (s *Supervisor) handleCertificateResponse(settings *protobufs.OpAMPConnectionSettings) (bool, error) {
	cert := settings.GetCertificate()
	if cert == nil {
		return false, nil
	}

	certPEM := cert.GetCert()
	if len(certPEM) == 0 {
		return false, nil
	}

	s.logger.Info("Received certificate from server")

	// Branch 1: enrollment (HasPendingEnrollment takes precedence — during enrollment
	// both HasPendingEnrollment() and pendingCSR != nil are true)
	if s.authManager.HasPendingEnrollment() {
		s.logger.Info("Completing enrollment with received certificate")
		if err := s.authManager.CompleteEnrollment(certPEM); err != nil {
			return false, fmt.Errorf("failed to complete enrollment: %w", err)
		}

		s.mu.Lock()
		s.pendingCSR = nil
		s.mu.Unlock()

		s.logger.Info("Enrollment completed successfully",
			zap.String("cert_fingerprint", s.authManager.CertFingerprint()),
		)
		return true, nil
	}

	// Branch 2: renewal
	s.mu.RLock()
	hasPendingCSR := s.pendingCSR != nil
	s.mu.RUnlock()

	if hasPendingCSR {
		s.logger.Info("Completing certificate renewal")
		// CompleteRenewal takes auth.Manager mutex internally.
		// Must NOT hold s.mu here to avoid deadlock.
		if err := s.authManager.CompleteRenewal(certPEM); err != nil {
			return false, fmt.Errorf("failed to complete renewal: %w", err)
		}

		s.mu.Lock()
		s.pendingCSR = nil
		s.nextRenewalRetry = time.Time{}
		s.renewalBackoff = 0
		s.mu.Unlock()

		s.logger.Info("Certificate renewal completed successfully",
			zap.String("cert_fingerprint", s.authManager.CertFingerprint()),
		)

		// Best-effort post-renewal actions dispatched to the work queue so they
		// don't block the synchronous OnOpampConnectionSettings callback.
		// The collector restart can wait on process shutdown timeouts, and the
		// own-logs reload must serialize with handleOwnLogs.
		if !s.enqueueWork(context.Background(), func(wCtx context.Context) {
			if s.commander != nil {
				if err := s.commander.Restart(wCtx); err != nil {
					s.logger.Error("Failed to restart collector after certificate renewal", zap.Error(err))
				}
			}
			if s.ownLogsManager != nil {
				if err := s.reloadOwnLogsCert(wCtx); err != nil {
					s.logger.Warn("Failed to reload own-logs certificate", zap.Error(err))
				}
			}
		}) {
			s.logger.Warn("Failed to enqueue post-renewal actions")
		}

		// Return false: renewal does not require an OpAMP reconnect. The JWT
		// HeaderFunc picks up the new cert thumbprint automatically on the
		// next request. Only enrollment returns true (to switch from the
		// enrollment token to JWT auth).
		return false, nil
	}

	// Branch 3: no pending request
	s.logger.Debug("No pending enrollment or renewal, ignoring certificate")
	return false, nil
}

// reloadOwnLogsCert reloads the own-logs OTLP exporter's client certificate
// after a certificate renewal.
func (s *Supervisor) reloadOwnLogsCert(ctx context.Context) error {
	if s.ownLogsManager == nil || s.currentOwnLogs == nil || s.currentOwnLogs.TLSConfig == nil {
		return nil
	}

	certPath := s.authManager.GetSigningCertPath()
	keyPath := s.authManager.GetSigningKeyPath()

	if err := s.currentOwnLogs.LoadClientCert(certPath, keyPath); err != nil {
		return fmt.Errorf("load client cert: %w", err)
	}

	res := ownlogs.BuildResource(ServiceName, version.Version(), s.instanceUID)
	return s.ownLogsManager.Apply(ctx, *s.currentOwnLogs, res)
}

// awaitCollectorHealthy polls the health monitor until the collector reports
// healthy or the timeout expires. This is needed because Commander.Start()
// returns immediately when crash recovery is enabled (MaxRetries >= 1).
//
// If the health HTTP endpoint is never reachable (only connection errors, never
// an HTTP response) and the process is still running at timeout, we treat the
// collector as healthy. This avoids false rollbacks when the health endpoint is
// temporarily unavailable (e.g. slow bind, port conflict).
//
// However, if the endpoint IS reachable but returns non-2xx responses, that is a
// definitive unhealthy signal and the process-alive fallback does not apply.
func (s *Supervisor) awaitCollectorHealthy(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Wait for startup grace period before polling, giving the collector
	// time to bind its health endpoint after a restart. This mirrors the
	// same delay used by the background health monitor (StartPolling).
	if grace := s.agentCfg.Health.StartupGracePeriod; grace > 0 {
		s.logger.Debug("Waiting for startup grace period before health polling",
			zap.Duration("duration", grace))
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastStatus *healthmonitor.HealthStatus
	// Track whether we ever got an HTTP response (even an unhealthy one).
	// The process-alive fallback only applies when the endpoint was never reachable.
	endpointReached := false

	for {
		status, err := s.healthMonitor.CheckHealth(ctx)
		if err == nil && status.Healthy {
			return nil
		}

		if err == nil {
			// Got an HTTP response but non-2xx — definitive unhealthy signal.
			endpointReached = true
			lastStatus = status
		} else {
			// Connection error — endpoint not reachable (yet).
			if s.commander.IsRunning() {
				s.logger.Debug("Health endpoint unreachable but process alive, waiting",
					zap.Error(err))
			}
		}

		select {
		case <-ctx.Done():
			// If the endpoint was reachable but consistently unhealthy, report failure.
			if endpointReached {
				if lastStatus != nil {
					return fmt.Errorf("collector not healthy after %v: %s", timeout, lastStatus.ErrorMessage)
				}
				return fmt.Errorf("collector not healthy after %v", timeout)
			}
			// Endpoint was never reachable. Fall back to process-alive check.
			if s.commander.IsRunning() {
				s.logger.Warn("Health endpoint never became reachable, but process is running; treating as healthy")
				return nil
			}
			return fmt.Errorf("collector not healthy after %v: health endpoint unreachable and process not running", timeout)
		case <-ticker.C:
		}
	}
}

// rollbackAndRecover rolls back to the previous config and restarts the
// collector. Used when the new config fails (either restart error or health
// check failure). Reports FAILED status to the server.
func (s *Supervisor) rollbackAndRecover(ctx context.Context, configHash []byte, originalErr error) {
	if rbErr := s.configManager.RollbackConfig(); rbErr != nil {
		s.logger.Error("Failed to roll back config, skipping recovery restart", zap.Error(rbErr))
	} else {
		// Restart with rolled-back config. The collector may be stopped
		// (Restart = Stop + Start, failed Start leaves process down) or
		// running with a bad config (health check failed).
		if restartErr := s.commander.Restart(ctx); restartErr != nil {
			s.logger.Error("Failed to restart collector after rollback", zap.Error(restartErr))
		}
	}

	s.reportRemoteConfigStatus(ctx,
		protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED,
		fmt.Sprintf("config apply failed: %v", originalErr),
		configHash,
	)
}

// reportRemoteConfigStatus reports config status to the OpAMP server and persists it to disk.
func (s *Supervisor) reportRemoteConfigStatus(_ context.Context, status protobufs.RemoteConfigStatuses, errMsg string, configHash []byte) {
	s.mu.RLock()
	client := s.opampClient
	s.mu.RUnlock()

	if client != nil {
		if err := client.SetRemoteConfigStatus(&protobufs.RemoteConfigStatus{
			Status:               status,
			ErrorMessage:         errMsg,
			LastRemoteConfigHash: configHash,
		}); err != nil {
			s.logger.Warn("Failed to report remote config status", zap.Error(err))
		}
	}

	if err := s.configManager.SaveRemoteConfigStatus(status, errMsg, configHash); err != nil {
		s.logger.Warn("Failed to persist remote config status", zap.Error(err))
	}
}

// reportEffectiveConfig reports the effective config to the OpAMP server.
func (s *Supervisor) reportEffectiveConfig(ctx context.Context, effectiveConfig []byte) {
	s.mu.RLock()
	client := s.opampClient
	s.mu.RUnlock()

	if client != nil {
		if err := client.SetEffectiveConfig(ctx, map[string]*protobufs.AgentConfigFile{
			"collector.yaml": {Body: effectiveConfig},
		}); err != nil {
			s.logger.Warn("Failed to report effective config", zap.Error(err))
		}
	}
}

// createOpAMPCallbacks creates the OpAMP client callbacks.
// This is extracted to avoid duplication when recreating the client.
func (s *Supervisor) createOpAMPCallbacks() *opamp.Callbacks {
	return &opamp.Callbacks{
		OnConnect: func(ctx context.Context) {
			s.logger.Debug("Connected to OpAMP server", zap.String("endpoint", s.connectionSettingsManager.GetCurrent().Endpoint))
		},
		OnConnectFailed: func(ctx context.Context, err error) {
			s.logger.Error("Failed to connect to OpAMP server", zap.Error(err))
		},
		OnError: func(ctx context.Context, err *protobufs.ServerErrorResponse) {
			s.logger.Error("OpAMP server error", zap.Error(fmt.Errorf("%s: %s", err.GetType(), err.GetErrorMessage())))
		},
		OnRemoteConfig: func(ctx context.Context, cfg *protobufs.AgentRemoteConfig) bool {
			s.logger.Info("Received remote configuration")

			result, err := s.configManager.ApplyRemoteConfig(ctx, cfg)
			if err != nil {
				s.logger.Error("Failed to apply remote config", zap.Error(err))
				s.reportRemoteConfigStatus(ctx,
					protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED,
					err.Error(),
					cfg.GetConfigHash(),
				)
				return false
			}

			if !result.Changed {
				return true
			}

			// Restart collector with new config.
			// Restart = Stop + Start, so if Start fails the collector is down.
			// On failure we roll back the config file and re-start the collector
			// with the previous config to avoid leaving it stopped.
			s.logger.Info("Config changed, restarting collector")
			if err := s.commander.Restart(ctx); err != nil {
				s.logger.Error("Failed to restart collector with new config", zap.Error(err))
				s.rollbackAndRecover(ctx, cfg.GetConfigHash(), err)
				return false
			}

			// Confirm the collector is healthy with the new config.
			// Commander.Start() returns immediately when crash recovery is
			// enabled (MaxRetries >= 1), so we must poll health to confirm
			// the process actually started successfully.
			if err := s.awaitCollectorHealthy(ctx, s.agentCfg.ConfigApplyTimeout); err != nil {
				s.logger.Error("Collector unhealthy after restart", zap.Error(err))
				s.rollbackAndRecover(ctx, cfg.GetConfigHash(), err)
				return false
			}

			// Report effective config
			s.reportEffectiveConfig(ctx, result.EffectiveConfig)

			// Report success
			s.reportRemoteConfigStatus(ctx,
				protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
				"",
				cfg.GetConfigHash(),
			)

			return true
		},
		OnOpampConnectionSettings: func(ctx context.Context, settings *protobufs.OpAMPConnectionSettings) error {
			s.logger.Info("Received connection settings update")

			// Phase 1: validate and prepare (synchronous, returns status to opamp-go)
			pending, err := s.prepareConnectionSettings(ctx, settings)
			if err != nil {
				return err
			}
			if pending == nil {
				s.logger.Debug("No connection settings changes requiring reconnection")
				return nil // no reconnection needed
			}

			// Phase 2: reconnect (async on worker, can't block callback)
			if !s.enqueueWork(ctx, func(wCtx context.Context) {
				s.applyConnectionSettings(wCtx, pending)
			}) {
				// Enqueue failed (context cancelled during shutdown or opamp-go timeout).
				// Commit staged file so new settings are persisted for next restart.
				// The OpAMP spec says the server SHOULD NOT re-send unchanged settings.
				s.logger.Warn("Failed to enqueue connection settings apply (context cancelled), persisting for next restart")
				if commitErr := pending.stagedFile.Commit(); commitErr != nil {
					s.logger.Error("Failed to commit staged settings file", zap.Error(commitErr))
					if cleanupErr := pending.stagedFile.Cleanup(); cleanupErr != nil {
						s.logger.Error("Failed to clean up staged settings file", zap.Error(cleanupErr))
					}
					return fmt.Errorf("failed to persist settings for deferred apply: %w", commitErr)
				}

				// Restore old settings as in-memory baseline since we're not applying
				// the new settings at runtime. They're persisted on disk for next restart.
				s.connectionSettingsManager.SetCurrent(pending.oldSettings)
			}

			// TODO: Returning nil here reports APPLIED to the server, but reconnect has not
			// completed yet. This is optimistic — see "Status Reporting Limitations" in the
			// design doc. Once opamp-go exposes a public SetConnectionSettingsStatus API,
			// applyConnectionSettings should send late FAILED/APPLIED after async reconnect.
			return nil
		},
		OnPackagesAvailable: func(ctx context.Context, packages *protobufs.PackagesAvailable) bool {
			// TODO: Implement package handling - opamp-go/client/types.PackagesSyncer
			s.logger.Warn("TODO: Received packages available", zap.String("packages", fmt.Sprintf("%v", packages.GetPackages())))
			return false
		},
		OnCommand: func(ctx context.Context, command *protobufs.ServerToAgentCommand) error {
			s.logger.Warn("TODO: Received command", zap.String("type", command.GetType().String()))
			return nil
		},
		OnCustomMessage: func(ctx context.Context, customMessage *protobufs.CustomMessage) {
			s.logger.Debug("Received custom message",
				zap.String("capability", customMessage.GetCapability()),
				zap.String("type", customMessage.GetType()),
			)

			// Forward custom messages to the local OpAMP server (collector)
			s.forwardCustomMessage(ctx, customMessage)
		},
		OnOwnLogs: func(ctx context.Context, settings *protobufs.TelemetryConnectionSettings) {
			if !s.enqueueWork(ctx, func(wCtx context.Context) {
				s.handleOwnLogs(wCtx, settings)
			}) {
				s.logger.Warn("Failed to enqueue own_logs apply")
			}
		},
		SaveRemoteConfigStatus: func(ctx context.Context, status *protobufs.RemoteConfigStatus) {
			s.logger.Debug("SaveRemoteConfigStatus callback invoked",
				zap.String("status", status.GetStatus().String()),
			)
			if err := s.configManager.SaveRemoteConfigStatus(
				status.GetStatus(),
				status.GetErrorMessage(),
				status.GetLastRemoteConfigHash(),
			); err != nil {
				s.logger.Warn("Failed to persist remote config status from callback", zap.Error(err))
			}
		},
	}
}

// forwardCustomMessage forwards a custom message from the upstream server to the local collector.
func (s *Supervisor) forwardCustomMessage(ctx context.Context, customMessage *protobufs.CustomMessage) {
	s.mu.RLock()
	server := s.opampServer
	s.mu.RUnlock()

	if server == nil {
		s.logger.Warn("Cannot forward custom message: local OpAMP server not running")
		return
	}

	// Create a ServerToAgent message containing the custom message
	msg := &protobufs.ServerToAgent{
		CustomMessage: customMessage,
	}

	// Broadcast to all connected collectors (typically just one)
	server.Broadcast(ctx, msg)
	s.logger.Debug("Forwarded custom message to collector")
}

// handleOwnLogs processes own_logs connection settings from the OpAMP server.
// It applies settings to the supervisor's own logger, persists them, and restarts
// the collector so it picks up the new own-logs.yaml at startup.
func (s *Supervisor) handleOwnLogs(ctx context.Context, settings *protobufs.TelemetryConnectionSettings) {
	if s.ownLogsManager == nil {
		s.logger.Warn("Received own_logs settings but own logs manager is not configured")
		return
	}

	// Empty endpoint signals "stop sending own logs".
	if settings.GetDestinationEndpoint() == "" {
		shouldDisable := s.currentOwnLogs != nil
		if !shouldDisable && s.ownLogsPersistence != nil {
			_, exists, err := s.ownLogsPersistence.Load()
			if err != nil {
				s.logger.Error("Failed to load persisted own_logs settings during disable", zap.Error(err))
				return
			}
			shouldDisable = exists
		}
		if !shouldDisable {
			s.logger.Debug("Own logs already disabled, skipping apply")
			return
		}
		s.logger.Info("Received own_logs with empty endpoint, disabling OTLP log export")
		if err := s.ownLogsManager.Disable(ctx); err != nil {
			s.logger.Error("Failed to disable own_logs export", zap.Error(err))
		}
		if s.ownLogsPersistence != nil {
			if err := s.ownLogsPersistence.Delete(); err != nil {
				s.logger.Error("Failed to delete persisted own_logs settings, skipping collector restart", zap.Error(err))
				return
			}
		}
		s.currentOwnLogs = nil
		// TODO: If own_logs and a config change arrive close together, the collector
		// may be restarted twice. This is harmless but wasteful. Consider coalescing
		// restarts in the future.
		s.restartCollector(ctx)
		return
	}

	s.logger.Info("Received own_logs connection settings",
		zap.String("endpoint", settings.GetDestinationEndpoint()),
	)

	converted, err := ownlogs.ConvertSettings(settings,
		s.authManager.GetSigningCertPath(),
		s.authManager.GetSigningKeyPath(),
	)
	if err != nil {
		s.logger.Error("Failed to convert own_logs settings", zap.Error(err))
		return
	}

	if s.currentOwnLogs != nil && s.currentOwnLogs.Equal(converted) {
		s.logger.Debug("Own logs settings unchanged, skipping apply")
		return
	}

	res := ownlogs.BuildResource(ServiceName, version.Version(), s.instanceUID)

	if err := s.ownLogsManager.Apply(ctx, converted, res); err != nil {
		s.logger.Error("Failed to apply own_logs settings", zap.Error(err))
		return
	}

	// Persist for restart. Only restart the collector if persistence succeeds,
	// otherwise the collector would read stale or missing settings.
	if s.ownLogsPersistence != nil {
		if err := s.ownLogsPersistence.Save(converted); err != nil {
			s.logger.Error("Failed to persist own_logs settings, skipping collector restart", zap.Error(err))
			return
		}
	}
	settingsCopy := converted
	s.currentOwnLogs = &settingsCopy

	s.logger.Info("Own logs OTLP export enabled",
		zap.String("endpoint", converted.Endpoint),
	)
	// Restart collector so it picks up the new own-logs.yaml at startup.
	// Unlike remote config updates, this does not change collector.yaml, and
	// the collector treats own-logs startup errors as non-fatal. There is
	// therefore no config rollback path here; a restart failure is just the
	// ordinary stop/start failure mode and is logged below.
	s.restartCollector(ctx)
}

// restartCollector restarts the collector process as a best-effort follow-up
// to own_logs changes. Failures are logged but not returned because own_logs
// updates are handled asynchronously and have no status/error response path
// back to the OpAMP server.
func (s *Supervisor) restartCollector(ctx context.Context) {
	s.logger.Info("Restarting collector to apply own_logs changes")
	if err := s.commander.Restart(ctx); err != nil {
		s.logger.Error("Failed to restart collector after own_logs change", zap.Error(err))
	}
}

// checkCertificateRenewal checks if the certificate needs renewal and initiates
// or retries the renewal process.
func (s *Supervisor) checkCertificateRenewal() {
	s.logger.Debug("Checking certificate renewal")

	if !s.authManager.IsEnrolled() {
		return
	}

	if s.authManager.CertificateExpired() {
		s.logger.Error("Certificate expired, renewal pending")
	}

	if s.authManager.HasPendingEnrollment() {
		return // enrollment in progress, not our concern
	}

	s.mu.RLock()
	hasPendingCSR := s.pendingCSR != nil
	nextRetry := s.nextRenewalRetry
	s.mu.RUnlock()

	if !hasPendingCSR {
		fraction := s.authCfg.RenewalFraction
		if fraction == 0 {
			fraction = 0.75 // default if unset
		}
		if s.authManager.CertificateNeedsRenewal(fraction) {
			s.requestCertificateRenewal()
		}
		return
	}

	// Renewal pending — check retry/response timeout
	if time.Now().After(nextRetry) {
		s.requestCertificateRenewal()
	}
}

// requestCertificateRenewal generates a renewal CSR and sends it via OpAMP.
func (s *Supervisor) requestCertificateRenewal() {
	s.logger.Debug("Requesting certificate renewal")

	csrPEM, err := s.authManager.PrepareRenewal(s.instanceUID)
	if err != nil {
		s.logger.Error("Failed to prepare renewal CSR", zap.Error(err))
		s.advanceRenewalBackoff()
		return
	}

	s.mu.RLock()
	client := s.opampClient
	s.mu.RUnlock()

	if client == nil {
		s.logger.Warn("OpAMP client not available for certificate renewal")
		s.advanceRenewalBackoff()
		return
	}

	s.mu.Lock()
	s.pendingCSR = csrPEM
	s.mu.Unlock()

	if err := client.RequestConnectionSettings(csrPEM); err != nil {
		s.logger.Warn("Failed to send certificate renewal request", zap.Error(err))
		s.advanceRenewalBackoff()
		return
	}

	// Request sent successfully — set response timeout
	s.mu.Lock()
	s.nextRenewalRetry = time.Now().Add(renewalResponseTimeout)
	s.mu.Unlock()

	s.logger.Info("Certificate renewal requested, awaiting response")
}

// advanceRenewalBackoff advances the exponential backoff for renewal retries.
func (s *Supervisor) advanceRenewalBackoff() {
	s.mu.Lock()
	if s.renewalBackoff == 0 {
		s.renewalBackoff = s.renewalBackoffCfg.Initial
	} else {
		s.renewalBackoff = time.Duration(float64(s.renewalBackoff) * s.renewalBackoffCfg.Multiplier)
	}
	if s.renewalBackoff > s.renewalBackoffCfg.Max {
		s.renewalBackoff = s.renewalBackoffCfg.Max
	}
	s.nextRenewalRetry = time.Now().Add(s.renewalBackoff)
	s.mu.Unlock()
}
