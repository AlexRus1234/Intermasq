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

// Root handlers: status, setup, login, reload, arp, next-ip, leases,
// new-devices, events (SSE), bulk lease→static. Per-domain handlers live
// in sibling handler_*.go files:
//
//   - handlers_hosts.go    static dhcp-host= CRUD, bulk, CSV, templates
//   - handlers_aliases.go  DNS alias CRUD, bulk, CSV
//   - handlers_config.go   visual config editor, raw file editor, file delete
//   - handlers_safety.go   rollback, history, ZIP backup/restore
//   - handlers_users.go    user management, logout

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"intermask/internal/audit"
	"intermask/internal/auth"
	"intermask/internal/control"
	"intermask/internal/dnsmasq"
	"intermask/internal/models"
	"intermask/internal/netstate"
	"intermask/internal/validate"
	"intermask/internal/version"
)

func statusHandler(c *gin.Context) {
	isActive := control.CheckDnsmasqStatus()
	setupRequired := auth.UserCount() == 0
	dnsmasq.Mu.Lock()
	defer dnsmasq.Mu.Unlock()
	c.JSON(200, gin.H{
		"setup_required": setupRequired,
		"version":        version.Version,
		"dnsmasq_active": isActive,
	})
}

// maxPasswordBytes is the bcrypt input limit (72 bytes). golang.org/x/crypto
// v0.48+ returns ErrPasswordTooLong for anything longer; if a caller ignores
// that error the resulting empty hash bricks the account (can never log in).
// We reject oversize passwords up-front AND treat any bcrypt error as
// password_too_long so the account is never persisted with a broken hash.
const maxPasswordBytes = 72

func setupHandler(c *gin.Context) {
	if auth.UserCount() > 0 {
		c.JSON(403, gin.H{"error": "already_setup"})
		return
	}
	var req models.AuthReq
	if err := c.BindJSON(&req); err != nil {
		return
	}
	if req.Username == "" || req.Password == "" {
		c.JSON(400, gin.H{"error": "missing_fields"})
		return
	}
	if len(req.Username) > 64 {
		c.JSON(400, gin.H{"error": "username_too_long"})
		return
	}
	if len(req.Password) > maxPasswordBytes {
		c.JSON(400, gin.H{"error": "password_too_long"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(400, gin.H{"error": "password_too_long"})
		return
	}
	if err := auth.AddUser(req.Username, string(hash)); err != nil {
		if errors.Is(err, auth.ErrUserExists) {
			c.JSON(403, gin.H{"error": "already_setup"})
			return
		}
		c.JSON(500, gin.H{"error": "save_error"})
		return
	}
	c.JSON(200, gin.H{"token": auth.MakeToken(req.Username)})
}

// loginHandler authenticates by username + bcrypt-hashed password and
// issues a JWT. On success it also clears the rate-limit counter for the
// caller's IP so a legitimate user who fat-fingered their password twice
// and then typed it correctly is not left counting against the limit.
func loginHandler(c *gin.Context) {
	var req models.AuthReq
	if err := c.BindJSON(&req); err != nil {
		return
	}
	hash, ok := auth.GetUser(req.Username)
	if !ok || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		c.JSON(401, gin.H{"error": "invalid_credentials"})
		return
	}
	auth.RateLimitReset(c.ClientIP())
	c.JSON(200, gin.H{"token": auth.MakeToken(req.Username)})
}

func getArpHandler(c *gin.Context) {
	c.JSON(200, netstate.GetArpTable())
}

func nextIPHandler(c *gin.Context) {
	cidr := c.Query("range")
	if cidr == "" {
		c.JSON(400, gin.H{"error": "range_required"})
		return
	}
	ip, err := dnsmasq.FindFreeIP(cidr)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ip": ip})
}

func reloadHandler(c *gin.Context) {
	if err := control.ReloadDnsmasq(); err != nil {
		c.JSON(400, gin.H{"error": "reload_error"})
		return
	}

	audit.WriteAudit(audit.AuditEntry{
		User:   getUser(c),
		Action: "reload",
	})

	c.JSON(200, gin.H{"status": "reloaded"})
}

func getLeasesHandler(c *gin.Context) {
	c.JSON(200, netstate.ParseLeases())
}

func getUser(c *gin.Context) string {
	u, _ := c.Get("user")
	user, _ := u.(string)
	return user
}

func getNewDevicesHandler(c *gin.Context) {
	c.JSON(200, netstate.GetNewDevices())
}

// bulkLeaseToStaticHandler writes one dhcp-host= line per selected lease.
// IMPORTANT: this endpoint does NOT run `dnsmasq --test` after writing —
// the typical use case is "I just plugged in 5 new devices and want them
// all in static at once", and re-testing the config 5 times would be
// wasteful. The UI therefore surfaces a prominent reminder to click
// "Apply" before expecting the changes to take effect.
func bulkLeaseToStaticHandler(c *gin.Context) {
	var req models.BulkLeaseToStaticReq
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}
	if !dnsmasq.IsSafePath(req.File) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	if len(req.Leases) == 0 {
		c.JSON(400, gin.H{"error": "no_leases"})
		return
	}

	for _, l := range req.Leases {
		if !validate.ValidMAC(l.Mac) {
			c.JSON(400, gin.H{"error": "invalid_mac", "mac": l.Mac})
			return
		}
		macConflicts := dnsmasq.FindHostsByMac(l.Mac)
		if len(macConflicts) > 0 {
			c.JSON(409, gin.H{"error": "mac_duplicate", "conflicts": macConflicts})
			return
		}
	}

	dnsmasq.Mu.Lock()
	defer dnsmasq.Mu.Unlock()

	dnsmasq.CreateLocalBackup(req.File)

	content, err := os.ReadFile(req.File)
	if err != nil && !os.IsNotExist(err) {
		c.JSON(500, gin.H{"error": "read_error"})
		return
	}

	lines := strings.Split(string(content), "\n")
	newLines := []string{}
	newMacs := make(map[string]bool)
	for _, l := range req.Leases {
		if validate.ValidMAC(l.Mac) {
			newMacs[strings.ToLower(l.Mac)] = true
		}
	}

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "dhcp-host=") {
			parts := strings.Split(line, ",")
			skip := false
			for _, p := range parts {
				if newMacs[strings.ToLower(strings.TrimSpace(p))] {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}
		if strings.TrimSpace(line) != "" {
			newLines = append(newLines, line)
		}
	}

	count := 0
	for _, l := range req.Leases {
		if !validate.ValidMAC(l.Mac) {
			continue
		}
		hostname := l.Hostname
		if hostname == "*" || hostname == "" {
			hostname = "device-" + strings.ReplaceAll(strings.ToLower(l.Mac), ":", "")[:8]
		}
		newLines = append(newLines, dnsmasq.FormatDhcpHostLine(models.HostEntry{
			Mac: l.Mac, Hostname: hostname, Ip: l.Ip, File: req.File,
		}))
		count++
	}

	if err := os.WriteFile(req.File, []byte(strings.Join(newLines, "\n")+"\n"), 0644); err != nil {
		c.JSON(500, gin.H{"error": "write_error"})
		return
	}

	audit.WriteAudit(audit.AuditEntry{
		User:   getUser(c),
		Action: "bulk_lease_to_static",
		File:   req.File,
		Mac:    fmt.Sprintf("%d leases", count),
	})

	c.JSON(200, gin.H{"status": "ok", "count": count})
}

// eventsHandler streams SSE updates for ARP table + dnsmasq status. Each
// connected client gets a small buffered channel; the broadcaster (sse.go)
// pushes a message only when a value actually changes.
func eventsHandler(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	client := &control.Client{Ch: make(chan string, 10)}
	control.Register(client)
	defer control.Unregister(client)

	arp := netstate.GetArpTable()
	c.SSEvent("arp", arp)
	c.Writer.Flush()

	for {
		select {
		case msg := <-client.Ch:
			c.Writer.Write([]byte(msg))
			c.Writer.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}
