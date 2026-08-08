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

package webapi

// Handler-level integration tests (L2) for endpoints not yet covered by
// dnsmasq_test.go, plus Gap 3 edge cases (IPv6, unicode, empty/comments-only
// .conf, concurrent writes). All tests use httptest.NewRecorder +
// gin.CreateTestContext to exercise handler logic directly without spinning
// up a real HTTP server. dnsmasq --test is NOT available on Windows test
// hosts, so tests that would trigger writeConfigWithTest / writeFileRaw
// skip on non-Linux platforms.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"intermask/internal/audit"
	"intermask/internal/auth"
	"intermask/internal/dnsmasq"
	"intermask/internal/models"
	"intermask/internal/netstate"
	templatepkg "intermask/internal/templates"
)

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
	// A6 regression: JSON bulk response must include a count field, mirroring
	// the CSV import path.
	var resp struct {
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v (body: %s)", err, w.Body.String())
	}
	if resp.Count != 2 {
		t.Errorf("expected count=2 in bulk JSON response, got %d (body: %s)", resp.Count, w.Body.String())
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

// TestPutFileHandlerRejectsUnsafePath locks the path-traversal defence for
// PUT /api/files/:name (A11, defense-in-depth). Vectors carry a .conf
// extension so the extension check does not short-circuit. The substring
// filter fires first today; isSafePath-after-Join is the redundant layer. No
// write is attempted because the substring check rejects before writeFileRaw,
// so this test is safe on Windows (no dnsmasq --test).
func TestPutFileHandlerRejectsUnsafePath(t *testing.T) {
	cases := []string{
		"../etc/evil.conf",
		"..\\evil.conf",
		"../../etc/dnsmasq.conf",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			newTestDir(t)
			w, c := newJSONContext("PUT", "/api/files/x.conf", `{"content":"x"}`)
			c.Params = gin.Params{{Key: "name", Value: name}}
			putFileHandler(c)
			if w.Code != 403 {
				t.Fatalf("expected 403 for traversal name %q, got %d: %s", name, w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "access_denied") {
				t.Errorf("expected access_denied body for %q, got: %s", name, w.Body.String())
			}
		})
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
	*dnsmasq.HistoryDir = t.TempDir()
	*dnsmasq.HistoryDepth = 5
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
	*templatepkg.TemplatesPath = filepath.Join(dir, "templates.json")
	resetTemplates()

	body := `{"name":"Test Template","hostname_pattern":"device-{NNN}","ip_range":"10.0.0.0/24","target_file":"` + jsonPath(filepath.Join(dir, "hosts.conf")) + `"}`
	w, c := newJSONContext("POST", "/api/templates", body)
	createTemplateHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !hasTemplate("test-template") {
		t.Error("template should be stored with derived ID 'test-template'")
	}
}

func TestCreateTemplateHandler_Duplicate(t *testing.T) {
	dir := newTestDir(t)
	*templatepkg.TemplatesPath = filepath.Join(dir, "templates.json")
	resetTemplates()
	setTemplate("test-template", models.Template{ID: "test-template", Name: "Test Template"})

	body := `{"name":"Test Template","hostname_pattern":"device-{NNN}","ip_range":"10.0.0.0/24","target_file":"` + jsonPath(filepath.Join(dir, "hosts.conf")) + `"}`
	w, c := newJSONContext("POST", "/api/templates", body)
	createTemplateHandler(c)

	if w.Code != 409 {
		t.Fatalf("expected 409 for duplicate template, got %d", w.Code)
	}
}

func TestCreateTemplateHandler_MissingFields(t *testing.T) {
	dir := newTestDir(t)
	*templatepkg.TemplatesPath = filepath.Join(dir, "templates.json")
	resetTemplates()

	body := `{"name":"Empty"}`
	w, c := newJSONContext("POST", "/api/templates", body)
	createTemplateHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for missing fields, got %d", w.Code)
	}
}

func TestDeleteTemplateHandler_Success(t *testing.T) {
	dir := newTestDir(t)
	*templatepkg.TemplatesPath = filepath.Join(dir, "templates.json")
	resetTemplates()
	setTemplate("test-template", models.Template{ID: "test-template", Name: "Test"})

	w, c := newJSONContext("DELETE", "/api/templates/test-template", "")
	c.Params = gin.Params{{Key: "id", Value: "test-template"}}
	deleteTemplateHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if hasTemplate("test-template") {
		t.Error("template should be deleted")
	}
}

func TestDeleteTemplateHandler_NotFound(t *testing.T) {
	dir := newTestDir(t)
	*templatepkg.TemplatesPath = filepath.Join(dir, "templates.json")
	resetTemplates()

	w, c := newJSONContext("DELETE", "/api/templates/missing", "")
	c.Params = gin.Params{{Key: "id", Value: "missing"}}
	deleteTemplateHandler(c)

	if w.Code != 404 {
		t.Fatalf("expected 404 for missing template, got %d", w.Code)
	}
}

// ===== Gap 3: Edge cases =====

// TestValidateHostFields_IPv6 and TestValidHostname_Unicode moved to
// internal/validate (white-box). TestReadAllHosts_EmptyFile,
// TestReadAllHosts_CommentsOnly, TestReadAllHosts_MultipleFiles and
// TestParseDhcpHostLine_TrailingNewline moved to internal/dnsmasq (stage 4).

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

// TestRestoreBackupZip_EmptyArchive and TestRestoreBackupZip_ValidArchive
// moved to internal/dnsmasq (stage 5): they exercise restoreBackupZip
// directly (not the handler), so they live next to the implementation.

// ===== Read-only GET handlers (L2) =====

func TestGetHostsHandler_ReturnsHosts(t *testing.T) {
	dir := newTestDir(t)
	os.WriteFile(filepath.Join(dir, "hosts.conf"),
		[]byte("dhcp-host=aa:bb:cc:dd:ee:01,h1,10.0.0.1\ndhcp-host=aa:bb:cc:dd:ee:02,h2,10.0.0.2\n"), 0644)

	w, c := newJSONContext("GET", "/api/hosts", "")
	getHostsHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "aa:bb:cc:dd:ee:01") {
		t.Error("response should contain host MAC")
	}
}

func TestGetHostsHandler_EmptyDir(t *testing.T) {
	newTestDir(t)

	w, c := newJSONContext("GET", "/api/hosts", "")
	getHostsHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "[]" {
		t.Errorf("expected empty array, got %s", w.Body.String())
	}
}

func TestGetAliasesHandler_ReturnsAliases(t *testing.T) {
	dir := newTestDir(t)
	os.WriteFile(filepath.Join(dir, "aliases.conf"),
		[]byte("address=/nas.local/10.0.0.5\ncname=www.local,nas.local\n"), 0644)

	w, c := newJSONContext("GET", "/api/aliases", "")
	getAliasesHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "nas.local") {
		t.Error("response should contain alias domain")
	}
}

func TestGetConfigHandler_ReturnsSnapshot(t *testing.T) {
	dir := newTestDir(t)
	os.WriteFile(filepath.Join(dir, "test.conf"), []byte("domain-needed\nbogus-priv\n"), 0644)

	w, c := newJSONContext("GET", "/api/config", "")
	getConfigHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "files") {
		t.Error("snapshot should have 'files' field")
	}
}

func TestGetDhcpRangesHandler(t *testing.T) {
	dir := newTestDir(t)
	os.WriteFile(filepath.Join(dir, "dhcp.conf"),
		[]byte("dhcp-range=192.168.1.50,192.168.1.150,255.255.255.0,12h\n"), 0644)

	w, c := newJSONContext("GET", "/api/templates/ranges", "")
	getDhcpRangesHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ranges") {
		t.Error("response should have 'ranges' field")
	}
}

func TestGetTemplatesHandler_Empty(t *testing.T) {
	resetTemplates()

	w, c := newJSONContext("GET", "/api/templates", "")
	getTemplatesHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetUsersHandler_ReturnsUsers(t *testing.T) {
	dir := t.TempDir()
	*auth.DBPath = filepath.Join(dir, "users.json")
	setUsers(map[string]string{"admin": "hash", "alice": "hash2"})

	w, c := newJSONContext("GET", "/api/users", "")
	getUsersHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "admin") {
		t.Error("response should list admin user")
	}
}

func TestGetArpHandler_ReturnsTable(t *testing.T) {
	dir := t.TempDir()
	arpFile := filepath.Join(dir, "arp.txt")
	os.WriteFile(arpFile, []byte("IP address     HW type  Flags  HW address           Mask Device\n"+
		"192.168.1.10   0x1      0x2    aa:bb:cc:dd:ee:ff     *    eth0\n"), 0644)
	*netstate.ArpPath = arpFile
	newTestDir(t)

	w, c := newJSONContext("GET", "/api/arp", "")
	getArpHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "aa:bb:cc:dd:ee:ff") {
		t.Error("response should contain ARP MAC")
	}
}

func TestGetLeasesHandler_ReturnsLeases(t *testing.T) {
	dir := t.TempDir()
	leasesFile := filepath.Join(dir, "leases")
	os.WriteFile(leasesFile, []byte("1000 aa:bb:cc:dd:ee:ff 192.168.1.10 phone *\n"), 0644)
	*netstate.LeasesPath = leasesFile
	newTestDir(t)

	w, c := newJSONContext("GET", "/api/leases", "")
	getLeasesHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "192.168.1.10") {
		t.Error("response should contain lease IP")
	}
}

func TestGetLeasesHandler_NoFile(t *testing.T) {
	*netstate.LeasesPath = filepath.Join(t.TempDir(), "no-leases")
	newTestDir(t)

	w, c := newJSONContext("GET", "/api/leases", "")
	getLeasesHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200 (empty array), got %d", w.Code)
	}
	if w.Body.String() != "[]" {
		t.Errorf("expected empty array, got %s", w.Body.String())
	}
}

func TestGetNewDevicesHandler(t *testing.T) {
	dir := t.TempDir()
	arpFile := filepath.Join(dir, "arp.txt")
	os.WriteFile(arpFile, []byte("IP address     HW type  Flags  HW address           Mask Device\n"+
		"192.168.1.10   0x1      0x2    11:22:33:44:55:01     *    eth0\n"), 0644)
	*netstate.ArpPath = arpFile
	*netstate.LeasesPath = filepath.Join(dir, "empty.leases")
	newTestDir(t)

	w, c := newJSONContext("GET", "/api/new-devices", "")
	getNewDevicesHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// 11:22:33:44:55:01 is in ARP but not in hosts or leases → new device.
	if !strings.Contains(w.Body.String(), "11:22:33:44:55:01") {
		t.Error("response should contain the unknown device MAC")
	}
}

func TestNextIPHandler_Success(t *testing.T) {
	newTestDir(t)

	w, c := newJSONContext("GET", "/api/hosts/next-ip?range=10.99.0.0/24", "")
	nextIPHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "10.99.0.") {
		t.Errorf("expected IP in 10.99.0.x range, got %s", w.Body.String())
	}
}

func TestNextIPHandler_MissingRange(t *testing.T) {
	w, c := newJSONContext("GET", "/api/hosts/next-ip", "")
	nextIPHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for missing range, got %d", w.Code)
	}
}

func TestNextIPHandler_InvalidCIDR(t *testing.T) {
	w, c := newJSONContext("GET", "/api/hosts/next-ip?range=not-a-cidr", "")
	nextIPHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid CIDR, got %d", w.Code)
	}
}

func TestHistoryListHandler_ReturnsVersions(t *testing.T) {
	dir := newTestDir(t)
	*dnsmasq.HistoryDir = t.TempDir()
	*dnsmasq.HistoryDepth = 10
	file := filepath.Join(dir, "test.conf")
	os.WriteFile(file, []byte("domain-needed\n"), 0644)

	// Trigger a history save by calling createLocalBackup.
	dnsmasq.CreateLocalBackup(file)
	dnsmasq.CreateLocalBackup(file)

	w, c := newJSONContext("GET", "/api/history?file="+url.QueryEscape(file), "")
	historyListHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "versions") {
		t.Error("response should have 'versions' field")
	}
}

func TestHistoryListHandler_MissingFile(t *testing.T) {
	w, c := newJSONContext("GET", "/api/history", "")
	historyListHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for missing file param, got %d", w.Code)
	}
}

func TestAuditHandler_ReturnsEntries(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.log")
	entry := `{"timestamp":"2026-01-01T00:00:00Z","user":"admin","action":"add","mac":"aa:bb:cc:dd:ee:ff"}` + "\n"
	os.WriteFile(auditPath, []byte(entry), 0644)
	*audit.AuditLogPath = auditPath

	w, c := newJSONContext("GET", "/api/audit", "")
	audit.Handler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "add") {
		t.Error("response should contain audit action")
	}
}

func TestAuditHandler_NoLogFile(t *testing.T) {
	*audit.AuditLogPath = filepath.Join(t.TempDir(), "no-audit.log")

	w, c := newJSONContext("GET", "/api/audit", "")
	audit.Handler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200 (empty array), got %d", w.Code)
	}
	if w.Body.String() != "[]" {
		t.Errorf("expected empty array, got %s", w.Body.String())
	}
}

func TestBackupHandler_ReturnsZip(t *testing.T) {
	dir := newTestDir(t)
	os.WriteFile(filepath.Join(dir, "hosts.conf"), []byte("domain-needed\n"), 0644)

	w, c := newJSONContext("GET", "/api/backup", "")
	backupHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/zip") {
		t.Errorf("expected application/zip content-type, got %s", ct)
	}
	if w.Body.Len() < 50 {
		t.Errorf("zip body too small: %d bytes", w.Body.Len())
	}
}

// ===== Setup handler =====

func TestSetupHandler_Success(t *testing.T) {
	dir := t.TempDir()
	*auth.DBPath = filepath.Join(dir, "users.json")
	auth.ClearUsers()
	auth.SetSecretForTest(t, []byte("test-secret-key-32-bytes-long!!"))

	w, c := newJSONContext("POST", "/api/setup", `{"username":"admin","password":"secret123"}`)
	setupHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "token") {
		t.Error("response should contain a token")
	}
	if !auth.HasUser("admin") {
		t.Error("admin user should be created")
	}
}

func TestSetupHandler_AlreadySetup(t *testing.T) {
	dir := t.TempDir()
	*auth.DBPath = filepath.Join(dir, "users.json")
	setUsers(map[string]string{"admin": "hash"})

	w, c := newJSONContext("POST", "/api/setup", `{"username":"admin","password":"secret123"}`)
	setupHandler(c)

	if w.Code != 403 {
		t.Fatalf("expected 403 when already set up, got %d", w.Code)
	}
}

// ===== Export handlers =====

func TestExportCSVHandler(t *testing.T) {
	dir := newTestDir(t)
	os.WriteFile(filepath.Join(dir, "hosts.conf"),
		[]byte("dhcp-host=aa:bb:cc:dd:ee:ff,host1,192.168.1.10\n"), 0644)

	w, c := newJSONContext("GET", "/api/hosts/csv", "")
	exportCSVHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "aa:bb:cc:dd:ee:ff") {
		t.Error("CSV should contain host MAC")
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/csv") {
		t.Errorf("expected text/csv, got %s", ct)
	}
}

func TestExportAliasesCSVHandler(t *testing.T) {
	dir := newTestDir(t)
	os.WriteFile(filepath.Join(dir, "aliases.conf"),
		[]byte("address=/nas.local/10.0.0.5\n"), 0644)

	w, c := newJSONContext("GET", "/api/aliases/csv", "")
	exportAliasesCSVHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "nas.local") {
		t.Error("CSV should contain alias domain")
	}
}

// ===== Import handlers (multipart) =====

func TestImportCSVHandler_Success(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "hosts.csv")
	part.Write([]byte("mac,ip,hostname\naa:bb:cc:dd:ee:01,10.0.0.1,h1\n"))
	writer.WriteField("target_file", file)
	writer.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/hosts/csv", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Set("user", "admin")
	importCSVHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "count") {
		t.Error("response should have count field")
	}
}

func TestImportCSVHandler_NoFile(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("target_file", file)
	writer.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/hosts/csv", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Set("user", "admin")
	importCSVHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for missing file, got %d", w.Code)
	}
}

func TestImportAliasesCSVHandler_Success(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "aliases.conf")
	os.WriteFile(file, []byte(""), 0644)

	bodyBuf := &bytes.Buffer{}
	writer := multipart.NewWriter(bodyBuf)
	part, _ := writer.CreateFormFile("file", "aliases.csv")
	part.Write([]byte("type,domain,target\nA,a.test,10.0.0.1\n"))
	writer.WriteField("target_file", file)
	writer.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/aliases/csv", bodyBuf)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Set("user", "admin")
	importAliasesCSVHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "count") {
		t.Error("response should have count field")
	}
}

func TestImportAliasesCSVHandler_NoFile(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "aliases.conf")

	bodyBuf := &bytes.Buffer{}
	writer := multipart.NewWriter(bodyBuf)
	writer.WriteField("target_file", file)
	writer.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/aliases/csv", bodyBuf)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Set("user", "admin")
	importAliasesCSVHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for missing file, got %d", w.Code)
	}
}

// ===== Apply template handler =====

func TestApplyTemplateHandler_Success(t *testing.T) {
	dir := newTestDir(t)
	targetFile := filepath.Join(dir, "hosts.conf")
	os.WriteFile(targetFile, []byte(""), 0644)

	resetTemplates()
	setTemplate("test-tpl", models.Template{ID: "test-tpl", Name: "Test", IPRange: "10.99.0.0/24", HostnamePattern: "dev-{NNN}", TargetFile: targetFile})

	w, c := newJSONContext("POST", "/api/hosts/apply-template", `{"mac":"aa:bb:cc:dd:ee:ff","template_id":"test-tpl"}`)
	applyTemplateHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "10.99.0.") {
		t.Errorf("response should contain generated IP in 10.99.0.x: %s", w.Body.String())
	}
}

func TestApplyTemplateHandler_NotFound(t *testing.T) {
	newTestDir(t)
	resetTemplates()

	w, c := newJSONContext("POST", "/api/hosts/apply-template", `{"mac":"aa:bb:cc:dd:ee:ff","template_id":"missing"}`)
	applyTemplateHandler(c)

	if w.Code != 404 {
		t.Fatalf("expected 404 for missing template, got %d", w.Code)
	}
}

func TestApplyTemplateHandler_BadMAC(t *testing.T) {
	newTestDir(t)
	resetTemplates()
	setTemplate("test-tpl", models.Template{ID: "test-tpl"})

	w, c := newJSONContext("POST", "/api/hosts/apply-template", `{"mac":"bad","template_id":"test-tpl"}`)
	applyTemplateHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for bad MAC, got %d", w.Code)
	}
}

// ===== Coverage sweep T-A: pure helpers =====

// TestCoalesce verifies the trivial fallback helper.
func TestCoalesce(t *testing.T) {
	if got := coalesce("a", "b"); got != "a" {
		t.Errorf("coalesce(a,b) = %q, want a", got)
	}
	if got := coalesce("", "b"); got != "b" {
		t.Errorf("coalesce(\"\",b) = %q, want b", got)
	}
	if got := coalesce("", ""); got != "" {
		t.Errorf("coalesce(\"\",\"\") = %q, want empty", got)
	}
	if got := coalesce("a", ""); got != "a" {
		t.Errorf("coalesce(a,\"\") = %q, want a", got)
	}
}

// TestValidateHostTags / TestNormalizeHostTags moved to internal/validate.

// ===== Coverage sweep §3 (Этап 3): handler success-ветки =====
//
// These tests close the success/feature branches flagged in
// логи/Quality_sweep.md §3 for handlers that do NOT depend on a real
// `dnsmasq --test` (so they run on Windows too, raising both local and CI
// coverage). The dnsmasq-dependent success paths already live in
// linux_test.go (Coverage sweep B via fakeDnsmasq); here we add the
// handler-level 400 (dnsmasq_test_failed) branches there as well.

// ----- historyDiffHandler: success + diff branches (was 44%) -----

// TestHistoryDiffHandler_Success_Current covers the success-200 path with
// to="" (diff a stored version against the current on-disk content). This
// is the primary feature of the endpoint and was entirely uncovered.
func TestHistoryDiffHandler_Success_Current(t *testing.T) {
	dir := newTestDir(t)
	*dnsmasq.HistoryDir = t.TempDir()
	*dnsmasq.HistoryDepth = 5
	file := filepath.Join(dir, "h.conf")
	if err := os.WriteFile(file, []byte("line1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dnsmasq.CreateLocalBackup(file) // snapshot "line1\n"
	fromVer := firstVersion(t, file)
	// Mutate the on-disk content so the diff has something to say.
	if err := os.WriteFile(file, []byte("line1\nline2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	w, c := newJSONContext("GET", "/api/history/diff?file="+url.QueryEscape(file)+"&from="+url.QueryEscape(fromVer), "")
	historyDiffHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "\"diff\"") {
		t.Errorf("response missing diff field: %s", body)
	}
	if !strings.Contains(body, "+line2") {
		t.Errorf("diff should mark line2 as added: %s", body)
	}
}

// TestHistoryDiffHandler_Success_VersionToVersion covers the to=<version>
// branch: diff between two stored versions (not the current file).
func TestHistoryDiffHandler_Success_VersionToVersion(t *testing.T) {
	dir := newTestDir(t)
	*dnsmasq.HistoryDir = t.TempDir()
	*dnsmasq.HistoryDepth = 5
	file := filepath.Join(dir, "h.conf")
	if err := os.WriteFile(file, []byte("alpha\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dnsmasq.CreateLocalBackup(file) // version A: "alpha\n"
	if err := os.WriteFile(file, []byte("alpha\nbeta\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dnsmasq.CreateLocalBackup(file) // version B: "alpha\nbeta\n"

	versions, err := dnsmasq.ListHistory(file)
	if err != nil {
		t.Fatalf("listHistory: %v", err)
	}
	if len(versions) < 2 {
		t.Fatalf("expected >=2 versions, got %d", len(versions))
	}
	// listHistory returns newest-first: versions[0]=B, versions[1]=A.
	fromOld, toNew := versions[1].Version, versions[0].Version

	w, c := newJSONContext("GET", "/api/history/diff?file="+url.QueryEscape(file)+"&from="+url.QueryEscape(fromOld)+"&to="+url.QueryEscape(toNew), "")
	historyDiffHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "+beta") {
		t.Errorf("diff should mark beta as added: %s", w.Body.String())
	}
}

// TestHistoryDiffHandler_UnsafePath covers the isSafePath guard (400
// invalid_path) — defense-in-depth for the history diff endpoint.
func TestHistoryDiffHandler_UnsafePath(t *testing.T) {
	w, c := newJSONContext("GET", "/api/history/diff?file=/etc/passwd&from=20240101-000000", "")
	historyDiffHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400 for unsafe path, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_path") {
		t.Errorf("expected invalid_path body, got: %s", w.Body.String())
	}
}

// TestHistoryDiffHandler_CurrentNotFound covers the to="" branch when the
// current file no longer exists on disk (404 current_not_found).
func TestHistoryDiffHandler_CurrentNotFound(t *testing.T) {
	dir := newTestDir(t)
	*dnsmasq.HistoryDir = t.TempDir()
	*dnsmasq.HistoryDepth = 5
	file := filepath.Join(dir, "h.conf")
	if err := os.WriteFile(file, []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dnsmasq.CreateLocalBackup(file)
	fromVer := firstVersion(t, file)
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}

	w, c := newJSONContext("GET", "/api/history/diff?file="+url.QueryEscape(file)+"&from="+url.QueryEscape(fromVer), "")
	historyDiffHandler(c)
	if w.Code != 404 {
		t.Fatalf("expected 404 (current_not_found), got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "current_not_found") {
		t.Errorf("expected current_not_found body, got: %s", w.Body.String())
	}
}

// TestHistoryDiffHandler_UnknownToVersion covers the to=<bad-version> branch
// (404 version_not_found for the "to" side).
func TestHistoryDiffHandler_UnknownToVersion(t *testing.T) {
	dir := newTestDir(t)
	*dnsmasq.HistoryDir = t.TempDir()
	*dnsmasq.HistoryDepth = 5
	file := filepath.Join(dir, "h.conf")
	if err := os.WriteFile(file, []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dnsmasq.CreateLocalBackup(file)
	fromVer := firstVersion(t, file)

	w, c := newJSONContext("GET", "/api/history/diff?file="+url.QueryEscape(file)+"&from="+url.QueryEscape(fromVer)+"&to=19990101-000000", "")
	historyDiffHandler(c)
	if w.Code != 404 {
		t.Fatalf("expected 404 for unknown 'to' version, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "version_not_found") {
		t.Errorf("expected version_not_found body, got: %s", w.Body.String())
	}
}

// ----- changePasswordHandler: success 200 path (was 50%) -----

// TestChangePasswordHandler_Success exercises the full success path with a
// real bcrypt hash: correct old password → hash regenerated → 200. The
// pre-existing TestChangePassword used a dummy "$2a$10$1" hash that bcrypt
// rejects, so the success branch was never reached.
func TestChangePasswordHandler_Success(t *testing.T) {
	dir := t.TempDir()
	*auth.DBPath = filepath.Join(dir, "users.json")
	hash, err := bcrypt.GenerateFromPassword([]byte("old-secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	setUsers(map[string]string{"admin": string(hash)})

	w, c := newJSONContext("POST", "/api/users/password", `{"old_password":"old-secret","new_password":"new-secret"}`)
	changePasswordHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	newHash, ok := auth.GetUser("admin")
	if !ok {
		t.Fatal("admin user vanished after password change")
	}
	if newHash == string(hash) {
		t.Error("password hash should have changed")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(newHash), []byte("new-secret")); err != nil {
		t.Errorf("new hash does not verify against new password: %v", err)
	}
	// Old password must no longer verify.
	if err := bcrypt.CompareHashAndPassword([]byte(newHash), []byte("old-secret")); err == nil {
		t.Error("old password should no longer verify")
	}
}

// TestChangePasswordHandler_EmptyNewPassword covers the missing_fields guard
// (empty new_password → 400), a branch not previously exercised.
func TestChangePasswordHandler_EmptyNewPassword(t *testing.T) {
	dir := t.TempDir()
	*auth.DBPath = filepath.Join(dir, "users.json")
	setUsers(map[string]string{"admin": "$2a$10$irrelevant"})

	w, c := newJSONContext("POST", "/api/users/password", `{"old_password":"x","new_password":""}`)
	changePasswordHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for empty new_password, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "missing_fields") {
		t.Errorf("expected missing_fields body, got: %s", w.Body.String())
	}
}
