// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/entry"
)

func TestParseValidTimestamp(t *testing.T) {
	timestamp, err := parseTimestamp("2020-07-30T01:01:01.123456789Z")
	require.NoError(t, err)
	expected, _ := time.Parse(time.RFC3339Nano, "2020-07-30T01:01:01.123456789Z")
	require.Equal(t, expected, timestamp)
}

func TestParseInvalidTimestamp(t *testing.T) {
	timestamp, err := parseTimestamp("invalid")
	require.Error(t, err)
	require.WithinDuration(t, time.Now(), timestamp, time.Second)
}

func TestParseSeverity(t *testing.T) {
	require.Equal(t, entry.Fatal, parseSeverity("Critical", ""))
	require.Equal(t, entry.Error, parseSeverity("Error", ""))
	require.Equal(t, entry.Warn, parseSeverity("Warning", ""))
	require.Equal(t, entry.Info, parseSeverity("Information", ""))
	require.Equal(t, entry.Default, parseSeverity("Unknown", ""))
	require.Equal(t, entry.Fatal, parseSeverity("", "1"))
	require.Equal(t, entry.Error, parseSeverity("", "2"))
	require.Equal(t, entry.Warn, parseSeverity("", "3"))
	require.Equal(t, entry.Info, parseSeverity("", "4"))
	require.Equal(t, entry.Default, parseSeverity("", "0"))
}

func TestParseSeverity_NumericPreferred(t *testing.T) {
	// Numeric level should take priority over rendered string
	require.Equal(t, entry.Error, parseSeverity("Fehler", "2"))   // German "Error"
	require.Equal(t, entry.Warn, parseSeverity("Warnung", "3"))   // German "Warning"
	require.Equal(t, entry.Fatal, parseSeverity("Critique", "1")) // French "Critical"
	require.Equal(t, entry.Info, parseSeverity("情報", "4"))        // Japanese "Information"
}

func TestParseSeverity_FallbackToRendered(t *testing.T) {
	// When numeric level is empty or unrecognized, fall back to rendered string
	require.Equal(t, entry.Error, parseSeverity("Error", ""))
	require.Equal(t, entry.Warn, parseSeverity("Warning", "99"))
}

func TestParseBody(t *testing.T) {
	xml := &EventXML{
		EventID: EventID{
			ID:         1,
			Qualifiers: 2,
		},
		Provider: Provider{
			Name:            "provider",
			GUID:            "guid",
			EventSourceName: "event source",
		},
		TimeCreated: TimeCreated{
			SystemTime: "2020-07-30T01:01:01.123456789Z",
		},
		Computer: "computer",
		Channel:  "application",
		RecordID: 1,
		Level:    "Information",
		Message:  "message",
		Task:     "task",
		Opcode:   "opcode",
		Keywords: []string{"keyword"},
		EventData: EventData{
			Data: []Data{{Name: "1st_name", Value: "value"}, {Name: "2nd_name", Value: "another_value"}},
		},
		RenderedLevel:    "rendered_level",
		RenderedTask:     "rendered_task",
		RenderedOpcode:   "rendered_opcode",
		RenderedKeywords: []string{"RenderedKeywords"},
		Version:          0,
	}

	expected := map[string]any{
		"event_id": map[string]any{
			"id":         uint32(1),
			"qualifiers": uint16(2),
		},
		"provider": map[string]any{
			"name":         "provider",
			"guid":         "guid",
			"event_source": "event source",
		},
		"system_time": "2020-07-30T01:01:01.123456789Z",
		"computer":    "computer",
		"channel":     "application",
		"record_id":   uint64(1),
		"level":       "rendered_level",
		"message":     "message",
		"task":        "rendered_task",
		"opcode":      "rendered_opcode",
		"keywords":    []string{"RenderedKeywords"},
		"event_data": map[string]any{
			"1st_name": "value",
			"2nd_name": "another_value",
		},
		"version": uint8(0),
	}

	require.Equal(t, expected, formattedBody(xml))
}

func TestParseBodySecurityExecution(t *testing.T) {
	xml := &EventXML{
		EventID: EventID{
			ID:         1,
			Qualifiers: 2,
		},
		Provider: Provider{
			Name:            "provider",
			GUID:            "guid",
			EventSourceName: "event source",
		},
		TimeCreated: TimeCreated{
			SystemTime: "2020-07-30T01:01:01.123456789Z",
		},
		Computer: "computer",
		Channel:  "application",
		RecordID: 1,
		Level:    "Information",
		Message:  "message",
		Task:     "task",
		Opcode:   "opcode",
		Keywords: []string{"keyword"},
		EventData: EventData{
			Data: []Data{{Name: "name", Value: "value"}, {Name: "another_name", Value: "another_value"}},
		},
		Execution: &Execution{
			ProcessID: 13,
			ThreadID:  102,
		},
		Security: &Security{
			UserID: "my-user-id",
		},
		RenderedLevel:    "rendered_level",
		RenderedTask:     "rendered_task",
		RenderedOpcode:   "rendered_opcode",
		RenderedKeywords: []string{"RenderedKeywords"},
		Version:          0,
	}

	expected := map[string]any{
		"event_id": map[string]any{
			"id":         uint32(1),
			"qualifiers": uint16(2),
		},
		"provider": map[string]any{
			"name":         "provider",
			"guid":         "guid",
			"event_source": "event source",
		},
		"system_time": "2020-07-30T01:01:01.123456789Z",
		"computer":    "computer",
		"channel":     "application",
		"record_id":   uint64(1),
		"level":       "rendered_level",
		"message":     "message",
		"task":        "rendered_task",
		"opcode":      "rendered_opcode",
		"keywords":    []string{"RenderedKeywords"},
		"execution": map[string]any{
			"process_id": uint(13),
			"thread_id":  uint(102),
		},
		"security": map[string]any{
			"user_id": "my-user-id",
		},
		"event_data": map[string]any{
			"name":         "value",
			"another_name": "another_value",
		},
		"version": uint8(0),
	}

	require.Equal(t, expected, formattedBody(xml))
}

func TestParseBodyFullExecution(t *testing.T) {
	processorID := uint(3)
	sessionID := uint(2)
	kernelTime := uint(3)
	userTime := uint(100)
	processorTime := uint(200)

	xml := &EventXML{
		EventID: EventID{
			ID:         1,
			Qualifiers: 2,
		},
		Provider: Provider{
			Name:            "provider",
			GUID:            "guid",
			EventSourceName: "event source",
		},
		TimeCreated: TimeCreated{
			SystemTime: "2020-07-30T01:01:01.123456789Z",
		},
		Computer: "computer",
		Channel:  "application",
		RecordID: 1,
		Level:    "Information",
		Message:  "message",
		Task:     "task",
		Opcode:   "opcode",
		Keywords: []string{"keyword"},
		EventData: EventData{
			Data: []Data{{Name: "name", Value: "value"}, {Name: "another_name", Value: "another_value"}},
		},
		Execution: &Execution{
			ProcessID:     13,
			ThreadID:      102,
			ProcessorID:   &processorID,
			SessionID:     &sessionID,
			KernelTime:    &kernelTime,
			UserTime:      &userTime,
			ProcessorTime: &processorTime,
		},
		Security: &Security{
			UserID: "my-user-id",
		},
		RenderedLevel:    "rendered_level",
		RenderedTask:     "rendered_task",
		RenderedOpcode:   "rendered_opcode",
		RenderedKeywords: []string{"RenderedKeywords"},
		Version:          0,
	}

	expected := map[string]any{
		"event_id": map[string]any{
			"id":         uint32(1),
			"qualifiers": uint16(2),
		},
		"provider": map[string]any{
			"name":         "provider",
			"guid":         "guid",
			"event_source": "event source",
		},
		"system_time": "2020-07-30T01:01:01.123456789Z",
		"computer":    "computer",
		"channel":     "application",
		"record_id":   uint64(1),
		"level":       "rendered_level",
		"message":     "message",
		"task":        "rendered_task",
		"opcode":      "rendered_opcode",
		"keywords":    []string{"RenderedKeywords"},
		"execution": map[string]any{
			"process_id":     uint(13),
			"thread_id":      uint(102),
			"processor_id":   processorID,
			"session_id":     sessionID,
			"kernel_time":    kernelTime,
			"user_time":      userTime,
			"processor_time": processorTime,
		},
		"security": map[string]any{
			"user_id": "my-user-id",
		},
		"event_data": map[string]any{
			"name":         "value",
			"another_name": "another_value",
		},
		"version": uint8(0),
	}

	require.Equal(t, expected, formattedBody(xml))
}

func TestParseBodyCorrelation(t *testing.T) {
	activityIDGuid := "{11111111-1111-1111-1111-111111111111}"
	relatedActivityIDGuid := "{22222222-2222-2222-2222-222222222222}"
	xml := &EventXML{
		EventID: EventID{
			ID:         1,
			Qualifiers: 2,
		},
		Provider: Provider{
			Name:            "provider",
			GUID:            "guid",
			EventSourceName: "event source",
		},
		TimeCreated: TimeCreated{
			SystemTime: "2020-07-30T01:01:01.123456789Z",
		},
		Computer: "computer",
		Channel:  "application",
		RecordID: 1,
		Level:    "Information",
		Message:  "message",
		Task:     "task",
		Opcode:   "opcode",
		Keywords: []string{"keyword"},
		EventData: EventData{
			Data: []Data{{Name: "1st_name", Value: "value"}, {Name: "2nd_name", Value: "another_value"}},
		},
		RenderedLevel:    "rendered_level",
		RenderedTask:     "rendered_task",
		RenderedOpcode:   "rendered_opcode",
		RenderedKeywords: []string{"RenderedKeywords"},
		Correlation: &Correlation{
			ActivityID:        &activityIDGuid,
			RelatedActivityID: &relatedActivityIDGuid,
		},
		Version: 1,
	}

	expected := map[string]any{
		"event_id": map[string]any{
			"id":         uint32(1),
			"qualifiers": uint16(2),
		},
		"provider": map[string]any{
			"name":         "provider",
			"guid":         "guid",
			"event_source": "event source",
		},
		"system_time": "2020-07-30T01:01:01.123456789Z",
		"computer":    "computer",
		"channel":     "application",
		"record_id":   uint64(1),
		"level":       "rendered_level",
		"message":     "message",
		"task":        "rendered_task",
		"opcode":      "rendered_opcode",
		"keywords":    []string{"RenderedKeywords"},
		"event_data": map[string]any{
			"1st_name": "value",
			"2nd_name": "another_value",
		},
		"correlation": map[string]any{
			"activity_id":         "{11111111-1111-1111-1111-111111111111}",
			"related_activity_id": "{22222222-2222-2222-2222-222222222222}",
		},
		"version": uint8(1),
	}

	require.Equal(t, expected, formattedBody(xml))
}

func TestParseNoRendered(t *testing.T) {
	xml := &EventXML{
		EventID: EventID{
			ID:         1,
			Qualifiers: 2,
		},
		Provider: Provider{
			Name:            "provider",
			GUID:            "guid",
			EventSourceName: "event source",
		},
		TimeCreated: TimeCreated{
			SystemTime: "2020-07-30T01:01:01.123456789Z",
		},
		Computer: "computer",
		Channel:  "application",
		RecordID: 1,
		Level:    "Information",
		Message:  "message",
		Task:     "task",
		Opcode:   "opcode",
		Keywords: []string{"keyword"},
		EventData: EventData{
			Data: []Data{{Name: "name", Value: "value"}, {Name: "another_name", Value: "another_value"}},
		},
		Version: 0,
	}

	expected := map[string]any{
		"event_id": map[string]any{
			"id":         uint32(1),
			"qualifiers": uint16(2),
		},
		"provider": map[string]any{
			"name":         "provider",
			"guid":         "guid",
			"event_source": "event source",
		},
		"system_time": "2020-07-30T01:01:01.123456789Z",
		"computer":    "computer",
		"channel":     "application",
		"record_id":   uint64(1),
		"level":       "Information",
		"message":     "message",
		"task":        "task",
		"opcode":      "opcode",
		"keywords":    []string{"keyword"},
		"event_data": map[string]any{
			"name":         "value",
			"another_name": "another_value",
		},
		"version": uint8(0),
	}

	require.Equal(t, expected, formattedBody(xml))
}

func TestParseBodySecurity(t *testing.T) {
	xml := &EventXML{
		EventID: EventID{
			ID:         1,
			Qualifiers: 2,
		},
		Provider: Provider{
			Name:            "provider",
			GUID:            "guid",
			EventSourceName: "event source",
		},
		TimeCreated: TimeCreated{
			SystemTime: "2020-07-30T01:01:01.123456789Z",
		},
		Computer: "computer",
		Channel:  "Security",
		RecordID: 1,
		Level:    "Information",
		Message:  "message",
		Task:     "task",
		Opcode:   "opcode",
		Keywords: []string{"keyword"},
		EventData: EventData{
			Data: []Data{{Name: "name", Value: "value"}, {Name: "another_name", Value: "another_value"}},
		},
		RenderedLevel:    "rendered_level",
		RenderedTask:     "rendered_task",
		RenderedOpcode:   "rendered_opcode",
		RenderedKeywords: []string{"RenderedKeywords"},
		Version:          0,
	}

	expected := map[string]any{
		"event_id": map[string]any{
			"id":         uint32(1),
			"qualifiers": uint16(2),
		},
		"provider": map[string]any{
			"name":         "provider",
			"guid":         "guid",
			"event_source": "event source",
		},
		"system_time": "2020-07-30T01:01:01.123456789Z",
		"computer":    "computer",
		"channel":     "Security",
		"record_id":   uint64(1),
		"level":       "rendered_level",
		"message":     "message",
		"task":        "rendered_task",
		"opcode":      "rendered_opcode",
		"keywords":    []string{"RenderedKeywords"},
		"event_data": map[string]any{
			"name":         "value",
			"another_name": "another_value",
		},
		"version": uint8(0),
	}

	require.Equal(t, expected, formattedBody(xml))
}

func TestParseEventData(t *testing.T) {
	// Named entries with Binary — EventData Name attribute is not preserved (matches winlogbeat)
	xmlMap := &EventXML{
		EventData: EventData{
			Name:   "EVENT_DATA",
			Data:   []Data{{Name: "name", Value: "value"}},
			Binary: "2D20",
		},
	}

	parsed := formattedBody(xmlMap)
	expectedMap := map[string]any{
		"name":   "value",
		"Binary": "2D20",
	}
	require.Equal(t, expectedMap, parsed["event_data"])

	// Mixed named and unnamed — unnamed get paramN keys (1-based index)
	xmlMixed := &EventXML{
		EventData: EventData{
			Data: []Data{{Name: "name", Value: "value"}, {Value: "no_name"}},
		},
	}

	parsed = formattedBody(xmlMixed)
	expectedMixed := map[string]any{
		"name":   "value",
		"param2": "no_name",
	}
	require.Equal(t, expectedMixed, parsed["event_data"])

	// Empty values are dropped
	xmlEmpty := &EventXML{
		EventData: EventData{
			Data: []Data{{Name: "name", Value: ""}, {Name: "other", Value: "kept"}},
		},
	}

	parsed = formattedBody(xmlEmpty)
	expectedEmpty := map[string]any{
		"other": "kept",
	}
	require.Equal(t, expectedEmpty, parsed["event_data"])

	// Duplicate keys — first occurrence wins
	xmlDup := &EventXML{
		EventData: EventData{
			Data: []Data{{Name: "key", Value: "first"}, {Name: "key", Value: "second"}},
		},
	}

	parsed = formattedBody(xmlDup)
	expectedDup := map[string]any{
		"key": "first",
	}
	require.Equal(t, expectedDup, parsed["event_data"])
}

func TestUnmarshalEventXML_InvalidChars(t *testing.T) {
	// XML with control characters that are invalid in XML 1.0
	raw := `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
		<System>
			<Provider Name="Test"/>
			<EventID>1</EventID>
			<Level>4</Level>
			<Channel>App</Channel>
			<Computer>test` + "\x01\x08" + `</Computer>
			<TimeCreated SystemTime="2024-01-15T10:00:00.000Z"/>
			<EventRecordID>1</EventRecordID>
		</System>
	</Event>`
	event, err := unmarshalEventXML([]byte(raw))
	require.NoError(t, err)
	require.Equal(t, "test", event.Computer) // control chars stripped
}

func TestInvalidUnmarshal(t *testing.T) {
	_, err := unmarshalEventXML([]byte("Test \n Invalid \t Unmarshal"))
	require.Error(t, err)
}

func TestUnmarshalWithEventData(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "xmlSample.xml"))
	require.NoError(t, err)

	event, err := unmarshalEventXML(data)
	require.NoError(t, err)

	xml := &EventXML{
		EventID: EventID{
			ID:         16384,
			Qualifiers: 16384,
		},
		Provider: Provider{
			Name:            "Microsoft-Windows-Security-SPP",
			GUID:            "{E23B33B0-C8C9-472C-A5F9-F2BDFEA0F156}",
			EventSourceName: "Software Protection Platform Service",
		},
		TimeCreated: TimeCreated{
			SystemTime: "2022-04-22T10:20:52.3778625Z",
		},
		Computer:  "computer",
		Channel:   "Application",
		RecordID:  23401,
		Level:     "4",
		Message:   "",
		Task:      "0",
		Opcode:    "0",
		Execution: &Execution{},
		Security:  &Security{},
		EventData: EventData{
			Data: []Data{
				{Name: "Time", Value: "2022-04-28T19:48:52Z"},
				{Name: "Source", Value: "RulesEngine"},
			},
		},
		Keywords:    []string{"0x80000000000000"},
		Original:    string(data),
		Correlation: &Correlation{},
	}

	require.Equal(t, xml, event)
}

func TestUnmarshalWithCorrelation(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "xmlWithCorrelation.xml"))
	require.NoError(t, err)

	event, err := unmarshalEventXML(data)
	require.NoError(t, err)

	activityIDGuid := "{11111111-1111-1111-1111-111111111111}"
	relatedActivityIDGuid := "{22222222-2222-2222-2222-222222222222}"

	xml := &EventXML{
		EventID: EventID{
			ID:         4624,
			Qualifiers: 0,
		},
		Provider: Provider{
			Name: "Microsoft-Windows-Security-Auditing",
			GUID: "{54849625-5478-4994-a5ba-3e3b0328c30d}",
		},
		TimeCreated: TimeCreated{
			SystemTime: "2025-12-02T23:33:05.2167526Z",
		},
		Computer: "computer",
		Channel:  "Security",
		RecordID: 13177,
		Level:    "0",
		Message:  "",
		Task:     "12544",
		Opcode:   "0",
		EventData: EventData{
			Data:   []Data{{Name: "SubjectDomainName", Value: "WORKGROUP"}},
			Binary: "",
		},
		Keywords: []string{"0x8020000000000000"},
		Security: &Security{},
		Execution: &Execution{
			ProcessID: 800,
			ThreadID:  7852,
		},
		Original: string(data),
		Correlation: &Correlation{
			ActivityID:        &activityIDGuid,
			RelatedActivityID: &relatedActivityIDGuid,
		},
		Version: 2,
	}

	require.Equal(t, xml, event)
}

func TestUnmarshalWithAnonymousEventDataEntries(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "xmlWithAnonymousEventDataEntries.xml"))
	require.NoError(t, err)

	event, err := unmarshalEventXML(data)
	require.NoError(t, err)

	xml := &EventXML{
		EventID: EventID{
			ID:         8194,
			Qualifiers: 0,
		},
		Provider: Provider{
			Name: "VSS",
		},
		TimeCreated: TimeCreated{
			SystemTime: "2023-10-19T21:57:58.0685414Z",
		},
		Computer: "computer",
		Channel:  "Application",
		RecordID: 383972,
		Level:    "2",
		Message:  "",
		Task:     "0",
		Opcode:   "0",
		EventData: EventData{
			Data:   []Data{{Name: "", Value: "1st_value"}, {Name: "", Value: "2nd_value"}},
			Binary: "2D20",
		},
		Keywords:    []string{"0x80000000000000"},
		Security:    &Security{},
		Execution:   &Execution{},
		Original:    string(data),
		Correlation: &Correlation{},
		Version:     0,
	}

	require.Equal(t, xml, event)
}

func TestUnmarshalWithUserData(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "xmlSampleUserData.xml"))
	require.NoError(t, err)

	event, err := unmarshalEventXML(data)
	require.NoError(t, err)

	require.NotNil(t, event.UserData)
	require.Equal(t, "LogFileCleared", event.UserData.Name.Local)
	require.Len(t, event.UserData.Data, 6)

	expected := &EventXML{
		EventID: EventID{
			ID: 1102,
		},
		Provider: Provider{
			Name: "Microsoft-Windows-Eventlog",
			GUID: "{fc65ddd8-d6ef-4962-83d5-6e5cfe9ce148}",
		},
		TimeCreated: TimeCreated{
			SystemTime: "2023-10-12T10:38:24.543506200Z",
		},
		Computer: "test.example.com",
		Channel:  "Security",
		RecordID: 2590526,
		Level:    "4",
		Message:  "",
		Task:     "104",
		Opcode:   "0",
		Keywords: []string{"0x4020000000000000"},
		Security: &Security{
			UserID: "S-1-5-18",
		},
		Execution: &Execution{
			ProcessID: 1472,
			ThreadID:  7784,
		},
		UserData:    event.UserData, // compare structurally below
		Original:    string(data),
		Correlation: &Correlation{},
		Version:     1,
	}

	require.Equal(t, expected, event)
}

func TestFormattedBody_ProcessingError(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "xmlSampleProcessingError.xml"))
	require.NoError(t, err)

	event, err := unmarshalEventXML(data)
	require.NoError(t, err)

	body := formattedBody(event)
	errData, ok := body["error"].(map[string]any)
	require.True(t, ok, "error should be present")
	require.Equal(t, uint32(15027), errData["code"])
	require.Equal(t, "message", errData["data_item_name"])
}

func TestFormattedBody_AuditSuccess(t *testing.T) {
	event := &EventXML{
		Keywords: []string{"0x8020000000000000"}, // audit success bit set
	}
	body := formattedBody(event)
	require.Equal(t, "success", body["outcome"])
}

func TestFormattedBody_AuditFailure(t *testing.T) {
	event := &EventXML{
		Keywords: []string{"0x8010000000000000"}, // audit failure bit set
	}
	body := formattedBody(event)
	require.Equal(t, "failure", body["outcome"])
}

func TestFormattedBody_NoAuditKeyword(t *testing.T) {
	event := &EventXML{
		Keywords: []string{"0x8000000000000000"},
	}
	body := formattedBody(event)
	_, hasOutcome := body["outcome"]
	require.False(t, hasOutcome)
}

func TestFormattedBodyWithUserData(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "xmlSampleUserData.xml"))
	require.NoError(t, err)

	event, err := unmarshalEventXML(data)
	require.NoError(t, err)

	body := formattedBody(event)
	ud, ok := body["user_data"].(map[string]any)
	require.True(t, ok, "user_data should be present")
	require.Equal(t, "LogFileCleared", ud["xml_name"])
	require.Equal(t, "S-1-5-21-1148437859-4135665037-1195073887-1000", ud["SubjectUserSid"])
	require.Equal(t, "test_user", ud["SubjectUserName"])
	require.Equal(t, "TEST", ud["SubjectDomainName"])
	require.Equal(t, "0xa8bb72", ud["SubjectLogonId"])
	require.Equal(t, "4536", ud["ClientProcessId"])
	require.Equal(t, "17732923532772643", ud["ClientProcessStartKey"])
}

func TestFormattedBodyUserData_EmptyValuesDropped(t *testing.T) {
	event := &EventXML{
		UserData: &UserData{
			Name: xml.Name{Local: "TestEvent"},
			Data: []UserDataEntry{
				{XMLName: xml.Name{Local: "Key1"}, Value: "val1"},
				{XMLName: xml.Name{Local: "Key2"}, Value: ""},
				{XMLName: xml.Name{Local: "Key3"}, Value: "val3"},
			},
		},
	}
	body := formattedBody(event)
	ud := body["user_data"].(map[string]any)
	require.Equal(t, "TestEvent", ud["xml_name"])
	require.Equal(t, "val1", ud["Key1"])
	require.Equal(t, "val3", ud["Key3"])
	_, hasKey2 := ud["Key2"]
	require.False(t, hasKey2, "empty values should be dropped")
}

func TestFormattedBodyUserData_DuplicateKeysFirstWins(t *testing.T) {
	event := &EventXML{
		UserData: &UserData{
			Name: xml.Name{Local: "TestEvent"},
			Data: []UserDataEntry{
				{XMLName: xml.Name{Local: "Key"}, Value: "first"},
				{XMLName: xml.Name{Local: "Key"}, Value: "second"},
			},
		},
	}
	body := formattedBody(event)
	ud := body["user_data"].(map[string]any)
	require.Equal(t, "first", ud["Key"])
}

func TestFormattedBodyUserData_AllEmptyNoUserData(t *testing.T) {
	event := &EventXML{
		UserData: &UserData{
			Name: xml.Name{Local: "TestEvent"},
			Data: []UserDataEntry{
				{XMLName: xml.Name{Local: "Key1"}, Value: ""},
			},
		},
	}
	body := formattedBody(event)
	// UserData still present because Data slice is non-empty (even though all values empty)
	ud := body["user_data"].(map[string]any)
	require.Equal(t, "TestEvent", ud["xml_name"])
	require.Len(t, ud, 1, "only xml_name should remain")
}
