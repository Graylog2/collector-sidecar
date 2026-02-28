// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"text/template"
)

// windowsParamPattern matches %N or %N!format! in Windows message templates.
var windowsParamPattern = regexp.MustCompile(`%(\d+)(?:![^!]*!)?`)

// convertWindowsTemplate converts a Windows message template string to Go
// text/template syntax. %1 becomes {{eventParam . 0}}, %2 becomes
// {{eventParam . 1}}, etc. Format specifiers like %1!d! are stripped.
func convertWindowsTemplate(s string) string {
	return windowsParamPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract the digit run directly from the match string.
		// Match is guaranteed to start with % followed by digits.
		end := 1
		for end < len(match) && match[end] >= '0' && match[end] <= '9' {
			end++
		}
		n, err := strconv.Atoi(match[1:end])
		if err != nil || n < 1 {
			return match
		}
		return fmt.Sprintf("{{eventParam . %d}}", n-1) // 1-based to 0-based
	})
}

// templateFuncs provides the eventParam helper.
var templateFuncs = template.FuncMap{
	"eventParam": func(params []string, index int) string {
		if index >= 0 && index < len(params) {
			return params[index]
		}
		// Preserve original placeholder for missing params
		return fmt.Sprintf("%%%d", index+1)
	},
}

// compileTemplate converts a Windows message template to a compiled Go template.
func compileTemplate(name, windowsTemplate string) (*template.Template, error) {
	goTemplate := convertWindowsTemplate(windowsTemplate)
	return template.New(name).Funcs(templateFuncs).Parse(goTemplate)
}

// executeTemplate runs a compiled template with the given parameter values.
func executeTemplate(tmpl *template.Template, params []string) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// templateKey identifies a message template by event ID and version.
type templateKey struct {
	eventID uint32
	version uint8
}

// templateCache stores compiled message templates per provider.
// The cache is unbounded but the number of distinct (eventID, version) pairs
// per provider is limited by the provider's manifest, typically < 1000.
type templateCache struct {
	templates map[templateKey]*template.Template
	loaded    bool // true after loadTemplates has been attempted
}

func newTemplateCache() *templateCache {
	return &templateCache{
		templates: make(map[templateKey]*template.Template),
	}
}

func (c *templateCache) get(eventID uint32, version uint8) (*template.Template, bool) {
	t, ok := c.templates[templateKey{eventID, version}]
	return t, ok
}

func (c *templateCache) put(eventID uint32, version uint8, tmpl *template.Template) {
	c.templates[templateKey{eventID, version}] = tmpl
}

// extractEventParams collects the positional parameter values from an event's
// EventData or UserData, in order. These are used as %1, %2, ... substitutions
// in message templates.
func extractEventParams(e *EventXML) []string {
	if len(e.EventData.Data) > 0 {
		params := make([]string, len(e.EventData.Data))
		for i, d := range e.EventData.Data {
			params[i] = d.Value
		}
		return params
	}
	if e.UserData != nil && len(e.UserData.Data) > 0 {
		params := make([]string, len(e.UserData.Data))
		for i, d := range e.UserData.Data {
			params[i] = d.Value
		}
		return params
	}
	return nil
}
