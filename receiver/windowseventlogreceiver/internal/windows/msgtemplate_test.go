// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertWindowsTemplate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "User %1 logged on from %2", `User {{eventParam . 0}} logged on from {{eventParam . 1}}`},
		{"with format spec", "Size: %1!d! bytes", `Size: {{eventParam . 0}} bytes`},
		{"no params", "System started", "System started"},
		{"adjacent", "%1%2", `{{eventParam . 0}}{{eventParam . 1}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, convertWindowsTemplate(tt.input))
		})
	}
}

func TestExecuteTemplate(t *testing.T) {
	tmpl, err := compileTemplate("test", "User %1 logged on from %2")
	require.NoError(t, err)

	result, err := executeTemplate(tmpl, []string{"admin", "10.0.0.1"})
	require.NoError(t, err)
	require.Equal(t, "User admin logged on from 10.0.0.1", result)
}

func TestExecuteTemplate_MissingParam(t *testing.T) {
	tmpl, err := compileTemplate("test", "User %1 action %2 on %3")
	require.NoError(t, err)

	// Only 1 param provided, %2 and %3 should be preserved as placeholders
	result, err := executeTemplate(tmpl, []string{"admin"})
	require.NoError(t, err)
	require.Equal(t, "User admin action %2 on %3", result)
}

func TestExecuteTemplate_NoParams(t *testing.T) {
	tmpl, err := compileTemplate("test", "System restarted")
	require.NoError(t, err)

	result, err := executeTemplate(tmpl, nil)
	require.NoError(t, err)
	require.Equal(t, "System restarted", result)
}

func TestExtractEventParams_EventData(t *testing.T) {
	e := &EventXML{
		EventData: EventData{
			Data: []Data{
				{Name: "User", Value: "admin"},
				{Name: "IP", Value: "10.0.0.1"},
			},
		},
	}
	params := extractEventParams(e)
	require.Equal(t, []string{"admin", "10.0.0.1"}, params)
}

func TestExtractEventParams_UserData(t *testing.T) {
	e := &EventXML{
		UserData: &UserData{
			Data: []UserDataEntry{
				{Value: "S-1-5-21-123"},
				{Value: "testuser"},
			},
		},
	}
	params := extractEventParams(e)
	require.Equal(t, []string{"S-1-5-21-123", "testuser"}, params)
}

func TestExtractEventParams_Empty(t *testing.T) {
	e := &EventXML{}
	params := extractEventParams(e)
	require.Nil(t, params)
}

func TestExtractEventParams_EventDataPreferred(t *testing.T) {
	// When both EventData and UserData are present, EventData wins
	e := &EventXML{
		EventData: EventData{
			Data: []Data{{Name: "A", Value: "from_event"}},
		},
		UserData: &UserData{
			Data: []UserDataEntry{{Value: "from_user"}},
		},
	}
	params := extractEventParams(e)
	require.Equal(t, []string{"from_event"}, params)
}

func TestTemplateCacheLoaded(t *testing.T) {
	cache := newTemplateCache()
	require.False(t, cache.loaded)
	cache.loaded = true
	require.True(t, cache.loaded)
}

func TestTemplateCache(t *testing.T) {
	cache := newTemplateCache()

	// Miss returns false
	_, ok := cache.get(1000, 0)
	require.False(t, ok)

	// Store and retrieve
	tmpl, err := compileTemplate("test", "Hello %1")
	require.NoError(t, err)
	cache.put(1000, 0, tmpl)

	got, ok := cache.get(1000, 0)
	require.True(t, ok)
	result, err := executeTemplate(got, []string{"world"})
	require.NoError(t, err)
	require.Equal(t, "Hello world", result)

	// Different version is a different key
	_, ok = cache.get(1000, 1)
	require.False(t, ok)

	// Different event ID is a different key
	_, ok = cache.get(1001, 0)
	require.False(t, ok)
}
