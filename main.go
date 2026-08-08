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
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/swaggo/files"
	"github.com/swaggo/gin-swagger"
	_ "intermask/docs"
	authpkg "intermask/internal/auth"
	"intermask/internal/bins"
	"intermask/internal/control"
	"intermask/internal/dnsmasq"
	"intermask/internal/initd"
	"intermask/internal/metrics"
	"intermask/internal/plugins"
	templatepkg "intermask/internal/templates"
	"intermask/internal/version"
	"intermask/internal/webapi"
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
	// Plugin discovery dirs (PluginsDir/SocketsDir) live in internal/plugins.
)

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
	startSSEBroadcasterFn   = control.StartBroadcaster
	startDNSHealthCheckerFn = metrics.StartDNSHealthChecker
)

func main() {
	flag.Parse()
	r, err := setupServer()
	if err != nil {
		fmt.Printf("[FATAL] setup: %v\n", err)
		os.Exit(1)
	}
	// On SIGTERM/SIGINT kill plugin children before exiting. Supervisors on
	// openrc/runit/sysvinit (and a bare `kill`) terminate only the main PID,
	// so without this the plugin processes stay alive after the server is
	// gone. (restart-self does its own Stop() before invoking the supervisor,
	// this covers stop / manual kill on every init system.)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		sig := <-sigCh
		fmt.Printf("\n[SHUTDOWN] received %s, stopping plugin processes\n", sig)
		plugins.Stop()
		os.Exit(0)
	}()
	fmt.Printf("Intermasq %s Started on :%s\n", version.Version, *Port)
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

	plugins.Load(r)
	startSSEBroadcasterFn()
	startDNSHealthCheckerFn()

	// /metrics is exposed outside the /api group so Prometheus can scrape it
	// at the conventional URL. Authentication is handled inside the handler
	// (Bearer / X-API-Key / ?token=) so the scrape_url can stay self-contained.
	r.GET("/metrics", metrics.Handler)

	// Every /api route (status, setup, login, the auth-protected CRUD + SSE
	// group, restart-self, plugins) is wired by the webapi package. ciMode
	// gates the self-restart side-effect so the CI test harness never kills
	// itself; /metrics, swagger and the static FS stay here (non-/api).
	webapi.Register(r, *CiMode)

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	staticFS, _ := fs.Sub(staticFiles, "frontend/dist")
	r.NoRoute(gin.WrapH(http.FileServer(http.FS(staticFS))))

	return r, nil
}
