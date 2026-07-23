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

// Handler-level integration tests (L2) for endpoints not yet covered by
// dnsmasq_test.go, plus Gap 3 edge cases (IPv6, unicode, empty/comments-only
// .conf, concurrent writes). All tests use httptest.NewRecorder +
// gin.CreateTestContext to exercise handler logic directly without spinning
// up a real HTTP server. dnsmasq --test is NOT available on Windows test
// hosts, so tests that would trigger writeConfigWithTest / writeFileRaw
// skip on non-Linux platforms.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

// ===== Test helpers =====

// newTestDir creates a temp dir, points *ConfigDir at it, and returns the dir.
// t.TempDir auto-cleans on test completion.
func newTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	*ConfigDir = dir
	return dir
}

// newJSONContext builds a gin test context with a JSON body and admin user.
func newJSONContext(method, target, body string) (*httptest.ResponseRecorder, *gin.Context) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	return w, c
}

// jsonPath escapes a file path for embedding in a JSON string literal.
func jsonPath(p string) string {
	return strings.ReplaceAll(p, "\\", "\\\\")
}

// ===== Host handlers (L2) =====

func TestDeleteHostHandler_Success(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte("dhcp-host=aa:bb:cc:dd:ee:ff,host1,192.168.1.10\n"), 0644)

	w, c := newJSONContext("DELETE", "/api/hosts/aa:bb:cc:dd:ee:ff?file="+url.QueryEscape(file), "")
	c.Params = gin.Params{{Key: "mac", Value: "aa:bb:cc:dd:ee:ff"}}
	deleteHostHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if strings.Contains(string(content), "aa:bb:cc:dd:ee:ff") {
		t.Error("host should be removed from file")
	}
}

func TestDeleteHostHandler_NotFound(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	w, c := newJSONContext("DELETE", "/api/hosts/aa:bb:cc:dd:ee:ff?file="+url.QueryEscape(file), "")
	c.Params = gin.Params{{Key: "mac", Value: "aa:bb:cc:dd:ee:ff"}}
	deleteHostHandler(c)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteHostHandler_BadMAC(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	w, c := newJSONContext("DELETE", "/api/hosts/not-a-mac?file="+url.QueryEscape(file), "")
	c.Params = gin.Params{{Key: "mac", Value: "not-a-mac"}}
	deleteHostHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for bad MAC, got %d", w.Code)
	}
}

func TestDeleteHostHandler_UnsafeFile(t *testing.T) {
	newTestDir(t)

	w, c := newJSONContext("DELETE", "/api/hosts/aa:bb:cc:dd:ee:ff?file="+url.QueryEscape("/etc/passwd"), "")
	c.Params = gin.Params{{Key: "mac", Value: "aa:bb:cc:dd:ee:ff"}}
	deleteHostHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for unsafe file, got %d", w.Code)
	}
}

func TestBulkAddHostsHandler_Success(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	body := `{"file":"` + jsonPath(file) + `","hosts":[{"mac":"aa:bb:cc:dd:ee:01","ip":"10.0.0.1","hostname":"h1"},{"mac":"aa:bb:cc:dd:ee:02","ip":"10.0.0.2","hostname":"h2"}]}`
	w, c := newJSONContext("POST", "/api/hosts/bulk", body)
	bulkAddHostsHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "aa:bb:cc:dd:ee:01") || !strings.Contains(string(content), "aa:bb:cc:dd:ee:02") {
		t.Errorf("both hosts should be in file:\n%s", content)
	}
}

func TestBulkAddHostsHandler_InvalidMAC(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	body := `{"file":"` + jsonPath(file) + `","hosts":[{"mac":"bad-mac","ip":"10.0.0.1"}]}`
	w, c := newJSONContext("POST", "/api/hosts/bulk", body)
	bulkAddHostsHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid MAC, got %d", w.Code)
	}
}

func TestBulkAddHostsHandler_InBatchIPConflict(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	body := `{"file":"` + jsonPath(file) + `","hosts":[{"mac":"aa:bb:cc:dd:ee:01","ip":"10.0.0.5"},{"mac":"aa:bb:cc:dd:ee:02","ip":"10.0.0.5"}]}`
	w, c := newJSONContext("POST", "/api/hosts/bulk", body)
	bulkAddHostsHandler(c)

	if w.Code != 409 {
		t.Fatalf("expected 409 for in-batch IP conflict, got %d", w.Code)
	}
}

func TestBulkAddHostsHandler_UnsafeFile(t *testing.T) {
	newTestDir(t)

	body := `{"file":"/etc/passwd","hosts":[{"mac":"aa:bb:cc:dd:ee:01","ip":"10.0.0.1"}]}`
	w, c := newJSONContext("POST", "/api/hosts/bulk", body)
	bulkAddHostsHandler(c)

	if w.Code != 403 {
		t.Fatalf("expected 403 for unsafe file, got %d", w.Code)
	}
}

func TestBulkMoveHandler_Success(t *testing.T) {
	dir := newTestDir(t)
	src := filepath.Join(dir, "src.conf")
	dst := filepath.Join(dir, "dst.conf")
	os.WriteFile(src, []byte("dhcp-host=aa:bb:cc:dd:ee:ff,host1,192.168.1.10\n"), 0644)
	os.WriteFile(dst, []byte(""), 0644)

	body := `{"target":"` + jsonPath(dst) + `","hosts":[{"mac":"aa:bb:cc:dd:ee:ff","file":"` + jsonPath(src) + `"}]}`
	w, c := newJSONContext("POST", "/api/hosts/bulk-move", body)
	bulkMoveHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	srcContent, _ := os.ReadFile(src)
	if strings.Contains(string(srcContent), "aa:bb:cc:dd:ee:ff") {
		t.Error("host should be removed from source")
	}
	dstContent, _ := os.ReadFile(dst)
	if !strings.Contains(string(dstContent), "aa:bb:cc:dd:ee:ff") {
		t.Error("host should be added to target")
	}
}

func TestBulkMoveHandler_NoHosts(t *testing.T) {
	dir := newTestDir(t)
	dst := filepath.Join(dir, "dst.conf")

	body := `{"target":"` + jsonPath(dst) + `","hosts":[]}`
	w, c := newJSONContext("POST", "/api/hosts/bulk-move", body)
	bulkMoveHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for empty hosts, got %d", w.Code)
	}
}

func TestBulkMoveHandler_SameFile(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")

	body := `{"target":"` + jsonPath(file) + `","hosts":[{"mac":"aa:bb:cc:dd:ee:ff","file":"` + jsonPath(file) + `"}]}`
	w, c := newJSONContext("POST", "/api/hosts/bulk-move", body)
	bulkMoveHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for same file, got %d", w.Code)
	}
}

func TestBulkMoveHandler_UnsafeTarget(t *testing.T) {
	dir := newTestDir(t)
	src := filepath.Join(dir, "src.conf")

	body := `{"target":"/etc/evil.conf","hosts":[{"mac":"aa:bb:cc:dd:ee:ff","file":"` + jsonPath(src) + `"}]}`
	w, c := newJSONContext("POST", "/api/hosts/bulk-move", body)
	bulkMoveHandler(c)

	if w.Code != 403 {
		t.Fatalf("expected 403 for unsafe target, got %d", w.Code)
	}
}

func TestBulkEditHandler_PrefixTransform(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte("dhcp-host=aa:bb:cc:dd:ee:01,host1,10.0.0.50\n"), 0644)

	body := `{"hosts":[{"mac":"aa:bb:cc:dd:ee:01","file":"` + jsonPath(file) + `"}],"ip_transform":{"old_prefix":"10.0.0","new_prefix":"10.0.1"}}`
	w, c := newJSONContext("POST", "/api/hosts/bulk-edit", body)
	bulkEditHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "10.0.1.50") {
		t.Errorf("expected IP transformed to 10.0.1.50:\n%s", content)
	}
}

func TestBulkEditHandler_NoHosts(t *testing.T) {
	body := `{"hosts":[],"ip_transform":{"old_prefix":"10.0.0","new_prefix":"10.0.1"}}`
	w, c := newJSONContext("POST", "/api/hosts/bulk-edit", body)
	bulkEditHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for empty hosts, got %d", w.Code)
	}
}

func TestBulkEditHandler_PartialPrefix(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")

	body := `{"hosts":[{"mac":"aa:bb:cc:dd:ee:01","file":"` + jsonPath(file) + `"}],"ip_transform":{"old_prefix":"","new_prefix":"10.0.1"}}`
	w, c := newJSONContext("POST", "/api/hosts/bulk-edit", body)
	bulkEditHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for partial prefix, got %d", w.Code)
	}
}

func TestBulkEditHandler_PrefixMismatch(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte("dhcp-host=aa:bb:cc:dd:ee:01,host1,192.168.1.10\n"), 0644)

	body := `{"hosts":[{"mac":"aa:bb:cc:dd:ee:01","file":"` + jsonPath(file) + `"}],"ip_transform":{"old_prefix":"10.0.0","new_prefix":"10.0.1"}}`
	w, c := newJSONContext("POST", "/api/hosts/bulk-edit", body)
	bulkEditHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for prefix not matched, got %d: %s", w.Code, w.Body.String())
	}
}

// ===== Alias handlers (L2) =====

func TestDeleteAliasHandler_Success(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "aliases.conf")
	os.WriteFile(file, []byte("address=/nas.local/10.0.0.5\n"), 0644)

	body := `{"type":"A","domain":"nas.local","file":"` + jsonPath(file) + `"}`
	w, c := newJSONContext("POST", "/api/aliases/delete", body)
	deleteAliasHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if strings.Contains(string(content), "nas.local") {
		t.Error("alias should be removed from file")
	}
}

func TestDeleteAliasHandler_NotFound(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "aliases.conf")
	os.WriteFile(file, []byte(""), 0644)

	body := `{"type":"A","domain":"missing.local","file":"` + jsonPath(file) + `"}`
	w, c := newJSONContext("POST", "/api/aliases/delete", body)
	deleteAliasHandler(c)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteAliasHandler_BadType(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "aliases.conf")

	body := `{"type":"PTR","domain":"nas.local","file":"` + jsonPath(file) + `"}`
	w, c := newJSONContext("POST", "/api/aliases/delete", body)
	deleteAliasHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for PTR (UI only supports A/CNAME), got %d", w.Code)
	}
}

func TestBulkAddAliasesHandler_Success(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "aliases.conf")
	os.WriteFile(file, []byte(""), 0644)

	body := `{"file":"` + jsonPath(file) + `","aliases":[{"type":"A","domain":"a.test","target":"10.0.0.1"},{"type":"CNAME","domain":"b.test","target":"a.test"}]}`
	w, c := newJSONContext("POST", "/api/aliases/bulk", body)
	bulkAddAliasesHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "a.test") || !strings.Contains(string(content), "b.test") {
		t.Errorf("both aliases should be in file:\n%s", content)
	}
}

func TestBulkAddAliasesHandler_NoValidEntries(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "aliases.conf")
	os.WriteFile(file, []byte(""), 0644)

	body := `{"file":"` + jsonPath(file) + `","aliases":[{"type":"A","domain":"bad","target":"not-an-ip"}]}`
	w, c := newJSONContext("POST", "/api/aliases/bulk", body)
	bulkAddAliasesHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for no valid entries, got %d", w.Code)
	}
}

func TestBulkAddAliasesHandler_InBatchDup(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "aliases.conf")
	os.WriteFile(file, []byte(""), 0644)

	body := `{"file":"` + jsonPath(file) + `","aliases":[{"type":"A","domain":"dup.test","target":"10.0.0.1"},{"type":"A","domain":"dup.test","target":"10.0.0.2"}]}`
	w, c := newJSONContext("POST", "/api/aliases/bulk", body)
	bulkAddAliasesHandler(c)

	if w.Code != 409 {
		t.Fatalf("expected 409 for in-batch duplicate, got %d", w.Code)
	}
}

func TestBulkAddAliasesHandler_UnsafeFile(t *testing.T) {
	body := `{"file":"/etc/evil.conf","aliases":[{"type":"A","domain":"x.test","target":"10.0.0.1"}]}`
	w, c := newJSONContext("POST", "/api/aliases/bulk", body)
	bulkAddAliasesHandler(c)

	if w.Code != 403 {
		t.Fatalf("expected 403 for unsafe file, got %d", w.Code)
	}
}

// ===== Config handlers (L2) — validation only (dnsmasq --test on Linux CI) =====

func TestUpdateConfigHandler_BadKey(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "test.conf")
	os.WriteFile(file, []byte(""), 0644)

	body := `{"file":"` + jsonPath(file) + `","directives":[{"key":"123bad","value":"","active":true}]}`
	w, c := newJSONContext("PUT", "/api/config", body)
	updateConfigHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for bad directive key, got %d", w.Code)
	}
}

func TestUpdateConfigHandler_UppercaseKey(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "test.conf")
	os.WriteFile(file, []byte(""), 0644)

	body := `{"file":"` + jsonPath(file) + `","directives":[{"key":"BadKey","value":"","active":true}]}`
	w, c := newJSONContext("PUT", "/api/config", body)
	updateConfigHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for uppercase key, got %d", w.Code)
	}
}

func TestUpdateConfigHandler_NewlineInValue(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "test.conf")
	os.WriteFile(file, []byte(""), 0644)

	body := `{"file":"` + jsonPath(file) + `","directives":[{"key":"server","value":"1.1.1.1\n8.8.8.8","active":true}]}`
	w, c := newJSONContext("PUT", "/api/config", body)
	updateConfigHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for newline in value, got %d", w.Code)
	}
}

func TestUpdateConfigHandler_UnsafeFile(t *testing.T) {
	body := `{"file":"/etc/evil.conf","directives":[]}`
	w, c := newJSONContext("PUT", "/api/config", body)
	updateConfigHandler(c)

	if w.Code != 403 {
		t.Fatalf("expected 403 for unsafe file, got %d", w.Code)
	}
}

func TestUpdateConfigHandler_BindError(t *testing.T) {
	w, c := newJSONContext("PUT", "/api/config", "not-json")
	updateConfigHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for bind error, got %d", w.Code)
	}
}

func TestPutFileHandler_NonConfName(t *testing.T) {
	w, c := newJSONContext("PUT", "/api/files/passwd", `{"content":"x"}`)
	c.Params = gin.Params{{Key: "name", Value: "passwd"}}
	putFileHandler(c)

	if w.Code != 403 {
		t.Fatalf("expected 403 for non-.conf name, got %d", w.Code)
	}
}

func TestPutFileHandler_PathSeparator(t *testing.T) {
	w, c := newJSONContext("PUT", "/api/files/..%2Fetc%2Fpasswd", `{"content":"x"}`)
	c.Params = gin.Params{{Key: "name", Value: "../etc/passwd"}}
	putFileHandler(c)

	if w.Code != 403 {
		t.Fatalf("expected 403 for path separator in name, got %d", w.Code)
	}
}

// ===== Safety handlers (L2) =====

func TestHistoryDiffHandler_MissingParams(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "test.conf")
	os.WriteFile(file, []byte("test\n"), 0644)

	w, c := newJSONContext("GET", "/api/history/diff?file="+url.QueryEscape(file), "")
	historyDiffHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for missing 'from' param, got %d", w.Code)
	}
}

func TestHistoryDiffHandler_UnknownVersion(t *testing.T) {
	dir := newTestDir(t)
	*HistoryDir = t.TempDir()
	*HistoryDepth = 5
	file := filepath.Join(dir, "test.conf")
	os.WriteFile(file, []byte("test\n"), 0644)

	w, c := newJSONContext("GET", "/api/history/diff?file="+url.QueryEscape(file)+"&from=19990101-000000", "")
	historyDiffHandler(c)

	if w.Code != 404 {
		t.Fatalf("expected 404 for unknown version, got %d", w.Code)
	}
}

func TestHistoryRestoreHandler_MissingVersion(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "test.conf")

	body := `{"file":"` + jsonPath(file) + `"}`
	w, c := newJSONContext("POST", "/api/history/restore", body)
	historyRestoreHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for missing version, got %d", w.Code)
	}
}

func TestHistoryRestoreHandler_UnsafePath(t *testing.T) {
	body := `{"file":"/etc/passwd","version":"20240101-000000"}`
	w, c := newJSONContext("POST", "/api/history/restore", body)
	historyRestoreHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for unsafe path, got %d", w.Code)
	}
}

func TestRestoreBackupHandler_NoFile(t *testing.T) {
	w, c := newJSONContext("POST", "/api/backup/restore", "")
	restoreBackupHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for missing file, got %d", w.Code)
	}
}

func TestRestoreBackupHandler_InvalidZip(t *testing.T) {
	newTestDir(t)
	body := "this is definitely not a zip file"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/backup/restore", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/zip")
	c.Set("user", "admin")
	restoreBackupHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid zip, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRollbackHandler_UnsafePath(t *testing.T) {
	body := `{"file":"/etc/passwd"}`
	w, c := newJSONContext("POST", "/api/rollback", body)
	rollbackHandler(c)

	if w.Code != 500 {
		t.Fatalf("expected 500 for rollback of unsafe path (rollbackFile returns error), got %d", w.Code)
	}
}

// ===== Template handlers (L2) =====

func TestCreateTemplateHandler_Success(t *testing.T) {
	dir := newTestDir(t)
	*TemplatesPath = filepath.Join(dir, "templates.json")
	templates = make(map[string]Template)

	body := `{"name":"Test Template","hostname_pattern":"device-{NNN}","ip_range":"10.0.0.0/24","target_file":"` + jsonPath(filepath.Join(dir, "hosts.conf")) + `"}`
	w, c := newJSONContext("POST", "/api/templates", body)
	createTemplateHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, ok := templates["test-template"]; !ok {
		t.Error("template should be stored with derived ID 'test-template'")
	}
}

func TestCreateTemplateHandler_Duplicate(t *testing.T) {
	dir := newTestDir(t)
	*TemplatesPath = filepath.Join(dir, "templates.json")
	templates = map[string]Template{"test-template": {ID: "test-template", Name: "Test Template"}}

	body := `{"name":"Test Template","hostname_pattern":"device-{NNN}","ip_range":"10.0.0.0/24","target_file":"` + jsonPath(filepath.Join(dir, "hosts.conf")) + `"}`
	w, c := newJSONContext("POST", "/api/templates", body)
	createTemplateHandler(c)

	if w.Code != 409 {
		t.Fatalf("expected 409 for duplicate template, got %d", w.Code)
	}
}

func TestCreateTemplateHandler_MissingFields(t *testing.T) {
	dir := newTestDir(t)
	*TemplatesPath = filepath.Join(dir, "templates.json")
	templates = make(map[string]Template)

	body := `{"name":"Empty"}`
	w, c := newJSONContext("POST", "/api/templates", body)
	createTemplateHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for missing fields, got %d", w.Code)
	}
}

func TestDeleteTemplateHandler_Success(t *testing.T) {
	dir := newTestDir(t)
	*TemplatesPath = filepath.Join(dir, "templates.json")
	templates = map[string]Template{"test-template": {ID: "test-template", Name: "Test"}}

	w, c := newJSONContext("DELETE", "/api/templates/test-template", "")
	c.Params = gin.Params{{Key: "id", Value: "test-template"}}
	deleteTemplateHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if _, ok := templates["test-template"]; ok {
		t.Error("template should be deleted")
	}
}

func TestDeleteTemplateHandler_NotFound(t *testing.T) {
	dir := newTestDir(t)
	*TemplatesPath = filepath.Join(dir, "templates.json")
	templates = make(map[string]Template)

	w, c := newJSONContext("DELETE", "/api/templates/missing", "")
	c.Params = gin.Params{{Key: "id", Value: "missing"}}
	deleteTemplateHandler(c)

	if w.Code != 404 {
		t.Fatalf("expected 404 for missing template, got %d", w.Code)
	}
}

// ===== Metrics handler (L2) =====

// setupMetricsGlobals wires up the globals metricsHandler touches: SecretKey
// for auth, sysCaller for checkDnsmasqStatus, and ConfigDir for readAllHosts.
// All originals are restored on test completion.
func setupMetricsGlobals(t *testing.T) {
	t.Helper()
	newTestDir(t)
	origKey := SecretKey
	SecretKey = []byte("test-secret-key-32-bytes-long!!")
	origCaller := sysCaller
	sysCaller = &NoneCaller{}
	t.Cleanup(func() {
		SecretKey = origKey
		sysCaller = origCaller
	})
}

func TestMetricsHandler_NoAuth_401(t *testing.T) {
	setupMetricsGlobals(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/metrics", nil)
	metricsHandler(c)

	if w.Code != 401 {
		t.Fatalf("expected 401 without auth, got %d", w.Code)
	}
}

func TestMetricsHandler_APIKey_200(t *testing.T) {
	setupMetricsGlobals(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/metrics", nil)
	c.Request.Header.Set("X-API-Key", "test-secret-key-32-bytes-long!!")
	metricsHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200 with valid API key, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMetricsHandler_WrongAPIKey_401(t *testing.T) {
	setupMetricsGlobals(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/metrics", nil)
	c.Request.Header.Set("X-API-Key", "wrong-key")
	metricsHandler(c)

	if w.Code != 401 {
		t.Fatalf("expected 401 with wrong API key, got %d", w.Code)
	}
}

func TestMetricsHandler_TokenQuery_200(t *testing.T) {
	setupMetricsGlobals(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/metrics?token=test-secret-key-32-bytes-long!!", nil)
	metricsHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200 with ?token= query param, got %d", w.Code)
	}
}

// ===== Gap 3: Edge cases =====

// TestValidateHostFields_IPv6 confirms that net.ParseIP in validateHostFields
// accepts IPv6 addresses. dnsmasq itself supports IPv6 in dhcp-host, so the
// panel should not reject them at the validation layer.
func TestValidateHostFields_IPv6(t *testing.T) {
	cases := []struct {
		name string
		mac  string
		ip   string
		want bool
	}{
		{"ipv6 loopback", "aa:bb:cc:dd:ee:ff", "::1", true},
		{"ipv6 full", "aa:bb:cc:dd:ee:ff", "2001:db8::1", true},
		{"ipv6 link-local", "aa:bb:cc:dd:ee:ff", "fe80::1", true},
		{"ipv6 invalid", "aa:bb:cc:dd:ee:ff", "not-ipv6", false},
		{"ipv6 empty", "aa:bb:cc:dd:ee:ff", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateHostFields(tc.mac, tc.ip, "")
			if got != tc.want {
				t.Errorf("validateHostFields(%q,%q,...) = %v, want %v", tc.mac, tc.ip, got, tc.want)
			}
		})
	}
}

// TestValidHostname_Unicode verifies that non-ASCII hostnames are rejected.
// hostnameRegex is [a-zA-Z0-9]... which deliberately excludes UTF-8 multibyte.
func TestValidHostname_Unicode(t *testing.T) {
	cases := []struct {
		hostname string
		want     bool
	}{
		{"host", true},    // ASCII baseline
		{"höst", false},   // Latin Extended
		{"сервер", false}, // Cyrillic
		{"サーバ", false},    // Japanese
		{"hōst", false},   // Maori macron
		{"host₀₁", false}, // Unicode subscripts
	}
	for _, tc := range cases {
		got := validHostname(tc.hostname)
		if got != tc.want {
			t.Errorf("validHostname(%q) = %v, want %v", tc.hostname, got, tc.want)
		}
	}
}

// TestReadAllHosts_EmptyFile confirms that an empty .conf file yields zero
// hosts without errors. This covers the "fresh install" scenario where the
// panel has just created a new config file but no hosts have been added.
func TestReadAllHosts_EmptyFile(t *testing.T) {
	dir := newTestDir(t)
	os.WriteFile(filepath.Join(dir, "empty.conf"), []byte(""), 0644)
	hosts := readAllHosts()
	if len(hosts) != 0 {
		t.Errorf("expected 0 hosts from empty file, got %d", len(hosts))
	}
}

// TestReadAllHosts_CommentsOnly verifies that a .conf file containing only
// comments (including commented-out dhcp-host lines) yields zero hosts.
func TestReadAllHosts_CommentsOnly(t *testing.T) {
	dir := newTestDir(t)
	content := "# header comment\n# another comment\n#dhcp-host=aa:bb:cc:dd:ee:ff,host,1.2.3.4\n"
	os.WriteFile(filepath.Join(dir, "comments.conf"), []byte(content), 0644)
	hosts := readAllHosts()
	if len(hosts) != 0 {
		t.Errorf("expected 0 hosts from comments-only file, got %d: %+v", len(hosts), hosts)
	}
}

// TestReadAllHosts_MultipleFiles confirms that hosts from multiple .conf
// files are aggregated correctly, and non-.conf files are ignored.
func TestReadAllHosts_MultipleFiles(t *testing.T) {
	dir := newTestDir(t)
	os.WriteFile(filepath.Join(dir, "10-hosts.conf"), []byte("dhcp-host=aa:bb:cc:dd:ee:01,h1,10.0.0.1\n"), 0644)
	os.WriteFile(filepath.Join(dir, "20-more.conf"), []byte("dhcp-host=aa:bb:cc:dd:ee:02,h2,10.0.0.2\n"), 0644)
	os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("dhcp-host=aa:bb:cc:dd:ee:03,h3,10.0.0.3\n"), 0644)
	os.WriteFile(filepath.Join(dir, "ignore.bak"), []byte("dhcp-host=aa:bb:cc:dd:ee:04,h4,10.0.0.4\n"), 0644)

	hosts := readAllHosts()
	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts (only .conf files), got %d: %+v", len(hosts), hosts)
	}
}

// TestParseDhcpHostLine_TrailingNewline confirms that a line with trailing
// \r\n (Windows CRLF) doesn't produce a phantom empty-field hostname.
func TestParseDhcpHostLine_TrailingNewline(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "test.conf")

	entry, ok := parseDhcpHostLine("dhcp-host=aa:bb:cc:dd:ee:ff,host1,192.168.1.10\r", file)
	if !ok {
		t.Fatal("expected parse success")
	}
	if entry.Hostname != "host1" {
		t.Errorf("hostname should be 'host1', got %q (CR contamination?)", entry.Hostname)
	}
}

// TestConcurrentAddHost_NoCorruption races 10 goroutines adding distinct
// hosts to the same file. The global mutex serialises the read+write
// critical section, so all 10 should land in the file.
func TestConcurrentAddHost_NoCorruption(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			mac := fmt.Sprintf("aa:bb:cc:dd:ee:%02x", n+1)
			ip := fmt.Sprintf("10.0.0.%d", n+1)
			host := fmt.Sprintf("host%d", n)
			body := fmt.Sprintf(`{"mac":%q,"ip":%q,"hostname":%q,"file":%q}`, mac, ip, host, jsonPath(file))
			w, c := newJSONContext("POST", "/api/hosts", body)
			addHostHandler(c)
			if w.Code != 200 {
				t.Errorf("goroutine %d: expected 200, got %d: %s", n, w.Code, w.Body.String())
			}
		}(i)
	}
	wg.Wait()

	content, _ := os.ReadFile(file)
	count := strings.Count(string(content), "dhcp-host=")
	if count != 10 {
		t.Errorf("expected 10 dhcp-host lines, got %d:\n%s", count, content)
	}
}

// TestConcurrentAddHost_SameMAC verifies that concurrent adds of the SAME
// MAC do not produce duplicate lines in the file. The conflict check in
// addHostHandler (findHostsByMac) runs OUTSIDE the mutex, so multiple
// goroutines can pass it before any write lands — they all get 200. But
// the write logic inside the mutex filters existing lines with the same
// MAC before appending, so the file ends up with exactly 1 entry.
// We assert the end state (file integrity), not the individual status codes.
func TestConcurrentAddHost_SameMAC(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","ip":"10.0.0.99","hostname":"race","file":%q}`, jsonPath(file))
			w, c := newJSONContext("POST", "/api/hosts", body)
			addHostHandler(c)
			if w.Code != 200 && w.Code != 409 {
				t.Errorf("unexpected status %d: %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()

	content, _ := os.ReadFile(file)
	count := strings.Count(string(content), "aa:bb:cc:dd:ee:ff")
	if count != 1 {
		t.Errorf("expected exactly 1 dhcp-host line for the MAC, got %d:\n%s", count, content)
	}
}

// TestRestoreBackupZip_EmptyArchive verifies that a ZIP with no .conf files
// is rejected with "no_valid_conf_files".
func TestRestoreBackupZip_EmptyArchive(t *testing.T) {
	newTestDir(t)

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	fw, _ := zw.Create("readme.txt")
	fw.Write([]byte("no conf files here"))
	zw.Close()

	err := restoreBackupZip(buf.Bytes())
	if err == nil {
		t.Fatal("expected error for empty archive (no .conf files)")
	}
	if !strings.Contains(err.Error(), "no_valid_conf_files") {
		t.Errorf("expected 'no_valid_conf_files' error, got: %v", err)
	}
}

// TestRestoreBackupZip_ValidArchive confirms that a well-formed ZIP with
// .conf files restores correctly on Linux CI (dnsmasq --test passes).
// On non-Linux the dnsmasq binary is unavailable so the test skips.
func TestRestoreBackupZip_ValidArchive(t *testing.T) {
	if dnsmasqBin() == "" {
		t.Skip("dnsmasq binary not found, skipping restore test")
	}

	dir := newTestDir(t)
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	fw, _ := zw.Create("restored.conf")
	fw.Write([]byte("domain-needed\nbogus-priv\n"))
	zw.Close()

	err := restoreBackupZip(buf.Bytes())
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "restored.conf"))
	if err != nil {
		t.Fatalf("restored file should exist: %v", err)
	}
	if !strings.Contains(string(content), "domain-needed") {
		t.Error("restored content should contain 'domain-needed'")
	}
}
