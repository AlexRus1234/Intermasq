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

package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swaggo/files"
	"github.com/swaggo/gin-swagger"
	_ "intermask/docs"
	"intermask/internal/audit"
	authpkg "intermask/internal/auth"
	"intermask/internal/bins"
	"intermask/internal/dnsmasq"
	"intermask/internal/initd"
	templatepkg "intermask/internal/templates"
)

//go:embed frontend/dist/*
var staticFiles embed.FS

var (
	Port         = flag.String("port", "8081", "Port to listen on")
	InitSystem   = flag.String("init-system", "auto", "Init system: auto, systemd, systemd-user, openrc, runit, sysvinit, none")
	SystemdScope = flag.String("systemd-scope", "", "Legacy flag: auto, system, user, none (overrides -init-system if set)")
	CiMode       = flag.Bool("ci-mode", false, "CI mode: disables self-restart")

	// Binary path overrides live in internal/bins (registered on the
	// default flag set at package init): -dnsmasq-bin / -sudo-bin /
	// -systemctl-bin / -service-bin / -rc-service-bin / -sv-bin. Empty value
	// means: resolve via $PATH, then fall back to well-known absolute paths.
	// ConfigDir is the registered -conf-dir flag in internal/dnsmasq;
	// HistoryDir / HistoryDepth are also registered there.
	PluginsDir = "/etc/intermasq/plugins"
	SocketsDir = "/run/intermasq/sockets"
)

var (
	loadedPlugins []PluginManifest
)

type PluginManifest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Bin  string `json:"bin"`
}

func init() {
	// SECURITY: refuse to start with the default/well-known signing key.
	// Without this, an attacker who reads the source can forge any JWT.
	// Operator MUST set INTERMASQ_SECRET (e.g. `openssl rand -hex 32`).
	if len(authpkg.SecretKey) == 0 {
		fmt.Fprintln(os.Stderr, "[FATAL] INTERMASQ_SECRET environment variable is not set.")
		fmt.Fprintln(os.Stderr, "        Generate one with:  openssl rand -hex 32")
		fmt.Fprintln(os.Stderr, "        and export it before starting intermasq.")
		os.Exit(1)
	}
}

// startSSEBroadcasterFn / startDNSHealthCheckerFn are the launch points for
// the long-lived background goroutines started by setupServer. They are
// package vars (indirection seams) so tests can swap them to no-ops while
// exercising the bootstrap (TestSetupServer_*): the real goroutines read
// flag-owned paths (*ConfigDir / *ArpPath / ...) that test cleanup restores
// concurrently, which is a data race under `-race`. Production bootstrap
// keeps using the real starters.
var (
	startSSEBroadcasterFn   = startSSEBroadcaster
	startDNSHealthCheckerFn = startDNSHealthChecker
)

func loadPlugins(r *gin.Engine) {
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

func main() {
	flag.Parse()
	r, err := setupServer()
	if err != nil {
		fmt.Printf("[FATAL] setup: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Intermasq v3.0 Started on :%s\n", *Port)
	if err := r.Run(":" + *Port); err != nil {
		fmt.Printf("[FATAL] Server failed: %v\n", err)
		os.Exit(1)
	}
}

// setupServer performs all one-time bootstrap between flag.Parse() and
// r.Run(): resolves external binaries, loads users/templates, picks the
// init-system caller, builds the gin engine, registers every route and
// starts the SSE / DNS-health background goroutines. It returns the
// configured engine ready to be served by main()'s blocking r.Run().
//
// Extracted from main() so the bootstrap logic is unit-testable in
// isolation (TestSetupServer) without invoking the blocking server. main()
// keeps only the Run() + os.Exit plumbing, which is intentionally left
// uncovered (see логи/Coverage_sweep.md §6).
func setupServer() (*gin.Engine, error) {
	bins.Resolve()
	authpkg.LoadUsers()
	templatepkg.Load()
	if err := dnsmasq.EnsureHistoryDir(); err != nil {
		fmt.Printf("[HISTORY] Failed to create dir %s: %v\n", *dnsmasq.HistoryDir, err)
	}

	initValue := *InitSystem
	if *SystemdScope != "" {
		mapped := initd.MapLegacyScope(*SystemdScope)
		if *SystemdScope != "auto" {
			fmt.Printf("[INIT] Warning: -systemd-scope is deprecated, use -init-system=%s\n", mapped)
		}
		if mapped != "auto" {
			initValue = mapped
		}
	}

	initd.Init(initValue)

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	loadPlugins(r)
	startSSEBroadcasterFn()
	startDNSHealthCheckerFn()

	// /metrics is exposed outside the /api group so Prometheus can scrape it
	// at the conventional URL. Authentication is handled inside the handler
	// (Bearer / X-API-Key / ?token=) so the scrape_url can stay self-contained.
	r.GET("/metrics", metricsHandler)

	api := r.Group("/api")
	{
		api.GET("/status", statusHandler)
		api.POST("/setup", setupHandler)
		api.POST("/login", authpkg.RateLimitMiddleware(10, time.Minute), loginHandler)

		protected := api.Group("/")
		protected.Use(authpkg.Middleware)
		auth := protected
		{
			protected.GET("/plugins", func(c *gin.Context) { c.JSON(200, loadedPlugins) })
			protected.POST("/restart-self", func(c *gin.Context) {
				c.JSON(200, gin.H{"status": "restarting"})
				if !*CiMode {
					go func() {
						if err := initd.Current().RestartSelf(); err != nil {
							fmt.Printf("[INIT] self-restart failed: %v\n", err)
						}
					}()
				}
			})
			protected.GET("/hosts", getHostsHandler)
			auth.GET("/hosts/next-ip", nextIPHandler)
			auth.POST("/hosts/apply-template", applyTemplateHandler)
			auth.GET("/leases", getLeasesHandler)
			auth.GET("/arp", getArpHandler)
			auth.GET("/audit", audit.Handler)
			auth.GET("/hosts/csv", exportCSVHandler)
			auth.POST("/hosts/csv", importCSVHandler)
			auth.POST("/hosts", addHostHandler)
			auth.POST("/hosts/bulk", bulkAddHostsHandler)
			auth.POST("/hosts/bulk-move", bulkMoveHandler)
			auth.POST("/hosts/bulk-edit", bulkEditHandler)
			auth.DELETE("/hosts/:mac", deleteHostHandler)
			auth.GET("/templates", getTemplatesHandler)
			auth.POST("/templates", createTemplateHandler)
			auth.DELETE("/templates/:id", deleteTemplateHandler)
			auth.GET("/templates/ranges", getDhcpRangesHandler)
			auth.GET("/config", getConfigHandler)
			auth.PUT("/config", updateConfigHandler)
			auth.POST("/config/file", createConfigFileHandler)
			auth.DELETE("/config/file", deleteConfigFileHandler)
			auth.GET("/config/templates", listConfigTemplatesHandler)
			auth.GET("/aliases", getAliasesHandler)
			auth.POST("/aliases", addAliasHandler)
			auth.POST("/aliases/bulk", bulkAddAliasesHandler)
			auth.POST("/aliases/delete", deleteAliasHandler)
			auth.GET("/aliases/csv", exportAliasesCSVHandler)
			auth.POST("/aliases/csv", importAliasesCSVHandler)
			auth.POST("/rollback", rollbackHandler)
			auth.GET("/history", historyListHandler)
			auth.GET("/history/diff", historyDiffHandler)
			auth.POST("/history/restore", historyRestoreHandler)
			auth.POST("/reload", reloadHandler)
			auth.GET("/backup", backupHandler)
			auth.POST("/backup/restore", restoreBackupHandler)
			auth.GET("/files/:name", getFileHandler)
			auth.PUT("/files/:name", putFileHandler)
			auth.GET("/events", eventsHandler)
			auth.GET("/users", getUsersHandler)
			auth.POST("/users", createUserHandler)
			auth.DELETE("/users/:name", deleteUserHandler)
			auth.POST("/users/password", changePasswordHandler)
			auth.POST("/logout", logoutHandler)
			auth.GET("/new-devices", getNewDevicesHandler)
			auth.POST("/leases/to-static", bulkLeaseToStaticHandler)
		}
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	staticFS, _ := fs.Sub(staticFiles, "frontend/dist")
	r.NoRoute(gin.WrapH(http.FileServer(http.FS(staticFS))))

	return r, nil
}
