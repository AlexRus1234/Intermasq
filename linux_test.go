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

// Coverage sweep block B (логи/Coverage_sweep.md §2.B): Linux-gated tests
// that exercise dnsmasq-dependent success paths by injecting a fake
// `dnsmasq` shell-script via the writable package var `dnsmasqBinPath`
// (see bins.go:30). On Windows the shebang script is not executable by
// `os/exec`, so every test here skips on runtime.GOOS == "windows".
//
// Not safe to run in t.Parallel(): we mutate the global `dnsmasqBinPath` and
// `sysCaller` package vars. Tests save/restore via t.Cleanup.

import (
	"archive/zip"
	"bytes"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// fakeDnsmasq writes a shell-script "dnsmasq" that exits with `exitCode`
// into a temp dir, points the package var `dnsmasqBinPath` at it, and
// registers cleanup to restore the previous value. The script honours the
// shebang convention; on non-Linux hosts callers must t.Skip() beforehand.
//
// If exitCode is 0, the script behaves like the real dnsmasq's `--test`
// success path. If exitCode != 0, the script prints a marker to stderr+stdout
// (so CombinedOutput captures a non-empty string) and exits with that code.
func fakeDnsmasq(t *testing.T, exitCode int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-dnsmasq shell-script unsupported on Windows")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "dnsmasq")
	var script string
	if exitCode == 0 {
		script = "#!/bin/sh\nexit 0\n"
	} else {
		script = "#!/bin/sh\necho 'fake dnsmasq: test failed'\nexit " + itoa(exitCode) + "\n"
	}
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake dnsmasq: %v", err)
	}
	orig := dnsmasqBinPath
	dnsmasqBinPath = bin
	t.Cleanup(func() { dnsmasqBinPath = orig })
}

// itoa is a tiny stdlib-free itoa so we avoid importing strconv just for the
// script-assembly line above. Supports non-negative ints only.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// withSysCaller swaps the global `sysCaller` for the duration of the test.
// Used to make reloadDnsmasq's Restart() call deterministic (success via
// NoneCaller, failure via failCaller below).
func withSysCaller(t *testing.T, c SystemCaller) {
	t.Helper()
	orig := sysCaller
	sysCaller = c
	t.Cleanup(func() { sysCaller = orig })
}

// failCaller is a minimal SystemCaller whose Restart returns an error and
// IsActive returns false. Used to drive the Post-test-failure branch of
// reloadDnsmasq (where dnsmasq --test succeeded but the init caller failed).
type failCaller struct{}

func (f *failCaller) IsActive(service string) bool { return false }
func (f *failCaller) Restart(service string) error { return errFailCallerRestart }
func (f *failCaller) RestartSelf() error           { return errFailCallerRestart }
func (f *failCaller) String() string               { return "fail" }

var errFailCallerRestart = errors.New("caller restart failed")

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

// ===== T-B.1 writeConfigWithTest =====

func TestWriteConfigWithTest_Success(t *testing.T) {
	fakeDnsmasq(t, 0)
	dir := newTestDir(t)
	path := filepath.Join(dir, "x.conf")
	if err := os.WriteFile(path, []byte("# old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigWithTest(path, []byte("# new\ndomain=lan\n")); err != nil {
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
	err := writeConfigWithTest(path, []byte("# would-be-bad\n"))
	if err == nil || !strings.HasPrefix(err.Error(), "dnsmasq_test_failed") {
		t.Fatalf("expected dnsmasq_test_failed, got %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, orig) {
		t.Errorf("rollback failed: file content changed (got %q, want %q)", got, orig)
	}
}

// ===== T-B.2 restoreHistoryVersion =====

// newestHistoryVersion reads HistoryDir via listHistory and returns the
// identifier of the most recent stored version for filePath (listHistory
// already sorts newest-first). Fatal if no version is present.
func newestHistoryVersion(t *testing.T, filePath string) string {
	t.Helper()
	versions, err := listHistory(filePath)
	if err != nil {
		t.Fatalf("listHistory: %v", err)
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
	saveHistory(path)
	version := newestHistoryVersion(t, path)
	// Now mutate the file, then restore the snapshot.
	if err := os.WriteFile(path, []byte("domain=other.lan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := restoreHistoryVersion(path, version); err != nil {
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
	saveHistory(path)
	version := newestHistoryVersion(t, path)
	mutated := []byte("domain=mutated\n")
	if err := os.WriteFile(path, mutated, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := restoreHistoryVersion(path, version); err == nil || !strings.HasPrefix(err.Error(), "dnsmasq_test_failed") {
		t.Fatalf("expected dnsmasq_test_failed, got %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, mutated) {
		t.Errorf("expected rollback to mutated content (pre-restore), got %q", got)
	}
}

// ===== T-B.3 + T-B.4 reloadDnsmasq / reloadHandler =====

func TestReloadDnsmasq_Success(t *testing.T) {
	fakeDnsmasq(t, 0)
	withSysCaller(t, &NoneCaller{})
	if err := reloadDnsmasq(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestReloadDnsmasq_TestFail(t *testing.T) {
	fakeDnsmasq(t, 1)
	withSysCaller(t, &NoneCaller{})
	if err := reloadDnsmasq(); err == nil || !strings.Contains(err.Error(), "fake dnsmasq") {
		t.Fatalf("expected dnsmasq-test failure propagated, got %v", err)
	}
}

func TestReloadDnsmasq_CallerFail(t *testing.T) {
	fakeDnsmasq(t, 0)
	withSysCaller(t, &failCaller{})
	if err := reloadDnsmasq(); err == nil {
		t.Fatal("expected caller-restart error, got nil")
	}
}

func TestReloadHandler_200(t *testing.T) {
	fakeDnsmasq(t, 0)
	withSysCaller(t, &NoneCaller{})
	w, c := newJSONContext("POST", "/api/reload", "")
	reloadHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReloadHandler_400(t *testing.T) {
	fakeDnsmasq(t, 1)
	withSysCaller(t, &NoneCaller{})
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
	saveHistory(path)
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

// multipartWriter fills `dst` with a multipart/form-data body carrying a
// single field named "file" with the given filename and contents, and
// returns the writer so the caller can read FormDataContentType().
func multipartWriter(t *testing.T, dst *bytes.Buffer, filename string, content []byte) *multipart.Writer {
	t.Helper()
	mw := multipart.NewWriter(dst)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return mw
}

// ===== T-C.5 loadPlugins =====

// TestLoadPlugins_FakeDir exercises loadPlugins against a temp PluginsDir
// with a manifest.json + a trivial shell-script binary. Verifies:
//   - the reverse-proxy route /plugins/<id>/* is registered on the engine,
//   - loadedPlugins gains exactly one entry matching the manifest id,
//   - SocketsDir is created.
//
// Linux-only: the fake plugin is a #!/bin/sh "sleep 1" stub so cmd.Start()
// succeeds. On Windows the shebang trick doesn't work → skip.
func TestLoadPlugins_FakeDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake plugin binary (shell-script) unsupported on Windows")
	}

	// Override the two package vars loadPlugins reads. Cleanup restores.
	origPluginsDir := PluginsDir
	origSocketsDir := SocketsDir
	origLoaded := loadedPlugins
	pluginsRoot := t.TempDir()
	socketsRoot := t.TempDir()
	PluginsDir = pluginsRoot
	SocketsDir = socketsRoot
	loadedPlugins = nil
	t.Cleanup(func() {
		PluginsDir = origPluginsDir
		SocketsDir = origSocketsDir
		loadedPlugins = origLoaded
	})

	// Build a fake plugin directory: manifest.json + executable shell-script.
	pluginDir := filepath.Join(pluginsRoot, "demo")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"id":"demo","name":"Demo Plugin","bin":"demo"}`)
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	// "sleep 60" keeps the process alive for the duration of the test so
	// cmd.Start() succeeds but the script does nothing. It will be reaped
	// when the test process tears down its temp dir.
	script := []byte("#!/bin/sh\nsleep 60\n")
	binPath := filepath.Join(pluginDir, "demo")
	if err := os.WriteFile(binPath, script, 0o755); err != nil {
		t.Fatal(err)
	}

	// Use a throwaway gin engine in release mode to keep the log quiet.
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	loadPlugins(r)

	// Assert SocketsDir was created (loadPlugins does MkdirAll).
	if _, err := os.Stat(SocketsDir); err != nil {
		t.Errorf("expected SocketsDir created, got stat err: %v", err)
	}

	// Assert the reverse-proxy route is registered.
	var found bool
	for _, route := range r.Routes() {
		if strings.HasSuffix(route.Path, "/plugins/demo/*any") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected /plugins/demo/*any route registered; got routes: %+v", r.Routes())
	}

	// Assert loadedPlugins gained the demo entry.
	var saw bool
	for _, p := range loadedPlugins {
		if p.ID == "demo" && p.Name == "Demo Plugin" {
			saw = true
			break
		}
	}
	if !saw {
		t.Errorf("expected loadedPlugins to include demo; got %+v", loadedPlugins)
	}
}

// TestLoadPlugins_NoDir covers the early-return path when PluginsDir does
// not exist: loadPlugins must simply return without panicking and without
// registering any plugin routes. Portable (no exec).
func TestLoadPlugins_NoDir(t *testing.T) {
	origPluginsDir := PluginsDir
	origSocketsDir := SocketsDir
	origLoaded := loadedPlugins
	PluginsDir = "/path/that/does/not/exist/plugins-12345"
	SocketsDir = filepath.Join(t.TempDir(), "sockets")
	loadedPlugins = nil
	t.Cleanup(func() {
		PluginsDir = origPluginsDir
		SocketsDir = origSocketsDir
		loadedPlugins = origLoaded
	})

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	loadPlugins(r)

	if len(loadedPlugins) != 0 {
		t.Errorf("expected no plugins loaded from missing dir, got %+v", loadedPlugins)
	}
	for _, route := range r.Routes() {
		if strings.Contains(route.Path, "/plugins/") {
			t.Errorf("expected no plugin routes; got %s", route.Path)
		}
	}
}

// TestLoadPlugins_BrokenManifest covers the json.Unmarshal-failure path:
// a malformed manifest.json must be silently skipped (loadPlugins just
// `continue`s). Linux-only because we still register the bin exec.
func TestLoadPlugins_BrokenManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake plugin binary (shell-script) unsupported on Windows")
	}
	origPluginsDir := PluginsDir
	origSocketsDir := SocketsDir
	origLoaded := loadedPlugins
	pluginsRoot := t.TempDir()
	PluginsDir = pluginsRoot
	SocketsDir = filepath.Join(t.TempDir(), "socks")
	loadedPlugins = nil
	t.Cleanup(func() {
		PluginsDir = origPluginsDir
		SocketsDir = origSocketsDir
		loadedPlugins = origLoaded
	})

	pluginDir := filepath.Join(pluginsRoot, "broken")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	loadPlugins(r)

	if len(loadedPlugins) != 0 {
		t.Errorf("expected no plugins from bad manifest, got %+v", loadedPlugins)
	}
	for _, route := range r.Routes() {
		if strings.Contains(route.Path, "/plugins/broken") {
			t.Errorf("broken manifest should not register a route; got %s", route.Path)
		}
	}
}
