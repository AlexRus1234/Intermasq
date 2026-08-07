// Intermasq - Web panel for dnsmasq
// Copyright (C) 2026 AlexRus1234
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package dnsmasq

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupHistoryEnv prepares temp ConfigDir and HistoryDir for history tests.
// Migrated from main package's dnsmasq_test.go (stage 5).
func setupHistoryEnv(t *testing.T) (confDir, histDir string) {
	t.Helper()
	confDir = t.TempDir()
	histDir = t.TempDir()
	*ConfigDir = confDir
	*HistoryDir = histDir
	*HistoryDepth = 10
	return confDir, histDir
}

// ========== SaveHistory ==========

func TestSaveHistoryCreatesVersion(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	path := filepath.Join(confDir, "hosts.conf")
	if err := os.WriteFile(path, []byte("dhcp-host=aa:bb:cc:dd:ee:ff,1.2.3.4,host\n"), 0644); err != nil {
		t.Fatal(err)
	}
	SaveHistory(path)
	versions, err := ListHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if !HistoryVersionRegex.MatchString(versions[0].Version) {
		t.Errorf("bad version id: %q", versions[0].Version)
	}
}

func TestSaveHistoryNoOpForMissingFile(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	path := filepath.Join(confDir, "nope.conf")
	SaveHistory(path)
	versions, err := ListHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("expected 0 versions for missing file, got %d", len(versions))
	}
}

func TestSaveHistoryRejectsUnsafePath(t *testing.T) {
	setupHistoryEnv(t)
	// Path outside ConfigDir — must be ignored.
	SaveHistory("/etc/passwd")
	versions, _ := ListHistory("/etc/passwd")
	if len(versions) != 0 {
		t.Fatalf("history written for unsafe path")
	}
}

// ========== RotateHistory ==========

func TestRotateHistoryKeepsDepth(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	*HistoryDepth = 3
	path := filepath.Join(confDir, "hosts.conf")
	if err := os.WriteFile(path, []byte("v0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Save 5 versions with distinct mtimes so rotation order is stable.
	for i := 0; i < 5; i++ {
		os.WriteFile(path, []byte("v"+string(rune('0'+i))+"\n"), 0644)
		SaveHistory(path)
		// Bump mtime of just-written history file so sort is deterministic.
		entries, _ := os.ReadDir(*HistoryDir)
		for _, e := range entries {
			full := filepath.Join(*HistoryDir, e.Name())
			mtime := time.Now().Add(time.Duration(i) * time.Minute)
			os.Chtimes(full, mtime, mtime)
		}
	}
	versions, err := ListHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions after rotation, got %d", len(versions))
	}
}

// ========== ReadHistoryVersion ==========

func TestReadHistoryVersionInvalid(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	path := filepath.Join(confDir, "hosts.conf")
	os.WriteFile(path, []byte("x\n"), 0644)
	if _, err := ReadHistoryVersion(path, "../escape"); err == nil {
		t.Fatal("expected error for invalid version")
	}
	if _, err := ReadHistoryVersion(path, "not-a-date"); err == nil {
		t.Fatal("expected error for non-date version")
	}
}

// ========== ListHistory ==========

func TestListHistorySortedNewestFirst(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	path := filepath.Join(confDir, "hosts.conf")
	os.WriteFile(path, []byte("a\n"), 0644)
	SaveHistory(path)
	os.Chtimes(filepath.Join(*HistoryDir, historyFilePrefix(path)+firstVersion(t, path)+".bak"), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	os.WriteFile(path, []byte("b\n"), 0644)
	SaveHistory(path)
	v, err := ListHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(v))
	}
	if v[0].Version < v[1].Version {
		t.Errorf("expected newest first, got %q before %q", v[0].Version, v[1].Version)
	}
}

// firstVersion returns the single stored version id for path (test helper).
func firstVersion(t *testing.T, path string) string {
	t.Helper()
	v, err := ListHistory(path)
	if err != nil || len(v) != 1 {
		t.Fatalf("firstVersion: %v (%d)", err, len(v))
	}
	return v[0].Version
}

// ========== UnifiedDiff ==========

func TestUnifiedDiffAddsAndRemoves(t *testing.T) {
	a := "line1\nline2\nline3\n"
	bText := "line1\nlineX\nline3\nline4\n"
	d := UnifiedDiff(a, bText, "a", "b")
	if !strings.Contains(d, "-line2") {
		t.Errorf("diff missing removal: %s", d)
	}
	if !strings.Contains(d, "+lineX") || !strings.Contains(d, "+line4") {
		t.Errorf("diff missing additions: %s", d)
	}
	if strings.Contains(d, "+line1") || strings.Contains(d, "-line1") {
		t.Errorf("common line should not appear: %s", d)
	}
}

func TestUnifiedDiffEmptyA(t *testing.T) {
	d := UnifiedDiff("", "x\ny\n", "a", "b")
	if !strings.Contains(d, "+x") || !strings.Contains(d, "+y") {
		t.Errorf("expected both lines added: %s", d)
	}
}
