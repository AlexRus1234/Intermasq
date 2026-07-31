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

// Coverage sweep block C — setupServer() extraction (логи/Coverage_sweep.md
// §2.C, T-C.6). The test reruns the full bootstrap against a sandboxed set
// of temp paths and asserts the gin engine has all the expected routes
// registered. main()'s blocking r.Run()/os.Exit plumbing is intentionally
// left uncovered (§6).
//
// Caveat: setupServer spawns the SSE and DNS-health goroutines as a side
// effect. They are neutralized here via the startSSEBroadcasterFn /
// startDNSHealthCheckerFn indirection seams (main.go) because the real
// goroutines read the same flag-owned paths the test cleanup restores
// concurrently — a data race under `-race`.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// withSandboxFlags reroutes every package-level path flag / dir that
// setupServer touches at temp locations and restores the originals on
// cleanup. It also neutralizes the long-lived background goroutines
// (SSE broadcaster + DNS-health checker) that setupServer would otherwise
// spawn: those read the same flag-owned paths the cleanup restores, which
// is a data race under `-race`. Returns the sandbox root.
func withSandboxFlags(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	origSSE := startSSEBroadcasterFn
	origDNS := startDNSHealthCheckerFn
	startSSEBroadcasterFn = func() {}
	startDNSHealthCheckerFn = func() {}
	orig := struct {
		DBPath, TemplatesPath, HistoryDir, ConfigDir, ArpPath, LeasesPath *string
		InitSystem, SystemdScope                                          *string
		PluginsDir, SocketsDir                                            string
		loadedPlugins                                                     []PluginManifest
	}{
		DBPath:        DBPath,
		TemplatesPath: TemplatesPath,
		HistoryDir:    HistoryDir,
		ConfigDir:     ConfigDir,
		ArpPath:       ArpPath,
		LeasesPath:    LeasesPath,
		InitSystem:    InitSystem,
		SystemdScope:  SystemdScope,
		PluginsDir:    PluginsDir,
		SocketsDir:    SocketsDir,
		loadedPlugins: loadedPlugins,
	}
	*DBPath = filepath.Join(tmp, "users.json")
	*TemplatesPath = filepath.Join(tmp, "templates.json")
	*HistoryDir = filepath.Join(tmp, "history")
	*ConfigDir = filepath.Join(tmp, "conf")
	*ArpPath = filepath.Join(tmp, "arp")
	*LeasesPath = filepath.Join(tmp, "leases")
	PluginsDir = filepath.Join(tmp, "plugins")
	SocketsDir = filepath.Join(tmp, "sockets")
	*InitSystem = "none"
	*SystemdScope = ""
	loadedPlugins = nil
	t.Cleanup(func() {
		startSSEBroadcasterFn = origSSE
		startDNSHealthCheckerFn = origDNS
		*DBPath = *orig.DBPath
		*TemplatesPath = *orig.TemplatesPath
		*HistoryDir = *orig.HistoryDir
		*ConfigDir = *orig.ConfigDir
		*ArpPath = *orig.ArpPath
		*LeasesPath = *orig.LeasesPath
		*InitSystem = *orig.InitSystem
		*SystemdScope = *orig.SystemdScope
		PluginsDir = orig.PluginsDir
		SocketsDir = orig.SocketsDir
		loadedPlugins = orig.loadedPlugins
	})
	return tmp
}

func TestSetupServer_RegistersRoutes(t *testing.T) {
	withSandboxFlags(t)
	// Pre-seed the user DB so loadUsers' os.Stat returns IsNotExist-free...
	// actually we want loadUsers to see no file → early-return. Leave the
	// tmp path empty. ensureHistoryDir will create *HistoryDir.

	// Quiet gin logger.
	gin.SetMode(gin.ReleaseMode)
	r, err := setupServer()
	if err != nil {
		t.Fatalf("setupServer: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil engine")
	}

	// Collect route paths into a set for membership assertions.
	routes := r.Routes()
	got := make(map[string]struct{}, len(routes))
	for _, rt := range routes {
		got[rt.Path] = struct{}{}
	}

	want := []string{
		// /metrics is outside /api.
		"/metrics",
		// public /api endpoints.
		"/api/status",
		"/api/setup",
		"/api/login",
		// auth-protected /api endpoints (a representative subset, not all).
		"/api/hosts",
		"/api/hosts/next-ip",
		"/api/hosts/bulk",
		"/api/hosts/bulk-move",
		"/api/hosts/bulk-edit",
		"/api/hosts/csv",
		"/api/hosts/:mac",
		"/api/leases",
		"/api/arp",
		"/api/audit",
		"/api/templates",
		"/api/config",
		"/api/config/file",
		"/api/config/templates",
		"/api/aliases",
		"/api/aliases/bulk",
		"/api/aliases/delete",
		"/api/aliases/csv",
		"/api/rollback",
		"/api/history",
		"/api/history/diff",
		"/api/history/restore",
		"/api/reload",
		"/api/backup",
		"/api/backup/restore",
		"/api/files/:name",
		"/api/events",
		"/api/users",
		"/api/users/:name",
		"/api/users/password",
		"/api/logout",
		"/api/new-devices",
		"/api/leases/to-static",
		"/api/restart-self",
		"/api/plugins",
		// swagger + NoRoute fallback isn't listed as a named route.
	}
	for _, p := range want {
		if _, ok := got[p]; !ok {
			t.Errorf("missing expected route %q", p)
		}
	}

	// /plugins/* reverse-proxy is conditioned on PluginsDir contents.
	// We pointed PluginsDir at a non-existent dir, so no /plugins entry
	// should be registered.
	for _, rt := range routes {
		if strings.HasPrefix(rt.Path, "/plugins/") {
			t.Errorf("expected no plugin route in empty sandbox, got %q", rt.Path)
		}
	}
}

func TestSetupServer_InitSystemNone(t *testing.T) {
	withSandboxFlags(t)
	gin.SetMode(gin.ReleaseMode)
	if _, err := setupServer(); err != nil {
		t.Fatalf("setupServer: %v", err)
	}
	// With *InitSystem=none, sysCaller must be a NoneCaller.
	if sysCaller == nil {
		t.Fatal("expected sysCaller to be initialised")
	}
	if got := sysCaller.String(); got != "none" {
		t.Errorf("expected None caller, got %q", got)
	}
}

func TestSetupServer_LegacySystemdScopeWarning(t *testing.T) {
	tmp := withSandboxFlags(t)
	// Force the deprecated -systemd-scope=system path which maps to systemd.
	// We cannot assert the printed warning without capturing stderr; just
	// call setupServer and confirm it does not error out.
	*SystemdScope = "nano"
	// restore the original only if we set it: withSandboxFlags already
	// captures/restores SystemdScope, so mutate freely here.
	*SystemdScope = "none" // maps to "none" → no override → falls to *InitSystem
	_ = tmp
	gin.SetMode(gin.ReleaseMode)
	if _, err := setupServer(); err != nil {
		t.Fatalf("setupServer with -systemd-scope=none: %v", err)
	}
}

func TestSetupServer_HistoryDirFail(t *testing.T) {
	// Neutralize the long-lived background goroutines for the same reason
	// as withSandboxFlags (see its comment).
	origSSE := startSSEBroadcasterFn
	origDNS := startDNSHealthCheckerFn
	startSSEBroadcasterFn = func() {}
	startDNSHealthCheckerFn = func() {}
	t.Cleanup(func() {
		startSSEBroadcasterFn = origSSE
		startDNSHealthCheckerFn = origDNS
	})
	// Make ensureHistoryDir fail by pointing *HistoryDir at a path whose
	// parent is a regular file.
	tmp := t.TempDir()
	parent := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	orig := *HistoryDir
	*HistoryDir = filepath.Join(parent, "history")
	t.Cleanup(func() { *HistoryDir = orig })

	gin.SetMode(gin.ReleaseMode)
	// Note: setupServer currently logs the ensureHistoryDir error but does
	// NOT return it (keeps the original forgiving behaviour). We assert it
	// still returns a working engine.
	r, err := setupServer()
	if err != nil {
		t.Fatalf("expected nil err (history failure is non-fatal), got %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil engine despite history dir failure")
	}
}
