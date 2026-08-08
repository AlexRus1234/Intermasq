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
	"runtime"
	"strings"
	"testing"

	"intermask/internal/models"
)

// ========== IsSafePath ==========

// TestIsSafePath pins the A11 defense-in-depth layer (IsSafePath) DIRECTLY,
// independently of the handler-level substring filter. Every external HTTP
// traversal vector today carries "/" or "\", so the substring filter in
// getFileHandler/putFileHandler rejects it BEFORE IsSafePath-after-Join ever
// fires. There is no external HTTP vector that bypasses the substring filter
// but is caught by IsSafePath by design — IsSafePath exists precisely as the
// second gate in case the substring filter is ever weakened (e.g. to allow
// "/" in names) or a new call site forgets it. This test pins that second
// gate on its own.
//
// The "/etc/dnsmasq.d_evil/host.conf" case is the discriminating one: it
// catches a regression that drops the path-separator from the HasPrefix
// check (strings.HasPrefix(cleanPath, cleanDir+sep) → ...HasPrefix(_, cleanDir)),
// which would let a sibling directory whose name shares a prefix with
// ConfigDir pass as "inside". Mutate IsSafePath that way and this case fails.
//
// Migrated from the main package's dnsmasq_test.go (stage 5 of the
// modularization) so the test sits next to the implementation it pins.
func TestIsSafePath(t *testing.T) {
	*ConfigDir = "/etc/dnsmasq.d"
	tests := []struct {
		path     string
		expected bool
	}{
		{"/etc/dnsmasq.d/host.conf", true},
		{"/etc/dnsmasq.d/sub/host.conf", true},
		{"/etc/dnsmasq.d", true},
		{"/etc/passwd", false},
		{"/etc/dnsmasq.d_evil/host.conf", false},
		{"../etc/passwd", false},
	}

	for _, tt := range tests {
		result := IsSafePath(tt.path)
		if result != tt.expected {
			t.Errorf("IsSafePath(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

// ========== ReadFileRaw ==========

// TestReadFileRaw covers the happy path: a file inside ConfigDir is read
// verbatim.
func TestReadFileRaw(t *testing.T) {
	dir := newTestDir(t)
	path := filepath.Join(dir, "raw.conf")
	os.WriteFile(path, []byte("server=1.2.3.4\n"), 0644)
	content, err := ReadFileRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "server=1.2.3.4\n" {
		t.Errorf("unexpected content: %q", content)
	}
}

// TestReadFileRawUnsafePath confirms that a path outside ConfigDir is
// refused with os.ErrPermission (the chokepoint guards every reader).
func TestReadFileRawUnsafePath(t *testing.T) {
	newTestDir(t)
	_, err := ReadFileRaw("/etc/passwd")
	if err != os.ErrPermission {
		t.Errorf("expected ErrPermission, got %v", err)
	}
}

// TestReadFileRawNotExist surfaces the os.ReadFile error for a missing
// file (handler maps this to a 404 file_not_found response).
func TestReadFileRawNotExist(t *testing.T) {
	newTestDir(t)
	path := filepath.Join(*ConfigDir, "nope.conf")
	_, err := ReadFileRaw(path)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ========== WriteFileRaw ==========

// TestWriteFileRaw exercises the unsafe-path rejection branch. The
// happy-path write is covered by the Linux-only TestWriteConfigWithTest_*
// (it requires a working dnsmasq binary, which is unavailable on Windows
// test hosts). The .bak creation is also covered by TestWriteFileRaw below
// at the main-package level — here we only pin the permission gate.
func TestWriteFileRaw(t *testing.T) {
	dir := newTestDir(t)
	*HistoryDir = t.TempDir()
	*HistoryDepth = 5
	path := filepath.Join(dir, "writetest.conf")
	os.WriteFile(path, []byte("old\n"), 0644)
	// WriteFileRaw runs dnsmasq --test; on Windows dnsmasq is not installed
	// so the test fails AFTER the .bak is taken. The .bak must exist
	// regardless of test outcome — that is what this assertion checks.
	_ = WriteFileRaw(path, []byte("server=8.8.8.8\n"))
	_, err := os.Stat(path + ".bak")
	if os.IsNotExist(err) {
		t.Error(".bak should exist even if dnsmasq --test fails")
	}
}

// TestWriteFileRawUnsafePath confirms the permission gate rejects writes
// outside ConfigDir without invoking dnsmasq.
func TestWriteFileRawUnsafePath(t *testing.T) {
	newTestDir(t)
	err := WriteFileRaw("/etc/passwd", []byte("x"))
	if err != os.ErrPermission {
		t.Errorf("expected ErrPermission, got %v", err)
	}
}

// ========== EnsureAliasesFile ==========

// TestEnsureAliasesFile covers all three branches:
//  1. Path traversal attempt → ErrPermission.
//  2. New file inside ConfigDir → created with header.
//  3. Already exists → no-op (preserve prior content).
func TestEnsureAliasesFile(t *testing.T) {
	dir := newTestDir(t)

	// 1. Path traversal attempt → ErrPermission.
	unsafe := filepath.Join(dir, "..", "escape.conf")
	if err := EnsureAliasesFile(unsafe); err != os.ErrPermission {
		t.Fatalf("expected os.ErrPermission for unsafe path, got %v", err)
	}

	// 2. New file inside ConfigDir → created with header.
	good := filepath.Join(dir, "aliases.conf")
	if err := EnsureAliasesFile(good); err != nil {
		t.Fatalf("EnsureAliasesFile err: %v", err)
	}
	data, err := os.ReadFile(good)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "# DNS aliases") {
		t.Errorf("expected header comment, got: %q", string(data))
	}

	// 3. Already exists → no-op (preserve prior content).
	stamped := []byte("address=/existing/x\n")
	if err := os.WriteFile(good, stamped, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAliasesFile(good); err != nil {
		t.Fatalf("EnsureAliasesFile on existing err: %v", err)
	}
	after, _ := os.ReadFile(good)
	if string(after) != string(stamped) {
		t.Errorf("existing file modified: before %q, after %q", stamped, after)
	}
}

// ========== RemoveAliasLine ==========

// TestRemoveAliasLine verifies the address= variant: only the matching A
// record is removed; CNAME and unrelated A records stay intact.
func TestRemoveAliasLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	content := []byte("address=/nas.lan/192.168.1.10\ncname=wiki,nas.lan\naddress=/other/10.0.0.1\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveAliasLine(path, "A", "nas.lan")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected removal")
	}
	out, _ := os.ReadFile(path)
	s := string(out)
	if strings.Contains(s, "address=/nas.lan/") {
		t.Errorf("nas.lan not removed:\n%s", s)
	}
	if !strings.Contains(s, "cname=wiki,nas.lan") {
		t.Errorf("cname should be preserved:\n%s", s)
	}
	if !strings.Contains(s, "address=/other/10.0.0.1") {
		t.Errorf("other A should be preserved:\n%s", s)
	}
}

// TestRemoveAliasLineNotFound returns removed=false (not an error) when the
// requested type+domain is not present in the file.
func TestRemoveAliasLineNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	if err := os.WriteFile(path, []byte("address=/nas.lan/192.168.1.10\n"), 0644); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveAliasLine(path, "A", "missing.lan")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("expected no removal for missing domain")
	}
}

// TestRemoveAliasLinePTR exercises the ptr-record= variant: the matching PTR
// is removed while unrelated directives are preserved.
func TestRemoveAliasLinePTR(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	content := []byte("ptr-record=10.1.168.192.in-addr.arpa,nas.lan\naddress=/other/10.0.0.1\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveAliasLine(path, "PTR", "10.1.168.192.in-addr.arpa")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected PTR line to be removed")
	}
	out, _ := os.ReadFile(path)
	if strings.Contains(string(out), "ptr-record=") {
		t.Errorf("PTR not removed:\n%s", out)
	}
	if !strings.Contains(string(out), "address=/other/10.0.0.1") {
		t.Errorf("A record should be preserved:\n%s", out)
	}
}

// TestRemoveAliasLineTXT exercises the txt-record= variant.
func TestRemoveAliasLineTXT(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	content := []byte("txt-record=nas.lan,v=spf1 -all\ncname=wiki,nas.lan\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveAliasLine(path, "TXT", "nas.lan")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected TXT line to be removed")
	}
	out, _ := os.ReadFile(path)
	if strings.Contains(string(out), "txt-record=") {
		t.Errorf("TXT not removed:\n%s", out)
	}
	if !strings.Contains(string(out), "cname=wiki,nas.lan") {
		t.Errorf("CNAME should be preserved:\n%s", out)
	}
}

// ========== AppendHostLine / AppendAliasLine ==========

// TestAppendHostLine_PreservesExistingContent is the happy path: existing
// lines stay intact and the new line is appended on its own line.
func TestAppendHostLine_PreservesExistingContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.conf")
	original := []byte("dhcp-host=aa:bb:cc:dd:ee:01,h1,10.0.0.1\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := AppendHostLine(path, models.HostEntry{Mac: "aa:bb:cc:dd:ee:02", Hostname: "h2", Ip: "10.0.0.2"}); err != nil {
		t.Fatalf("AppendHostLine: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.HasPrefix(s, "dhcp-host=aa:bb:cc:dd:ee:01,h1,10.0.0.1\n") {
		t.Errorf("existing line lost:\n%s", s)
	}
	if !strings.Contains(s, "dhcp-host=aa:bb:cc:dd:ee:02,h2,10.0.0.2") {
		t.Errorf("new line not appended:\n%s", s)
	}
}

// TestAppendAliasLine_PreservesExistingContent is the alias happy path.
func TestAppendAliasLine_PreservesExistingContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	original := []byte("address=/a.test/10.0.0.1\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := AppendAliasLine(path, models.DnsAliasEntry{Type: "CNAME", Domain: "b.test", Target: "a.test"}); err != nil {
		t.Fatalf("AppendAliasLine: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.HasPrefix(s, "address=/a.test/10.0.0.1\n") {
		t.Errorf("existing line lost:\n%s", s)
	}
	if !strings.Contains(s, "cname=b.test,a.test") {
		t.Errorf("new line not appended:\n%s", s)
	}
}

// TestAppendHostLine_ReadErrorPreservesData is the regression test for the
// data-loss bug: when ReadFile fails with a non-IsNotExist error, the file
// must NOT be rewritten with just the new line. Reproduced by making the
// file write-only (mode 0o200) so ReadFile is denied while WriteFile is still
// allowed — exactly the "transient read error" footgun. Skips on Windows
// (no unix permission bits) and when reads bypass permissions (e.g. root).
func TestAppendHostLine_ReadErrorPreservesData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits not honoured on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.conf")
	original := []byte("dhcp-host=aa:bb:cc:dd:ee:01,h1,10.0.0.1\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o200); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0644) })

	// Self-check: if reads still succeed we cannot reproduce the read-error
	// path (e.g. running as root) — skip rather than pass vacuously.
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("running with read bypass (root?) — cannot simulate read error")
	}

	err := AppendHostLine(path, models.HostEntry{Mac: "aa:bb:cc:dd:ee:02", Ip: "10.0.0.2"})
	if err == nil {
		_ = os.Chmod(path, 0644)
		t.Fatal("expected the read error to be propagated, got nil")
	}
	// Restore readability and assert the original content survived.
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("file must be left untouched on read failure;\n got: %q\nwant: %q", got, original)
	}
}

// TestAppendAliasLine_ReadErrorPreservesData mirrors the host regression test
// for AppendAliasLine (same data-loss footgun at write.go).
func TestAppendAliasLine_ReadErrorPreservesData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits not honoured on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	original := []byte("address=/a.test/10.0.0.1\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o200); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0644) })

	if _, err := os.ReadFile(path); err == nil {
		t.Skip("running with read bypass (root?) — cannot simulate read error")
	}

	err := AppendAliasLine(path, models.DnsAliasEntry{Type: "CNAME", Domain: "b.test", Target: "a.test"})
	if err == nil {
		_ = os.Chmod(path, 0644)
		t.Fatal("expected the read error to be propagated, got nil")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("file must be left untouched on read failure;\n got: %q\nwant: %q", got, original)
	}
}
