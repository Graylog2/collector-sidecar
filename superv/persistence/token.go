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

package persistence

import (
	"os"
	"path/filepath"
	"time"

	"github.com/goccy/go-yaml"
)

const agentTokenFile = "agent_token.yaml"

// AgentToken represents the persisted agent authentication token.
type AgentToken struct {
	Token      string    `yaml:"token"`
	ReceivedAt time.Time `yaml:"received_at"`
	ExpiresAt  time.Time `yaml:"expires_at"`
}

// SaveAgentToken saves the agent token to disk with secure permissions (0600).
func SaveAgentToken(authDir string, token *AgentToken) error {
	if err := os.MkdirAll(authDir, 0700); err != nil {
		return err
	}

	content, err := yaml.Marshal(token)
	if err != nil {
		return err
	}

	filePath := filepath.Join(authDir, agentTokenFile)
	return os.WriteFile(filePath, content, 0600)
}

// LoadAgentToken loads the agent token from disk.
func LoadAgentToken(authDir string) (*AgentToken, error) {
	filePath := filepath.Join(authDir, agentTokenFile)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var token AgentToken
	if err := yaml.Unmarshal(content, &token); err != nil {
		return nil, err
	}

	return &token, nil
}

// IsExpired returns true if the token has expired.
func (t *AgentToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsExpiringSoon returns true if the token will expire within the given duration.
func (t *AgentToken) IsExpiringSoon(threshold time.Duration) bool {
	return time.Now().Add(threshold).After(t.ExpiresAt)
}

// DeleteAgentToken removes the agent token file.
func DeleteAgentToken(authDir string) error {
	filePath := filepath.Join(authDir, agentTokenFile)
	err := os.Remove(filePath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
