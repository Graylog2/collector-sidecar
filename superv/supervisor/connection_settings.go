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
	"fmt"
	"time"

	"github.com/Graylog2/collector-sidecar/superv/persistence"
	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

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
