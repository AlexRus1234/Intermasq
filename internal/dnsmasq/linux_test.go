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

// Coverage sweep block B (логи/Coverage_sweep.md §2.B): Linux-gated tests
// that exercise dnsmasq-dependent success paths by injecting a fake
// `dnsmasq` shell-script via bins.SetPathForTest. On Windows the shebang
// script is not executable by `os/exec`, so every test here skips on
// runtime.GOOS == "windows".
//
// Migrated from package main's linux_test.go (stage 5 of the
// modularization). The handler-level wiring tests
// (TestPutFileHandler_*, TestUpdateConfigHandler_*, TestRestoreBackupHandler_*,
// TestReload*, TestHistoryRestoreHandler_Success) STAY in package main
// because they exercise handler code which lives there; they keep their own
// copies of the fake-dnsmasq helpers (see /linux_test.go in main).

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withHistoryDir points *HistoryDir at a temp dir for the duration of the
// test. Restores the previous value on cleanup.
func withHistoryDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := *HistoryDir
	*HistoryDir = dir
	t.Cleanup(func() { *HistoryDir = orig })
	return dir
}

// ===== T-B.1 WriteConfigWithTest =====

func TestWriteConfigWithTest_Success(t *testing.T) {
	fakeDnsmasq(t, 0)
	dir := newTestDir(t)
	path := filepath.Join(dir, "x.conf")
	if err := os.WriteFile(path, []byte("# old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfigWithTest(path, []byte("# new\ndomain=lan\n")); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Contains(got, []byte("domain=lan")) {
		t.Errorf("file not updated: %q", got)
	}
}

func TestWriteConfigWithTest_TestFailRollback(t *testing.T) {
	fakeDnsmasq(t, 1)
	dir := newTestDir(t)
	path := filepath.Join(dir, "x.conf")
	orig := []byte("# preserved\n")
	if err := os.WriteFile(path, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	err := WriteConfigWithTest(path, []byte("# would-be-bad\n"))
	if err == nil || !strings.HasPrefix(err.Error(), "dnsmasq_test_failed") {
		t.Fatalf("expected dnsmasq_test_failed, got %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, orig) {
		t.Errorf("rollback failed: file content changed (got %q, want %q)", got, orig)
	}
}

// TestWriteConfigWithTest_StrictFakeRejectsInvalid exercises the full
// WriteConfigWithTest wiring WITH content validation. The plain fakeDnsmasq
// (`exit 0`) accepts any garbage, so TestWriteConfigWithTest_TestFailRollback
// above only proves the rollback plumbing fires when dnsmasq exits non-zero —
// it cannot prove dnsmasq was pointed at the just-written file. fakeDnsmasqStrict
// actually reads --conf-file=<path> and rejects the `# INVALID` marker, so a
// pass here means: the content was written, dnsmasq was invoked on THAT file,
// and the rejection correctly surfaced as `dnsmasq_test_failed` with rollback.
// The accept case proves the strict fake does not over-reject valid content.
func TestWriteConfigWithTest_StrictFakeRejectsInvalid(t *testing.T) {
	fakeDnsmasqStrict(t)
	dir := newTestDir(t)
	path := filepath.Join(dir, "strict.conf")

	// Valid content (no marker) → accepted, file updated.
	valid := []byte("# valid\ndomain=lan\n")
	if err := WriteConfigWithTest(path, valid); err != nil {
		t.Fatalf("valid content rejected: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Contains(got, []byte("domain=lan")) {
		t.Errorf("valid content not written: %q", got)
	}

	// Invalid content (marker present) → dnsmasq_test_failed + rollback to valid.
	orig := valid
	invalid := []byte("# INVALID\ndhcp-host=not-checked\n")
	err := WriteConfigWithTest(path, invalid)
	if err == nil {
		t.Fatal("expected dnsmasq_test_failed for # INVALID marker, got nil")
	}
	if !strings.HasPrefix(err.Error(), "dnsmasq_test_failed") {
		t.Fatalf("expected dnsmasq_test_failed prefix, got: %v", err)
	}
	got, _ = os.ReadFile(path)
	if !bytes.Equal(got, orig) {
		t.Errorf("rollback failed: file changed after rejection (got %q, want %q)", got, orig)
	}
}

// ===== T-B.2 RestoreHistoryVersion =====

// newestHistoryVersion reads HistoryDir via ListHistory and returns the
// identifier of the most recent stored version for filePath (ListHistory
// already sorts newest-first). Fatal if no version is present.
func newestHistoryVersion(t *testing.T, filePath string) string {
	t.Helper()
	versions, err := ListHistory(filePath)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("no history versions produced")
	}
	return versions[0].Version
}

func TestRestoreHistoryVersion_Success(t *testing.T) {
	fakeDnsmasq(t, 0)
	dir := newTestDir(t)
	withHistoryDir(t)
	path := filepath.Join(dir, "x.conf")
	v1 := []byte("domain=lan\n")
	if err := os.WriteFile(path, v1, 0o644); err != nil {
		t.Fatal(err)
	}
	SaveHistory(path)
	version := newestHistoryVersion(t, path)
	// Now mutate the file, then restore the snapshot.
	if err := os.WriteFile(path, []byte("domain=other.lan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RestoreHistoryVersion(path, version); err != nil {
		t.Fatalf("restore err: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, v1) {
		t.Errorf("restore mismatch: got %q, want %q", got, v1)
	}
}

func TestRestoreHistoryVersion_TestFailRollback(t *testing.T) {
	fakeDnsmasq(t, 1)
	dir := newTestDir(t)
	withHistoryDir(t)
	path := filepath.Join(dir, "x.conf")
	v1 := []byte("domain=lan\n")
	if err := os.WriteFile(path, v1, 0o644); err != nil {
		t.Fatal(err)
	}
	SaveHistory(path)
	version := newestHistoryVersion(t, path)
	mutated := []byte("domain=mutated\n")
	if err := os.WriteFile(path, mutated, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RestoreHistoryVersion(path, version); err == nil || !strings.HasPrefix(err.Error(), "dnsmasq_test_failed") {
		t.Fatalf("expected dnsmasq_test_failed, got %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, mutated) {
		t.Errorf("expected rollback to mutated content (pre-restore), got %q", got)
	}
}
