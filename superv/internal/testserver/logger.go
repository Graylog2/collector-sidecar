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

package testserver

import (
	"crypto/x509"
	"errors"
	"sync"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ErrTimeout is returned when WaitFor times out.
var ErrTimeout = errors.New("timeout waiting for event")

// Logger receives events from the server.
type Logger interface {
	Log(event Event)
}

// EventRecorder extends Logger with query and wait capabilities.
type EventRecorder interface {
	Logger
	Events() []Event
	EventsFor(agentID string) []Event
	Clear()
	WaitFor(predicate func(Event) bool, timeout time.Duration) (Event, error)
	WaitForKind(kind EventKind, timeout time.Duration) (Event, error)
}

// TestRecorder implements EventRecorder for use in tests.
type TestRecorder struct {
	mu     sync.Mutex
	events []Event
	cond   *sync.Cond
	Inner  Logger // Optional: forward events to another logger
}

// NewTestRecorder creates a new TestRecorder.
func NewTestRecorder() *TestRecorder {
	r := &TestRecorder{}
	r.cond = sync.NewCond(&r.mu)
	return r
}

// Log records an event and wakes up any waiters.
func (r *TestRecorder) Log(event Event) {
	if r.Inner != nil {
		r.Inner.Log(event)
	}

	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
	r.cond.Broadcast()
}

// Events returns a copy of all recorded events.
func (r *TestRecorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Event, len(r.events))
	copy(result, r.events)
	return result
}

// EventsFor returns events for a specific agent.
func (r *TestRecorder) EventsFor(agentID string) []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []Event
	for _, e := range r.events {
		if e.AgentID == agentID {
			result = append(result, e)
		}
	}
	return result
}

// Clear removes all recorded events.
func (r *TestRecorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
}

// WaitFor blocks until an event matching the predicate occurs or timeout.
func (r *TestRecorder) WaitFor(predicate func(Event) bool, timeout time.Duration) (Event, error) {
	deadline := time.Now().Add(timeout)

	r.mu.Lock()
	defer r.mu.Unlock()

	for {
		// Check existing events
		for _, e := range r.events {
			if predicate(e) {
				return e, nil
			}
		}

		// Check timeout
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return Event{}, ErrTimeout
		}

		// Wait for new events with timeout
		done := make(chan struct{})
		go func() {
			time.Sleep(remaining)
			r.cond.Broadcast()
			close(done)
		}()

		r.cond.Wait()

		select {
		case <-done:
			// Timeout occurred
		default:
			// New event arrived
		}
	}
}

// WaitForKind is a convenience wrapper for WaitFor.
func (r *TestRecorder) WaitForKind(kind EventKind, timeout time.Duration) (Event, error) {
	return r.WaitFor(func(e Event) bool {
		return e.Kind == kind
	}, timeout)
}

// Verbosity controls how much detail DebugLogger outputs.
type Verbosity int

const (
	// VerbosityDefault logs connect/disconnect, health, config status.
	VerbosityDefault Verbosity = 0
	// VerbosityDetailed adds description, effective config, packages, capabilities.
	VerbosityDetailed Verbosity = 1
	// VerbosityFull adds complete protobuf dump for every message.
	VerbosityFull Verbosity = 2
)

// DebugLogger logs events using zap.
type DebugLogger struct {
	logger    *zap.Logger
	verbosity Verbosity
}

// NewDebugLogger creates a new DebugLogger.
func NewDebugLogger(verbosity Verbosity, json bool) *DebugLogger {
	var logger *zap.Logger
	var err error

	if json {
		cfg := zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		logger, err = cfg.Build()
	} else {
		cfg := zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		logger, err = cfg.Build()
	}

	if err != nil {
		// Fallback to nop logger if config fails
		logger = zap.NewNop()
	}

	return &DebugLogger{
		logger:    logger,
		verbosity: verbosity,
	}
}

// Log logs an event if it matches the verbosity level.
func (l *DebugLogger) Log(event Event) {
	if !l.shouldLog(event.Kind) {
		return
	}

	fields := []zap.Field{
		zap.String("agent", event.AgentID),
	}

	// Add formatted data fields based on event kind
	fields = append(fields, l.formatEventData(event)...)

	l.logger.Info(string(event.Kind), fields...)
}

// formatEventData returns zap fields for the event data based on kind.
func (l *DebugLogger) formatEventData(event Event) []zap.Field {
	if event.Data == nil {
		return nil
	}

	switch event.Kind {
	case EventHealth:
		if health, ok := event.Data.(*protobufs.ComponentHealth); ok {
			fields := []zap.Field{zap.Bool("healthy", health.Healthy)}
			if health.LastError != "" {
				fields = append(fields, zap.String("error", health.LastError))
			}
			if health.StartTimeUnixNano > 0 {
				fields = append(fields, zap.Time("start_time", time.Unix(0, int64(health.StartTimeUnixNano))))
			}
			return fields
		}

	case EventConfigStatus:
		if status, ok := event.Data.(*protobufs.RemoteConfigStatus); ok {
			statusStr := formatConfigStatus(status.Status)
			fields := []zap.Field{zap.String("status", statusStr)}
			if status.ErrorMessage != "" {
				fields = append(fields, zap.String("error", status.ErrorMessage))
			}
			return fields
		}

	case EventAgentDescription:
		if desc, ok := event.Data.(*protobufs.AgentDescription); ok {
			fields := []zap.Field{}
			for _, attr := range desc.IdentifyingAttributes {
				fields = append(fields, zap.Any(attr.Key, formatAnyValue(attr.Value)))
			}
			return fields
		}

	case EventEffectiveConfig:
		if cfg, ok := event.Data.(*protobufs.EffectiveConfig); ok {
			if cfg.ConfigMap != nil && cfg.ConfigMap.ConfigMap != nil {
				for name, file := range cfg.ConfigMap.ConfigMap {
					key := "config"
					if name != "" {
						key = name
					}
					return []zap.Field{zap.Int(key+"_bytes", len(file.Body))}
				}
			}
			return nil
		}

	case EventCSRReceived:
		if csr, ok := event.Data.(*x509.CertificateRequest); ok {
			return []zap.Field{
				zap.String("cn", csr.Subject.CommonName),
				zap.Strings("org", csr.Subject.Organization),
			}
		}

	case EventCertIssued:
		if cert, ok := event.Data.(*x509.Certificate); ok {
			return []zap.Field{
				zap.String("cn", cert.Subject.CommonName),
				zap.Time("not_after", cert.NotAfter),
			}
		}

	case EventPackageStatus:
		if pkgs, ok := event.Data.(*protobufs.PackageStatuses); ok {
			return []zap.Field{
				zap.Int("packages", len(pkgs.Packages)),
				zap.String("error", pkgs.ErrorMessage),
			}
		}

	case EventCustomCapabilities:
		if caps, ok := event.Data.(*protobufs.CustomCapabilities); ok {
			return []zap.Field{zap.Strings("capabilities", caps.Capabilities)}
		}
	}

	// Fallback: log raw data for unknown types or full verbosity
	return []zap.Field{zap.Any("data", event.Data)}
}

// formatConfigStatus converts RemoteConfigStatuses to a readable string.
func formatConfigStatus(status protobufs.RemoteConfigStatuses) string {
	switch status {
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_UNSET:
		return "unset"
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED:
		return "applied"
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLYING:
		return "applying"
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED:
		return "failed"
	default:
		return "unknown"
	}
}

// formatAnyValue extracts the value from a protobufs.AnyValue.
func formatAnyValue(val *protobufs.AnyValue) any {
	if val == nil {
		return nil
	}
	switch v := val.Value.(type) {
	case *protobufs.AnyValue_StringValue:
		return v.StringValue
	case *protobufs.AnyValue_IntValue:
		return v.IntValue
	case *protobufs.AnyValue_DoubleValue:
		return v.DoubleValue
	case *protobufs.AnyValue_BoolValue:
		return v.BoolValue
	case *protobufs.AnyValue_BytesValue:
		return v.BytesValue
	default:
		return val
	}
}

// shouldLog returns true if the event kind should be logged at the current verbosity.
func (l *DebugLogger) shouldLog(kind EventKind) bool {
	switch kind {
	case EventAgentConnect, EventAgentDisconnect, EventHealth, EventConfigStatus:
		return true // Always log at default verbosity
	case EventAgentDescription, EventEffectiveConfig, EventPackageStatus,
		EventCustomCapabilities, EventCSRReceived, EventCertIssued:
		return l.verbosity >= VerbosityDetailed
	case EventAgentMessage:
		return l.verbosity >= VerbosityFull
	default:
		return true
	}
}

// Sync flushes any buffered log entries.
func (l *DebugLogger) Sync() error {
	return l.logger.Sync()
}
