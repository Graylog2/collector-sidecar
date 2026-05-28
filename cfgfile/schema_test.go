// Copyright (C) 2020 Graylog, Inc.
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

package cfgfile

import (
	"testing"

	ucfg "github.com/elastic/go-ucfg"
	"github.com/elastic/go-ucfg/yaml"
)

// unpackConfig mirrors how cfgfile.Read applies a YAML document on top of the
// InitDefaults-provided defaults, isolating the default/override merge behavior.
func unpackConfig(t *testing.T, yamlDoc string) *SidecarConfig {
	t.Helper()
	c, err := yaml.NewConfig([]byte(yamlDoc), ucfg.PathSep("."))
	if err != nil {
		t.Fatalf("failed to parse test config: %v", err)
	}
	cfg := &SidecarConfig{}
	if err := c.Unpack(cfg, ucfg.PathSep(".")); err != nil {
		t.Fatalf("failed to unpack test config: %v", err)
	}
	return cfg
}

// platformDefaultAccesslist returns the accesslist the application ships with
// for the current platform.
func platformDefaultAccesslist() []string {
	cfg := &SidecarConfig{}
	cfg.InitDefaults()
	return cfg.CollectorBinariesAccesslist
}

// TestAccesslistReplacesDefaults guards a security-relevant invariant: a
// user-supplied collector_binaries_accesslist must FULLY replace the built-in
// default list, never merge with it.
//
// go-ucfg merges slices by index, so without the `,replace` struct-tag option a
// user list shorter than the default would inherit the default's trailing
// entries, silently allowing binaries the user never listed. See the
// `,replace` tag on SidecarConfig.CollectorBinariesAccesslist.
func TestAccesslistReplacesDefaults(t *testing.T) {
	defaults := platformDefaultAccesslist()
	if len(defaults) < 2 {
		t.Fatalf("test assumes a default accesslist with at least 2 entries, got %d", len(defaults))
	}

	// A custom list strictly shorter than the defaults is the case that would
	// leak default entries under index-merge semantics.
	cfg := unpackConfig(t, "collector_binaries_accesslist:\n  - /custom/beat\n  - /another/beat\n")

	want := []string{"/custom/beat", "/another/beat"}
	got := cfg.CollectorBinariesAccesslist
	if len(got) != len(want) {
		t.Fatalf("configured accesslist did not replace defaults: want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("accesslist entry %d: want %q, got %q (full: %v)", i, want[i], got[i], got)
		}
	}

	// Explicitly assert no default entry leaked into the effective list.
	for _, d := range defaults {
		for _, g := range got {
			if g == d {
				t.Fatalf("default entry %q leaked into a user-configured accesslist: %v", d, got)
			}
		}
	}
}

// TestAccesslistDefaultsWhenOmitted verifies that omitting the accesslist keeps
// the platform defaults intact (so users are not silently left with an empty,
// allow-all list).
func TestAccesslistDefaultsWhenOmitted(t *testing.T) {
	cfg := unpackConfig(t, "server_url: http://127.0.0.1:9000/api/\n")

	defaults := platformDefaultAccesslist()
	if len(cfg.CollectorBinariesAccesslist) != len(defaults) {
		t.Fatalf("omitted accesslist should fall back to %d defaults, got %d: %v",
			len(defaults), len(cfg.CollectorBinariesAccesslist), cfg.CollectorBinariesAccesslist)
	}
}

// TestAccesslistExplicitEmpty verifies that an explicit empty list is preserved
// as empty (the documented way to allow all binaries), rather than being
// repopulated from the defaults.
func TestAccesslistExplicitEmpty(t *testing.T) {
	cfg := unpackConfig(t, "collector_binaries_accesslist: []\n")

	if len(cfg.CollectorBinariesAccesslist) != 0 {
		t.Fatalf("explicit empty accesslist should stay empty, got %v", cfg.CollectorBinariesAccesslist)
	}
}
