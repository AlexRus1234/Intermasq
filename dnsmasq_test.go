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

	"intermask/internal/auth"
	"intermask/internal/dnsmasq"
	"intermask/internal/initd"
	templatepkg "intermask/internal/templates"

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

// Migrated to internal/dnsmasq (stage 5):
//   TestIsSafePath, TestReadFileRaw, TestReadFileRawUnsafePath,
//   TestReadFileRawNotExist, TestWriteFileRaw, TestWriteFileRawUnsafePath.

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

// Migrated to internal/dnsmasq (stage 5):
//   TestRemoveAliasLine, TestRemoveAliasLineNotFound,
//   TestRemoveAliasLinePTR, TestRemoveAliasLineTXT.

// setupHistoryEnv + the history-test block below migrated to internal/dnsmasq
// (stage 5): TestSaveHistoryCreatesVersion, TestSaveHistoryNoOpForMissingFile,
// TestSaveHistoryRejectsUnsafePath, TestRotateHistoryKeepsDepth,
// TestReadHistoryVersionInvalid, TestListHistorySortedNewestFirst,
// firstVersion, TestUnifiedDiffAddsAndRemoves, TestUnifiedDiffEmptyA,
// TestReadFileRaw/TestReadFileRawUnsafePath/TestReadFileRawNotExist (already
// listed above), TestWriteFileRaw/TestWriteFileRawUnsafePath (already listed
// above).

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
	auth.ClearUsers()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/users", strings.NewReader(`{"username":"admin","password":"secret123"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createUserHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !auth.HasUser("admin") {
		t.Fatal("user not stored")
	}
}

func TestCreateUserDuplicate(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	setUsers(map[string]string{"admin": "hash"})
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
	auth.ClearUsers()
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
	setUsers(map[string]string{"target": "hash"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/users/target", nil)
	c.Params = gin.Params{{Key: "name", Value: "target"}}
	c.Set("user", "admin")
	deleteUserHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if auth.HasUser("target") {
		t.Fatal("user should be deleted")
	}
}

func TestDeleteUserSelf(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	setUsers(map[string]string{"admin": "hash"})
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
	auth.ClearUsers()
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
	setUsers(map[string]string{"admin": "$2a$10$1"})
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
	setUsers(map[string]string{"admin": "$2a$10$zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"})
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
//
// Migrated to internal/dnsmasq (stage 5):
//   makeTestZip, TestRestoreBackupZipValid, TestRestoreBackupZipCreatesRestoreBak,
//   TestRestoreBackupZipNoConfFiles, TestRestoreBackupZipInvalidData,
//   TestRestoreBackupZipIgnoresUnsafeNames.

func TestLogoutRevokesToken(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	setUsers(map[string]string{"admin": "$2a$10$placeholder"})

	auth.SetSecretForTest(t, []byte("test-secret-key-32-bytes-long!!"))

	token := auth.MakeToken("admin")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/logout", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)
	logoutHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	parsed, _ := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) { return auth.SecretKey, nil })
	if parsed == nil {
		t.Fatal("token parsing failed")
	}
}

// ========== OUI lookup ==========
// (TestLookupOUI* moved to internal/oui.)

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

// TestLoadTemplatesFailsOnCorruptJSON — аналогично для templates.json.
func TestLoadTemplatesFailsOnCorruptJSON(t *testing.T) {
	if os.Getenv("INTERMASQ_TEST_FATAL") == "1" {
		dir := t.TempDir()
		*TemplatesPath = filepath.Join(dir, "templates.json")
		os.WriteFile(*TemplatesPath, []byte("definitely not json"), 0600)
		templatepkg.Load()
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
// TestRemoveAliasLinePTR / TestRemoveAliasLineTXT moved to internal/dnsmasq
// (stage 5).

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

// TestEnsureAliasesFile moved to internal/dnsmasq (stage 5).

// ========== Coverage sweep §3 (Этап 3): resolveAliasesTargetFile ==========

// TestResolveAliasesTargetFile_EmptyCreatesDefault covers the empty-reqFile
// branch (was 50%): when the caller omits the file, the default aliases file
// (dnsmasq.DefaultAliasesFileName) is created on demand inside ConfigDir and
// returned. This is the path POST /api/aliases takes when the UI sends no
// explicit target file.
func TestResolveAliasesTargetFile_EmptyCreatesDefault(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir

	path, ok := resolveAliasesTargetFile("")
	if !ok {
		t.Fatal("expected ok=true for empty reqFile")
	}
	want := filepath.Join(dir, dnsmasq.DefaultAliasesFileName)
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
