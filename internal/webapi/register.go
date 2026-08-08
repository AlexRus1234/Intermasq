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

package webapi

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"intermask/internal/audit"
	authpkg "intermask/internal/auth"
	"intermask/internal/initd"
	"intermask/internal/plugins"
)

// Register wires every /api route onto r. It is the single entry point main's
// setupServer calls after building the gin engine; main keeps the non-/api
// routes (/metrics, swagger, static FS) and the bootstrap ordering.
//
// ciMode gates the self-restart side-effect of POST /api/restart-self: under
// -ci-mode the handler replies 200 but does not actually restart the process,
// so the CI test harness never kills itself. The routes, methods, status
// codes and middleware ordering are unchanged from the pre-refactor inline
// registration that lived in setupServer.
func Register(r *gin.Engine, ciMode bool) {
	api := r.Group("/api")
	{
		api.GET("/status", statusHandler)
		api.POST("/setup", setupHandler)
		api.POST("/login", authpkg.RateLimitMiddleware(10, time.Minute), loginHandler)

		protected := api.Group("/")
		protected.Use(authpkg.Middleware)
		auth := protected
		admin := protected.Group("/")
		admin.Use(authpkg.AdminMiddleware)
		{
			protected.GET("/plugins", func(c *gin.Context) { c.JSON(200, plugins.Loaded()) })
			admin.POST("/restart-self", func(c *gin.Context) {
				c.JSON(200, gin.H{"status": "restarting"})
				if !ciMode {
					go func() {
						// Kill plugin children before the supervisor restarts
						// us: on openrc/runit/sysvinit only the main PID is
						// killed, so without this the old plugins survive and
						// pile up as duplicates after the restart.
						plugins.Stop()
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
			admin.POST("/rollback", rollbackHandler)
			auth.GET("/history", historyListHandler)
			auth.GET("/history/diff", historyDiffHandler)
			admin.POST("/history/restore", historyRestoreHandler)
			admin.POST("/reload", reloadHandler)
			auth.GET("/backup", backupHandler)
			admin.POST("/backup/restore", restoreBackupHandler)
			auth.GET("/files/:name", getFileHandler)
			admin.PUT("/files/:name", putFileHandler)
			auth.GET("/events", eventsHandler)
			admin.GET("/users", getUsersHandler)
			admin.POST("/users", createUserHandler)
			admin.DELETE("/users/:name", deleteUserHandler)
			auth.POST("/users/password", changePasswordHandler)
			auth.POST("/logout", logoutHandler)
			auth.GET("/new-devices", getNewDevicesHandler)
			auth.POST("/leases/to-static", bulkLeaseToStaticHandler)
		}
	}
}
