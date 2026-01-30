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

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server/types"
	"go.uber.org/zap"

	"github.com/Graylog2/collector-sidecar/superv/auth"
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
	logger        *zap.Logger
	cfg           config.Config
	instanceUID   string
	authManager   *auth.Manager
	configManager *configmanager.Manager
	healthMonitor *healthmonitor.Monitor
	healthCancel  context.CancelFunc
	commander     *keen.Commander
	opampClient   *opamp.Client
	opampServer   *opamp.Server
	mu            sync.RWMutex
	running       bool

	// Pending enrollment CSR (set during enrollment, cleared after completion)
	pendingCSR []byte
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

	// Create OpAMP client for upstream server
	clientCallbacks := &opamp.Callbacks{
		OnConnect: func(ctx context.Context) {
			s.logger.Info("Connected to OpAMP server")

			// If we have a pending enrollment CSR, send it now
			s.mu.RLock()
			csr := s.pendingCSR
			s.mu.RUnlock()

			if len(csr) > 0 && s.opampClient != nil {
				s.logger.Info("Sending CSR for enrollment via OpAMP")
				if err := s.opampClient.RequestConnectionSettings(csr); err != nil {
					s.logger.Error("Failed to send CSR", zap.Error(err))
				}
			}
		},
		OnConnectFailed: func(ctx context.Context, err error) {
			s.logger.Error("Failed to connect to OpAMP server", zap.Error(err))
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

				// Report effective config
				if err := s.opampClient.SetEffectiveConfig(map[string]*protobufs.AgentConfigFile{
					"collector.yaml": {
						Body: result.EffectiveConfig,
					},
				}); err != nil {
					s.logger.Warn("Failed to report effective config", zap.Error(err))
				}
			}

			return true
		},
		OnOpampConnectionSettings: func(ctx context.Context, settings *protobufs.OpAMPConnectionSettings) error {
			s.logger.Info("Received connection settings update")

			// Handle certificate from enrollment response
			if cert := settings.GetCertificate(); cert != nil {
				if certPEM := cert.GetCert(); len(certPEM) > 0 {
					s.logger.Info("Received certificate from server")

					// Check if we have a pending enrollment
					if s.authManager.HasPendingEnrollment() {
						s.logger.Info("Completing enrollment with received certificate")
						if err := s.authManager.CompleteEnrollment(certPEM); err != nil {
							s.logger.Error("Failed to complete enrollment", zap.Error(err))
							return err
						}

						// Clear the pending CSR
						s.mu.Lock()
						s.pendingCSR = nil
						s.mu.Unlock()

						s.logger.Info("Enrollment completed successfully",
							zap.String("cert_fingerprint", s.authManager.CertFingerprint()),
						)
					}
				}
			}

			// TODO: Handle certificate renewal (re-enroll before expiry)
			// TODO: Handle other connection settings (endpoint changes, headers, etc.)
			return nil
		},
	}

	// Build headers with authentication
	headers, err := s.buildAuthHeaders()
	if err != nil {
		s.opampServer.Stop(ctx)
		return fmt.Errorf("failed to build auth headers: %w", err)
	}

	opampClient, err := opamp.NewClient(s.logger, opamp.ClientConfig{
		Endpoint:    s.cfg.Server.Endpoint,
		InstanceUID: s.instanceUID,
		Headers:     headers,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: s.cfg.Auth.InsecureTLS,
		},
		Capabilities: opamp.Capabilities{
			ReportsStatus:                  true,
			AcceptsRemoteConfig:            true,
			ReportsEffectiveConfig:         true,
			ReportsHealth:                  true,
			AcceptsOpAMPConnectionSettings: true,
			AcceptsRestartCommand:          true,
		},
	}, clientCallbacks)
	if err != nil {
		s.opampServer.Stop(ctx)
		return err
	}
	s.opampClient = opampClient

	// Set initial agent description before starting
	if err := s.opampClient.SetAgentDescription(s.createAgentDescription()); err != nil {
		s.opampServer.Stop(ctx)
		return err
	}

	// Set initial health status before starting
	if err := s.opampClient.SetHealth(&protobufs.ComponentHealth{
		Healthy: true,
	}); err != nil {
		s.opampServer.Stop(ctx)
		return err
	}

	// Start OpAMP client
	if err := s.opampClient.Start(ctx); err != nil {
		s.opampServer.Stop(ctx)
		return err
	}

	// Start health monitoring with a cancellable context
	healthCtx, healthCancel := context.WithCancel(ctx)
	s.healthCancel = healthCancel
	healthUpdates := s.healthMonitor.StartPolling(healthCtx)
	go func() {
		for status := range healthUpdates {
			// TODO: Pass commander as AgentStateProvider once it implements the interface
			// This will enable accurate agent start time reporting per OpAMP spec
			if err := s.opampClient.SetHealth(status.ToComponentHealth(nil)); err != nil {
				s.logger.Warn("Failed to report health", zap.Error(err))
			}
		}
	}()

	// Start the collector agent
	if err := s.commander.Start(ctx); err != nil {
		s.opampClient.Stop(ctx)
		s.opampServer.Stop(ctx)
		return err
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
