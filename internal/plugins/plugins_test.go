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
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
