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
	"regexp"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swaggo/files"
	"github.com/swaggo/gin-swagger"
	_ "intermask/docs"
)

//go:embed frontend/dist/*
var staticFiles embed.FS

var (
	Port          = flag.String("port", "8081", "Port to listen on")
	DBPath        = flag.String("db", "/etc/intermasq/users.json", "Path to user database")
	ConfigDir     = flag.String("conf-dir", "/etc/dnsmasq.d", "Directory with dnsmasq configs")
	LeasesPath    = flag.String("leases", "/var/lib/misc/dnsmasq.leases", "Path to dnsmasq.leases")
	ArpPath       = flag.String("arp-file", "/proc/net/arp", "Path to ARP table file")
	InitSystem    = flag.String("init-system", "auto", "Init system: auto, systemd, systemd-user, openrc, runit, sysvinit, none")
	SystemdScope  = flag.String("systemd-scope", "", "Legacy flag: auto, system, user, none (overrides -init-system if set)")
	CiMode        = flag.Bool("ci-mode", false, "CI mode: disables self-restart")

	// Binary path overrides. Empty value means: resolve via $PATH, then
	// fall back to well-known absolute paths. Needed for distros (Alpine,
	// older Debian) where these binaries live under /bin or /sbin rather
	// than /usr/bin /usr/sbin.
	DnsmasqBin   = flag.String("dnsmasq-bin", "", "Path to dnsmasq binary (auto-resolved via $PATH if empty)")
	SudoBin      = flag.String("sudo-bin", "", "Path to sudo binary (auto-resolved if empty)")
	SystemctlBin = flag.String("systemctl-bin", "", "Path to systemctl binary (auto-resolved if empty)")
	ServiceBin   = flag.String("service-bin", "", "Path to sysvinit service binary (auto-resolved if empty)")
	RcServiceBin = flag.String("rc-service-bin", "", "Path to OpenRC rc-service binary (auto-resolved if empty)")
	SvBin        = flag.String("sv-bin", "", "Path to runit sv binary (auto-resolved if empty)")
	AuditLogPath  = flag.String("audit-log", "/etc/intermasq/audit.log", "Path to audit log file")
	TemplatesPath = flag.String("templates", "/etc/intermasq/templates.json", "Path to templates file")
	HistoryDir    = flag.String("history-dir", "/etc/intermasq/history", "Directory for versioned config history")
	HistoryDepth  = flag.Int("history-depth", 10, "Maximum number of history versions per file")
	PluginsDir    = "/etc/intermasq/plugins"
	SocketsDir    = "/run/intermasq/sockets"
	SecretKey     = []byte(os.Getenv("INTERMASQ_SECRET"))
	// DefaultAliasesFile is the file created on first alias add when no
	// explicit target file is provided. Relative to ConfigDir.
	DefaultAliasesFileName = "10-dns-aliases.conf"
)

var (
	macRegex         = regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`)
	hostnameRegex    = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	aliasDomainRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-.]*[a-zA-Z0-9])?$`)
	// dhcpTagRegex validates a single dhcp-host tag qualifier. dnsmasq
	// accepts "set:<name>" (assigns a tag to the host) and "tag:<name>"
	// (host matches only if that tag is already set by dhcp-match).
	// "id:..." (client-id) is intentionally out of scope for the UI.
	dhcpTagRegex     = regexp.MustCompile(`^(set|tag):[a-zA-Z0-9_][a-zA-Z0-9_-]*$`)
	mu               sync.Mutex
	loadedPlugins    []PluginManifest
)

type PluginManifest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Bin  string `json:"bin"`
}

// validHostname reports whether s is a syntactically valid DNS hostname
// per RFC 952 / RFC 1123 / RFC 1034: each dot-separated label is 1-63 chars,
// alphanumeric boundaries with hyphens allowed inside, total length <=253.
// Used for dhcp-host hostnames written by the panel.
func validHostname(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	return hostnameRegex.MatchString(s)
}

func init() {
	// SECURITY: refuse to start with the default/well-known signing key.
	// Without this, an attacker who reads the source can forge any JWT.
	// Operator MUST set INTERMASQ_SECRET (e.g. `openssl rand -hex 32`).
	if len(SecretKey) == 0 {
		fmt.Fprintln(os.Stderr, "[FATAL] INTERMASQ_SECRET environment variable is not set.")
		fmt.Fprintln(os.Stderr, "        Generate one with:  openssl rand -hex 32")
		fmt.Fprintln(os.Stderr, "        and export it before starting intermasq.")
		os.Exit(1)
	}
}

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
	resolveBins()
	loadUsers()
	loadTemplates()
	if err := ensureHistoryDir(); err != nil {
		fmt.Printf("[HISTORY] Failed to create dir %s: %v\n", *HistoryDir, err)
	}

	initValue := *InitSystem
	if *SystemdScope != "" {
		mapped := mapLegacyScope(*SystemdScope)
		if *SystemdScope != "auto" {
			fmt.Printf("[INIT] Warning: -systemd-scope is deprecated, use -init-system=%s\n", mapped)
		}
		if mapped != "auto" {
			initValue = mapped
		}
	}

	initSystemCaller(initValue)

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	loadPlugins(r)
	startSSEBroadcaster()
	startDNSHealthChecker()

	// /metrics is exposed outside the /api group so Prometheus can scrape it
	// at the conventional URL. Authentication is handled inside the handler
	// (Bearer / X-API-Key / ?token=) so the scrape_url can stay self-contained.
	r.GET("/metrics", metricsHandler)

	api := r.Group("/api")
	{
		api.GET("/status", statusHandler)
		api.POST("/setup", setupHandler)
		api.POST("/login", rateLimitMiddleware(10, time.Minute), loginHandler)

		auth := api.Group("/")
		auth.Use(authMiddleware)
		{
			auth.GET("/plugins", func(c *gin.Context) { c.JSON(200, loadedPlugins) })
			auth.POST("/restart-self", func(c *gin.Context) {
				c.JSON(200, gin.H{"status": "restarting"})
				if !*CiMode {
					go func() {
						if err := sysCaller.RestartSelf(); err != nil {
							fmt.Printf("[INIT] self-restart failed: %v\n", err)
						}
					}()
				}
			})
			auth.GET("/hosts", getHostsHandler)
			auth.GET("/hosts/next-ip", nextIPHandler)
			auth.POST("/hosts/apply-template", applyTemplateHandler)
			auth.GET("/leases", getLeasesHandler)
			auth.GET("/arp", getArpHandler)
			auth.GET("/audit", auditHandler)
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

	fmt.Printf("Intermasq v3.0 Started on :%s\n", *Port)
	if err := r.Run(":" + *Port); err != nil {
		fmt.Printf("[FATAL] Server failed: %v\n", err)
		os.Exit(1)
	}
}
