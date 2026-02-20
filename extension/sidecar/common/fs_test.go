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

package common

import "testing"

func TestSanitizePathComponent(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "simple name", input: "myconfig", want: "myconfig"},
		{name: "name with extension", input: "filebeat.conf", want: "filebeat.conf"},
		{name: "relative traversal", input: "../../etc/shadow", want: "shadow"},
		{name: "leading dot-dot", input: "../foo", want: "foo"},
		{name: "nested subdirectory", input: "a/b/c", want: "c"},
		{name: "absolute path unix", input: "/etc/passwd", want: "passwd"},
		{name: "dot only", input: ".", wantErr: true},
		{name: "dot-dot only", input: "..", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
		{name: "hidden file", input: ".hidden", want: ".hidden"},
		{name: "trailing slash", input: "foo/", want: "foo"},
		{name: "dot-dot with trailing slash", input: "../", wantErr: true},
		{name: "root slash", input: "/", wantErr: true},
		{name: "multiple slashes", input: "////", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizePathComponent(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("SanitizePathComponent(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("SanitizePathComponent(%q) returned unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("SanitizePathComponent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
