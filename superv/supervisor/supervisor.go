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
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	"github.com/Graylog2/collector-sidecar/superv/healthmonitor"
	"github.com/Graylog2/collector-sidecar/superv/keen"
	"github.com/Graylog2/collector-sidecar/superv/opamp"
	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"github.com/Graylog2/collector-sidecar/superv/version"
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

	// Serialized worker for state-mutating operations.
	workQueue  chan workFunc
	workCtx    context.Context
	workCancel context.CancelFunc
	workWg     sync.WaitGroup

	// Pending enrollment CSR (set during enrollment, cleared after completion)
	pendingCSR []byte

	// createClientFunc creates and starts a new OpAMP client for the given
	// settings. Defaults to createAndStartClient; overridden in tests for
	// interleaving control.
	createClientFunc func(ctx context.Context, settings connection.Settings) (*opamp.Client, error)
}

// New creates a new Supervisor instance.
func New(logger *zap.Logger, cfg config.Config) (*Supervisor, error) {
	// Load or create instance UID
	uid, err := persistence.LoadOrCreateInstanceUID(cfg.Persistence.Dir)
	if err != nil {
		return nil, err
	}

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
				connSettings.Endpoint = enrollEndpoint
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

	// Initialize config manager
	configMgr := configmanager.New(logger.Named("config"), configmanager.Config{
		ConfigDir:      filepath.Join(cfg.Persistence.Dir, "config"),
		OutputPath:     filepath.Join(cfg.Persistence.Dir, "config", "collector.yaml"),
		LocalOverrides: cfg.Agent.Config.LocalOverrides,
		LocalEndpoint:  cfg.LocalServer.Endpoint,
		InstanceUID:    uid,
	})

	// Initialize health monitor
	healthMon := healthmonitor.New(logger.Named("health"), healthmonitor.Config{
		Endpoint: cfg.Agent.Health.Endpoint,
		Timeout:  cfg.Agent.Health.Timeout,
		Interval: cfg.Agent.Health.Interval,
	})

	s := &Supervisor{
		logger:                    logger,
		agentCfg:                  cfg.Agent,
		authCfg:                   cfg.Server.Auth,
		localServerCfg:            cfg.LocalServer,
		persistenceDir:            cfg.Persistence.Dir,
		instanceUID:               uid,
		authManager:               authMgr,
		configManager:             configMgr,
		healthMonitor:             healthMon,
		connectionSettingsManager: connSettingsMgr,
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

	// Initialize authentication
	if err := s.initAuth(ctx); err != nil {
		return fmt.Errorf("authentication initialization failed: %w", err)
	}

	// Determine effective config path
	// TODO: Write actual merged config when remote config handling is implemented
	configPath := filepath.Join(s.persistenceDir, "effective.yaml")

	// Expand template variables in agent args
	expandedArgs, err := s.expandArgs(s.agentCfg.Args, configPath)
	if err != nil {
		return fmt.Errorf("failed to expand agent args: %w", err)
	}

	// Create commander for agent process management
	cmd, err := keen.New(s.logger, s.persistenceDir, keen.Config{
		Executable:      s.agentCfg.Executable,
		Args:            expandedArgs,
		Env:             s.agentCfg.Env,
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

	// Start health monitoring with a cancellable context
	healthCtx, healthCancel := context.WithCancel(ctx)
	s.healthCancel = healthCancel
	healthUpdates := s.healthMonitor.StartPolling(healthCtx)
	s.healthWg.Go(func() {
		for status := range healthUpdates {
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
		}
	})

	// Start the collector agent
	if err := s.commander.Start(ctx); err != nil {
		healthCancel()
		s.healthWg.Wait()

		// Nil out opampClient under lock (it was published above) before stopping,
		// so the health goroutine (now drained) and callbacks see nil.
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

	return nil
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
				Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: "opamp-supervisor"}},
			},
			{
				Key:   "service.instance.id",
				Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: s.instanceUID}},
			},
		},
		NonIdentifyingAttributes: []*protobufs.KeyValue{
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
		},
	}
}

// discoverComponents discovers available components from the collector.
// This is a best-effort operation - failures are logged but don't prevent startup.
func (s *Supervisor) discoverComponents(ctx context.Context) *protobufs.AvailableComponents {
	cfg := components.DiscoverConfig{
		Executable: s.agentCfg.Executable,
		Timeout:    10 * time.Second,
	}

	discovered, err := components.Discover(ctx, cfg)
	if err != nil {
		s.logger.Warn("Failed to discover components", zap.Error(err))
		return nil
	}

	s.logger.Info("Discovered available components",
		zap.Int("receivers", len(discovered.Receivers)),
		zap.Int("processors", len(discovered.Processors)),
		zap.Int("exporters", len(discovered.Exporters)),
		zap.Int("extensions", len(discovered.Extensions)),
	)

	return discovered.ToProto()
}

// createAndStartClient creates a new OpAMP client with current config, sets it up, and starts it.
// This is used when reconnecting with new or restored connection settings.
func (s *Supervisor) createAndStartClient(ctx context.Context, settings connection.Settings) (*opamp.Client, error) {
	headers, err := s.buildAuthHeaders(settings)
	if err != nil {
		return nil, fmt.Errorf("build auth headers: %w", err)
	}

	minVersion, maxVersion, err := settings.TLS.ToTLSMinMaxVersion()
	if err != nil {
		return nil, fmt.Errorf("invalid TLS settings: %w", err)
	}

	client, err := opamp.NewClient(s.logger, opamp.ClientConfig{
		Endpoint:    settings.Endpoint,
		InstanceUID: s.instanceUID,
		Headers:     headers,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: settings.TLS.Insecure,
			MinVersion:         minVersion,
			MaxVersion:         maxVersion,
		},
		Capabilities: opamp.Capabilities{
			AcceptsRemoteConfig:             true,
			ReportsEffectiveConfig:          true,
			ReportsHealth:                   true,
			AcceptsOpAMPConnectionSettings:  true,
			ReportsConnectionSettingsStatus: true,
			AcceptsRestartCommand:           true,
			ReportsHeartbeat:                true,
			ReportsAvailableComponents:      true,
		},
	}, s.createOpAMPCallbacks())
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	if err := client.SetAgentDescription(s.createAgentDescription()); err != nil {
		return nil, fmt.Errorf("set agent description: %w", err)
	}

	if err := client.SetHealth(&protobufs.ComponentHealth{Healthy: true}); err != nil {
		return nil, fmt.Errorf("set health: %w", err)
	}

	// Discover and set available components (required before Start when capability is set)
	if err := s.setClientAvailableComponents(ctx, client); err != nil {
		return nil, fmt.Errorf("set available components: %w", err)
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

// setClientAvailableComponents discovers and sets available components on the given client.
func (s *Supervisor) setClientAvailableComponents(ctx context.Context, client *opamp.Client) error {
	availableComponents := s.discoverComponents(ctx)
	if availableComponents == nil {
		// Use empty components if discovery fails (still need valid hash for opamp-go)
		availableComponents = (&components.Components{}).ToProto()
	}
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

// buildAuthHeaders creates HTTP headers including authentication.
// During enrollment, uses the enrollment JWT as bearer token.
// After enrollment, generates a new JWT signed with the client certificate.
func (s *Supervisor) buildAuthHeaders(settings connection.Settings) (http.Header, error) {
	headers := toHTTPHeaders(settings.Headers)

	// If we're not enrolled yet, use the enrollment JWT
	if !s.authManager.IsEnrolled() {
		if jwt := s.authManager.EnrollmentJWT(); jwt != "" {
			headers.Set("Authorization", "Bearer "+jwt)
		}
		return headers, nil
	}

	authHeader, err := s.authManager.GetAuthorizationHeader()
	if err != nil {
		return nil, err
	}
	headers.Set("Authorization", authHeader)

	return headers, nil
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

	newlyEnrolled, err := s.handleEnrollmentCertificate(settings)
	if err != nil {
		return nil, fmt.Errorf("enrollment certificate handling failed: %w", err)
	}

	// Update the wrapper's stored heartbeat interval for use when creating
	// a new client after reconnect. Note: opamp-go already updated the
	// sender's heartbeat interval in rcvOpampConnectionSettings before
	// invoking this callback, so this only affects the wrapper's state.
	if interval := settings.GetHeartbeatIntervalSeconds(); interval > 0 {
		s.mu.RLock()
		client := s.opampClient
		s.mu.RUnlock()

		if client != nil {
			client.SetHeartbeatInterval(time.Duration(interval) * time.Second)
		}
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

// handleEnrollmentCertificate handles certificate from enrollment response.
func (s *Supervisor) handleEnrollmentCertificate(settings *protobufs.OpAMPConnectionSettings) (bool, error) {
	cert := settings.GetCertificate()
	if cert == nil {
		return false, nil
	}

	certPEM := cert.GetCert()
	if len(certPEM) == 0 {
		return false, nil
	}

	s.logger.Info("Received certificate from server")

	// Check if we have a pending enrollment
	if !s.authManager.HasPendingEnrollment() {
		s.logger.Debug("No pending enrollment, ignoring certificate")
		return false, nil
	}

	s.logger.Info("Completing enrollment with received certificate")
	if err := s.authManager.CompleteEnrollment(certPEM); err != nil {
		return false, fmt.Errorf("failed to complete enrollment: %w", err)
	}

	// Clear the pending CSR
	s.mu.Lock()
	s.pendingCSR = nil
	s.mu.Unlock()

	s.logger.Info("Enrollment completed successfully",
		zap.String("cert_fingerprint", s.authManager.CertFingerprint()),
	)

	return true, nil
}

// createOpAMPCallbacks creates the OpAMP client callbacks.
// This is extracted to avoid duplication when recreating the client.
func (s *Supervisor) createOpAMPCallbacks() *opamp.Callbacks {
	return &opamp.Callbacks{
		OnConnect: func(ctx context.Context) {
			s.logger.Info("Connected to OpAMP server", zap.String("endpoint", s.connectionSettingsManager.GetCurrent().Endpoint))
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
				return false
			}

			if result.Changed {
				// TODO: Reload collector via commander when implemented
				s.logger.Info("Config changed, collector reload needed")

				s.mu.RLock()
				client := s.opampClient
				s.mu.RUnlock()

				// Report effective config
				if client != nil {
					if err := client.SetEffectiveConfig(ctx, map[string]*protobufs.AgentConfigFile{
						"collector.yaml": {
							Body: result.EffectiveConfig,
						},
					}); err != nil {
						s.logger.Warn("Failed to report effective config", zap.Error(err))
					}
				}
			}

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
			s.logger.Warn("TODO: Received packages available: %s", zap.String("packages", fmt.Sprintf("%v", packages.GetPackages())))
			return false
		},
		OnCommand: func(ctx context.Context, command *protobufs.ServerToAgentCommand) error {
			s.logger.Warn("TODO: Received command: %s", zap.String("type", command.GetType().String()))
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
