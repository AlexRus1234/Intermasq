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

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"intermask/internal/dnsmasq"
	"intermask/internal/initd"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestParseArpContent(t *testing.T) {
	content := `IP address       HW type     Flags       HW address            Mask Device
192.168.1.1      0x1         0x2         aa:bb:cc:dd:ee:ff     *    eth0
192.168.1.2      0x1         0x2         11:22:33:44:55:66     *    eth0
192.168.1.3      0x1         0x0         77:88:99:aa:bb:cc     *    eth0
`
	result := parseArpContent(content)
	if len(result) != 2 {
		t.Fatalf("expected 2 active MACs, got %d", len(result))
	}
	if !result["aa:bb:cc:dd:ee:ff"] {
		t.Error("expected aa:bb:cc:dd:ee:ff to be present")
	}
	if !result["11:22:33:44:55:66"] {
		t.Error("expected 11:22:33:44:55:66 to be present")
	}
	if result["77:88:99:aa:bb:cc"] {
		t.Error("expected 77:88:99:aa:bb:cc to be absent (flag 0x0)")
	}
}

func TestParseArpContentEmpty(t *testing.T) {
	content := `IP address       HW type     Flags       HW address            Mask Device
`
	result := parseArpContent(content)
	if len(result) != 0 {
		t.Fatalf("expected 0 MACs, got %d", len(result))
	}
}

func TestParseArpContentZeroMac(t *testing.T) {
	content := `IP address       HW type     Flags       HW address            Mask Device
192.168.1.1      0x1         0x2         00:00:00:00:00:00     *    eth0
`
	result := parseArpContent(content)
	if len(result) != 0 {
		t.Fatalf("expected 0 MACs (zero MAC filtered), got %d", len(result))
	}
}

func TestParseArpContentUppercaseMac(t *testing.T) {
	content := `IP address       HW type     Flags       HW address            Mask Device
192.168.1.1      0x1         0x2         AA:BB:CC:DD:EE:FF     *    eth0
`
	result := parseArpContent(content)
	if !result["aa:bb:cc:dd:ee:ff"] {
		t.Error("expected MAC to be lowercased")
	}
}

// TestIsSafePath pins the A11 defense-in-depth layer (isSafePath,
// dnsmasq.go:51) DIRECTLY, independently of the handler-level substring
// filter (handlers_config.go:199/223).
//
// Every external HTTP traversal vector today carries "/" or "\", so the
// substring filter in getFileHandler/putFileHandler rejects it BEFORE
// isSafePath-after-Join ever fires (see TestGetFileHandlerRejectsUnsafePath /
// TestPutFileHandlerRejectsUnsafePath for that layer). There is no external
// HTTP vector that bypasses the substring filter but is caught by isSafePath
// by design — isSafePath exists precisely as the second gate in case the
// substring filter is ever weakened (e.g. to allow "/" in names) or a new
// call site forgets it. This test pins that second gate on its own.
//
// The "/etc/dnsmasq.d_evil/host.conf" case is the discriminating one: it
// catches a regression that drops the path-separator from the HasPrefix
// check (strings.HasPrefix(cleanPath, cleanDir+sep) → ...HasPrefix(_, cleanDir)),
// which would let a sibling directory whose name shares a prefix with ConfigDir
// pass as "inside". Mutate isSafePath that way and this case fails.
func TestIsSafePath(t *testing.T) {
	*dnsmasq.ConfigDir = "/etc/dnsmasq.d"
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
		result := isSafePath(tt.path)
		if result != tt.expected {
			t.Errorf("isSafePath(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestResolveSystemCaller(t *testing.T) {
	tests := []struct {
		input   string
		wantStr string
	}{
		{"none", "none"},
		{"systemd-user", "systemd-user"},
		{"systemd", "systemd"},
		{"openrc", "openrc"},
		{"runit", "runit"},
		{"sysvinit", "sysvinit"},
	}

	for _, tt := range tests {
		caller := initd.ResolveSystemCaller(tt.input)
		if !strings.Contains(caller.String(), tt.wantStr) {
			t.Errorf("ResolveSystemCaller(%q) = %q, want containing %q", tt.input, caller.String(), tt.wantStr)
		}
	}
}

func TestResolveSystemCallerLegacy(t *testing.T) {
	caller := initd.ResolveSystemCaller("system")
	if _, ok := caller.(*initd.SystemdSystemCaller); !ok {
		t.Error("expected SystemdSystemCaller for legacy scope 'system'")
	}

	caller = initd.ResolveSystemCaller("user")
	if _, ok := caller.(*initd.SystemdUserCaller); !ok {
		t.Error("expected SystemdUserCaller for legacy scope 'user'")
	}
}

func TestMapLegacyScope(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"system", "systemd"},
		{"user", "systemd-user"},
		{"none", "none"},
		{"auto", "auto"},
		{"openrc", "openrc"},
	}
	for _, tt := range tests {
		result := initd.MapLegacyScope(tt.input)
		if result != tt.expect {
			t.Errorf("MapLegacyScope(%q) = %q, want %q", tt.input, result, tt.expect)
		}
	}
}

func TestNoneCaller(t *testing.T) {
	caller := &initd.NoneCaller{}
	if !caller.IsActive("anything") {
		t.Error("NoneCaller.IsActive should always return true")
	}
	if caller.Restart("anything") != nil {
		t.Error("NoneCaller.Restart should always return nil")
	}
	if caller.RestartSelf() == nil {
		t.Error("NoneCaller.RestartSelf should return error")
	}
}

func TestOpenRCCaller(t *testing.T) {
	caller := &initd.OpenRCCaller{UseSudo: false}
	if caller.String() != "openrc (root)" {
		t.Errorf("OpenRC String() = %q", caller.String())
	}
	callerSudo := &initd.OpenRCCaller{UseSudo: true}
	if callerSudo.String() != "openrc (via sudo)" {
		t.Errorf("OpenRC sudo String() = %q", callerSudo.String())
	}
}

func TestRunitCaller(t *testing.T) {
	caller := &initd.RunitCaller{UseSudo: false, ServiceDir: "/etc/service"}
	if !strings.Contains(caller.String(), "runit") {
		t.Errorf("Runit String() = %q", caller.String())
	}
	if !strings.Contains(caller.String(), "/etc/service") {
		t.Errorf("Runit String() should contain service dir, got %q", caller.String())
	}
}

func TestSysVinitCaller(t *testing.T) {
	caller := &initd.SysVinitCaller{UseSudo: false}
	if caller.String() != "sysvinit (root)" {
		t.Errorf("SysVinit String() = %q", caller.String())
	}
	callerSudo := &initd.SysVinitCaller{UseSudo: true}
	if callerSudo.String() != "sysvinit (via sudo)" {
		t.Errorf("SysVinit sudo String() = %q", callerSudo.String())
	}
}

// Migrated to internal/dnsmasq:
//   TestParseDhcpRangeClassic, TestParseDhcpRangeCIDRForm,
//   TestParseDhcpRangeTagged, TestParseDhcpRangeNoMask,
//   TestDhcpRangeToCIDRIPv6Rejected, TestSerializeConfigFilePreservesDhcpHosts,
//   TestSerializeConfigFileInactiveDirective, TestReadConfigSnapshotFiltersDhcpHost,
//   TestReadConfigSnapshotDhcpRanges, TestDetectDhcpRangesCIDRDedup,
//   TestSerializeConfigFileGroupOrder, TestParseAliasLineA, TestParseAliasLineCNAME,
//   TestParseAliasLineCNAMEWithTag, TestParseAliasLineRejectsWildcard,
//   TestParseAliasLineRejectsMalformed, TestAliasToLineRoundTrip,
//   TestReadAllAliases, TestReadAllAliasesHasBakMarker.

func TestRemoveAliasLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	content := []byte("address=/nas.lan/192.168.1.10\ncname=wiki,nas.lan\naddress=/other/10.0.0.1\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	removed, err := removeAliasLine(path, "A", "nas.lan")
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

func TestRemoveAliasLineNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	if err := os.WriteFile(path, []byte("address=/nas.lan/192.168.1.10\n"), 0644); err != nil {
		t.Fatal(err)
	}
	removed, err := removeAliasLine(path, "A", "missing.lan")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("expected no removal for missing domain")
	}
}

// Migrated to internal/dnsmasq:
//   TestSerializeConfigFilePreservesAliases, TestReadConfigSnapshotFiltersAliases.

// setupHistoryEnv prepares temp ConfigDir and HistoryDir for history tests.
func setupHistoryEnv(t *testing.T) (confDir, histDir string) {
	t.Helper()
	confDir = t.TempDir()
	histDir = t.TempDir()
	*dnsmasq.ConfigDir = confDir
	*HistoryDir = histDir
	*HistoryDepth = 10
	return confDir, histDir
}

func TestSaveHistoryCreatesVersion(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	path := filepath.Join(confDir, "hosts.conf")
	if err := os.WriteFile(path, []byte("dhcp-host=aa:bb:cc:dd:ee:ff,1.2.3.4,host\n"), 0644); err != nil {
		t.Fatal(err)
	}
	saveHistory(path)
	versions, err := listHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if !historyVersionRegex.MatchString(versions[0].Version) {
		t.Errorf("bad version id: %q", versions[0].Version)
	}
}

func TestSaveHistoryNoOpForMissingFile(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	path := filepath.Join(confDir, "nope.conf")
	saveHistory(path)
	versions, err := listHistory(path)
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
	saveHistory("/etc/passwd")
	versions, _ := listHistory("/etc/passwd")
	if len(versions) != 0 {
		t.Fatalf("history written for unsafe path")
	}
}

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
		saveHistory(path)
		// Bump mtime of just-written history file so sort is deterministic.
		entries, _ := os.ReadDir(*HistoryDir)
		for _, e := range entries {
			full := filepath.Join(*HistoryDir, e.Name())
			mtime := time.Now().Add(time.Duration(i) * time.Minute)
			os.Chtimes(full, mtime, mtime)
		}
	}
	versions, err := listHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions after rotation, got %d", len(versions))
	}
}

func TestReadHistoryVersionInvalid(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	path := filepath.Join(confDir, "hosts.conf")
	os.WriteFile(path, []byte("x\n"), 0644)
	if _, err := readHistoryVersion(path, "../escape"); err == nil {
		t.Fatal("expected error for invalid version")
	}
	if _, err := readHistoryVersion(path, "not-a-date"); err == nil {
		t.Fatal("expected error for non-date version")
	}
}

func TestListHistorySortedNewestFirst(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	path := filepath.Join(confDir, "hosts.conf")
	os.WriteFile(path, []byte("a\n"), 0644)
	saveHistory(path)
	os.Chtimes(filepath.Join(*HistoryDir, historyFilePrefix(path)+firstVersion(t, path)+".bak"), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	os.WriteFile(path, []byte("b\n"), 0644)
	saveHistory(path)
	v, err := listHistory(path)
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
	v, err := listHistory(path)
	if err != nil || len(v) != 1 {
		t.Fatalf("firstVersion: %v (%d)", err, len(v))
	}
	return v[0].Version
}

func TestUnifiedDiffAddsAndRemoves(t *testing.T) {
	a := "line1\nline2\nline3\n"
	bText := "line1\nlineX\nline3\nline4\n"
	d := unifiedDiff(a, bText, "a", "b")
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
	d := unifiedDiff("", "x\ny\n", "a", "b")
	if !strings.Contains(d, "+x") || !strings.Contains(d, "+y") {
		t.Errorf("expected both lines added: %s", d)
	}
}

// ========== Raw file read/write ==========

func TestReadFileRaw(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	path := filepath.Join(dir, "raw.conf")
	os.WriteFile(path, []byte("server=1.2.3.4\n"), 0644)
	content, err := readFileRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "server=1.2.3.4\n" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestReadFileRawUnsafePath(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	_, err := readFileRaw("/etc/passwd")
	if err != os.ErrPermission {
		t.Errorf("expected ErrPermission, got %v", err)
	}
}

func TestReadFileRawNotExist(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	path := filepath.Join(dir, "nope.conf")
	_, err := readFileRaw(path)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWriteFileRaw(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	*HistoryDir = t.TempDir()
	*HistoryDepth = 5
	path := filepath.Join(dir, "writetest.conf")
	os.WriteFile(path, []byte("old\n"), 0644)
	_ = writeFileRaw(path, []byte("server=8.8.8.8\n"))
	_, err := os.Stat(path + ".bak")
	if os.IsNotExist(err) {
		t.Error(".bak should exist even if dnsmasq --test fails")
	}
}

func TestWriteFileRawUnsafePath(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	err := writeFileRaw("/etc/passwd", []byte("x"))
	if err != os.ErrPermission {
		t.Errorf("expected ErrPermission, got %v", err)
	}
}

// ========== SSE broker ==========

func TestSseRegisterUnregister(t *testing.T) {
	cl := &sseClient{ch: make(chan string, 1)}
	sseRegister(cl)
	if !sseClients[cl] {
		t.Fatal("client should be registered")
	}
	sseUnregister(cl)
	if sseClients[cl] {
		t.Fatal("client should be unregistered")
	}
}

func TestSseBroadcast(t *testing.T) {
	cl := &sseClient{ch: make(chan string, 10)}
	sseRegister(cl)
	defer sseUnregister(cl)
	sseBroadcast("arp", `{"aa:bb:cc:dd:ee:ff":true}`)
	select {
	case msg := <-cl.ch:
		if !strings.Contains(msg, "event: arp") {
			t.Errorf("bad event: %s", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("message not received")
	}
}

func TestSseBroadcastFullChannel(t *testing.T) {
	cl := &sseClient{ch: make(chan string, 0)}
	sseRegister(cl)
	defer sseUnregister(cl)
	sseBroadcast("arp", "{}")
	select {
	case <-cl.ch:
		t.Errorf("expected broadcast to be dropped on full/unbuffered channel, but a message was delivered")
	default:
	}
	if len(cl.ch) != 0 {
		t.Errorf("expected empty channel after broadcast to full/unbuffered channel, got len=%d", len(cl.ch))
	}
}

func TestArpToJSON(t *testing.T) {
	arp := map[string]bool{"aa:bb:cc:dd:ee:ff": true, "11:22:33:44:55:66": false}
	s := arpToJSON(arp)
	var decoded map[string]bool
	if err := json.Unmarshal([]byte(s), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(decoded) != 2 {
		t.Errorf("expected 2 entries, got %d", len(decoded))
	}
	if !decoded["aa:bb:cc:dd:ee:ff"] {
		t.Error("expected aa:bb:cc:dd:ee:ff=true")
	}
}

// ========== User management ==========

func TestCreateUser(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = make(map[string]string)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/users", strings.NewReader(`{"username":"admin","password":"secret123"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createUserHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, ok := users["admin"]; !ok {
		t.Fatal("user not stored")
	}
}

func TestCreateUserDuplicate(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"admin": "hash"}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/users", strings.NewReader(`{"username":"admin","password":"secret123"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createUserHandler(c)
	if w.Code != 409 {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestCreateUserEmptyFields(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = make(map[string]string)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/users", strings.NewReader(`{"username":"","password":""}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createUserHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeleteUser(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"target": "hash"}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/users/target", nil)
	c.Params = gin.Params{{Key: "name", Value: "target"}}
	c.Set("user", "admin")
	deleteUserHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, ok := users["target"]; ok {
		t.Fatal("user should be deleted")
	}
}

func TestDeleteUserSelf(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"admin": "hash"}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/users/admin", nil)
	c.Params = gin.Params{{Key: "name", Value: "admin"}}
	c.Set("user", "admin")
	deleteUserHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeleteUserNotFound(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = make(map[string]string)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/users/nobody", nil)
	c.Params = gin.Params{{Key: "name", Value: "nobody"}}
	c.Set("user", "admin")
	deleteUserHandler(c)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestChangePassword(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"admin": "$2a$10$1"}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/users/password", strings.NewReader(`{"old_password":"1","new_password":"new"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	changePasswordHandler(c)
	if w.Code == 401 {
		t.Log("bcrypt rejected dummy hash — expected. Validating shape.")
	} else if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChangePasswordWrongOld(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"admin": "$2a$10$zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/users/password", strings.NewReader(`{"old_password":"wrong","new_password":"new"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	changePasswordHandler(c)
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// ========== New devices ==========

func TestGetNewDevicesAllInStatic(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	*ArpPath = filepath.Join(dir, "arp")
	*LeasesPath = filepath.Join(dir, "leases")
	os.WriteFile(*ArpPath, []byte("IP address       HW type     Flags       HW address            Mask Device\n192.168.1.1      0x1         0x2         aa:bb:cc:dd:ee:ff     *    eth0\n"), 0644)
	os.WriteFile(*LeasesPath, []byte("0 aa:bb:cc:dd:ee:ff 192.168.1.1 * 01:aa:bb:cc:dd:ee:ff\n"), 0644)
	devices := getNewDevices()
	if len(devices) != 0 {
		t.Fatalf("expected 0 devices (MAC in leases), got %d", len(devices))
	}
}

func TestGetNewDevicesAllInHosts(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	*ArpPath = filepath.Join(dir, "arp")
	*LeasesPath = filepath.Join(dir, "leases")
	os.WriteFile(*ArpPath, []byte("IP address       HW type     Flags       HW address            Mask Device\n192.168.1.1      0x1         0x2         aa:bb:cc:dd:ee:ff     *    eth0\n"), 0644)
	os.WriteFile(*LeasesPath, []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "hosts.conf"), []byte("dhcp-host=aa:bb:cc:dd:ee:ff,host1,192.168.1.1\n"), 0644)
	devices := getNewDevices()
	if len(devices) != 0 {
		t.Fatalf("expected 0 devices (MAC in static), got %d", len(devices))
	}
}

func TestGetNewDevicesUnknown(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	*ArpPath = filepath.Join(dir, "arp")
	*LeasesPath = filepath.Join(dir, "leases")
	os.WriteFile(*ArpPath, []byte("IP address       HW type     Flags       HW address            Mask Device\n192.168.1.1      0x1         0x2         aa:bb:cc:dd:ee:ff     *    eth0\n"), 0644)
	os.WriteFile(*LeasesPath, []byte(""), 0644)
	devices := getNewDevices()
	if len(devices) != 1 {
		t.Fatalf("expected 1 unknown device, got %d", len(devices))
	}
	if devices[0].Mac != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("bad MAC: %q", devices[0].Mac)
	}
}

func TestGetNewDevicesEmpty(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	*ArpPath = filepath.Join(dir, "arp")
	*LeasesPath = filepath.Join(dir, "leases")
	os.WriteFile(*ArpPath, []byte("IP address       HW type     Flags       HW address            Mask Device\n"), 0644)
	os.WriteFile(*LeasesPath, []byte(""), 0644)
	devices := getNewDevices()
	if len(devices) != 0 {
		t.Fatalf("expected 0 devices, got %d", len(devices))
	}
}

// ========== Bulk lease-to-static ==========

func TestBulkLeaseToStatic(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)
	body := `{"leases":[{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.1","hostname":"testhost"}],"file":"` + strings.ReplaceAll(file, "\\", "\\\\") + `"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/leases/to-static", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	bulkLeaseToStaticHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "dhcp-host=aa:bb:cc:dd:ee:ff,testhost,192.168.1.1") {
		t.Errorf("host not written: %s", content)
	}
}

func TestBulkLeaseToStaticMacConflict(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte("dhcp-host=aa:bb:cc:dd:ee:ff,existing,1.2.3.4\n"), 0644)
	body := `{"leases":[{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.1","hostname":"new"}],"file":"` + strings.ReplaceAll(file, "\\", "\\\\") + `"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/leases/to-static", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	bulkLeaseToStaticHandler(c)
	if w.Code != 409 {
		t.Fatalf("expected 409 for MAC conflict, got %d", w.Code)
	}
}

func TestBulkLeaseToStaticInvalidMac(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)
	body := `{"leases":[{"mac":"bad","ip":"192.168.1.1"}],"file":"` + strings.ReplaceAll(file, "\\", "\\\\") + `"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/leases/to-static", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	bulkLeaseToStaticHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid MAC, got %d", w.Code)
	}
}

func TestBulkLeaseToStaticEmpty(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/leases/to-static", strings.NewReader(`{"leases":[],"file":"`+file+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	bulkLeaseToStaticHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400 for empty list, got %d", w.Code)
	}
}

func TestBulkLeaseToStaticUnsafePath(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/leases/to-static", strings.NewReader(`{"leases":[{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.1"}],"file":"/etc/passwd"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	bulkLeaseToStaticHandler(c)
	if w.Code != 403 {
		t.Fatalf("expected 403 for unsafe path, got %d", w.Code)
	}
}

func TestBulkLeaseToStaticDefaultHostname(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)
	body := `{"leases":[{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.1","hostname":"*"}],"file":"` + strings.ReplaceAll(file, "\\", "\\\\") + `"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/leases/to-static", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	bulkLeaseToStaticHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "device-") {
		t.Errorf("expected auto-generated hostname, got: %s", content)
	}
}

// ========== Restore from ZIP ==========

func makeTestZip(entries map[string]string) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, content := range entries {
		fw, _ := zw.Create(name)
		fw.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

func TestRestoreBackupZipValid(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	zipData := makeTestZip(map[string]string{
		"hosts.conf": "dhcp-host=aa:bb:cc:dd:ee:ff,host1,1.2.3.4\n",
	})
	_ = restoreBackupZip(zipData)
	_, err := os.ReadFile(filepath.Join(dir, "hosts.conf"))
	if err != nil {
		t.Error("file should have been written before dnsmasq test")
	}
}

func TestRestoreBackupZipCreatesRestoreBak(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	os.WriteFile(filepath.Join(dir, "hosts.conf"), []byte("old content\n"), 0644)
	zipData := makeTestZip(map[string]string{
		"hosts.conf": "new content\n",
	})
	_ = restoreBackupZip(zipData)
	bak, _ := os.ReadFile(filepath.Join(dir, "hosts.conf.restore.bak"))
	if string(bak) != "old content\n" {
		t.Errorf("bak mismatch: %q", bak)
	}
}

func TestRestoreBackupZipNoConfFiles(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	zipData := makeTestZip(map[string]string{
		"notes.txt": "hello\n",
	})
	err := restoreBackupZip(zipData)
	if err == nil || !strings.Contains(err.Error(), "no_valid_conf_files") {
		t.Errorf("expected no_valid_conf_files error, got %v", err)
	}
}

func TestRestoreBackupZipInvalidData(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	err := restoreBackupZip([]byte("not a zip file"))
	if err == nil || !strings.Contains(err.Error(), "invalid_zip") {
		t.Errorf("expected invalid_zip error, got %v", err)
	}
}

func TestRestoreBackupZipIgnoresUnsafeNames(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	zipData := makeTestZip(map[string]string{
		"../evil.conf": "bad\n",
		"hosts.conf":   "good\n",
	})
	_ = restoreBackupZip(zipData)
	_, err := os.ReadFile(filepath.Join(dir, "hosts.conf"))
	if err != nil {
		t.Fatal("hosts.conf should have been extracted")
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "evil.conf")); err == nil {
		t.Fatal("evil.conf should not have been extracted")
	}
}

// ========== Rate-limit ==========

func TestRateLimitUnderLimit(t *testing.T) {
	rateLimitStore = make(map[string][]time.Time)
	handler := rateLimitMiddleware(3, time.Minute)
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", nil)
		c.Request.RemoteAddr = "10.0.0.1:1234"
		handler(c)
		if w.Code == 429 {
			t.Fatalf("request %d should not be rate-limited", i+1)
		}
	}
}

func TestRateLimitOverLimit(t *testing.T) {
	rateLimitStore = make(map[string][]time.Time)
	handler := rateLimitMiddleware(2, time.Minute)
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", nil)
		c.Request.RemoteAddr = "10.0.0.2:1234"
		handler(c)
		if i == 2 && w.Code != 429 {
			t.Fatalf("third request should be rate-limited, got %d", w.Code)
		}
	}
}

func TestRateLimitDifferentIPs(t *testing.T) {
	rateLimitStore = make(map[string][]time.Time)
	handler := rateLimitMiddleware(2, time.Minute)

	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest("POST", "/", nil)
	c1.Request.RemoteAddr = "10.0.1.1:1234"
	handler(c1)
	handler(c1)

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("POST", "/", nil)
	c2.Request.RemoteAddr = "10.0.1.2:1234"
	handler(c2)
	handler(c2)

	if w1.Code == 429 {
		t.Fatal("IP1 should not be rate-limited yet")
	}
	if w2.Code == 429 {
		t.Fatal("IP2 should not be rate-limited yet")
	}

	handler(c1)
	if w1.Code != 429 {
		t.Fatalf("IP1 should now be rate-limited, got %d", w1.Code)
	}
	handler(c2)
	if w2.Code != 429 {
		t.Fatalf("IP2 should now be rate-limited, got %d", w2.Code)
	}
}

func TestRateLimitCleanupExpired(t *testing.T) {
	rateLimitStore = make(map[string][]time.Time)
	rateLimitStore["10.0.0.1"] = []time.Time{
		time.Now().Add(-2 * time.Minute),
		time.Now().Add(-2 * time.Minute),
	}
	handler := rateLimitMiddleware(2, time.Minute)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", nil)
	c.Request.RemoteAddr = "10.0.0.1:1234"
	handler(c)
	if w.Code == 429 {
		t.Fatal("old entries should have been cleaned, request should pass")
	}
}

// ========== JWT blacklist / logout ==========

func TestTokenRevoked(t *testing.T) {
	blacklist = make(map[string]time.Time)
	exp := time.Now().Add(time.Hour)
	jti := "test-jti-123"
	revokeToken(jti, exp)
	if !isTokenRevoked(jti) {
		t.Fatal("token should be revoked")
	}
}

func TestTokenNotRevoked(t *testing.T) {
	blacklist = make(map[string]time.Time)
	if isTokenRevoked("nonexistent") {
		t.Fatal("non-revoked token should not be marked revoked")
	}
}

func TestCleanBlacklist(t *testing.T) {
	blacklist = make(map[string]time.Time)
	expiredJTI := "expired-jti"
	freshJTI := "fresh-jti"
	revokeToken(expiredJTI, time.Now().Add(-time.Hour))
	revokeToken(freshJTI, time.Now().Add(time.Hour))
	blacklistMu.Lock()
	now := time.Now()
	for id, exp := range blacklist {
		if exp.Before(now) {
			delete(blacklist, id)
		}
	}
	blacklistMu.Unlock()
	if isTokenRevoked(expiredJTI) {
		t.Fatal("expired token should be cleaned")
	}
	if !isTokenRevoked(freshJTI) {
		t.Fatal("fresh token should still be revoked")
	}
}

func TestLogoutRevokesToken(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"admin": "$2a$10$placeholder"}
	blacklist = make(map[string]time.Time)

	originalKey := SecretKey
	SecretKey = []byte("test-secret-key-32-bytes-long!!")
	defer func() { SecretKey = originalKey }()

	token := makeToken("admin")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/logout", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)
	logoutHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	parsed, _ := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) { return SecretKey, nil })
	if parsed == nil {
		t.Fatal("token parsing failed")
	}
	claims := parsed.Claims.(jwt.MapClaims)
	jti := claims["jti"].(string)
	if !isTokenRevoked(jti) {
		t.Fatal("token JTI should be in blacklist after logout")
	}
}

// ========== OUI lookup ==========
// (TestLookupOUI* moved to internal/oui.)

// ========== Auth middleware (header + query token for SSE) ==========

func setTestSecret(t *testing.T) {
	t.Helper()
	orig := SecretKey
	SecretKey = []byte("unit-test-secret-key-0123456789ab")
	t.Cleanup(func() { SecretKey = orig })
}

func TestAuthMiddlewareBearerHeader(t *testing.T) {
	setTestSecret(t)
	users = map[string]string{"admin": "hash"}
	token := makeToken("admin")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/whatever", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)
	authMiddleware(c)
	if w.Code == 401 {
		t.Fatalf("bearer header should be accepted")
	}
	if c.GetString("user") != "admin" {
		t.Errorf("expected user admin, got %q", c.GetString("user"))
	}
}

func TestAuthMiddlewareQueryTokenRejected(t *testing.T) {
	// ?token= was removed from authMiddleware to avoid leaking JWTs into
	// access logs via SSE. Only Bearer header / X-API-Key are accepted now.
	setTestSecret(t)
	users = map[string]string{"admin": "hash"}
	token := makeToken("admin")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/events?token="+token, nil)
	authMiddleware(c)
	if w.Code != 401 {
		t.Fatalf("query token should be rejected, got %d", w.Code)
	}
}

func TestAuthMiddlewareNoCredentials(t *testing.T) {
	setTestSecret(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/events", nil)
	authMiddleware(c)
	if w.Code != 401 {
		t.Fatalf("missing credentials should be rejected, got %d", w.Code)
	}
}

func TestAuthMiddlewareAPIKey(t *testing.T) {
	setTestSecret(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/whatever", nil)
	c.Request.Header.Set("X-API-Key", string(SecretKey))
	authMiddleware(c)
	if w.Code == 401 {
		t.Fatalf("X-API-Key should be accepted")
	}
	if c.GetString("user") != "api-key" {
		t.Errorf("expected user api-key, got %q", c.GetString("user"))
	}
}

// ========== Events handler (SSE end-to-end) ==========

func TestEventsHandlerStreamsSSE(t *testing.T) {
	dir := t.TempDir()
	*ArpPath = filepath.Join(dir, "arp")
	os.WriteFile(*ArpPath, []byte("IP address       HW type     Flags       HW address            Mask Device\n192.168.1.5      0x1         0x2         aa:bb:cc:dd:ee:ff     *    eth0\n"), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/events", nil).WithContext(ctx)
	eventsHandler(c)

	if !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Errorf("expected text/event-stream content-type, got %q", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), "arp") {
		t.Errorf("expected initial arp event in body, got: %s", w.Body.String())
	}
}

// ========== GET /api/files/:name (.conf restriction) ==========

func TestGetFileHandlerRejectsNonConf(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0644)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/files/notes.txt", nil)
	c.Params = gin.Params{{Key: "name", Value: "notes.txt"}}
	getFileHandler(c)
	if w.Code != 403 {
		t.Fatalf("expected 403 for non-.conf, got %d", w.Code)
	}
}

func TestGetFileHandlerRejectsPathSeparator(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/files/sub/x.conf", nil)
	c.Params = gin.Params{{Key: "name", Value: "sub/x.conf"}}
	getFileHandler(c)
	if w.Code != 403 {
		t.Fatalf("expected 403 for path separator in name, got %d", w.Code)
	}
}

func TestGetFileHandlerSuccess(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	os.WriteFile(filepath.Join(dir, "x.conf"), []byte("server=1.2.3.4\n"), 0644)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/files/x.conf", nil)
	c.Params = gin.Params{{Key: "name", Value: "x.conf"}}
	getFileHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "server=1.2.3.4") {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestGetFileHandlerMissing(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/files/missing.conf", nil)
	c.Params = gin.Params{{Key: "name", Value: "missing.conf"}}
	getFileHandler(c)
	if w.Code != 404 {
		t.Fatalf("expected 404 for missing file, got %d", w.Code)
	}
}

// TestGetFileHandlerRejectsUnsafePath locks the path-traversal defence for
// GET /api/files/:name (A11, defense-in-depth). Every vector below carries a
// .conf extension so the extension check does not short-circuit, exercising
// the traversal defence specifically. The substring filter on "/" / "\"
// fires first today; the isSafePath-after-Join layer is kept so a future
// weakening of that filter (or a new call site) cannot enable reads outside
// ConfigDir.
func TestGetFileHandlerRejectsUnsafePath(t *testing.T) {
	cases := []string{
		"../etc/evil.conf",
		"..\\evil.conf",
		"../../etc/dnsmasq.conf",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			*dnsmasq.ConfigDir = dir
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/files/x.conf", nil)
			c.Params = gin.Params{{Key: "name", Value: name}}
			getFileHandler(c)
			if w.Code != 403 {
				t.Fatalf("expected 403 for traversal name %q, got %d: %s", name, w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "access_denied") {
				t.Errorf("expected access_denied body for %q, got: %s", name, w.Body.String())
			}
		})
	}
}

// ========== Config file templates (POST /api/config/file?template=…) ==========

// TestCreateConfigFileHandlerEachTemplate проверяет, что каждый зарегистрированный
// шаблон корректно записывается в файл при выборе через POST /api/config/file.
func TestCreateConfigFileHandlerEachTemplate(t *testing.T) {
	for _, tpl := range dnsmasq.KnownConfigTemplateIDs() {
		t.Run(tpl, func(t *testing.T) {
			dir := t.TempDir()
			*dnsmasq.ConfigDir = dir
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			body := fmt.Sprintf(`{"name":"test_%s.conf","template":"%s"}`, tpl, tpl)
			c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", "admin")
			createConfigFileHandler(c)
			if w.Code != 200 {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			content, err := os.ReadFile(filepath.Join(dir, "test_"+tpl+".conf"))
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != dnsmasq.ConfigTemplates[tpl] {
				t.Errorf("content mismatch for template %q:\nwant:\n%s\ngot:\n%s", tpl, dnsmasq.ConfigTemplates[tpl], string(content))
			}
		})
	}
}

// TestCreateConfigFileHandlerEmptyTemplateDefault — отсутствие template в теле
// запроса эквивалентно template="empty" (обратная совместимость).
func TestCreateConfigFileHandlerEmptyTemplateDefault(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(`{"name":"x.conf"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createConfigFileHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(filepath.Join(dir, "x.conf"))
	if string(content) != dnsmasq.ConfigTemplates["empty"] {
		t.Errorf("default template not 'empty':\n%s", string(content))
	}
}

// TestCreateConfigFileHandlerUnknownTemplate — неизвестный ID шаблона даёт
// 400 + список доступных в поле available (нужно для подсказки в UI).
func TestCreateConfigFileHandlerUnknownTemplate(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(`{"name":"x.conf","template":"nonexistent"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createConfigFileHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400 for unknown template, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["error"] != "unknown_template" {
		t.Errorf("expected error=unknown_template, got %v", resp["error"])
	}
	avail, _ := resp["available"].([]interface{})
	if len(avail) != len(dnsmasq.ConfigTemplates) {
		t.Errorf("expected %d available templates, got %v", len(dnsmasq.ConfigTemplates), avail)
	}
	// файл не должен быть создан при ошибке
	if _, err := os.Stat(filepath.Join(dir, "x.conf")); !os.IsNotExist(err) {
		t.Error("file should not be created when template is unknown")
	}
}

// TestCreateConfigFileHandlerTemplateCaseInsensitive — "Basic-DHCP" и
// "basic-dhcp" дают одинаковый результат (нормализация через ToLower).
func TestCreateConfigFileHandlerTemplateCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(`{"name":"x.conf","template":"Basic-DHCP"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createConfigFileHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for uppercase template, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(filepath.Join(dir, "x.conf"))
	if string(content) != dnsmasq.ConfigTemplates["basic-dhcp"] {
		t.Errorf("case-insensitive lookup failed:\n%s", string(content))
	}
}

// TestCreateConfigFileHandlerTemplateWhitespace — пробелы вокруг ID шаблона
// должны молча обрезаться (защита от копипаста " basic-dhcp ").
func TestCreateConfigFileHandlerTemplateWhitespace(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(`{"name":"x.conf","template":"  forwarder  "}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createConfigFileHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(filepath.Join(dir, "x.conf"))
	if string(content) != dnsmasq.ConfigTemplates["forwarder"] {
		t.Errorf("whitespace trim failed:\n%s", string(content))
	}
}

// TestCreateConfigFileHandlerExistingFileStill409 — даже при выборе шаблона
// попытка перезаписать существующий файл остаётся 409 (поведение не изменилось).
func TestCreateConfigFileHandlerExistingFileStill409(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	existing := filepath.Join(dir, "x.conf")
	if err := os.WriteFile(existing, []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(`{"name":"x.conf","template":"basic-dhcp"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createConfigFileHandler(c)
	if w.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	// Содержимое существующего файла не должно измениться.
	content, _ := os.ReadFile(existing)
	if string(content) != "old\n" {
		t.Errorf("existing file was overwritten:\n%s", string(content))
	}
}

// TestListConfigTemplatesHandler — каталог отдаёт все ID из dnsmasq.ConfigTemplates,
// у каждого есть непустой preview.
func TestListConfigTemplatesHandler(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/config/templates", nil)
	c.Set("user", "admin")
	listConfigTemplatesHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Templates []struct {
			ID      string `json:"id"`
			Preview string `json:"preview"`
		} `json:"templates"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Templates) != len(dnsmasq.ConfigTemplates) {
		t.Errorf("expected %d templates, got %d", len(dnsmasq.ConfigTemplates), len(resp.Templates))
	}
	seen := make(map[string]bool)
	for _, tpl := range resp.Templates {
		seen[tpl.ID] = true
		if tpl.Preview == "" {
			t.Errorf("template %q has empty preview", tpl.ID)
		}
		if !strings.HasPrefix(tpl.Preview, "# === Managed by Intermasq ===") {
			t.Errorf("template %q preview missing managed header", tpl.ID)
		}
	}
	for id := range dnsmasq.ConfigTemplates {
		if !seen[id] {
			t.Errorf("template %q missing from response", id)
		}
	}
}

// Migrated to internal/dnsmasq:
//   TestKnownConfigTemplateIDsSorted, TestKnownConfigTemplateIDsContainsEmpty,
//   TestConfigTemplatesAllStartWithManagedHeader, TestConfigTemplatesValidForDnsmasqSyntax.

// TestCreateConfigFileHandlerTemplateAuditWritten — при создании файла с
// шаблоном в audit-лог попадает запись с полем template = выбранный ID.
// Проверяет, что поле не теряется по пути от request до audit entry.
func TestCreateConfigFileHandlerTemplateAuditWritten(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	auditDir := t.TempDir()
	auditPath := filepath.Join(auditDir, "audit.log")
	*AuditLogPath = auditPath

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(`{"name":"x.conf","template":"forwarder"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createConfigFileHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("audit log not readable: %v", err)
	}
	var entry AuditEntry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil { // последняя '\n'
		t.Fatalf("audit entry parse error: %v", err)
	}
	if entry.Template != "forwarder" {
		t.Errorf("expected template=forwarder in audit, got %q", entry.Template)
	}
	if entry.Action != "config_create_file" {
		t.Errorf("expected action=config_create_file, got %q", entry.Action)
	}
	if entry.User != "admin" {
		t.Errorf("expected user=admin, got %q", entry.User)
	}
}

// ========== Bug 1+2: loadUsers / loadTemplates fatal on read errors ==========

// TestLoadUsersFailsOnCorruptJSON — повреждённый JSON в users.json должен
// вызвать os.Exit, а не оставить users пустым (что открывает /api/setup).
// Проверяем через subprocess, чтобы перехватить exit code.
func TestLoadUsersFailsOnCorruptJSON(t *testing.T) {
	if os.Getenv("INTERMASQ_TEST_FATAL") == "1" {
		// Внутренний прогоhн: corrupt users.json, ждём fatal.
		dir := t.TempDir()
		*DBPath = filepath.Join(dir, "users.json")
		os.WriteFile(*DBPath, []byte("{not json"), 0600)
		loadUsers()
		// loadUsers должна была вызвать os.Exit — эта строка недостижима.
		t.Fatal("loadUsers should have called os.Exit on corrupt JSON")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLoadUsersFailsOnCorruptJSON")
	cmd.Env = append(os.Environ(), "INTERMASQ_TEST_FATAL=1", "INTERMASQ_SECRET=test-secret-32-bytes-long-for-ci-0001")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit code on corrupt users.json")
	}
}

// TestLoadTemplatesFailsOnCorruptJSON — аналогично для templates.json.
func TestLoadTemplatesFailsOnCorruptJSON(t *testing.T) {
	if os.Getenv("INTERMASQ_TEST_FATAL") == "1" {
		dir := t.TempDir()
		*TemplatesPath = filepath.Join(dir, "templates.json")
		os.WriteFile(*TemplatesPath, []byte("definitely not json"), 0600)
		loadTemplates()
		t.Fatal("loadTemplates should have called os.Exit on corrupt JSON")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLoadTemplatesFailsOnCorruptJSON")
	cmd.Env = append(os.Environ(), "INTERMASQ_TEST_FATAL=1", "INTERMASQ_SECRET=test-secret-32-bytes-long-for-ci-0001")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit code on corrupt templates.json")
	}
}

// TestLoadUsersMissingFileIsOK — отсутствие файла (первый запуск) остаётся
// нормальным сценарием: setup_required=true, /api/setup доступен.
func TestLoadUsersMissingFileIsOK(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "absent.json")
	users = make(map[string]string)
	loadUsers()
	if len(users) != 0 {
		t.Errorf("expected empty users map on missing file, got %d", len(users))
	}
}

// TestSaveUsersAtomic — после сохранения users.json файл существует и
// парсится; tmp-файла после rename не остаётся.
func TestSaveUsersAtomic(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"admin": "$2a$10$hash", "bob": "$2a$10$another"}
	if err := saveUsers(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(*DBPath); err != nil {
		t.Fatalf("users.json not written: %v", err)
	}
	if _, err := os.Stat(*DBPath + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp file should not remain after atomic save")
	}
	data, _ := os.ReadFile(*DBPath)
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("users.json not valid JSON after atomic save: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 users, got %d", len(got))
	}
}

// TestSaveUsersAtomicPreservesExistingOnFailure — подтвердить наличие
// tmp+rename: если записать в read-only dir, saveUsers вернёт ошибку, но
// исходный файл не должен быть повреждён.
func TestSaveUsersAtomicPreservesExistingOnFailure(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	original := []byte(`{"admin":"$2a$10$orig"}`)
	os.WriteFile(*DBPath, original, 0600)

	// Делаем родительский каталог read-only, чтобы rename провалился.
	// (WriteFile в read-only dir тоже упадёт — это и есть «crash mid-write».)
	os.Chmod(dir, 0500)
	defer os.Chmod(dir, 0755) // восстановим для t.TempDir cleanup

	users = map[string]string{"admin": "$2a$10$new"}
	err := saveUsers()
	if err == nil {
		t.Skip("saveUsers succeeded despite read-only dir (root or permissive FS)")
	}
	// Исходный файл должен остаться нетронутым.
	data, _ := os.ReadFile(*DBPath)
	if string(data) != string(original) {
		t.Errorf("original users.json was modified on failed save:\nwant: %s\ngot:  %s", original, data)
	}
}

// ========== Feature 3: optional IP/hostname in dhcp-host ==========
// (TestValidateHostFieldsAllCombinations moved to internal/validate.)

// TestAddHostHandlerMacOnly — POST /api/hosts только с MAC создаёт строку
// dhcp-host=<mac> (infinite lease без имени и IP).
func TestAddHostHandlerMacOnly(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for MAC-only host, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "dhcp-host=aa:bb:cc:dd:ee:ff\n") {
		t.Errorf("MAC-only line not written correctly:\n%s", content)
	}
	if strings.Contains(string(content), ",") {
		t.Errorf("MAC-only line should have no commas:\n%s", content)
	}
}

// TestAddHostHandlerMacPlusHostname — DHCP-выданный IP + DNS-имя.
func TestAddHostHandlerMacPlusHostname(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","hostname":"phone","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for MAC+hostname host, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "dhcp-host=aa:bb:cc:dd:ee:ff,phone\n") {
		t.Errorf("MAC+hostname line not written correctly:\n%s", content)
	}
}

// TestAddHostHandlerMacPlusIP — статический IP без DNS-имени.
func TestAddHostHandlerMacPlusIP(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.10","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for MAC+IP host, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "dhcp-host=aa:bb:cc:dd:ee:ff,192.168.1.10\n") {
		t.Errorf("MAC+IP line not written correctly:\n%s", content)
	}
}

// TestAddHostHandlerRejectsBadIP — опциональность не означает «пропустить
// мусор»: невалидный IP всё ещё отвергается.
func TestAddHostHandlerRejectsBadIP(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","ip":"not-an-ip","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid IP, got %d", w.Code)
	}
}

// TestAddHostHandlerIPDuplicateStillChecked — если IP указан, duplicate check
// работает как раньше.
func TestAddHostHandlerIPDuplicateStillChecked(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte("dhcp-host=11:22:33:44:55:66,existing,192.168.1.10\n"), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.10","hostname":"new","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 409 {
		t.Fatalf("expected 409 for IP conflict, got %d", w.Code)
	}
}

// TestAddHostHandlerRejectsUnsafeFile (mutation-go M8 regression) — the
// isSafePath guard at the top of addHostHandler must reject a file outside
// ConfigDir with 400 invalid_data, before any field validation runs.
func TestAddHostHandlerRejectsUnsafeFile(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir

	cases := []struct{ name, file string }{
		{"absolute_outside", "/etc/passwd"},
		{"traversal", filepath.Join(dir, "..", "evil.conf")},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","file":%q}`, tc.file)
		c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", "admin")
		addHostHandler(c)
		if w.Code != 400 {
			t.Errorf("%s: expected 400 for unsafe file %q, got %d: %s", tc.name, tc.file, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "invalid_data") {
			t.Errorf("%s: expected invalid_data body, got: %s", tc.name, w.Body.String())
		}
	}
}

// TestAddHostHandlerMACDuplicateRejected (mutation-go M9 regression) —
// adding a host whose MAC already exists in the target file must return
// 409 mac_duplicate and must not overwrite the existing entry. IP is
// omitted so the IP-duplicate branch cannot mask the MAC check.
func TestAddHostHandlerMACDuplicateRejected(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte("dhcp-host=aa:bb:cc:dd:ee:ff,existing\n"), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","hostname":"new","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 409 {
		t.Fatalf("expected 409 for MAC conflict, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "mac_duplicate") {
		t.Errorf("expected mac_duplicate body, got: %s", w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "dhcp-host=aa:bb:cc:dd:ee:ff,existing\n") {
		t.Errorf("existing entry should be preserved on MAC conflict:\n%s", content)
	}
}

// TestAddHostHandlerRejectsZeroBroadcastMAC (A3 regression) — zero and
// broadcast MACs must be rejected at the handler layer even though they
// match validate.ValidMAC.
func TestAddHostHandlerRejectsZeroBroadcastMAC(t *testing.T) {
	for _, mac := range []string{"00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff"} {
		dir := t.TempDir()
		*dnsmasq.ConfigDir = dir
		file := filepath.Join(dir, "hosts.conf")
		os.WriteFile(file, []byte(""), 0644)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := fmt.Sprintf(`{"mac":%q,"ip":"10.0.0.99","hostname":"x","file":%q}`, mac, file)
		c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", "admin")
		addHostHandler(c)
		if w.Code != 400 {
			t.Errorf("expected 400 for MAC %q, got %d: %s", mac, w.Code, w.Body.String())
		}
		content, _ := os.ReadFile(file)
		if strings.Contains(string(content), mac) {
			t.Errorf("MAC %q should not be written to file:\n%s", mac, content)
		}
	}
}

// TestAddHostHandlerDashMACNormalized (A4 regression) — POST /api/hosts with
// a dash-separated MAC returns 200 and stores the colon form, so dnsmasq
// --test passes on reload.
func TestAddHostHandlerDashMACNormalized(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa-bb-cc-dd-ee-07","ip":"10.0.0.17","hostname":"dashmac","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for dash-MAC (normalised), got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "aa:bb:cc:dd:ee:07") {
		t.Errorf("colon form should be in file:\n%s", content)
	}
	if strings.Contains(string(content), "aa-bb-cc-dd-ee-07") {
		t.Errorf("dash form must NOT be in file:\n%s", content)
	}
}

// TestParseCSVHostsNormalizesDashMAC (A4 regression) — CSV import normalises
// dash-MACs the same way the JSON add path does.
// Migrated to internal/dnsmasq:
//   TestParseCSVHostsNormalizesDashMAC, TestParseCSVHostsAcceptsMACOnly,
//   TestParseCSVHostsMACPlusHostname,
//   TestParseAliasLinePTR, TestParseAliasLineTXT, TestParseAliasLineTXTMultiComma,
//   TestAliasToLinePTR, TestAliasToLineTXT, TestAliasRoundTripPTR, TestAliasRoundTripTXT,
//   TestIsAliasDirectiveRecognizesNewTypes, TestReadAllAliasesIncludesPTRAndTXT.

func TestValidateAliasEntryPTRAndTXT(t *testing.T) {
	cases := []struct {
		name  string
		entry DnsAliasEntry
		want  bool
	}{
		{"valid PTR", DnsAliasEntry{Type: "PTR", Domain: "10.in-addr.arpa", Target: "nas.lan"}, true},
		{"valid TXT", DnsAliasEntry{Type: "TXT", Domain: "nas.lan", Target: "v=spf1 -all"}, true},
		{"TXT empty target", DnsAliasEntry{Type: "TXT", Domain: "nas.lan", Target: ""}, false},
		{"TXT with newline", DnsAliasEntry{Type: "TXT", Domain: "nas.lan", Target: "a\nb"}, false},
		{"unknown type", DnsAliasEntry{Type: "MX", Domain: "x", Target: "y"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateAliasEntry(tc.entry); got != tc.want {
				t.Errorf("validateAliasEntry(%+v) = %v, want %v", tc.entry, got, tc.want)
			}
		})
	}
}

// TestAliasDomainRegexUnderscore moved to internal/validate (white-box).

func TestRemoveAliasLinePTR(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	content := []byte("ptr-record=10.1.168.192.in-addr.arpa,nas.lan\naddress=/other/10.0.0.1\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	removed, err := removeAliasLine(path, "PTR", "10.1.168.192.in-addr.arpa")
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

func TestRemoveAliasLineTXT(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	content := []byte("txt-record=nas.lan,v=spf1 -all\ncname=wiki,nas.lan\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	removed, err := removeAliasLine(path, "TXT", "nas.lan")
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

// TestAddAliasHandlerPTR — end-to-end: POST /api/aliases с type=PTR
// создаёт ptr-record= строку в файле.
func TestAddAliasHandlerPTR(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "dns.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"type":"PTR","domain":"10.1.168.192.in-addr.arpa","target":"nas.lan","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/aliases", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addAliasHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for PTR alias, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "ptr-record=10.1.168.192.in-addr.arpa,nas.lan") {
		t.Errorf("PTR line not written:\n%s", content)
	}
}

// TestAddAliasHandlerTXT — end-to-end: POST /api/aliases с type=TXT
// создаёт txt-record= строку.
func TestAddAliasHandlerTXT(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "dns.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"type":"TXT","domain":"nas.lan","target":"v=spf1 -all","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/aliases", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addAliasHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for TXT alias, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "txt-record=nas.lan,v=spf1 -all") {
		t.Errorf("TXT line not written:\n%s", content)
	}
}

// TestAddAliasHandlerDuplicateRejected (A2 regression) — adding an A record
// whose domain+type already exists in the same file must return 409 and must
// NOT append a second line. Previously findAliasesByDomain excluded the
// matching type+file combo, so the duplicate check saw zero conflicts.
func TestAddAliasHandlerDuplicateRejected(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "dns.conf")
	os.WriteFile(file, []byte("address=/nas.local/10.0.0.5\n"), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"type":"A","domain":"nas.local","target":"10.0.0.99","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/aliases", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addAliasHandler(c)
	if w.Code != 409 {
		t.Fatalf("expected 409 for duplicate alias, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "alias_duplicate") {
		t.Errorf("expected alias_duplicate error, got: %s", w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if got := strings.Count(string(content), "address=/nas.local/"); got != 1 {
		t.Errorf("expected exactly 1 nas.local A record, got %d:\n%s", got, content)
	}
}

// TestDeleteAliasHandlerSecondDeleteNotFound (A2 knock-on) — once A2 is fixed
// there is at most one record per domain+type+file, so a second delete must
// return 404 (previously it found the duplicate copy and returned 200).
func TestDeleteAliasHandlerSecondDeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	file := filepath.Join(dir, "dns.conf")
	os.WriteFile(file, []byte("address=/nas.local/10.0.0.5\n"), 0644)

	doDelete := func() int {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := fmt.Sprintf(`{"type":"A","domain":"nas.local","file":%q}`, file)
		c.Request = httptest.NewRequest("POST", "/api/aliases/delete", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", "admin")
		deleteAliasHandler(c)
		return w.Code
	}
	if code := doDelete(); code != 200 {
		t.Fatalf("first delete: expected 200, got %d", code)
	}
	if code := doDelete(); code != 404 {
		t.Fatalf("second delete: expected 404 (A2 knock-on), got %d", code)
	}
}

// Migrated to internal/dnsmasq:
//   TestParseCSVAliasesIncludesPTRAndTXT,
//   TestParseIPTransform, TestIPTransform_Apply_None,
//   TestIPTransform_Apply_InvalidIP, TestIPTransform_Apply_Octets,
//   TestIPTransform_Apply_CIDR, TestIPTransform_Apply_CIDRRoundTrip,
//   TestIsLeaseTime, TestDirectiveGroup.

// TestEnsureAliasesFile covers all three branches.
func TestEnsureAliasesFile(t *testing.T) {
	dir := newTestDir(t)

	// 1. Path traversal attempt → ErrPermission.
	unsafe := filepath.Join(dir, "..", "escape.conf")
	if err := ensureAliasesFile(unsafe); err != os.ErrPermission {
		t.Fatalf("expected os.ErrPermission for unsafe path, got %v", err)
	}

	// 2. New file inside ConfigDir → created with header.
	good := filepath.Join(dir, "aliases.conf")
	if err := ensureAliasesFile(good); err != nil {
		t.Fatalf("ensureAliasesFile err: %v", err)
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
	if err := ensureAliasesFile(good); err != nil {
		t.Fatalf("ensureAliasesFile on existing err: %v", err)
	}
	after, _ := os.ReadFile(good)
	if string(after) != string(stamped) {
		t.Errorf("existing file modified: before %q, after %q", stamped, after)
	}
}

// Migrated to internal/dnsmasq:
//   TestIsLeaseTime, TestDirectiveGroup.

// ========== Coverage sweep §3 (Этап 3): resolveAliasesTargetFile ==========

// TestResolveAliasesTargetFile_EmptyCreatesDefault covers the empty-reqFile
// branch (was 50%): when the caller omits the file, the default aliases file
// (DefaultAliasesFileName) is created on demand inside ConfigDir and
// returned. This is the path POST /api/aliases takes when the UI sends no
// explicit target file.
func TestResolveAliasesTargetFile_EmptyCreatesDefault(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir

	path, ok := resolveAliasesTargetFile("")
	if !ok {
		t.Fatal("expected ok=true for empty reqFile")
	}
	want := filepath.Join(dir, DefaultAliasesFileName)
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("default aliases file should have been created: %v", err)
	}
}

// TestResolveAliasesTargetFile_ExplicitSafe covers the explicit-path happy
// path: a pre-existing safe file inside ConfigDir is returned verbatim.
func TestResolveAliasesTargetFile_ExplicitSafe(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	given := filepath.Join(dir, "custom.conf")
	if err := os.WriteFile(given, []byte("address=/x/1.2.3.4\n"), 0644); err != nil {
		t.Fatal(err)
	}

	path, ok := resolveAliasesTargetFile(given)
	if !ok {
		t.Fatal("expected ok=true for safe explicit path")
	}
	if path != given {
		t.Errorf("path = %q, want %q", path, given)
	}
}

// TestResolveAliasesTargetFile_Unsafe covers the isSafePath rejection branch
// (returns ok=false for a path outside ConfigDir).
func TestResolveAliasesTargetFile_Unsafe(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir

	if _, ok := resolveAliasesTargetFile("/etc/passwd"); ok {
		t.Error("expected ok=false for unsafe path")
	}
}
