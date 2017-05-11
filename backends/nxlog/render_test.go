// This file is part of Graylog.
//
// Graylog is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Graylog is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Graylog.  If not, see <http://www.gnu.org/licenses/>.

package nxlog

import (
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"strings"
	"testing"
)

func TestRenderSingleExtension(t *testing.T) {
	engine := &NxConfig{
		Extensions: []nxextension{{name: "test-extension", properties: map[string]string{"a": "b"}}},
	}

	result := engine.Render()

	expect := "<Extension test-extension>\n  a b\n</Extension>"
	if !strings.Contains(string(result), expect) {
		t.Fail()
	}
}

func TestRenderMultipleExtensions(t *testing.T) {
	engine := &NxConfig{
		Extensions: []nxextension{{name: "test-extension1", properties: map[string]string{"a": "b"}}},
	}
	addition := &nxextension{name: "test-extension2", properties: map[string]string{"c": "d"}}
	engine.Extensions = append(engine.Extensions, *addition)

	result := engine.Render()

	expect1 := "<Extension test-extension1>\n  a b\n"
	expect2 := "<Extension test-extension2>\n  c d\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderSingleInput(t *testing.T) {
	engine := &NxConfig{
		Inputs: []nxinput{{name: "test-input", properties: map[string]string{"a": "b"}}},
	}

	result := engine.Render()

	expect := "<Input test-input>\n  a b\n</Input>"
	if !strings.Contains(string(result), expect) {
		t.Fail()
	}
}

func TestRenderMultipleInputs(t *testing.T) {
	engine := &NxConfig{
		Inputs: []nxinput{{name: "test-input1", properties: map[string]string{"a": "b"}}},
	}
	addition := &nxinput{name: "test-input2", properties: map[string]string{"c": "d"}}
	engine.Inputs = append(engine.Inputs, *addition)

	result := engine.Render()

	expect1 := "<Input test-input1>\n  a b\n"
	expect2 := "<Input test-input2>\n  c d\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderSingleOutput(t *testing.T) {
	engine := &NxConfig{
		Outputs: []nxoutput{{name: "test-output", properties: map[string]string{"a": "b"}}},
	}

	result := engine.Render()

	expect := "<Output test-output>\n  a b\n</Output>"
	if !strings.Contains(string(result), expect) {
		t.Fail()
	}
}

func TestRenderMultipleOutputs(t *testing.T) {
	engine := &NxConfig{
		Outputs: []nxoutput{{name: "test-output1", properties: map[string]string{"a": "b"}}},
	}
	addition := &nxoutput{name: "test-output2", properties: map[string]string{"c": "d"}}
	engine.Outputs = append(engine.Outputs, *addition)

	result := engine.Render()

	expect1 := "<Output test-output1>\n  a b\n"
	expect2 := "<Output test-output2>\n  c d\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderSingleRoute(t *testing.T) {
	engine := &NxConfig{
		Routes: []nxroute{{name: "test-route", properties: map[string]string{"a": "b"}}},
	}

	result := engine.Render()

	expect := "<Route test-route>\n  a b\n</Route>"
	if !strings.Contains(string(result), expect) {
		t.Fail()
	}
}

func TestRenderMultipleRoutes(t *testing.T) {
	engine := &NxConfig{
		Routes: []nxroute{{name: "test-route1", properties: map[string]string{"a": "b"}}},
	}
	addition := &nxroute{name: "test-route2", properties: map[string]string{"c": "d"}}
	engine.Routes = append(engine.Routes, *addition)

	result := engine.Render()

	expect1 := "<Route test-route1>\n  a b\n"
	expect2 := "<Route test-route2>\n  c d\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderSingleSnippet(t *testing.T) {
	engine := &NxConfig{
		Context:  context.NewContext(),
		Snippets: []nxsnippet{{name: "test-snippet", value: "snippet-data"}},
	}

	result := engine.Render()

	expect := "snippet-data\n"
	if !strings.Contains(string(result), expect) {
		t.Fail()
	}
}

func TestRenderMultipleSnippets(t *testing.T) {
	engine := &NxConfig{
		Context:  context.NewContext(),
		Snippets: []nxsnippet{{name: "test-snippet1", value: "snippet-data"}},
	}
	addition := &nxsnippet{name: "test-snippet2", value: "data-snippet"}
	engine.Snippets = append(engine.Snippets, *addition)

	result := engine.Render()

	expect1 := "snippet-data\n"
	expect2 := "data-snippet\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderSnippetTemplate(t *testing.T) {
	engine := &NxConfig{
		Context:  context.NewContext(),
		Snippets: []nxsnippet{{name: "test-snippet", value: "{{.Version}}"}},
	}

	result := engine.Render()

	expect := common.CollectorVersion + "\n"
	if !strings.Contains(string(result), expect) {
		t.Fail()
	}
}

func TestRenderSnippetFailedTemplate(t *testing.T) {
	engine := &NxConfig{
		Context:  context.NewContext(),
		Snippets: []nxsnippet{{name: "test-snippet", value: "{{non valid template}}"}},
	}

	result := engine.Render()

	expect := "{{non valid template}}\n"
	if !strings.Contains(string(result), expect) {
		t.Fail()
	}
}

func TestRenderMultipleFileInputs(t *testing.T) {
	engine := &NxConfig{
		Canned: []nxcanned{{name: "test-file-input1", kind: "input-file", properties: map[string]interface{}{
			"path":          "/foo",
			"poll_interval": 1,
			"save_position": true,
			"read_last":     false,
			"recursive":     true,
			"rename_check":  false,
		}}},
	}
	addition := &nxcanned{name: "test-file-input2", kind: "input-file", properties: map[string]interface{}{
		"path":          "/bar",
		"poll_interval": 1,
		"save_position": true,
		"read_last":     false,
		"recursive":     true,
		"rename_check":  false,
	}}
	engine.Canned = append(engine.Canned, *addition)

	result := engine.Render()

	expect1 := "<Input test-file-input1>\n"
	expect2 := "<Input test-file-input2>\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderSingleFileInputWithFields(t *testing.T) {
	engine := &NxConfig{
		Canned: []nxcanned{{name: "test-file-input1", kind: "input-file", properties: map[string]interface{}{
			"path":          "/foo",
			"poll_interval": 1,
			"save_position": true,
			"read_last":     false,
			"recursive":     true,
			"rename_check":  false,
			"fields":        map[string]interface{}{"field1": "data", "field2": "data"},
		}}},
	}

	result := engine.Render()

	expect1 := "Exec $field1 = \"data\";\n"
	expect2 := "Exec $field2 = \"data\";\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderSingleFileInputWithMultilineSupport(t *testing.T) {
	engine := &NxConfig{
		Canned: []nxcanned{{name: "test-file-input1", kind: "input-file", properties: map[string]interface{}{
			"path":          "/foo",
			"poll_interval": 1,
			"save_position": true,
			"read_last":     false,
			"recursive":     true,
			"rename_check":  false,
			"multiline":     true,
		}}},
	}

	result := engine.Render()

	expect1 := "InputType test-file-input1-multiline\n"
	if !strings.Contains(string(result), expect1) {
		t.Fail()
	}
}

func TestRenderMultipleWindowsEventInputs(t *testing.T) {
	engine := &NxConfig{
		Canned: []nxcanned{{name: "test-windows-event-input1", kind: "input-windows-event-log", properties: map[string]interface{}{
			"poll_interval": 1,
			"save_position": true,
			"read_last":     false,
		}}},
	}
	addition := &nxcanned{name: "test-windows-event-input2", kind: "input-windows-event-log", properties: map[string]interface{}{
		"poll_interval": 1,
		"save_position": true,
		"read_last":     false,
	}}
	engine.Canned = append(engine.Canned, *addition)

	result := engine.Render()

	expect1 := "<Input test-windows-event-input1>\n"
	expect2 := "<Input test-windows-event-input2>\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderSingleWindowsEventInputWithChannel(t *testing.T) {
	engine := &NxConfig{
		Canned: []nxcanned{{name: "test-windows-event-input1", kind: "input-windows-event-log", properties: map[string]interface{}{
			"poll_interval": 1,
			"save_position": true,
			"read_last":     false,
			"channel":       "System",
		}}},
	}

	result := engine.Render()

	expect1 := "Channel System\n"
	if !strings.Contains(string(result), expect1) {
		t.Fail()
	}
}

func TestRenderSingleWindowsEventInputWithQuery(t *testing.T) {
	engine := &NxConfig{
		Canned: []nxcanned{{name: "test-windows-event-input1", kind: "input-windows-event-log", properties: map[string]interface{}{
			"poll_interval": 1,
			"save_position": true,
			"read_last":     false,
			"query":         "<QueryList> <Query Id=\"0\"> <Select Path=\"Microsoft-Windows-Sysmon/Operational\">*</Select> </Query></QueryList>",
		}}},
	}

	result := engine.Render()

	expect1 := "Query <QueryList> <Query Id=\"0\"> <Select Path=\"Microsoft-Windows-Sysmon/Operational\">*</Select> </Query></QueryList>\n"
	if !strings.Contains(string(result), expect1) {
		t.Fail()
	}
}

func TestRenderSingleWindowsEventInputWithFields(t *testing.T) {
	engine := &NxConfig{
		Canned: []nxcanned{{name: "test-windows-event-input1", kind: "input-windows-event-log", properties: map[string]interface{}{
			"poll_interval": 1,
			"save_position": true,
			"read_last":     false,
			"fields":        map[string]interface{}{"field1": "data", "field2": "data"},
		}}},
	}

	result := engine.Render()

	expect1 := "Exec $field1 = \"data\";\n"
	expect2 := "Exec $field2 = \"data\";\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderMultipleUdpSyslogInputs(t *testing.T) {
	engine := &NxConfig{
		Canned: []nxcanned{{name: "test-udp-syslog-input1", kind: "input-udp-syslog", properties: map[string]interface{}{
			"host": "127.0.0.1",
			"port": "514",
		}}},
	}
	addition := &nxcanned{name: "test-udp-syslog-input2", kind: "input-udp-syslog", properties: map[string]interface{}{
		"host": "127.0.0.1",
		"port": "514",
	}}
	engine.Canned = append(engine.Canned, *addition)

	result := engine.Render()

	expect1 := "<Input test-udp-syslog-input1>\n"
	expect2 := "<Input test-udp-syslog-input2>\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderSingleUdpSyslogInputWithFields(t *testing.T) {
	engine := &NxConfig{
		Canned: []nxcanned{{name: "test-udp-syslog-input1", kind: "input-udp-syslog", properties: map[string]interface{}{
			"host":   "127.0.0.1",
			"port":   "514",
			"fields": map[string]interface{}{"field1": "data", "field2": "data"},
		}}},
	}

	result := engine.Render()

	expect1 := "Exec $field1 = \"data\";\n"
	expect2 := "Exec $field2 = \"data\";\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderMultipleTcpSyslogInputs(t *testing.T) {
	engine := &NxConfig{
		Canned: []nxcanned{{name: "test-tcp-syslog-input1", kind: "input-tcp-syslog", properties: map[string]interface{}{
			"host": "127.0.0.1",
			"port": "514",
		}}},
	}
	addition := &nxcanned{name: "test-tcp-syslog-input2", kind: "input-tcp-syslog", properties: map[string]interface{}{
		"host": "127.0.0.1",
		"port": "514",
	}}
	engine.Canned = append(engine.Canned, *addition)

	result := engine.Render()

	expect1 := "<Input test-tcp-syslog-input1>\n"
	expect2 := "<Input test-tcp-syslog-input2>\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderSingleTcpSyslogInputWithFields(t *testing.T) {
	engine := &NxConfig{
		Canned: []nxcanned{{name: "test-tcp-syslog-input1", kind: "input-tcp-syslog", properties: map[string]interface{}{
			"host":   "127.0.0.1",
			"port":   "514",
			"fields": map[string]interface{}{"field1": "data", "field2": "data"},
		}}},
	}

	result := engine.Render()

	expect1 := "Exec $field1 = \"data\";\n"
	expect2 := "Exec $field2 = \"data\";\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderMultipleUdpGelfOutputs(t *testing.T) {
	engine := &NxConfig{
		Context: context.NewContext(),
		Canned: []nxcanned{{name: "test-udp-gelf-output1", kind: "output-gelf-udp", properties: map[string]interface{}{
			"server": "127.0.0.1",
			"port":   "12201",
		}}},
	}
	addition := &nxcanned{name: "test-udp-gelf-output2", kind: "output-gelf-udp", properties: map[string]interface{}{
		"server": "127.0.0.1",
		"port":   "12201",
	}}
	engine.Canned = append(engine.Canned, *addition)

	result := engine.Render()

	expect1 := "<Output test-udp-gelf-output1>\n"
	expect2 := "<Output test-udp-gelf-output2>\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderUdpGelfOutputWithFields(t *testing.T) {
	engine := &NxConfig{
		Context: context.NewContext(),
		Canned: []nxcanned{{name: "test-udp-gelf-output1", kind: "output-gelf-udp", properties: map[string]interface{}{
			"server": "127.0.0.1",
			"port":   "12201",
			"fields": map[string]interface{}{"field1": "data", "field2": "data"},
		}}},
	}

	result := engine.Render()

	expect1 := "Exec $field1 = \"data\";\n"
	expect2 := "Exec $field2 = \"data\";\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderMultipleTcpGelfOutputs(t *testing.T) {
	engine := &NxConfig{
		Context: context.NewContext(),
		Canned: []nxcanned{{name: "test-tcp-gelf-output1", kind: "output-gelf-tcp", properties: map[string]interface{}{
			"server": "127.0.0.1",
			"port":   "12201",
		}}},
	}
	addition := &nxcanned{name: "test-tcp-gelf-output2", kind: "output-gelf-tcp", properties: map[string]interface{}{
		"server": "127.0.0.1",
		"port":   "12201",
	}}
	engine.Canned = append(engine.Canned, *addition)

	result := engine.Render()

	expect1 := "<Output test-tcp-gelf-output1>\n"
	expect2 := "<Output test-tcp-gelf-output2>\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderTcpGelfOutputWithFields(t *testing.T) {
	engine := &NxConfig{
		Context: context.NewContext(),
		Canned: []nxcanned{{name: "test-tcp-gelf-output1", kind: "output-gelf-tcp", properties: map[string]interface{}{
			"server": "127.0.0.1",
			"port":   "12201",
			"fields": map[string]interface{}{"field1": "data", "field2": "data"},
		}}},
	}

	result := engine.Render()

	expect1 := "Exec $field1 = \"data\";\n"
	expect2 := "Exec $field2 = \"data\";\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderMultipleTcpTlsGelfOutputs(t *testing.T) {
	engine := &NxConfig{
		Context: context.NewContext(),
		Canned: []nxcanned{{name: "test-tls-gelf-output1", kind: "output-gelf-tcp-tls", properties: map[string]interface{}{
			"server": "127.0.0.1",
			"port":   "12201",
		}}},
	}
	addition := &nxcanned{name: "test-tls-gelf-output2", kind: "output-gelf-tcp-tls", properties: map[string]interface{}{
		"server": "127.0.0.1",
		"port":   "12201",
	}}
	engine.Canned = append(engine.Canned, *addition)

	result := engine.Render()

	expect1 := "<Output test-tls-gelf-output1>\n"
	expect2 := "<Output test-tls-gelf-output2>\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderTcpTlsGelfOutputWithFields(t *testing.T) {
	engine := &NxConfig{
		Context: context.NewContext(),
		Canned: []nxcanned{{name: "test-tls-gelf-output1", kind: "output-gelf-tcp-tls", properties: map[string]interface{}{
			"server": "127.0.0.1",
			"port":   "12201",
			"fields": map[string]interface{}{"field1": "data", "field2": "data"},
		}}},
	}

	result := engine.Render()

	expect1 := "Exec $field1 = \"data\";\n"
	expect2 := "Exec $field2 = \"data\";\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}

func TestRenderSingleTcpTlsGelfOutputWithAllowUntrust(t *testing.T) {
	engine := &NxConfig{
		Context: context.NewContext(),
		Canned: []nxcanned{{name: "test-tls-gelf-output1", kind: "output-gelf-tcp-tls", properties: map[string]interface{}{
			"server":          "127.0.0.1",
			"port":            "12201",
			"allow_untrusted": true,
		}}},
	}

	result := engine.Render()

	expect1 := "<Output test-tls-gelf-output1>\n"
	expect2 := "AllowUntrusted True\n"
	if !(strings.Contains(string(result), expect1) && strings.Contains(string(result), expect2)) {
		t.Fail()
	}
}
