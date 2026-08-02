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

// fakeDnsmasqArgvInspect is like fakeDnsmasq but the script also writes its
// own argv ($@) to a sibling log file before exiting. Used by wiring-tests
// (A13/A14 regression guards) that need to assert dnsmasq was invoked with
// `--conf-file=<path>` rather than bare `--test`. Returns the binary path
// (for completeness) and the log path (to be passed to readArgvLog).
//
// Unlike fakeDnsmasq, this helper does NOT echo a marker to stderr on
// exitCode!=0 — the focus here is argv capture, not error-body shape.
// Callers wanting the marker body for `dnsmasq_test_failed` matching should
// keep using fakeDnsmasq.
func fakeDnsmasqArgvInspect(t *testing.T, exitCode int) (binPath, logPath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-dnsmasq shell-script unsupported on Windows")
	}
	dir := t.TempDir()
	binPath = filepath.Join(dir, "dnsmasq")
	logPath = filepath.Join(dir, "argv.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + logPath + "\"\nexit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake dnsmasq (argv-inspect): %v", err)
	}
	orig := dnsmasqBinPath
	dnsmasqBinPath = binPath
	t.Cleanup(func() { dnsmasqBinPath = orig })
	return binPath, logPath
}

// readArgvLog reads the argv capture file produced by fakeDnsmasqArgvInspect.
// Fatal-fails the test if the file is missing — wiring tests always expect a
// capture (i.e. dnsmasq was actually invoked).
func readArgvLog(t *testing.T, logPath string) string {
	t.Helper()
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read argv log %s: %v", logPath, err)
	}
	return string(b)
}

// fakeBin writes a shell-script binary `name` (with `script` body, no
// shebang) into a temp dir, installs it under the matching *BinPath package
// var (bins.go:30-35) for the duration of the test, and registers cleanup to
// restore the previous value. Recognised names: "dnsmasq", "sudo",
// "systemctl", "service", "rc-service", "sv". Windows is skipped because the
// shebang trick is not honoured by os/exec there.
//
// This is the seam used by coverage sweep block D (§3.T-D): all SystemCaller
// methods call sudoBin()/systemctlBin()/... which read these package vars —
// so pointing them at fake scripts exercises the exec-wiring without a real
// init system. See логи/Coverage_sweep.md §2.D ("vanity-покрытие").
func fakeBin(t *testing.T, name, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script binary unsupported on Windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, name)
	full := "#!/bin/sh\n" + script + "\n"
	if err := os.WriteFile(bin, []byte(full), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
	setBinPath(t, name, bin)
}

// setBinPath assigns `bin` to the *BinPath var matching `name` and registers
// cleanup to restore the previous value. Unlike fakeBin, it does not write
// any file — useful when a test wants to point a var at an existing path.
func setBinPath(t *testing.T, name, bin string) {
	t.Helper()
	var orig string
	switch name {
	case "dnsmasq":
		orig = dnsmasqBinPath
		dnsmasqBinPath = bin
		t.Cleanup(func() { dnsmasqBinPath = orig })
	case "sudo":
		orig = sudoBinPath
		sudoBinPath = bin
		t.Cleanup(func() { sudoBinPath = orig })
	case "systemctl":
		orig = systemctlBinPath
		systemctlBinPath = bin
		t.Cleanup(func() { systemctlBinPath = orig })
	case "service":
		orig = serviceBinPath
		serviceBinPath = bin
		t.Cleanup(func() { serviceBinPath = orig })
	case "rc-service":
		orig = rcServiceBinPath
		rcServiceBinPath = bin
		t.Cleanup(func() { rcServiceBinPath = orig })
	case "sv":
		orig = svBinPath
		svBinPath = bin
		t.Cleanup(func() { svBinPath = orig })
	default:
		t.Fatalf("setBinPath: unknown binary name %q", name)
	}
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
