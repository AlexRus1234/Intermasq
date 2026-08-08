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

package plugins

// Coverage sweep block C — T-C.5 (логи/Coverage_sweep.md §2.C): white-box
// tests for Load. Moved verbatim from linux_test.go during stage 10 of the
// modularization. They mutate the package vars (PluginsDir/SocketsDir/
// loadedPlugins) directly since they live in-package; the cross-package
// withSandboxFlags in main goes through SetDirsForTest instead.

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestLoadPlugins_FakeDir exercises Load against a temp PluginsDir with a
// manifest.json + a trivial shell-script binary. Verifies:
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

	// Override the two package vars Load reads. Cleanup restores.
	origPluginsDir := PluginsDir
	origSocketsDir := SocketsDir
	origLoaded := loadedPlugins
	origCmds := startedCmds
	pluginsRoot := t.TempDir()
	socketsRoot := t.TempDir()
	PluginsDir = pluginsRoot
	SocketsDir = socketsRoot
	loadedPlugins = nil
	startedCmds = nil
	t.Cleanup(func() {
		Stop()
		PluginsDir = origPluginsDir
		SocketsDir = origSocketsDir
		loadedPlugins = origLoaded
		startedCmds = origCmds
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

	Load(r)

	// Assert SocketsDir was created (Load does MkdirAll).
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
// not exist: Load must simply return without panicking and without
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
	Load(r)

	if len(loadedPlugins) != 0 {
		t.Errorf("expected no plugins loaded from missing dir, got %+v", loadedPlugins)
	}
	for _, route := range r.Routes() {
		if strings.Contains(route.Path, "/plugins/") {
			t.Errorf("expected no plugin routes; got %s", route.Path)
		}
	}
}

func TestPluginRouteRequiresAuthentication(t *testing.T) {
	pluginsRoot := t.TempDir()
	pluginDir := filepath.Join(pluginsRoot, "demo")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(`{"id":"demo","name":"Demo","bin":"missing"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	SetDirsForTest(t, pluginsRoot, t.TempDir())

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	Load(r)
	req := httptest.NewRequest("GET", "/plugins/demo/health", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != 401 {
		t.Fatalf("expected unauthenticated plugin request to return 401, got %d", resp.Code)
	}
}

// TestLoadPlugins_BrokenManifest covers the json.Unmarshal-failure path:
// a malformed manifest.json must be silently skipped (Load just
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
	Load(r)

	if len(loadedPlugins) != 0 {
		t.Errorf("expected no plugins from bad manifest, got %+v", loadedPlugins)
	}
	for _, route := range r.Routes() {
		if strings.Contains(route.Path, "/plugins/broken") {
			t.Errorf("broken manifest should not register a route; got %s", route.Path)
		}
	}
}

// TestLoadPlugins_DuplicateIDNoPanic covers the startup-panic regression:
// two plugin subdirectories with the same manifest id used to register the
// /plugins/<id>/* route twice, which makes gin panic at boot. Now the first
// manifest wins and the duplicate is skipped — no panic, exactly one route,
// one loaded entry and one started process.
func TestLoadPlugins_DuplicateIDNoPanic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake plugin binary (shell-script) unsupported on Windows")
	}
	origPluginsDir := PluginsDir
	origSocketsDir := SocketsDir
	origLoaded := loadedPlugins
	origCmds := startedCmds
	pluginsRoot := t.TempDir()
	PluginsDir = pluginsRoot
	SocketsDir = t.TempDir()
	loadedPlugins = nil
	startedCmds = nil
	t.Cleanup(func() {
		Stop()
		PluginsDir = origPluginsDir
		SocketsDir = origSocketsDir
		loadedPlugins = origLoaded
		startedCmds = origCmds
	})

	// Two plugin dirs whose manifests share id "dup".
	for _, sub := range []string{"a", "b"} {
		pluginDir := filepath.Join(pluginsRoot, sub)
		if err := os.MkdirAll(pluginDir, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := []byte(`{"id":"dup","name":"Dup ` + sub + `","bin":"p"}`)
		if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), manifest, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pluginDir, "p"), []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	Load(r) // must NOT panic on the duplicate id

	routeCount := 0
	// r.Any registers the path under every HTTP method gin knows (9 on
	// current versions), so dedupe by path string — what matters is that the
	// /plugins/dup path is registered exactly once. (A duplicate Load would
	// have panicked inside gin at the second registration anyway.)
	distinctDup := make(map[string]bool)
	for _, route := range r.Routes() {
		if strings.HasSuffix(route.Path, "/plugins/dup/*any") {
			routeCount++
			distinctDup[route.Path] = true
		}
	}
	if len(distinctDup) != 1 {
		t.Errorf("expected exactly 1 distinct /plugins/dup path, got %d (route entries=%d)", len(distinctDup), routeCount)
	}
	dupLoaded := 0
	for _, p := range loadedPlugins {
		if p.ID == "dup" {
			dupLoaded++
		}
	}
	if dupLoaded != 1 {
		t.Errorf("expected exactly 1 loaded plugin for dup id, got %d", dupLoaded)
	}
	startedCmdsMu.Lock()
	started := len(startedCmds)
	startedCmdsMu.Unlock()
	if started != 1 {
		t.Errorf("expected exactly 1 started process for dup id, got %d", started)
	}
}

// TestStopKillsStartedProcesses covers the orphaned-plugin-process bug:
// before Stop existed, plugin children stayed alive after the server was
// restarted/stopped (on openrc/runit/sysvinit only the main PID is killed),
// piling up as duplicates. After Load, exactly one process is tracked; after
// Stop it is reaped and the tracked slice is cleared.
func TestStopKillsStartedProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake plugin binary (shell-script) unsupported on Windows")
	}
	origPluginsDir := PluginsDir
	origSocketsDir := SocketsDir
	origLoaded := loadedPlugins
	origCmds := startedCmds
	pluginsRoot := t.TempDir()
	PluginsDir = pluginsRoot
	SocketsDir = t.TempDir()
	loadedPlugins = nil
	startedCmds = nil
	t.Cleanup(func() {
		Stop()
		PluginsDir = origPluginsDir
		SocketsDir = origSocketsDir
		loadedPlugins = origLoaded
		startedCmds = origCmds
	})

	pluginDir := filepath.Join(pluginsRoot, "demo")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(`{"id":"demo","name":"Demo","bin":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "demo"), []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	Load(r)

	startedCmdsMu.Lock()
	if len(startedCmds) != 1 {
		startedCmdsMu.Unlock()
		t.Fatalf("expected 1 started plugin process, got %d", len(startedCmds))
	}
	pid := startedCmds[0].Process.Pid
	startedCmdsMu.Unlock()

	Stop()

	// Ground truth: after Kill+Wait the OS process must be gone. Probe with
	// signal 0 (nil return == still alive). We avoid relying on
	// cmd.ProcessState.Exited() because that can stay unset on some paths
	// (e.g. if the process self-exited and was reaped with ECHILD).
	proc, _ := os.FindProcess(pid)
	if err := proc.Signal(syscall.Signal(0)); err == nil {
		t.Fatalf("plugin process %d still alive after Stop", pid)
	}
	startedCmdsMu.Lock()
	remaining := len(startedCmds)
	startedCmdsMu.Unlock()
	if remaining != 0 {
		t.Errorf("expected startedCmds cleared after Stop, got %d", remaining)
	}
}

// TestStopIsIdempotent confirms a second Stop call is a harmless no-op (the
// restart-self path and the SIGTERM handler can both fire on the same
// shutdown).
func TestStopIsIdempotent(t *testing.T) {
	startedCmdsMu.Lock()
	startedCmds = nil
	startedCmdsMu.Unlock()
	Stop() // must not panic on an empty slice
	Stop() // and again
}
