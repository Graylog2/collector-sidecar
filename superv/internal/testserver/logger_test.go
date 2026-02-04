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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestRecorder_Log(t *testing.T) {
	r := NewTestRecorder()

	r.Log(Event{Kind: EventAgentConnect, AgentID: "agent-1"})
	r.Log(Event{Kind: EventHealth, AgentID: "agent-1"})

	events := r.Events()
	require.Len(t, events, 2)
	assert.Equal(t, EventAgentConnect, events[0].Kind)
	assert.Equal(t, EventHealth, events[1].Kind)
}

func TestTestRecorder_EventsFor(t *testing.T) {
	r := NewTestRecorder()

	r.Log(Event{Kind: EventAgentConnect, AgentID: "agent-1"})
	r.Log(Event{Kind: EventAgentConnect, AgentID: "agent-2"})
	r.Log(Event{Kind: EventHealth, AgentID: "agent-1"})

	events := r.EventsFor("agent-1")
	require.Len(t, events, 2)
	assert.Equal(t, "agent-1", events[0].AgentID)
	assert.Equal(t, "agent-1", events[1].AgentID)
}

func TestTestRecorder_Clear(t *testing.T) {
	r := NewTestRecorder()

	r.Log(Event{Kind: EventAgentConnect, AgentID: "agent-1"})
	require.Len(t, r.Events(), 1)

	r.Clear()
	assert.Empty(t, r.Events())
}

func TestTestRecorder_WaitForKind_AlreadyExists(t *testing.T) {
	r := NewTestRecorder()

	r.Log(Event{Kind: EventHealth, AgentID: "agent-1", Data: "test-data"})

	event, err := r.WaitForKind(EventHealth, 100*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, EventHealth, event.Kind)
	assert.Equal(t, "test-data", event.Data)
}

func TestTestRecorder_WaitForKind_ArrivesLater(t *testing.T) {
	r := NewTestRecorder()

	go func() {
		time.Sleep(50 * time.Millisecond)
		r.Log(Event{Kind: EventHealth, AgentID: "agent-1"})
	}()

	event, err := r.WaitForKind(EventHealth, 500*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, EventHealth, event.Kind)
}

func TestTestRecorder_WaitForKind_Timeout(t *testing.T) {
	r := NewTestRecorder()

	_, err := r.WaitForKind(EventHealth, 50*time.Millisecond)
	assert.ErrorIs(t, err, ErrTimeout)
}

func TestTestRecorder_WaitFor_CustomPredicate(t *testing.T) {
	r := NewTestRecorder()

	r.Log(Event{Kind: EventHealth, AgentID: "agent-1"})
	r.Log(Event{Kind: EventHealth, AgentID: "agent-2"})

	event, err := r.WaitFor(func(e Event) bool {
		return e.Kind == EventHealth && e.AgentID == "agent-2"
	}, 100*time.Millisecond)

	require.NoError(t, err)
	assert.Equal(t, "agent-2", event.AgentID)
}

func TestTestRecorder_Inner(t *testing.T) {
	inner := NewTestRecorder()
	outer := NewTestRecorder()
	outer.Inner = inner

	outer.Log(Event{Kind: EventHealth, AgentID: "agent-1"})

	assert.Len(t, outer.Events(), 1)
	assert.Len(t, inner.Events(), 1)
}

func TestDebugLogger_Verbosity(t *testing.T) {
	tests := []struct {
		name      string
		verbosity Verbosity
		kind      EventKind
		shouldLog bool
	}{
		{"default logs connect", VerbosityDefault, EventAgentConnect, true},
		{"default logs health", VerbosityDefault, EventHealth, true},
		{"default skips description", VerbosityDefault, EventAgentDescription, false},
		{"default skips message", VerbosityDefault, EventAgentMessage, false},
		{"detailed logs description", VerbosityDetailed, EventAgentDescription, true},
		{"detailed skips message", VerbosityDetailed, EventAgentMessage, false},
		{"full logs message", VerbosityFull, EventAgentMessage, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewDebugLogger(tt.verbosity, true)
			assert.Equal(t, tt.shouldLog, l.shouldLog(tt.kind))
		})
	}
}
