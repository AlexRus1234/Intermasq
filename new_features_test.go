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

// Tests for the v3.1 push: rate-limit reset on successful login, file
// deletion endpoint, concurrent user creation safety, usersMu enforcement.
// Kept in a separate file from dnsmasq_test.go so the new work is easy to
// review alongside the implementation diffs.

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"intermask/internal/dnsmasq"
	"intermask/internal/initd"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// ========================== Rate-limit reset ==========================

// TestRateLimitResetClearsIP verifies that rateLimitReset wipes the
// recorded attempts for the given IP, so the next request is treated as
// the first one in a fresh window.
func TestRateLimitResetClearsIP(t *testing.T) {
	rateLimitStore = make(map[string][]time.Time)
	handler := rateLimitMiddleware(3, time.Minute)

	// Two failed attempts — counter should now hold 2 entries for this IP.
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", nil)
		c.Request.RemoteAddr = "10.0.0.42:1234"
		handler(c)
	}
	rateLimitMu.Lock()
	n := len(rateLimitStore["10.0.0.42"])
	rateLimitMu.Unlock()
	if n != 2 {
		t.Fatalf("expected 2 attempts recorded, got %d", n)
	}

	rateLimitReset("10.0.0.42")

	rateLimitMu.Lock()
	_, present := rateLimitStore["10.0.0.42"]
	rateLimitMu.Unlock()
	if present {
		t.Fatal("rateLimitReset should delete the IP entry")
	}

	// After reset, the caller can submit maxAttempts requests again.
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", nil)
		c.Request.RemoteAddr = "10.0.0.42:1234"
		handler(c)
		if w.Code == 429 {
			t.Fatalf("request %d after reset should not be limited", i+1)
		}
	}
}

// TestRateLimitResetUnknownIP is a no-op safety net: clearing an IP that
// was never recorded must not panic and must not corrupt other entries.
func TestRateLimitResetUnknownIP(t *testing.T) {
	rateLimitStore = make(map[string][]time.Time)
	rateLimitStore["10.0.0.1"] = []time.Time{time.Now()}
	rateLimitReset("10.0.0.99") // not present — must not panic
	if _, ok := rateLimitStore["10.0.0.1"]; !ok {
		t.Fatal("reset of unrelated IP must not affect other entries")
	}
}

// TestLoginHandlerResetsRateLimit exercises the full loginHandler path.
// In production the rate-limit middleware populates rateLimitStore before
// loginHandler runs; in this test we populate it directly to simulate two
// failed attempts already recorded, then verify the successful login
// wipes the counter.
func TestLoginHandlerResetsRateLimit(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.DefaultCost)
	users = map[string]string{"alice": string(hash)}
	usersMu = sync.RWMutex{}
	rateLimitStore = make(map[string][]time.Time)

	// Simulate two failed attempts that the middleware would have recorded.
	rateLimitStore["10.0.0.7"] = []time.Time{time.Now(), time.Now()}

	// Successful login.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"alice","password":"correct-horse"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.RemoteAddr = "10.0.0.7:1"
	loginHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 on correct password, got %d: %s", w.Code, w.Body.String())
	}

	// Counter must be wiped — successful login resets the slate.
	rateLimitMu.Lock()
	_, present := rateLimitStore["10.0.0.7"]
	rateLimitMu.Unlock()
	if present {
		t.Fatal("successful login must clear rate-limit counter for that IP")
	}
}

// ========================== File deletion ==========================

// TestDeleteConfigFileRemovesFileAndBak checks that the backend function
// removes both the .conf file and its .bak sibling (a leftover .bak for a
// deleted file serves no purpose and confuses the UI's "show rollback
// button" logic).
func TestDeleteConfigFileRemovesFileAndBak(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	*HistoryDir = filepath.Join(dir, "history")
	*HistoryDepth = 5
	path := filepath.Join(dir, "old.conf")
	os.WriteFile(path, []byte("domain-needed\n"), 0644)
	os.WriteFile(path+".bak", []byte("domain-needed\n"), 0644)

	if err := deleteConfigFile(path); err != nil {
		t.Fatalf("deleteConfigFile failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error(".conf file should be removed")
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error(".bak sibling should also be removed")
	}
}

// TestDeleteConfigFileSavesHistory verifies that the file's content is
// snapshotted into versioned history BEFORE the physical removal, so the
// operator can recover via the history modal.
func TestDeleteConfigFileSavesHistory(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	*HistoryDir = filepath.Join(dir, "history")
	*HistoryDepth = 5
	path := filepath.Join(dir, "doomed.conf")
	content := []byte("# managed by Intermasq\ndomain-needed\nserver=1.1.1.1\n")
	os.WriteFile(path, content, 0644)

	if err := deleteConfigFile(path); err != nil {
		t.Fatalf("deleteConfigFile failed: %v", err)
	}

	versions, err := listHistory(path)
	if err != nil {
		t.Fatalf("listHistory after delete: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 history version saved before deletion, got %d", len(versions))
	}
	saved, err := readHistoryVersion(path, versions[0].Version)
	if err != nil {
		t.Fatalf("readHistoryVersion: %v", err)
	}
	if string(saved) != string(content) {
		t.Errorf("saved history content does not match pre-deletion file:\nwant: %q\ngot:  %q", content, saved)
	}
}

// TestDeleteConfigFileRejectsUnsafePath ensures path-traversal defence
// holds: a path outside ConfigDir must be refused with os.ErrPermission.
func TestDeleteConfigFileRejectsUnsafePath(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	*HistoryDir = filepath.Join(dir, "history")
	outside := filepath.Join(dir, "..", "escape.conf")
	err := deleteConfigFile(outside)
	if err != os.ErrPermission {
		t.Errorf("expected os.ErrPermission for path outside ConfigDir, got %v", err)
	}
}

// TestDeleteConfigFileMissingReturnsNotExist — deleting a file that
// doesn't exist should bubble up os.ErrNotExist so the handler can
// produce a 404.
func TestDeleteConfigFileMissingReturnsNotExist(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	*HistoryDir = filepath.Join(dir, "history")
	path := filepath.Join(dir, "ghost.conf")
	err := deleteConfigFile(path)
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

// TestDeleteConfigFileHandlerSuccess exercises the HTTP handler end-to-end:
// existing file → 200, audit entry written, snapshot returned without the
// deleted file.
func TestDeleteConfigFileHandlerSuccess(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	*HistoryDir = filepath.Join(dir, "history")
	*HistoryDepth = 5
	*AuditLogPath = filepath.Join(dir, "audit.log")

	path := filepath.Join(dir, "trash.conf")
	os.WriteFile(path, []byte("# Managed by Intermasq\ndomain-needed\n"), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/config/file", strings.NewReader(fmt.Sprintf(`{"file":%q}`, path)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	deleteConfigFileHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be physically deleted after successful handler call")
	}

	// Audit log should contain a config_delete_file entry.
	data, _ := os.ReadFile(*AuditLogPath)
	if !strings.Contains(string(data), "config_delete_file") {
		t.Errorf("audit log should record config_delete_file:\n%s", data)
	}

	// Response body must be a ConfigSnapshot, and the deleted file must
	// not appear in it.
	var snap ConfigSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("response is not a ConfigSnapshot: %v", err)
	}
	for _, f := range snap.Files {
		if f.Path == path {
			t.Errorf("deleted file should not appear in returned snapshot")
		}
	}
}

// TestDeleteConfigFileHandlerUnsafePath returns 403 + access_denied.
// The path must itself look like a .conf filename — otherwise the extension
// check (which comes first) returns 400 invalid_filename before isSafePath
// even runs.
func TestDeleteConfigFileHandlerUnsafePath(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	*HistoryDir = filepath.Join(dir, "history")

	evilPath := filepath.Join(dir, "..", "escape.conf")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/config/file", strings.NewReader(fmt.Sprintf(`{"file":%q}`, evilPath)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	deleteConfigFileHandler(c)

	if w.Code != 403 {
		t.Fatalf("expected 403 for unsafe path, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "access_denied") {
		t.Errorf("expected error=access_denied, got %s", w.Body.String())
	}
}

// TestDeleteConfigFileHandlerNonConfExtension returns 400 — we never
// delete files that aren't dnsmasq config files, even if they live in
// ConfigDir.
func TestDeleteConfigFileHandlerNonConfExtension(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	*HistoryDir = filepath.Join(dir, "history")
	path := filepath.Join(dir, "notes.txt")
	os.WriteFile(path, []byte("hi"), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/config/file", strings.NewReader(fmt.Sprintf(`{"file":%q}`, path)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	deleteConfigFileHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 for non-.conf path, got %d", w.Code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("non-.conf file should not be touched by the handler")
	}
}

// TestDeleteConfigFileHandlerMissing returns 404.
func TestDeleteConfigFileHandlerMissing(t *testing.T) {
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	*HistoryDir = filepath.Join(dir, "history")
	path := filepath.Join(dir, "ghost.conf")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/config/file", strings.NewReader(fmt.Sprintf(`{"file":%q}`, path)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	deleteConfigFileHandler(c)

	if w.Code != 404 {
		t.Fatalf("expected 404 for missing file, got %d: %s", w.Code, w.Body.String())
	}
}

// ========================== Concurrent user creation ==========================

// TestConcurrentCreateUserNoLostRecords hammers createUserHandler from
// many goroutines at once. Before introducing usersMu the global mutex
// still serialised the map write, but if any future refactor moves to a
// finer-grained lock this test will catch the regression: every username
// from every concurrent request must end up persisted to disk.
func TestConcurrentCreateUserNoLostRecords(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{}
	usersMu = sync.RWMutex{}
	*AuditLogPath = filepath.Join(dir, "audit.log")

	const n = 30
	var wg sync.WaitGroup
	var ok int64
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"username":"user-%02d","password":"pw-%02d"}`, i, i)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/users", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", "admin")
			createUserHandler(c)
			if w.Code == 200 {
				atomic.AddInt64(&ok, 1)
			}
		}(i)
	}
	wg.Wait()

	if int(ok) != n {
		t.Fatalf("expected %d successful creates, got %d", n, ok)
	}

	// Reload from disk and verify every user survived.
	users = map[string]string{}
	loadUsers()
	if len(users) != n {
		t.Errorf("expected %d persisted users, got %d", n, len(users))
	}
}

// TestConcurrentCreateUserDuplicateNoCorruption — many goroutines try to
// create the SAME username. Only one must win, the rest must get 409
// user_exists, and the underlying map + JSON file must stay consistent
// (no panic, no duplicate keys in JSON which would be a parse error on
// reload).
func TestConcurrentCreateUserDuplicateNoCorruption(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{}
	usersMu = sync.RWMutex{}
	*AuditLogPath = filepath.Join(dir, "audit.log")

	const n = 20
	var wg sync.WaitGroup
	var success, conflict int64
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			body := `{"username":"dup","password":"pw"}`
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/users", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", "admin")
			createUserHandler(c)
			switch w.Code {
			case 200:
				atomic.AddInt64(&success, 1)
			case 409:
				atomic.AddInt64(&conflict, 1)
			}
		}()
	}
	wg.Wait()

	if success != 1 {
		t.Errorf("expected exactly 1 successful create, got %d", success)
	}
	if conflict != n-1 {
		t.Errorf("expected %d conflicts, got %d", n-1, conflict)
	}

	// Reload must not fail (corrupted JSON would panic at startup).
	users = map[string]string{}
	loadUsers()
	if len(users) != 1 {
		t.Errorf("expected 1 persisted user, got %d", len(users))
	}
	if _, ok := users["dup"]; !ok {
		t.Error("user 'dup' not in reloaded map")
	}
}

// TestStatusHandlerSafeUnderConcurrentUserWrite guards the len(users)
// read in statusHandler against the data race that exists without
// usersMu: Go's runtime explicitly panics on concurrent map read+write,
// and len(map) counts as a read.
func TestStatusHandlerSafeUnderConcurrentUserWrite(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{}
	usersMu = sync.RWMutex{}
	*AuditLogPath = filepath.Join(dir, "audit.log")
	// statusHandler calls initd.Current().IsActive("dnsmasq"); the test
	// binary never runs initd.Init, so wire up the no-op caller manually.
	initd.SetCurrentForTest(t, &initd.NoneCaller{})

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n * 2)
	// Writers.
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"username":"racer-%02d","password":"x"}`, i)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/users", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", "admin")
			createUserHandler(c)
		}(i)
	}
	// Readers (status handler reads len(users)).
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/status", nil)
			statusHandler(c)
		}()
	}
	wg.Wait()
	// The test passes if no goroutine triggered "fatal error: concurrent map
	// read and map write" — that would crash the test binary outright.
}
