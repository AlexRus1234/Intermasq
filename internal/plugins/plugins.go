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

// Package plugins discovers, launches and reverse-proxies optional sidecar
// plugins discovered under PluginsDir.
//
// Each plugin lives in its own subdirectory <PluginsDir>/<id>/ containing a
// manifest.json (id/name/bin) and the executable named by bin. Load starts
// each binary with INTERMASQ_KEY (the panel signing secret) and
// PLUGIN_SOCKET (a per-plugin unix socket path under SocketsDir) in its
// environment, then mounts an httputil reverse-proxy at /plugins/<id>/* that
// dials that unix socket — so a plugin only has to bind the socket it is
// given and answer HTTP, no TCP port of its own. A plugin whose manifest is
// missing or malformed, or whose binary cannot be started, is silently
// skipped (proxy still mounts, process is not). The /api/plugins handler in
// package main reports the successfully-parsed manifests via Loaded.
package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"intermask/internal/auth"
)

var (
	// PluginsDir is the directory scanned for plugin subdirectories holding
	// manifest.json + binary. Exported because withSandboxFlags in
	// package main reroutes it at temp paths during tests, and the operator
	// deployment may also swap it before Load runs.
	PluginsDir = "/etc/intermasq/plugins"
	// SocketsDir is where per-plugin unix sockets are created. Exported for
	// the same reason as PluginsDir.
	SocketsDir = "/run/intermasq/sockets"
)

// loadedPlugins holds every manifest that Load successfully parsed and
// mounted. Unexported: the /api/plugins handler reads it via Loaded.
var loadedPlugins []PluginManifest

// startedCmds tracks every plugin process Launch started so Stop can kill
// them on shutdown/restart. Supervisors on openrc/runit/sysvinit kill only
// the main PID on stop/restart, so without an explicit Kill the plugin
// children stay alive and pile up as duplicates after restart-self.
var (
	startedCmdsMu sync.Mutex
	startedCmds   []*exec.Cmd
)

// PluginManifest is the on-disk shape of <plugin>/manifest.json. Load
// populates loadedPlugins with the parsed manifests; /api/plugins marshals
// them back to the frontend.
type PluginManifest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Bin  string `json:"bin"`
}

// Load scans PluginsDir for plugin subdirectories, launches each plugin
// binary (passing INTERMASQ_KEY + PLUGIN_SOCKET via the environment) and
// mounts a reverse-proxy at /plugins/<id>/* on r that dials the plugin's
// unix socket. Missing/unreadable dirs and malformed manifests are silently
// skipped. It is meant to be called once from setupServer.
func Load(r *gin.Engine) {
	os.MkdirAll(SocketsDir, 0770)

	entries, err := os.ReadDir(PluginsDir)
	if err != nil {
		return
	}

	// seen guards against two manifests sharing the same id: registering the
	// reverse-proxy route /plugins/<id>/* twice panics inside gin at startup,
	// and two sockets/plugins with the same id are meaningless anyway. The
	// first manifest wins; later ones are skipped.
	seen := make(map[string]bool)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		path := filepath.Join(PluginsDir, entry.Name())
		manifestPath := filepath.Join(path, "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}

		var p PluginManifest
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}

		if p.ID == "" || seen[p.ID] {
			fmt.Printf("[PLUGINS] Skipping plugin with empty or duplicate id %q (dir %s)\n", p.ID, entry.Name())
			continue
		}
		seen[p.ID] = true

		binPath := filepath.Join(path, p.Bin)
		sockPath := filepath.Join(SocketsDir, p.ID+".sock")

		if _, err := os.Stat(binPath); err == nil {
			cmd := exec.Command(binPath)
			cmd.Dir = path
			cmd.Env = append(os.Environ(),
				fmt.Sprintf("INTERMASQ_KEY=%s", os.Getenv("INTERMASQ_SECRET")),
				fmt.Sprintf("PLUGIN_SOCKET=%s", sockPath),
			)

			if err := cmd.Start(); err != nil {
				fmt.Printf("[PLUGINS] Error starting %s: %v\n", p.Name, err)
				continue
			}
			startedCmdsMu.Lock()
			startedCmds = append(startedCmds, cmd)
			startedCmdsMu.Unlock()
			fmt.Printf("[PLUGINS] Started %s on socket %s\n", p.Name, sockPath)
		}

		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = "http"
				req.URL.Host = "dummy"
			},
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", sockPath)
				},
			},
		}

		r.Any("/plugins/"+p.ID+"/*any", auth.Middleware, func(c *gin.Context) {
			c.Request.URL.Path = c.Param("any")
			proxy.ServeHTTP(c.Writer, c.Request)
		})

		loadedPlugins = append(loadedPlugins, p)
	}
}

// Loaded returns the manifests parsed by the last successful Load. Used by
// the /api/plugins handler; the returned slice aliases the package state
// (it is not a copy), matching the pre-modularization behaviour.
func Loaded() []PluginManifest {
	if loadedPlugins == nil {
		return []PluginManifest{}
	}
	return loadedPlugins
}

// Stop terminates every plugin process started by Load. It is best-effort
// and idempotent: already-exited processes are skipped, and the tracked
// slice is cleared so a second call is a no-op.
//
// Callers:
//   - the restart-self handler, BEFORE invoking the supervisor's restart, so
//     the old plugin processes die with the old server instead of being
//     orphaned (on openrc/runit/sysvinit the supervisor kills only the main
//     PID, leaving plugin children running → duplicates after restart);
//   - main's SIGTERM/SIGINT handler, so `systemctl stop` / a manual kill
//     also cleans up plugin children on every init system.
func Stop() {
	startedCmdsMu.Lock()
	cmds := startedCmds
	startedCmds = nil
	startedCmdsMu.Unlock()

	for _, cmd := range cmds {
		if cmd.Process == nil {
			continue
		}
		// Already reaped (e.g. plugin exited on its own). Wait was already
		// called, so skip to avoid the "waitid: no child process" path.
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			continue
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

// SetDirsForTest reroutes PluginsDir/SocketsDir at the given paths for the
// duration of the test, resets loadedPlugins and startedCmds to nil, and
// restores all of them on cleanup (calling Stop first so any plugin process
// a test started via Load is reaped instead of leaking out of the test run).
// It is the cross-package seam used by withSandboxFlags in package main
// (which used to mutate the package vars directly before the plugin code
// moved here).
//
// Exported for cross-package tests during modularization.
func SetDirsForTest(t *testing.T, pluginsDir, socketsDir string) {
	t.Helper()
	origPluginsDir := PluginsDir
	origSocketsDir := SocketsDir
	origLoaded := loadedPlugins
	origCmds := startedCmds
	PluginsDir = pluginsDir
	SocketsDir = socketsDir
	loadedPlugins = nil
	startedCmds = nil
	t.Cleanup(func() {
		Stop()
		PluginsDir = origPluginsDir
		SocketsDir = origSocketsDir
		loadedPlugins = origLoaded
		startedCmds = origCmds
	})
}
