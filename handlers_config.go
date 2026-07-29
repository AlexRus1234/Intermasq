package main

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
)

func getConfigHandler(c *gin.Context) {
	snap := readConfigSnapshot()
	c.JSON(200, snap)
}

func updateConfigHandler(c *gin.Context) {
	var req ConfigUpdateReq
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}
	if !isSafePath(req.File) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	directiveKeyValidator := directiveKeyRegex
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

	mu.Lock()
	defer mu.Unlock()

	content, err := serializeConfigFile(req.File, req.Directives)
	if err != nil {
		c.JSON(500, gin.H{"error": "serialize_error"})
		return
	}
	if err := writeConfigWithTest(req.File, content); err != nil {
		errMsg := err.Error()
		if strings.HasPrefix(errMsg, "dnsmasq_test_failed") {
			c.JSON(400, gin.H{"error": "dnsmasq_test_failed", "detail": strings.TrimPrefix(errMsg, "dnsmasq_test_failed: ")})
		} else {
			c.JSON(500, gin.H{"error": "write_error"})
		}
		return
	}

	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "config_update",
		File:   req.File,
		Mac:    fmt.Sprintf("%d directives", len(req.Directives)),
	})

	snap := readConfigSnapshot()
	c.JSON(200, snap)
}

func getDhcpRangesHandler(c *gin.Context) {
	c.JSON(200, gin.H{"ranges": detectDhcpRangesCIDR()})
}

func createConfigFileHandler(c *gin.Context) {
	var req CreateConfigFileReq
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
	content, ok := configTemplates[template]
	if !ok {
		c.JSON(400, gin.H{"error": "unknown_template", "template": template, "available": knownConfigTemplateIDs()})
		return
	}
	fullPath := filepath.Join(*ConfigDir, name)
	if !isSafePath(fullPath) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if _, err := os.Stat(fullPath); err == nil {
		c.JSON(409, gin.H{"error": "file_exists"})
		return
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		c.JSON(500, gin.H{"error": "write_error"})
		return
	}

	writeAudit(AuditEntry{
		User:     getUser(c),
		Action:   "config_create_file",
		File:     fullPath,
		Template: template,
	})

	snap := readConfigSnapshot()
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
	if !isSafePath(req.File) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if err := deleteConfigFile(req.File); err != nil {
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

	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "config_delete_file",
		File:   req.File,
	})

	snap := readConfigSnapshot()
	c.JSON(200, snap)
}

// listConfigTemplatesHandler отдаёт каталог известных шаблонов для UI:
// список ID + preview-содержимое.
func listConfigTemplatesHandler(c *gin.Context) {
	ids := knownConfigTemplateIDs()
	out := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		out = append(out, gin.H{
			"id":      id,
			"preview": configTemplates[id],
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
	path := filepath.Join(*ConfigDir, name)
	// Defense in depth (A11): the substring filter above already blocks any
	// separator-bearing name, so filepath.Join cannot escape ConfigDir today.
	// Re-check via isSafePath so a future weakening of the substring filter
	// (or a new call site) still cannot read outside ConfigDir. readFileRaw
	// checks isSafePath again — three layers, by design.
	if !isSafePath(path) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	content, err := readFileRaw(path)
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
	path := filepath.Join(*ConfigDir, name)
	// Defense in depth (A11): mirror getFileHandler — isSafePath after Join.
	if !isSafePath(path) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := c.BindJSON(&req); err != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if err := writeFileRaw(path, []byte(req.Content)); err != nil {
		if strings.HasPrefix(err.Error(), "dnsmasq_test_failed") {
			c.JSON(400, gin.H{"error": "dnsmasq_test_failed", "detail": strings.TrimPrefix(err.Error(), "dnsmasq_test_failed: ")})
		} else {
			c.JSON(500, gin.H{"error": "write_error"})
		}
		return
	}
	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "config_write_raw",
		File:   path,
	})
	c.JSON(200, gin.H{"status": "ok"})
}
