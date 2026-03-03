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

package opamp

import (
	"context"

	"github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
)

// Callbacks handles OpAMP client callbacks.
type Callbacks struct {
	OnConnect                 func(ctx context.Context)
	OnConnectFailed           func(ctx context.Context, err error)
	OnError                   func(ctx context.Context, err *protobufs.ServerErrorResponse)
	OnRemoteConfig            func(ctx context.Context, config *protobufs.AgentRemoteConfig) bool
	OnOpampConnectionSettings func(ctx context.Context, settings *protobufs.OpAMPConnectionSettings) error
	OnPackagesAvailable       func(ctx context.Context, packages *protobufs.PackagesAvailable) bool
	OnCommand                 func(ctx context.Context, command *protobufs.ServerToAgentCommand) error
	OnCustomMessage           func(ctx context.Context, customMessage *protobufs.CustomMessage)
	OnOwnLogs                 func(ctx context.Context, settings *protobufs.TelemetryConnectionSettings)
	SaveRemoteConfigStatus    func(ctx context.Context, status *protobufs.RemoteConfigStatus)
	GetEffectiveConfig        func(ctx context.Context) (*protobufs.EffectiveConfig, error)
}

// ToTypesCallbacks converts our Callbacks to opamp-go types.Callbacks.
func (c *Callbacks) ToTypesCallbacks() types.Callbacks {
	return types.Callbacks{
		OnConnect:       c.onConnect,
		OnConnectFailed: c.onConnectFailed,
		OnError:         c.onError,
		OnMessage:       c.onMessage,
		OnOpampConnectionSettings: func(ctx context.Context, settings *protobufs.OpAMPConnectionSettings) error {
			if c.OnOpampConnectionSettings != nil {
				return c.OnOpampConnectionSettings(ctx, settings)
			}
			return nil
		},
		OnCommand: func(ctx context.Context, command *protobufs.ServerToAgentCommand) error {
			if c.OnCommand != nil {
				return c.OnCommand(ctx, command)
			}
			return nil
		},
		SaveRemoteConfigStatus: func(ctx context.Context, status *protobufs.RemoteConfigStatus) {
			if c.SaveRemoteConfigStatus != nil {
				c.SaveRemoteConfigStatus(ctx, status)
			}
		},
		GetEffectiveConfig: func(ctx context.Context) (*protobufs.EffectiveConfig, error) {
			if c.GetEffectiveConfig != nil {
				return c.GetEffectiveConfig(ctx)
			}
			return nil, nil
		},
	}
}

func (c *Callbacks) onConnect(ctx context.Context) {
	if c.OnConnect != nil {
		c.OnConnect(ctx)
	}
}

func (c *Callbacks) onConnectFailed(ctx context.Context, err error) {
	if c.OnConnectFailed != nil {
		c.OnConnectFailed(ctx, err)
	}
}

func (c *Callbacks) onError(ctx context.Context, err *protobufs.ServerErrorResponse) {
	if c.OnError != nil {
		c.OnError(ctx, err)
	}
}

func (c *Callbacks) onMessage(ctx context.Context, msg *types.MessageData) {
	// Handle remote config
	if msg.RemoteConfig != nil && c.OnRemoteConfig != nil {
		c.OnRemoteConfig(ctx, msg.RemoteConfig)
	}

	// Handle packages
	if msg.PackagesAvailable != nil && c.OnPackagesAvailable != nil {
		c.OnPackagesAvailable(ctx, msg.PackagesAvailable)
	}

	// Handle custom messages
	if msg.CustomMessage != nil && c.OnCustomMessage != nil {
		c.OnCustomMessage(ctx, msg.CustomMessage)
	}

	// Handle own logs connection settings
	if msg.OwnLogsConnSettings != nil && c.OnOwnLogs != nil {
		c.OnOwnLogs(ctx, msg.OwnLogsConnSettings)
	}
}
