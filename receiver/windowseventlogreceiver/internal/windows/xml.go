// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/entry"
)

// EventXML is the rendered xml of an event.
type EventXML struct {
	Original                string       `xml:"-"`
	EventID                 EventID      `xml:"System>EventID"`
	Provider                Provider     `xml:"System>Provider"`
	Computer                string       `xml:"System>Computer"`
	Channel                 string       `xml:"System>Channel"`
	RecordID                uint64       `xml:"System>EventRecordID"`
	TimeCreated             TimeCreated  `xml:"System>TimeCreated"`
	Message                 string       `xml:"RenderingInfo>Message"`
	RenderedLevel           string       `xml:"RenderingInfo>Level"`
	Level                   string       `xml:"System>Level"`
	RenderedTask            string       `xml:"RenderingInfo>Task"`
	Task                    string       `xml:"System>Task"`
	RenderedOpcode          string       `xml:"RenderingInfo>Opcode"`
	Opcode                  string       `xml:"System>Opcode"`
	RenderedKeywords        []string     `xml:"RenderingInfo>Keywords>Keyword"`
	Keywords                []string     `xml:"System>Keywords"`
	Security                *Security    `xml:"System>Security"`
	Execution               *Execution   `xml:"System>Execution"`
	EventData               EventData    `xml:"EventData"`
	UserData                *UserData    `xml:"UserData"`
	Correlation             *Correlation `xml:"System>Correlation"`
	Version                 uint8        `xml:"System>Version"`
	RenderErrorCode         uint32       `xml:"ProcessingErrorData>ErrorCode"`
	RenderErrorDataItemName string       `xml:"ProcessingErrorData>DataItemName"`
}

// parseTimestamp will parse the timestamp of the event. If parsing fails,
// it returns time.Now() and a non-nil error so the caller can log a warning.
func parseTimestamp(ts string) (time.Time, error) {
	if timestamp, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return timestamp, nil
	}
	return time.Now(), fmt.Errorf("invalid timestamp %q, using current time", ts)
}

// parseSeverity will parse the severity of the event, preferring the
// numeric Level (locale-independent) over the rendered string.
func parseSeverity(renderedLevel, level string) entry.Severity {
	// Prefer numeric level (locale-independent)
	switch level {
	case "1":
		return entry.Fatal
	case "2":
		return entry.Error
	case "3":
		return entry.Warn
	case "4":
		return entry.Info
	}
	// Fall back to rendered string (English)
	switch renderedLevel {
	case "Critical":
		return entry.Fatal
	case "Error":
		return entry.Error
	case "Warning":
		return entry.Warn
	case "Information":
		return entry.Info
	}
	return entry.Default
}

const (
	keywordAuditFailure = 0x10000000000000
	keywordAuditSuccess = 0x20000000000000
)

// parseKeywordBits extracts the raw keyword bitmask from the Keywords field.
// Windows stores it as a hex string like "0x8020000000000000".
func parseKeywordBits(keywords []string) uint64 {
	for _, kw := range keywords {
		if strings.HasPrefix(kw, "0x") || strings.HasPrefix(kw, "0X") {
			if v, err := strconv.ParseUint(kw[2:], 16, 64); err == nil {
				return v
			}
		}
	}
	return 0
}

// formattedBody will parse a body from the event.
func formattedBody(e *EventXML) map[string]any {
	level := e.RenderedLevel
	if level == "" {
		level = e.Level
	}

	task := e.RenderedTask
	if task == "" {
		task = e.Task
	}

	opcode := e.RenderedOpcode
	if opcode == "" {
		opcode = e.Opcode
	}

	keywords := e.RenderedKeywords
	if keywords == nil {
		keywords = e.Keywords
	}

	body := map[string]any{
		"event_id": map[string]any{
			"qualifiers": e.EventID.Qualifiers,
			"id":         e.EventID.ID,
		},
		"provider": providerMap(e.Provider),
		"system_time": e.TimeCreated.SystemTime,
		"computer":    e.Computer,
		"channel":     e.Channel,
		"record_id":   e.RecordID,
		"level":       level,
		"message":     e.Message,
		"task":        task,
		"opcode":      opcode,
		"keywords":    keywords,
		"event_data":  parseEventData(e.EventData),
		"version":     e.Version,
	}

	if e.Security != nil && e.Security.UserID != "" {
		body["security"] = map[string]any{
			"user_id": e.Security.UserID,
		}
	}

	if e.Execution != nil {
		body["execution"] = e.Execution.asMap()
	}

	if e.Correlation != nil {
		if cm := e.Correlation.asMap(); len(cm) > 0 {
			body["correlation"] = cm
		}
	}

	if e.UserData != nil && len(e.UserData.Data) > 0 {
		ud := map[string]any{}
		ud["xml_name"] = e.UserData.Name.Local
		for _, d := range e.UserData.Data {
			if d.Value == "" {
				continue
			}
			key := d.XMLName.Local
			if _, exists := ud[key]; !exists {
				ud[key] = d.Value
			}
		}
		body["user_data"] = ud
	}

	if e.RenderErrorCode != 0 || e.RenderErrorDataItemName != "" {
		errMap := map[string]any{}
		if e.RenderErrorCode != 0 {
			errMap["code"] = e.RenderErrorCode
		}
		if e.RenderErrorDataItemName != "" {
			errMap["data_item_name"] = e.RenderErrorDataItemName
		}
		body["error"] = errMap
	}

	kwBits := parseKeywordBits(e.Keywords)
	if kwBits&keywordAuditFailure != 0 {
		body["outcome"] = "failure"
	} else if kwBits&keywordAuditSuccess != 0 {
		body["outcome"] = "success"
	}

	return body
}

// parseEventData converts EventData to a flat map, matching winlogbeat's behavior:
// - Named entries become direct key-value pairs (first occurrence wins for duplicates)
// - Unnamed entries get synthetic keys "param1", "param2", etc. (1-based position index)
// - Empty values are dropped
// - <Binary> is stored as "Binary" in the same map
// - The <EventData Name="..."> attribute is not preserved
//
// see: https://learn.microsoft.com/en-us/windows/win32/wes/eventschema-datafieldtype-complextype
func parseEventData(eventData EventData) map[string]any {
	outputMap := make(map[string]any, len(eventData.Data)+1)

	for i, data := range eventData.Data {
		if data.Value == "" {
			continue
		}
		key := data.Name
		if key == "" {
			key = fmt.Sprintf("param%d", i+1)
		}
		if _, exists := outputMap[key]; !exists {
			outputMap[key] = data.Value
		}
	}

	if eventData.Binary != "" {
		if _, exists := outputMap["Binary"]; !exists {
			outputMap["Binary"] = eventData.Binary
		}
	}

	return outputMap
}

// UserDataEntry represents a child element under the UserData wrapper.
// Unlike EventData where Name is an attribute, here the XML element name is the key.
type UserDataEntry struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

// UserData represents the <UserData> element which contains a single
// provider-specific child element with key-value data fields.
type UserData struct {
	Name xml.Name        // The child element's name (e.g. LogFileCleared)
	Data []UserDataEntry // Parsed child elements as key-value pairs
}

func (u *UserData) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			u.Name = t.Name
			var inner struct {
				Data []UserDataEntry `xml:",any"`
			}
			if err := d.DecodeElement(&inner, &t); err != nil {
				return err
			}
			u.Data = inner.Data
		case xml.EndElement:
			return nil
		}
	}
}

// EventID is the identifier of the event.
type EventID struct {
	Qualifiers uint16 `xml:"Qualifiers,attr"`
	ID         uint32 `xml:",chardata"`
}

// TimeCreated is the creation time of the event.
type TimeCreated struct {
	SystemTime string `xml:"SystemTime,attr"`
}

// Provider is the provider of the event.
type Provider struct {
	Name            string `xml:"Name,attr"`
	GUID            string `xml:"Guid,attr"`
	EventSourceName string `xml:"EventSourceName,attr"`
}

func providerMap(p Provider) map[string]any {
	m := map[string]any{
		"name": p.Name,
	}
	if p.GUID != "" {
		m["guid"] = p.GUID
	}
	if p.EventSourceName != "" {
		m["event_source"] = p.EventSourceName
	}
	return m
}

type EventData struct {
	// https://learn.microsoft.com/en-us/windows/win32/wes/eventschema-eventdatatype-complextype
	// ComplexData is not supported.
	Name   string `xml:"Name,attr"`
	Data   []Data `xml:"Data"`
	Binary string `xml:"Binary"`
}

type Data struct {
	// https://learn.microsoft.com/en-us/windows/win32/wes/eventschema-datafieldtype-complextype
	Name  string `xml:"Name,attr"`
	Value string `xml:",chardata"`
}

// Security contains info pertaining to the user triggering the event.
type Security struct {
	UserID string `xml:"UserID,attr"`
}

// Execution contains info pertaining to the process that triggered the event.
type Execution struct {
	// ProcessID and ThreadID are required on execution info
	ProcessID uint `xml:"ProcessID,attr"`
	ThreadID  uint `xml:"ThreadID,attr"`
	// These remaining fields are all optional for execution info
	ProcessorID   *uint `xml:"ProcessorID,attr"`
	SessionID     *uint `xml:"SessionID,attr"`
	KernelTime    *uint `xml:"KernelTime,attr"`
	UserTime      *uint `xml:"UserTime,attr"`
	ProcessorTime *uint `xml:"ProcessorTime,attr"`
}

func (e Execution) asMap() map[string]any {
	result := map[string]any{
		"process_id": e.ProcessID,
		"thread_id":  e.ThreadID,
	}

	if e.ProcessorID != nil {
		result["processor_id"] = *e.ProcessorID
	}

	if e.SessionID != nil {
		result["session_id"] = *e.SessionID
	}

	if e.KernelTime != nil {
		result["kernel_time"] = *e.KernelTime
	}

	if e.UserTime != nil {
		result["user_time"] = *e.UserTime
	}

	if e.ProcessorTime != nil {
		result["processor_time"] = *e.ProcessorTime
	}

	return result
}

// Correlation contains the activity identifiers that consumers can use to group related events together.
type Correlation struct {
	// ActivityID and RelatedActivityID are optional fields
	// https://learn.microsoft.com/en-us/windows/win32/wes/eventschema-correlation-systempropertiestype-element
	ActivityID        *string `xml:"ActivityID,attr"`
	RelatedActivityID *string `xml:"RelatedActivityID,attr"`
}

func (e Correlation) asMap() map[string]any {
	result := map[string]any{}

	if e.ActivityID != nil {
		result["activity_id"] = *e.ActivityID
	}

	if e.RelatedActivityID != nil {
		result["related_activity_id"] = *e.RelatedActivityID
	}

	return result
}

// sanitizeXML strips characters that are invalid in XML 1.0.
// Valid XML 1.0 chars: #x9 | #xA | #xD | [#x20-#xD7FF] | [#xE000-#xFFFD] | [#x10000-#x10FFFF]
func sanitizeXML(data []byte) []byte {
	return bytes.Map(func(r rune) rune {
		if r == 0x09 || r == 0x0A || r == 0x0D ||
			(r >= 0x20 && r <= 0xD7FF) ||
			(r >= 0xE000 && r <= 0xFFFD) ||
			(r >= 0x10000 && r <= 0x10FFFF) {
			return r
		}
		return -1 // drop invalid character
	}, data)
}

// unmarshalEventXML will unmarshal EventXML from xml bytes.
func unmarshalEventXML(data []byte) (*EventXML, error) {
	sanitized := sanitizeXML(data)
	var eventXML EventXML
	if err := xml.Unmarshal(sanitized, &eventXML); err != nil {
		return nil, fmt.Errorf("failed to unmarshal xml bytes into event: %w (%s)", err, string(data))
	}
	eventXML.Original = string(data) // raw bytes, not sanitized
	return &eventXML, nil
}
