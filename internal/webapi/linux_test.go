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

// Coverage sweep block B (логи/Coverage_sweep.md §2.B): Linux-gated tests
// that exercise dnsmasq-dependent success paths by injecting a fake
// `dnsmasq` shell-script via bins.SetPathForTest (internal/bins). On Windows
// the shebang script is not executable by `os/exec`, so every test here
// skips on runtime.GOOS == "windows".
//
// Not safe to run in t.Parallel(): we mutate the global bin-path state (via
// bins.SetPathForTest) and the `sysCaller` package var. Tests save/restore
// via t.Cleanup.
//
// The fake-bin harness (fakeDnsmasq / fakeDnsmasqArgvInspect /
// fakeDnsmasqStrict / fakeBin / setBinPath / itoa), the history helpers
// (withHistoryDir / newestHistoryVersion / firstVersion) and multipartWriter
// live in helpers_test.go.

import (
	"archive/zip"
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"intermask/internal/dnsmasq"
	"intermask/internal/initd"
)

// ===== T-B.3 + T-B.4 reloadDnsmasq / reloadHandler =====
// (TestReloadDnsmasq_Success / _TestFail / _CallerFail moved to internal/control
// during stage 9 of the modularization; the handler-level TestReloadHandler_*
// stay here.)

func TestReloadHandler_200(t *testing.T) {
	fakeDnsmasq(t, 0)
	initd.SetCurrentForTest(t, &initd.NoneCaller{})
	w, c := newJSONContext("POST", "/api/reload", "")
	reloadHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReloadHandler_400(t *testing.T) {
	fakeDnsmasq(t, 1)
	initd.SetCurrentForTest(t, &initd.NoneCaller{})
	w, c := newJSONContext("POST", "/api/reload", "")
	reloadHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ===== T-B.5 putFileHandler =====

func TestPutFileHandler_Success(t *testing.T) {
	fakeDnsmasq(t, 0)
	dir := newTestDir(t)
	name := "raw.conf"
	body := `{"content":"# edited\ndomain=lan\n"}`
	w, c := newJSONContext("PUT", "/api/files/"+name, body)
	c.Params = gin.Params{{Key: "name", Value: name}}
	putFileHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	got, _ := os.ReadFile(filepath.Join(dir, name))
	if !bytes.Contains(got, []byte("domain=lan")) {
		t.Errorf("file not written: %q", got)
	}
}

// ===== T-B.6 updateConfigHandler =====

func TestUpdateConfigHandler_Success(t *testing.T) {
	fakeDnsmasq(t, 0)
	dir := newTestDir(t)
	target := filepath.Join(dir, "conf.cfg")
	body := `{"file":"` + jsonPath(target) + `","directives":[{"key":"domain","value":"lan","active":true}]}`
	w, c := newJSONContext("PUT", "/api/config", body)
	updateConfigHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	got, _ := os.ReadFile(target)
	if !bytes.Contains(got, []byte("domain=lan")) {
		t.Errorf("file not serialized: %q", got)
	}
}

// ===== T-B.7 restoreBackupHandler =====

func TestRestoreBackupHandler_Success(t *testing.T) {
	fakeDnsmasq(t, 0)
	dir := newTestDir(t)

	// Build a ZIP containing one .conf entry.
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	w, err := zw.Create("alpha.conf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("# restored\ndomain=lan\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	// multipart upload: name=file, filename=backup.zip
	mpBuf := &bytes.Buffer{}
	mw := multipartWriter(t, mpBuf, "backup.zip", buf.Bytes())
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/api/backup/restore", mpBuf)
	c.Request.Header.Set("Content-Type", mw.FormDataContentType())
	c.Set("user", "admin")
	restoreBackupHandler(c)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got, _ := os.ReadFile(filepath.Join(dir, "alpha.conf"))
	if !bytes.Contains(got, []byte("domain=lan")) {
		t.Errorf("restore did not unpack: %q", got)
	}
}

// ===== T-B.8 historyRestoreHandler =====

func TestHistoryRestoreHandler_Success(t *testing.T) {
	fakeDnsmasq(t, 0)
	dir := newTestDir(t)
	withHistoryDir(t)
	path := filepath.Join(dir, "hist.conf")
	orig := []byte("domain=lan\n")
	if err := os.WriteFile(path, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	dnsmasq.SaveHistory(path)
	version := newestHistoryVersion(t, path)
	if err := os.WriteFile(path, []byte("domain=mutated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"file":"` + jsonPath(path) + `","version":"` + version + `"}`
	w, c := newJSONContext("POST", "/api/history/restore", body)
	historyRestoreHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, orig) {
		t.Errorf("restore mismatch: got %q, want %q", got, orig)
	}
}

// TestRollbackHandler_Success covers the successful rollback path with the
// dnsmasq validation required by RollbackFile. The fake keeps this test
// deterministic while fakeDnsmasq skips it on platforms that cannot execute
// the project's shell-script test double.
func TestRollbackHandler_Success(t *testing.T) {
	fakeDnsmasq(t, 0)
	dir := newTestDir(t)
	*dnsmasq.HistoryDir = t.TempDir()
	*dnsmasq.HistoryDepth = 5
	file := filepath.Join(dir, "r.conf")
	if err := os.WriteFile(file, []byte("new-broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file+".bak", []byte("old-good\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := `{"file":"` + jsonPath(file) + `"}`
	w, c := newJSONContext("POST", "/api/rollback", body)
	rollbackHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "rollback_ok") {
		t.Errorf("expected rollback_ok body, got: %s", w.Body.String())
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old-good\n" {
		t.Errorf("file should be restored from .bak: got %q", got)
	}
}

// ===== T-B.9 putFileHandler: dnsmasq-test-failure → 400 + rollback (A13) =====
//
// Coverage sweep §3 (Этап 3): the handler-level branch where writeFileRaw
// returns a dnsmasq_test_failed error. Coverage sweep B only exercised the
// success path (T-B.5); the rollback-on-invalid-syntax feature (A13) was
// left at the 0-coverage 400 branch. fakeDnsmasq(1) makes dnsmasq --test
// fail, so the file must be rolled back from .bak and the handler must
// respond 400 dnsmasq_test_failed.

func TestPutFileHandler_DnsmasqTestFail_400(t *testing.T) {
	fakeDnsmasq(t, 1)
	dir := newTestDir(t)
	name := "raw.conf"
	path := filepath.Join(dir, name)
	orig := []byte("# preserved\n")
	if err := os.WriteFile(path, orig, 0o644); err != nil {
		t.Fatal(err)
	}

	body := `{"content":"# would-be-bad\ndomain=lan\n"}`
	w, c := newJSONContext("PUT", "/api/files/"+name, body)
	c.Params = gin.Params{{Key: "name", Value: name}}
	putFileHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 on dnsmasq test failure, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "dnsmasq_test_failed") {
		t.Errorf("expected dnsmasq_test_failed body, got: %s", w.Body.String())
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, orig) {
		t.Errorf("rollback failed: file changed (got %q, want %q)", got, orig)
	}
}

// ===== T-B.10 updateConfigHandler: dnsmasq-test-failure → 400 + rollback =====

func TestUpdateConfigHandler_DnsmasqTestFail_400(t *testing.T) {
	fakeDnsmasq(t, 1)
	dir := newTestDir(t)
	target := filepath.Join(dir, "conf.conf")
	if err := os.WriteFile(target, []byte("# preserved\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := `{"file":"` + jsonPath(target) + `","directives":[{"key":"domain","value":"lan","active":true}]}`
	w, c := newJSONContext("PUT", "/api/config", body)
	updateConfigHandler(c)

	if w.Code != 400 {
		t.Fatalf("expected 400 on dnsmasq test failure, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "dnsmasq_test_failed") {
		t.Errorf("expected dnsmasq_test_failed body, got: %s", w.Body.String())
	}
	got, _ := os.ReadFile(target)
	if !bytes.Contains(got, []byte("preserved")) {
		t.Errorf("rollback failed: original content lost (got %q)", got)
	}
}

// ===== T-B.11 restoreBackupHandler: dnsmasq-test-failure → 400 + rollback =====

func TestRestoreBackupHandler_DnsmasqTestFail_400(t *testing.T) {
	fakeDnsmasq(t, 1)
	dir := newTestDir(t)
	// Pre-existing file that the restore would overwrite; after a failed
	// dnsmasq --test it must be rolled back from .restore.bak.
	existing := filepath.Join(dir, "alpha.conf")
	if err := os.WriteFile(existing, []byte("# original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	zipBuf := &bytes.Buffer{}
	zw := zip.NewWriter(zipBuf)
	fw, err := zw.Create("alpha.conf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("# restored\ndomain=lan\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	mpBuf := &bytes.Buffer{}
	mw := multipartWriter(t, mpBuf, "backup.zip", zipBuf.Bytes())
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/api/backup/restore", mpBuf)
	c.Request.Header.Set("Content-Type", mw.FormDataContentType())
	c.Set("user", "admin")
	restoreBackupHandler(c)

	if rec.Code != 400 {
		t.Fatalf("expected 400 on dnsmasq test failure, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dnsmasq_test_failed") {
		t.Errorf("expected dnsmasq_test_failed body, got: %s", rec.Body.String())
	}
	got, _ := os.ReadFile(existing)
	if !bytes.Contains(got, []byte("original")) {
		t.Errorf("rollback failed: original content lost (got %q)", got)
	}
}

// ===== A13/A14 wiring guards: dnsmasq must be invoked with --conf-file= =====
//
// predrel-test-remediation-P1 (логи/predrel-test-remediation-p1.md §P1.4):
// the existing _DnsmasqTestFail_400 tests above use fakeDnsmasq(t, 1) whose
// script body is `exit N` and IGNORES argv — so they pass identically whether
// dnsmasq is invoked as `--test` (the A13/A14 bug) or `--test --conf-file=`
// (the fix). These wiring tests use fakeDnsmasqArgvInspect to capture argv
// and assert the fix is actually in place. A future regression that drops
// `--conf-file=` (e.g. reverting dnsmasq.go:77/97 or backup.go's per-file
// loop) will fail here with a clear message rather than slip through.

// TestPutFileHandler_PassesConfFileToTest drives putFileHandler (which calls
// writeFileRaw → dnsmasq.go:77) with a succeeding fake dnsmasq and asserts
// the captured argv contains `--conf-file=`. Guards the A13-fix wiring for
// the raw PUT path.
func TestPutFileHandler_PassesConfFileToTest(t *testing.T) {
	_, logPath := fakeDnsmasqArgvInspect(t, 0)
	newTestDir(t)
	name := "raw.conf"
	body := `{"content":"domain=lan\n"}`
	w, c := newJSONContext("PUT", "/api/files/"+name, body)
	c.Params = gin.Params{{Key: "name", Value: name}}
	putFileHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	argv := readArgvLog(t, logPath)
	if !strings.Contains(argv, "--conf-file=") {
		t.Errorf("dnsmasq invoked without --conf-file= (A13 regression): argv=%q", argv)
	}
}

// TestUpdateConfigHandler_PassesConfFileToTest drives updateConfigHandler
// (which calls writeConfigWithTest → dnsmasq.go:97) and asserts the captured
// argv contains `--conf-file=`. Guards the A13-fix wiring for the directive
// editor path.
func TestUpdateConfigHandler_PassesConfFileToTest(t *testing.T) {
	_, logPath := fakeDnsmasqArgvInspect(t, 0)
	dir := newTestDir(t)
	target := filepath.Join(dir, "conf.conf")
	if err := os.WriteFile(target, []byte("# preserved\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := `{"file":"` + jsonPath(target) + `","directives":[{"key":"domain","value":"lan","active":true}]}`
	w, c := newJSONContext("PUT", "/api/config", body)
	updateConfigHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	argv := readArgvLog(t, logPath)
	if !strings.Contains(argv, "--conf-file=") {
		t.Errorf("dnsmasq invoked without --conf-file= (A13 regression): argv=%q", argv)
	}
}

// TestRestoreBackupHandler_PassesConfFileToTest drives restoreBackupHandler
// (which calls restoreBackupZip → backup.go) and asserts the captured argv
// contains `--conf-file=`. Guards the A14-fix wiring (per-file --conf-file=
// loop, predrel-test-remediation-P1, 2026-08-02). Before the A14 fix this
// test would have failed because restoreBackupZip called bare `dnsmasq --test`.
func TestRestoreBackupHandler_PassesConfFileToTest(t *testing.T) {
	_, logPath := fakeDnsmasqArgvInspect(t, 0)
	newTestDir(t)

	zipBuf := &bytes.Buffer{}
	zw := zip.NewWriter(zipBuf)
	fw, err := zw.Create("alpha.conf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("# restored\ndomain=lan\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	mpBuf := &bytes.Buffer{}
	mw := multipartWriter(t, mpBuf, "backup.zip", zipBuf.Bytes())
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/api/backup/restore", mpBuf)
	c.Request.Header.Set("Content-Type", mw.FormDataContentType())
	c.Set("user", "admin")
	restoreBackupHandler(c)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	argv := readArgvLog(t, logPath)
	if !strings.Contains(argv, "--conf-file=") {
		t.Errorf("dnsmasq invoked without --conf-file= (A14 regression): argv=%q", argv)
	}
}
