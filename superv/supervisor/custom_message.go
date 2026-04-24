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

	"github.com/open-telemetry/opamp-go/protobufs"
	"go.uber.org/zap"
)

// forwardCustomMessage forwards a custom message from the upstream server to the local collector.
func (s *Supervisor) forwardCustomMessage(ctx context.Context, customMessage *protobufs.CustomMessage) {
	s.logger.Debug("Received custom message",
		zap.String("capability", customMessage.GetCapability()),
		zap.String("type", customMessage.GetType()),
	)

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
