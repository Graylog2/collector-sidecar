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
