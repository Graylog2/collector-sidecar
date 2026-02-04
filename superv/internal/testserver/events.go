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

import "time"

// EventKind identifies the type of event.
type EventKind string

const (
	EventAgentConnect       EventKind = "agent.connect"
	EventAgentDisconnect    EventKind = "agent.disconnect"
	EventAgentMessage       EventKind = "agent.message"
	EventHealth             EventKind = "health"
	EventConfigStatus       EventKind = "config.status"
	EventEffectiveConfig    EventKind = "config.effective"
	EventCSRReceived        EventKind = "csr.received"
	EventCertIssued         EventKind = "cert.issued"
	EventAgentDescription   EventKind = "agent.description"
	EventPackageStatus      EventKind = "package.status"
	EventCustomCapabilities EventKind = "capabilities.custom"
)

// Event represents something that happened on the server.
// The Data field type depends on Kind:
//   - EventHealth -> *protobufs.ComponentHealth
//   - EventAgentDescription -> *protobufs.AgentDescription
//   - EventConfigStatus -> *protobufs.RemoteConfigStatus
//   - EventEffectiveConfig -> *protobufs.EffectiveConfig
//   - EventPackageStatus -> *protobufs.PackageStatuses
//   - EventCustomCapabilities -> *protobufs.CustomCapabilities
//   - EventAgentMessage -> *protobufs.AgentToServer
//   - EventCSRReceived -> *x509.CertificateRequest
//   - EventCertIssued -> *x509.Certificate
//   - EventAgentConnect, EventAgentDisconnect -> nil
type Event struct {
	Kind      EventKind
	Timestamp time.Time
	AgentID   string
	Data      any
}
