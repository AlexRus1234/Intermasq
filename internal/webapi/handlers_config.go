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

// HTTP handlers for the visual dnsmasq config editor + the raw file
// editor (GET/PUT /api/files/:name) + template-backed file creation +
// DELETE /api/config/file. Backend logic lives in config_snapshot.go and
// backup.go (deleteConfigFile).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"intermask/internal/audit"
	"intermask/internal/dnsmasq"
	"intermask/internal/models"
)

func getConfigHandler(c *gin.Context) {
	snap := dnsmasq.ReadConfigSnapshot()
	c.JSON(200, snap)
}

func updateConfigHandler(c *gin.Context) {
	var req models.ConfigUpdateReq
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}
	if !dnsmasq.IsSafePath(req.File) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	directiveKeyValidator := dnsmasq.DirectiveKeyRegex
	for _, d := range req.Directives {
		if !directiveKeyValidator.MatchString(d.Key) {
			c.JSON(400, gin.H{"error": "invalid_directive_key", "key": d.Key})
			return
		}
		if strings.Contains(d.Value, "\n") {
			c.JSON(400, gin.H{"error": "invalid_directive_value", "key": d.Key})
			return
		}
	}

	dnsmasq.Mu.Lock()
	defer dnsmasq.Mu.Unlock()

	content, err := dnsmasq.SerializeConfigFile(req.File, req.Directives)
	if err != nil {
		c.JSON(500, gin.H{"error": "serialize_error"})
		return
	}
	if err := dnsmasq.WriteConfigWithTest(req.File, content); err != nil {
		errMsg := err.Error()
		if strings.HasPrefix(errMsg, "dnsmasq_test_failed") {
			c.JSON(400, gin.H{"error": "dnsmasq_test_failed", "detail": strings.TrimPrefix(errMsg, "dnsmasq_test_failed: ")})
		} else {
			c.JSON(500, gin.H{"error": "write_error"})
		}
		return
	}

	audit.WriteAudit(audit.AuditEntry{
		User:   getUser(c),
		Action: "config_update",
		File:   req.File,
		Mac:    fmt.Sprintf("%d directives", len(req.Directives)),
	})

	snap := dnsmasq.ReadConfigSnapshot()
	c.JSON(200, snap)
}

func getDhcpRangesHandler(c *gin.Context) {
	c.JSON(200, gin.H{"ranges": dnsmasq.DetectDhcpRangesCIDR()})
}

func createConfigFileHandler(c *gin.Context) {
	var req models.CreateConfigFileReq
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || filepath.Ext(name) != ".conf" {
		c.JSON(400, gin.H{"error": "invalid_filename"})
		return
	}
	template := strings.ToLower(strings.TrimSpace(req.Template))
	if template == "" {
		template = "empty"
	}
	content, ok := dnsmasq.ConfigTemplates[template]
	if !ok {
		c.JSON(400, gin.H{"error": "unknown_template", "template": template, "available": dnsmasq.KnownConfigTemplateIDs()})
		return
	}
	fullPath := filepath.Join(*dnsmasq.ConfigDir, name)
	if !dnsmasq.IsSafePath(fullPath) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}

	dnsmasq.Mu.Lock()
	defer dnsmasq.Mu.Unlock()

	if _, err := os.Stat(fullPath); err == nil {
		c.JSON(409, gin.H{"error": "file_exists"})
		return
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		c.JSON(500, gin.H{"error": "write_error"})
		return
	}

	audit.WriteAudit(audit.AuditEntry{
		User:     getUser(c),
		Action:   "config_create_file",
		File:     fullPath,
		Template: template,
	})

	snap := dnsmasq.ReadConfigSnapshot()
	c.JSON(200, snap)
}

// deleteConfigFileHandler removes a .conf file from ConfigDir. The path is
// derived from the request body (not a URL param) to keep it consistent
// with the rest of the config API and to avoid gin's path-parameter quirks
// with names containing dots. On success returns the updated ConfigSnapshot
// so the UI can drop the deleted file tab without an extra round-trip.
//
// Body: {"file": "/etc/dnsmasq.d/old.conf"}
func deleteConfigFileHandler(c *gin.Context) {
	var req struct {
		File string `json:"file"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}
	name := filepath.Base(req.File)
	if name == "" || name == "." || name == string(os.PathSeparator) {
		c.JSON(400, gin.H{"error": "invalid_filename"})
		return
	}
	if filepath.Ext(name) != ".conf" {
		c.JSON(400, gin.H{"error": "invalid_filename"})
		return
	}
	if !dnsmasq.IsSafePath(req.File) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}

	dnsmasq.Mu.Lock()
	defer dnsmasq.Mu.Unlock()

	if err := dnsmasq.DeleteConfigFile(req.File); err != nil {
		if os.IsNotExist(err) {
			c.JSON(404, gin.H{"error": "file_not_found"})
			return
		}
		if err == os.ErrPermission {
			c.JSON(403, gin.H{"error": "access_denied"})
			return
		}
		c.JSON(500, gin.H{"error": "delete_error"})
		return
	}

	audit.WriteAudit(audit.AuditEntry{
		User:   getUser(c),
		Action: "config_delete_file",
		File:   req.File,
	})

	snap := dnsmasq.ReadConfigSnapshot()
	c.JSON(200, snap)
}

// listConfigTemplatesHandler РѕС‚РґР°С‘С‚ РєР°С‚Р°Р»РѕРі РёР·РІРµСЃС‚РЅС‹С… С€Р°Р±Р»РѕРЅРѕРІ РґР»СЏ UI:
// СЃРїРёСЃРѕРє ID + preview-СЃРѕРґРµСЂР¶РёРјРѕРµ.
func listConfigTemplatesHandler(c *gin.Context) {
	ids := dnsmasq.KnownConfigTemplateIDs()
	out := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		out = append(out, gin.H{
			"id":      id,
			"preview": dnsmasq.ConfigTemplates[id],
		})
	}
	c.JSON(200, gin.H{"templates": out})
}

// ===== Raw file editor (used for hand-editing a .conf file as plain text) =====

func getFileHandler(c *gin.Context) {
	name := c.Param("name")
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || filepath.Ext(name) != ".conf" {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	path := filepath.Join(*dnsmasq.ConfigDir, name)
	// Defense in depth (A11): the substring filter above already blocks any
	// separator-bearing name, so filepath.Join cannot escape ConfigDir today.
	// Re-check via isSafePath so a future weakening of the substring filter
	// (or a new call site) still cannot read outside ConfigDir. readFileRaw
	// checks isSafePath again вЂ” three layers, by design.
	if !dnsmasq.IsSafePath(path) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	content, err := dnsmasq.ReadFileRaw(path)
	if err != nil {
		c.JSON(404, gin.H{"error": "file_not_found"})
		return
	}
	c.JSON(200, gin.H{"path": path, "content": string(content)})
}

func putFileHandler(c *gin.Context) {
	name := c.Param("name")
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || filepath.Ext(name) != ".conf" {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	path := filepath.Join(*dnsmasq.ConfigDir, name)
	// Defense in depth (A11): mirror getFileHandler вЂ” isSafePath after Join.
	if !dnsmasq.IsSafePath(path) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := c.BindJSON(&req); err != nil {
		return
	}
	dnsmasq.Mu.Lock()
	defer dnsmasq.Mu.Unlock()
	if err := dnsmasq.WriteFileRaw(path, []byte(req.Content)); err != nil {
		if strings.HasPrefix(err.Error(), "dnsmasq_test_failed") {
			c.JSON(400, gin.H{"error": "dnsmasq_test_failed", "detail": strings.TrimPrefix(err.Error(), "dnsmasq_test_failed: ")})
		} else {
			c.JSON(500, gin.H{"error": "write_error"})
		}
		return
	}
	audit.WriteAudit(audit.AuditEntry{
		User:   getUser(c),
		Action: "config_write_raw",
		File:   path,
	})
	c.JSON(200, gin.H{"status": "ok"})
}
