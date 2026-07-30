// Intermasq - Web panel for dnsmasq
// Copyright (C) 2026 AlexRus1234
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// withTemplatesPath saves/restores *TemplatesPath and the global templates
// map so tests don't bleed into each other.
func withTemplatesPath(t *testing.T, path string) {
	t.Helper()
	origPath := *TemplatesPath
	origMap := templates
	*TemplatesPath = path
	templates = make(map[string]Template)
	t.Cleanup(func() {
		*TemplatesPath = origPath
		templates = origMap
	})
}

// TestLoadTemplates_NoFile covers the os.IsNotExist branch: parent dir is
// created and the function returns without populating the map.
func TestLoadTemplates_NoFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "sub", "templates.json")
	withTemplatesPath(t, path)

	loadTemplates()

	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("expected parent dir created, got: %v", err)
	}
	if len(templates) != 0 {
		t.Errorf("expected empty templates map, got %d entries", len(templates))
	}
}

// TestLoadTemplates_ValidJSON covers the success unmarshal path.
func TestLoadTemplates_ValidJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "templates.json")
	want := map[string]Template{
		"t1": {ID: "t1", Name: "Template 1", IPRange: "10.0.0.10-10.0.0.100", HostnamePattern: "host-{NNN}", TargetFile: ""},
	}
	data, _ := json.MarshalIndent(want, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	withTemplatesPath(t, path)

	loadTemplates()

	if len(templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(templates))
	}
	got, ok := templates["t1"]
	if !ok {
		t.Fatal("expected key t1 in templates map")
	}
	if got.HostnamePattern != "host-{NNN}" {
		t.Errorf("expected hostname_pattern host-{NNN}, got %q", got.HostnamePattern)
	}
}

// TestSaveTemplates_RoundTripAndAtomic covers saveTemplates' success path
// (MkdirAll + tmp write + rename) and verifies the file ends up at the
// canonical path with the marshalled content.
func TestSaveTemplates_RoundTripAndAtomic(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "deep", "templates.json")
	withTemplatesPath(t, path)
	templates["round"] = Template{ID: "round", Name: "Round", IPRange: "10.0.0.5-10.0.0.9", HostnamePattern: "h-{NNN}"}

	if err := saveTemplates(); err != nil {
		t.Fatalf("saveTemplates: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var got map[string]Template
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got["round"]; !ok {
		t.Errorf("expected key 'round' persisted, got %v", got)
	}
	// The tmp file must have been renamed (not left behind).
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Errorf("tmp file left behind at %s.tmp", path)
	}
}

// TestSaveTemplates_MkdirAllError covers the MkdirAll-failure path: the
// parent directory of *TemplatesPath is a regular file, so MkdirAll fails.
func TestSaveTemplates_MkdirAllError(t *testing.T) {
	tmp := t.TempDir()
	// Make the parent a regular file, not a directory.
	parent := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "templates.json") // parent is a file → MkdirAll fails
	withTemplatesPath(t, path)

	if err := saveTemplates(); err == nil {
		t.Fatal("expected MkdirAll error, got nil")
	}
}

// TestSaveTemplates_WriteError covers the WriteFile-failure path: the tmp
// destination is a directory, so os.WriteFile fails.
func TestSaveTemplates_WriteError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "templates.json")
	withTemplatesPath(t, path)
	templates["x"] = Template{Name: "x"}

	// Pre-create "<path>.tmp" as a directory so os.WriteFile inside
	// saveTemplates fails (it can't write into a directory entry).
	if err := os.Mkdir(path+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := saveTemplates(); err == nil {
		t.Fatal("expected WriteFile error, got nil")
	}
}

// TestGenHostnameFromPattern covers the {NNN} replacement with zero-padding.
func TestGenHostnameFromPattern(t *testing.T) {
	cases := []struct {
		pattern string
		index   int
		want    string
	}{
		{"host-{NNN}", 1, "host-001"},
		{"host-{NNN}", 42, "host-042"},
		{"host-{NNN}", 999, "host-999"},
		{"no-placeholder", 5, "no-placeholder"},
		{"{NNN}-only", 0, "000-only"},
	}
	for _, tc := range cases {
		got := genHostnameFromPattern(tc.pattern, tc.index)
		if got != tc.want {
			t.Errorf("genHostnameFromPattern(%q,%d) = %q, want %q", tc.pattern, tc.index, got, tc.want)
		}
	}
}

// TestCountHostsInFile covers the file-count helper including its read-error
// branch (nonexistent file → 0).
func TestCountHostsInFile(t *testing.T) {
	// Nonexistent → 0.
	if got := countHostsInFile("/path/that/does/not/exist/12345"); got != 0 {
		t.Errorf("expected 0 for missing file, got %d", got)
	}
	// Real file with several dhcp-host lines + noise.
	tmp := t.TempDir()
	f := filepath.Join(tmp, "hosts.conf")
	content := []byte("# header\ndhcp-host=aa:bb:cc:dd:ee:ff,h1,10.0.0.1\n" +
		"\n" +
		"dhcp-host=11:22:33:44:55:66,h2,10.0.0.2\n" +
		"# not a host: dhcp-host=...\n")
	if err := os.WriteFile(f, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := countHostsInFile(f); got != 2 {
		t.Errorf("expected 2 dhcp-host lines, got %d", got)
	}
}
