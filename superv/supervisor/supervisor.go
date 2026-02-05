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
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"
	"time"

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

// connectionSettingsSnapshot holds the previous connection state for rollback.
type connectionSettingsSnapshot struct {
	endpoint    string
	headers     http.Header
	tlsInsecure bool
	tlsCACert   string
	tlsMinVer   string
	proxyURL    string
}

// Supervisor coordinates the management of an OpenTelemetry Collector.
type Supervisor struct {
	logger        *zap.Logger
	cfg           config.Config
	instanceUID   string
	authManager   *auth.Manager
	configManager *configmanager.Manager
	healthMonitor *healthmonitor.Monitor
	healthCancel  context.CancelFunc
	healthWg      sync.WaitGroup
	commander     *keen.Commander
	opampClient   *opamp.Client
	opampServer   *opamp.Server
	mu            sync.RWMutex
	running       bool

	// Pending enrollment CSR (set during enrollment, cleared after completion)
	pendingCSR []byte

	// Connection settings snapshot for rollback
	connSettingsSnapshot *connectionSettingsSnapshot
}

// New creates a new Supervisor instance.
func New(logger *zap.Logger, cfg config.Config) (*Supervisor, error) {
	// Load or create instance UID
	uid, err := persistence.LoadOrCreateInstanceUID(cfg.Persistence.Dir)
	if err != nil {
		return nil, err
	}

	// Determine keys directory
	keysDir := cfg.Keys.Dir
	if keysDir == "" {
		keysDir = filepath.Join(cfg.Persistence.Dir, "keys")
	}

	// Create auth manager
	authMgr := auth.NewManager(logger.Named("auth"), auth.ManagerConfig{
		KeysDir:     keysDir,
		JWTLifetime: cfg.Auth.JWTLifetime,
		InsecureTLS: cfg.Auth.InsecureTLS,
	})

	// Initialize config manager
	configMgr := configmanager.New(logger.Named("config"), configmanager.Config{
		ConfigDir:      filepath.Join(cfg.Persistence.Dir, "config"),
		OutputPath:     filepath.Join(cfg.Persistence.Dir, "config", "collector.yaml"),
		LocalOverrides: cfg.Agent.Config.LocalOverrides,
		LocalEndpoint:  cfg.LocalOpAMP.Endpoint,
		InstanceUID:    uid,
	})

	// Initialize health monitor
	healthMon := healthmonitor.New(logger.Named("health"), healthmonitor.Config{
		Endpoint: cfg.Agent.Health.Endpoint,
		Timeout:  cfg.Agent.Health.Timeout,
		Interval: cfg.Agent.Health.Interval,
	})

	return &Supervisor{
		logger:        logger,
		cfg:           cfg,
		instanceUID:   uid,
		authManager:   authMgr,
		configManager: configMgr,
		healthMonitor: healthMon,
	}, nil
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
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	s.logger.Info("Starting supervisor",
		zap.String("instance_uid", s.instanceUID),
		zap.String("endpoint", s.cfg.Server.Endpoint),
	)

	// Initialize authentication
	if err := s.initAuth(ctx); err != nil {
		return fmt.Errorf("authentication initialization failed: %w", err)
	}

	// TODO: We need to clarify how the endpoint is set when enrollment is used.
	//       Config file? Or enrollment URL, received connection settings, which order, etc.

	// If endpoint doesn't have a path, derive it from the enrollment URL
	if s.cfg.Auth.EnrollmentURL != "" {
		u, err := url.Parse(s.cfg.Server.Endpoint)
		if err == nil && (u.Path == "" || u.Path == "/") {
			derivedEndpoint, err := deriveEndpointFromEnrollmentURL(s.cfg.Auth.EnrollmentURL)
			if err == nil {
				s.logger.Info("Derived OpAMP endpoint from enrollment URL",
					zap.String("endpoint", derivedEndpoint),
				)
				s.cfg.Server.Endpoint = derivedEndpoint
			}
		}
	}

	// Determine effective config path
	// TODO: Write actual merged config when remote config handling is implemented
	configPath := filepath.Join(s.cfg.Persistence.Dir, "effective.yaml")

	// Expand template variables in agent args
	expandedArgs, err := s.expandArgs(s.cfg.Agent.Args, configPath)
	if err != nil {
		return fmt.Errorf("failed to expand agent args: %w", err)
	}

	// Create commander for agent process management
	cmd, err := keen.New(s.logger, s.cfg.Persistence.Dir, keen.Config{
		Executable:      s.cfg.Agent.Executable,
		Args:            expandedArgs,
		Env:             s.cfg.Agent.Env,
		PassthroughLogs: s.cfg.Agent.PassthroughLogs,
	})
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
		ListenEndpoint: s.cfg.LocalOpAMP.Endpoint,
	}, serverCallbacks)
	if err != nil {
		return err
	}
	s.opampServer = opampServer

	// Start local OpAMP server
	if err := s.opampServer.Start(ctx); err != nil {
		return err
	}

	// Load any persisted connection settings from previous server interactions
	if err := s.loadPersistedConnectionSettings(); err != nil {
		s.logger.Warn("Failed to load persisted connection settings", zap.Error(err))
		// Continue with initial config if persisted settings fail to load
	}

	// Create and start OpAMP client for upstream server
	opampClient, err := s.createAndStartClient(ctx)
	if err != nil {
		s.opampServer.Stop(ctx)
		return fmt.Errorf("create opamp client: %w", err)
	}
	s.opampClient = opampClient

	// Start health monitoring with a cancellable context
	healthCtx, healthCancel := context.WithCancel(ctx)
	s.healthCancel = healthCancel
	healthUpdates := s.healthMonitor.StartPolling(healthCtx)
	s.healthWg.Go(func() {
		for status := range healthUpdates {
			// TODO: Pass commander as AgentStateProvider once it implements the interface
			// This will enable accurate agent start time reporting per OpAMP spec
			if err := s.opampClient.SetHealth(status.ToComponentHealth(nil)); err != nil {
				s.logger.Warn("Failed to report health", zap.Error(err))
			}
		}
	})

	// Configure crash recovery if enabled
	if s.cfg.Agent.Restart.MaxRetries > 0 {
		defaults := keen.DefaultBackoffConfig()
		backoffCfg := keen.BackoffConfig{
			MaxRetries:          s.cfg.Agent.Restart.MaxRetries,
			InitialInterval:     s.cfg.Agent.Restart.InitialInterval,
			MaxInterval:         s.cfg.Agent.Restart.MaxInterval,
			Multiplier:          s.cfg.Agent.Restart.Multiplier,
			RandomizationFactor: s.cfg.Agent.Restart.RandomizationFactor,
			StableAfter:         s.cfg.Agent.Restart.StableAfter,
		}
		// Apply defaults for zero values
		if backoffCfg.InitialInterval == 0 {
			backoffCfg.InitialInterval = defaults.InitialInterval
		}
		if backoffCfg.MaxInterval == 0 {
			backoffCfg.MaxInterval = defaults.MaxInterval
		}
		if backoffCfg.Multiplier == 0 {
			backoffCfg.Multiplier = defaults.Multiplier
		}
		if backoffCfg.StableAfter == 0 {
			backoffCfg.StableAfter = defaults.StableAfter
		}
		// Note: RandomizationFactor of 0 is valid (no jitter), so don't default it
		s.commander.SetBackoff(backoffCfg)
	}

	// Start the collector agent
	if s.cfg.Agent.Restart.MaxRetries > 0 {
		if err := s.commander.StartWithRecovery(ctx); err != nil {
			s.opampClient.Stop(ctx)
			s.opampServer.Stop(ctx)
			return fmt.Errorf("failed to start agent with recovery: %w", err)
		}
	} else {
		if err := s.commander.Start(ctx); err != nil {
			s.opampClient.Stop(ctx)
			s.opampServer.Stop(ctx)
			return fmt.Errorf("failed to start agent: %w", err)
		}
	}

	s.running = true
	return nil
}

// Stop stops the supervisor and the managed collector.
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("Stopping supervisor")

	// Stop health monitoring
	if s.healthCancel != nil {
		s.healthCancel()
	}
	s.healthWg.Wait()

	// Stop commander (agent)
	if s.commander != nil {
		if err := s.commander.Stop(ctx); err != nil {
			s.logger.Error("Error stopping agent", zap.Error(err))
		}
	}

	// Stop OpAMP client
	if s.opampClient != nil {
		if err := s.opampClient.Stop(ctx); err != nil {
			s.logger.Error("Error stopping OpAMP client", zap.Error(err))
		}
	}

	// Stop OpAMP server
	if s.opampServer != nil {
		if err := s.opampServer.Stop(ctx); err != nil {
			s.logger.Error("Error stopping OpAMP server", zap.Error(err))
		}
	}

	s.running = false
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
		Executable: s.cfg.Agent.Executable,
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
func (s *Supervisor) createAndStartClient(ctx context.Context) (*opamp.Client, error) {
	headers, err := s.buildAuthHeaders()
	if err != nil {
		return nil, fmt.Errorf("build auth headers: %w", err)
	}

	client, err := opamp.NewClient(s.logger, opamp.ClientConfig{
		Endpoint:    s.cfg.Server.Endpoint,
		InstanceUID: s.instanceUID,
		Headers:     headers,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: s.cfg.Auth.InsecureTLS || s.cfg.Server.TLS.Insecure,
		},
		Capabilities: opamp.Capabilities{
			AcceptsRemoteConfig:            true,
			ReportsEffectiveConfig:         true,
			ReportsHealth:                  true,
			AcceptsOpAMPConnectionSettings: true,
			AcceptsRestartCommand:          true,
			ReportsHeartbeat:               true,
			ReportsAvailableComponents:     true,
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

	// We need this in the enrollment process
	if len(s.pendingCSR) > 0 && client != nil {
		s.logger.Info("Sending CSR for enrollment via OpAMP")
		if err := client.RequestConnectionSettings(s.pendingCSR); err != nil {
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

		// Set server host from endpoint for JWT audience
		if host := s.extractHostFromEndpoint(); host != "" {
			s.authManager.SetServerHost(host)
		}

		s.logger.Info("Credentials loaded",
			zap.String("cert_fingerprint", s.authManager.CertFingerprint()),
		)
		return nil
	}

	// Need to enroll - prepare the CSR
	if s.cfg.Auth.EnrollmentURL == "" {
		return fmt.Errorf("not enrolled and no enrollment URL configured")
	}

	s.logger.Info("Preparing enrollment")
	result, err := s.authManager.PrepareEnrollment(ctx, s.cfg.Auth.EnrollmentURL, s.instanceUID)
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

// buildAuthHeaders creates HTTP headers including authentication.
// During enrollment, uses the enrollment JWT as bearer token.
// After enrollment, generates a new JWT signed with the client certificate.
func (s *Supervisor) buildAuthHeaders() (http.Header, error) {
	headers := s.cfg.Server.ToHTTPHeaders()

	// If we're not enrolled yet, use the enrollment JWT
	if !s.authManager.IsEnrolled() {
		if jwt := s.authManager.EnrollmentJWT(); jwt != "" {
			headers.Set("Authorization", "Bearer "+jwt)
		}
		return headers, nil
	}

	// Get server host for JWT audience
	audience := s.authManager.ServerHost()
	if audience == "" {
		audience = s.extractHostFromEndpoint()
	}

	if audience != "" {
		authHeader, err := s.authManager.GetAuthorizationHeader(audience)
		if err != nil {
			return nil, err
		}
		headers.Set("Authorization", authHeader)
	}

	return headers, nil
}

// extractHostFromEndpoint extracts the host from the server endpoint URL.
func (s *Supervisor) extractHostFromEndpoint() string {
	u, err := url.Parse(s.cfg.Server.Endpoint)
	if err != nil {
		return ""
	}
	return u.Host
}

// deriveEndpointFromEnrollmentURL derives the OpAMP endpoint from the enrollment URL.
// The enrollment URL format is: https://host:port/opamp/enroll/<jwt>
// The derived endpoint is: https://host:port/v1/opamp
func deriveEndpointFromEnrollmentURL(enrollmentURL string) (string, error) {
	u, err := url.Parse(enrollmentURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse enrollment URL: %w", err)
	}

	// Build the endpoint URL with the same scheme and host
	endpoint := &url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   "/v1/opamp",
	}

	return endpoint.String(), nil
}

// handleConnectionSettings processes connection settings updates from the server.
// This is called in a goroutine from the OnOpampConnectionSettings callback.
// Note: ctx should be context.Background() since the callback context may be cancelled.
func (s *Supervisor) handleConnectionSettings(ctx context.Context, settings *protobufs.OpAMPConnectionSettings) {
	if settings == nil {
		s.logger.Debug("Received nil connection settings, ignoring")
		return
	}

	// Handle enrollment certificate first (doesn't require reconnection)
	newlyEnrolled, err := s.handleEnrollmentCertificate(settings)
	if err != nil {
		s.logger.Error("Failed to handle enrollment certificate", zap.Error(err))
		s.reportConnectionSettingsStatus(false, fmt.Sprintf("enrollment certificate handling failed: %v", err))
		return
	}

	// Handle heartbeat interval update (doesn't require reconnection)
	if interval := settings.GetHeartbeatIntervalSeconds(); interval > 0 {
		newInterval := time.Duration(interval) * time.Second
		s.mu.RLock()
		client := s.opampClient
		s.mu.RUnlock()

		if client != nil {
			oldInterval := client.HeartbeatInterval()
			if newInterval != oldInterval {
				client.SetHeartbeatInterval(newInterval)
				s.logger.Info("Updated heartbeat interval",
					zap.Duration("old_interval", oldInterval),
					zap.Duration("new_interval", newInterval),
				)
			}
		}
	}

	// Check if any settings require reconnection
	if !s.connectionSettingsChanged(settings) && !newlyEnrolled {
		s.logger.Debug("No connection settings changes requiring reconnection")
		s.reportConnectionSettingsStatus(true, "")
		return
	}

	// Capture current state for rollback
	s.captureConnectionSnapshot()

	// Apply new settings with reconnection
	if err := s.applyConnectionSettings(ctx, settings); err != nil {
		s.logger.Error("Failed to apply connection settings, rolling back", zap.Error(err))
		if rollbackErr := s.rollbackConnectionSettings(ctx); rollbackErr != nil {
			s.logger.Error("Rollback also failed", zap.Error(rollbackErr))
		}
		s.reportConnectionSettingsStatus(false, fmt.Sprintf("failed to apply settings: %v", err))
		return
	}

	// Persist settings after successful reconnection
	if err := s.persistConnectionSettings(settings); err != nil {
		s.logger.Warn("Failed to persist connection settings", zap.Error(err))
		// Don't fail - we're already connected with new settings
	}

	// Clear snapshot after successful application
	s.mu.Lock()
	s.connSettingsSnapshot = nil
	s.mu.Unlock()

	s.reportConnectionSettingsStatus(true, "")
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

// connectionSettingsChanged checks if the settings require a reconnection.
// Returns true if endpoint, headers, TLS, or proxy settings have changed.
func (s *Supervisor) connectionSettingsChanged(settings *protobufs.OpAMPConnectionSettings) bool {
	// Check endpoint change
	if endpoint := settings.GetDestinationEndpoint(); endpoint != "" && endpoint != s.cfg.Server.Endpoint {
		s.logger.Debug("Endpoint change detected",
			zap.String("current", s.cfg.Server.Endpoint),
			zap.String("new", endpoint),
		)
		return true
	}

	// Check headers change
	if headers := settings.GetHeaders(); headers != nil {
		newHeaders := convertProtoHeaders(headers)
		if !headersEqual(s.cfg.Server.Headers, newHeaders) {
			s.logger.Debug("Headers change detected")
			return true
		}
	}

	// Check TLS settings change
	if tls := settings.GetTls(); tls != nil {
		// Any TLS settings provided indicates a change
		s.logger.Debug("TLS settings change detected")
		return true
	}

	// Check proxy settings change
	if proxy := settings.GetProxy(); proxy != nil {
		if proxyURL := proxy.GetUrl(); proxyURL != "" {
			s.logger.Debug("Proxy settings change detected")
			return true
		}
	}

	return false
}

// captureConnectionSnapshot saves the current connection state for rollback.
func (s *Supervisor) captureConnectionSnapshot() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.connSettingsSnapshot = &connectionSettingsSnapshot{
		endpoint:    s.cfg.Server.Endpoint,
		headers:     s.cfg.Server.ToHTTPHeaders(),
		tlsInsecure: s.cfg.Server.TLS.Insecure,
		tlsCACert:   s.cfg.Server.TLS.CACert,
		tlsMinVer:   s.cfg.Server.TLS.MinVersion,
	}

	s.logger.Debug("Captured connection settings snapshot",
		zap.String("endpoint", s.connSettingsSnapshot.endpoint),
	)
}

// applyConnectionSettings stops the client, applies new settings, and restarts.
func (s *Supervisor) applyConnectionSettings(ctx context.Context, settings *protobufs.OpAMPConnectionSettings) error {
	s.mu.Lock()
	client := s.opampClient
	s.mu.Unlock()

	if client == nil {
		return fmt.Errorf("opamp client not initialized")
	}

	// Stop the current client
	s.logger.Info("Stopping OpAMP client for connection settings update")
	if err := client.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop client: %w", err)
	}

	// Apply new endpoint
	if endpoint := settings.GetDestinationEndpoint(); endpoint != "" {
		s.cfg.Server.Endpoint = endpoint
	}

	// Apply new headers
	if headers := settings.GetHeaders(); headers != nil {
		s.cfg.Server.Headers = convertProtoHeaders(headers)
	}

	// Apply TLS settings
	if tlsSettings := settings.GetTls(); tlsSettings != nil {
		if caPEM := tlsSettings.GetCaPemContents(); caPEM != "" {
			s.cfg.Server.TLS.CACert = caPEM
		}
		s.cfg.Server.TLS.Insecure = tlsSettings.GetInsecureSkipVerify()
		if minVer := tlsSettings.GetMinVersion(); minVer != "" {
			s.cfg.Server.TLS.MinVersion = minVer
		}
	}

	// Create and start new client with updated settings
	s.logger.Info("Starting OpAMP client with new connection settings",
		zap.String("endpoint", s.cfg.Server.Endpoint),
	)
	newClient, err := s.createAndStartClient(ctx)
	if err != nil {
		return fmt.Errorf("apply connection settings: %w", err)
	}

	s.mu.Lock()
	s.opampClient = newClient
	s.mu.Unlock()

	return nil
}

// rollbackConnectionSettings restores the previous connection state.
func (s *Supervisor) rollbackConnectionSettings(ctx context.Context) error {
	s.mu.Lock()
	snapshot := s.connSettingsSnapshot
	s.mu.Unlock()

	if snapshot == nil {
		return fmt.Errorf("no snapshot available for rollback")
	}

	s.logger.Info("Rolling back connection settings",
		zap.String("endpoint", snapshot.endpoint),
	)

	// Restore the original endpoint
	s.cfg.Server.Endpoint = snapshot.endpoint

	// Restore original headers (extract from http.Header to map[string]string)
	if snapshot.headers != nil {
		s.cfg.Server.Headers = make(map[string]string)
		for k, v := range snapshot.headers {
			if len(v) > 0 {
				s.cfg.Server.Headers[k] = v[0]
			}
		}
	}

	// Restore TLS settings
	s.cfg.Server.TLS.Insecure = snapshot.tlsInsecure
	s.cfg.Server.TLS.CACert = snapshot.tlsCACert
	s.cfg.Server.TLS.MinVersion = snapshot.tlsMinVer

	// Create and start client with restored settings
	newClient, err := s.createAndStartClient(ctx)
	if err != nil {
		return fmt.Errorf("rollback connection settings: %w", err)
	}

	s.mu.Lock()
	s.opampClient = newClient
	s.connSettingsSnapshot = nil
	s.mu.Unlock()

	s.logger.Info("Connection settings rollback completed")
	return nil
}

// loadPersistedConnectionSettings loads any persisted connection settings from disk
// and applies them to the configuration. Settings from the server override initial config.
func (s *Supervisor) loadPersistedConnectionSettings() error {
	settings, err := persistence.LoadOpAMPSettings(s.cfg.Persistence.Dir)
	if err != nil {
		return fmt.Errorf("load persisted settings: %w", err)
	}
	if settings == nil {
		// No persisted settings, use initial config
		return nil
	}

	s.logger.Info("Applying persisted connection settings",
		zap.Time("updated_at", settings.UpdatedAt),
	)

	// Apply persisted endpoint if present
	if settings.Endpoint != "" {
		s.cfg.Server.Endpoint = settings.Endpoint
	}

	// Apply persisted headers if present
	if settings.Headers != nil {
		s.cfg.Server.Headers = settings.Headers
	}

	// Apply persisted TLS settings if present
	if settings.CACertPEM != "" {
		s.cfg.Server.TLS.CACert = settings.CACertPEM
	}

	// Note: HeartbeatInterval will be applied after client is created
	// via client.SetHeartbeatInterval() if needed

	return nil
}

// persistConnectionSettings saves the connection settings to disk.
func (s *Supervisor) persistConnectionSettings(settings *protobufs.OpAMPConnectionSettings) error {
	opampSettings := &persistence.OpAMPSettings{
		UpdatedAt: time.Now(),
	}

	if endpoint := settings.GetDestinationEndpoint(); endpoint != "" {
		opampSettings.Endpoint = endpoint
	}

	if headers := settings.GetHeaders(); headers != nil {
		opampSettings.Headers = convertProtoHeaders(headers)
	}

	if tlsSettings := settings.GetTls(); tlsSettings != nil {
		opampSettings.CACertPEM = tlsSettings.GetCaPemContents()
	}

	if proxy := settings.GetProxy(); proxy != nil {
		opampSettings.ProxyURL = proxy.GetUrl()
	}

	if interval := settings.GetHeartbeatIntervalSeconds(); interval > 0 {
		opampSettings.HeartbeatInterval = time.Duration(interval) * time.Second
	}

	return persistence.SaveOpAMPSettings(s.cfg.Persistence.Dir, opampSettings)
}

// reportConnectionSettingsStatus reports the result of applying connection settings.
func (s *Supervisor) reportConnectionSettingsStatus(success bool, errorMsg string) {
	s.mu.RLock()
	client := s.opampClient
	s.mu.RUnlock()

	if client == nil {
		return
	}

	var status protobufs.ConnectionSettingsStatuses
	if success {
		status = protobufs.ConnectionSettingsStatuses_ConnectionSettingsStatuses_APPLIED
	} else {
		status = protobufs.ConnectionSettingsStatuses_ConnectionSettingsStatuses_FAILED
	}

	statusMsg := &protobufs.ConnectionSettingsStatus{
		Status:       status,
		ErrorMessage: errorMsg,
	}

	if err := client.SetConnectionSettingsStatus(statusMsg); err != nil {
		s.logger.Warn("Failed to report connection settings status", zap.Error(err))
	}
}

// createOpAMPCallbacks creates the OpAMP client callbacks.
// This is extracted to avoid duplication when recreating the client.
func (s *Supervisor) createOpAMPCallbacks() *opamp.Callbacks {
	return &opamp.Callbacks{
		OnConnect: func(ctx context.Context) {
			s.logger.Info("Connected to OpAMP server", zap.String("endpoint", s.cfg.Server.Endpoint))

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

			// Handle connection settings in a goroutine to avoid blocking the callback.
			// Use a fresh context with timeout since the callback context may be cancelled
			// after this function returns.
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				s.handleConnectionSettings(ctx, settings)
			}()

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

// convertProtoHeaders converts protobufs.Headers to map[string]string.
func convertProtoHeaders(h *protobufs.Headers) map[string]string {
	if h == nil {
		return nil
	}
	result := make(map[string]string, len(h.Headers))
	for _, header := range h.Headers {
		result[header.Key] = header.Value
	}
	return result
}

// headersEqual compares two header maps for equality.
func headersEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}
