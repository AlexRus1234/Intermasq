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
	"testing"

	"github.com/gin-gonic/gin"
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

		r.Any("/plugins/"+p.ID+"/*any", func(c *gin.Context) {
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
	return loadedPlugins
}

// SetDirsForTest reroutes PluginsDir/SocketsDir at the given paths for the
// duration of the test, resets loadedPlugins to nil, and restores all three
// on cleanup. It is the cross-package seam used by withSandboxFlags in
// package main (which used to mutate the package vars directly before the
// plugin code moved here).
//
// Exported for cross-package tests during modularization.
func SetDirsForTest(t *testing.T, pluginsDir, socketsDir string) {
	t.Helper()
	origPluginsDir := PluginsDir
	origSocketsDir := SocketsDir
	origLoaded := loadedPlugins
	PluginsDir = pluginsDir
	SocketsDir = socketsDir
	loadedPlugins = nil
	t.Cleanup(func() {
		PluginsDir = origPluginsDir
		SocketsDir = origSocketsDir
		loadedPlugins = origLoaded
	})
}
