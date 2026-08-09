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

// Safety-net handlers: .bak rollback, multi-level versioned history
// (list/diff/restore), ZIP backup download, ZIP restore. The backend logic
// lives in internal/dnsmasq (history.go + backup.go there).

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"intermask/internal/audit"
	"intermask/internal/dnsmasq"
	"intermask/internal/models"
)

func rollbackHandler(c *gin.Context) {
	var req struct {
		File string `json:"file"`
	}
	if err := c.BindJSON(&req); err != nil {
		return
	}

	dnsmasq.Mu.Lock()
	defer dnsmasq.Mu.Unlock()

	if err := dnsmasq.RollbackFile(req.File); err != nil {
		c.JSON(500, gin.H{"error": "rollback_error"})
		return
	}

	audit.WriteAudit(audit.AuditEntry{
		User:   getUser(c),
		Action: "rollback",
		File:   req.File,
	})

	c.JSON(200, gin.H{"status": "rollback_ok"})
}

// historyListHandler returns the list of stored versions for a config file.
// Query: GET /api/history?file=<absolute path inside ConfigDir>
func historyListHandler(c *gin.Context) {
	file := c.Query("file")
	if file == "" {
		c.JSON(400, gin.H{"error": "file_required"})
		return
	}
	if !dnsmasq.IsSafePath(file) {
		c.JSON(400, gin.H{"error": "invalid_path"})
		return
	}
	versions, err := dnsmasq.ListHistory(file)
	if err != nil {
		c.JSON(500, gin.H{"error": "history_error"})
		return
	}
	c.JSON(200, gin.H{"versions": versions})
}

// historyDiffHandler returns a unified diff between two stored versions
// (or between a version and the current on-disk content when "to" is empty).
// Query: GET /api/history/diff?file=<path>&from=<v>&to=<v|current>
func historyDiffHandler(c *gin.Context) {
	file := c.Query("file")
	from := c.Query("from")
	to := c.Query("to")
	if file == "" || from == "" {
		c.JSON(400, gin.H{"error": "params_required"})
		return
	}
	if !dnsmasq.IsSafePath(file) {
		c.JSON(400, gin.H{"error": "invalid_path"})
		return
	}
	fromBytes, err := dnsmasq.ReadHistoryVersion(file, from)
	if err != nil {
		c.JSON(404, gin.H{"error": "version_not_found"})
		return
	}
	var toBytes []byte
	if to == "" || to == "current" {
		toBytes, err = os.ReadFile(file)
		if err != nil {
			c.JSON(404, gin.H{"error": "current_not_found"})
			return
		}
	} else {
		toBytes, err = dnsmasq.ReadHistoryVersion(file, to)
		if err != nil {
			c.JSON(404, gin.H{"error": "version_not_found"})
			return
		}
	}
	diff := dnsmasq.UnifiedDiff(string(fromBytes), string(toBytes), file+" (@"+from+")", file+" (@"+coalesce(to, "current")+")")
	c.JSON(200, gin.H{"diff": diff})
}

// historyRestoreHandler restores a config file to a stored version.
func historyRestoreHandler(c *gin.Context) {
	var req models.HistoryRestoreReq
	if err := c.BindJSON(&req); err != nil {
		return
	}
	if req.File == "" || req.Version == "" {
		c.JSON(400, gin.H{"error": "params_required"})
		return
	}
	if !dnsmasq.IsSafePath(req.File) {
		c.JSON(400, gin.H{"error": "invalid_path"})
		return
	}
	dnsmasq.Mu.Lock()
	defer dnsmasq.Mu.Unlock()
	if err := dnsmasq.RestoreHistoryVersion(req.File, req.Version); err != nil {
		c.JSON(500, gin.H{"error": "restore_error", "detail": err.Error()})
		return
	}
	audit.WriteAudit(audit.AuditEntry{
		User:    getUser(c),
		Action:  "restore",
		File:    req.File,
		Version: req.Version,
	})
	c.JSON(200, gin.H{"status": "restore_ok"})
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ===== ZIP backup / restore =====

func backupHandler(c *gin.Context) {
	zipBytes, fileName, err := dnsmasq.CreateBackupZip()
	if err != nil {
		c.JSON(500, gin.H{"error": "backup_error"})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	c.Data(200, "application/zip", zipBytes)
}

func restoreBackupHandler(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "no_file"})
		return
	}
	f, err := file.Open()
	if err != nil {
		c.JSON(500, gin.H{"error": "read_error"})
		return
	}
	defer f.Close()
	zipData, err := io.ReadAll(f)
	if err != nil {
		c.JSON(500, gin.H{"error": "read_error"})
		return
	}
	dnsmasq.Mu.Lock()
	defer dnsmasq.Mu.Unlock()
	if err := dnsmasq.RestoreBackupZip(zipData); err != nil {
		if strings.HasPrefix(err.Error(), "dnsmasq_test_failed") {
			c.JSON(400, gin.H{"error": "dnsmasq_test_failed", "detail": strings.TrimPrefix(err.Error(), "dnsmasq_test_failed: ")})
		} else {
			c.JSON(400, gin.H{"error": err.Error()})
		}
		return
	}
	audit.WriteAudit(audit.AuditEntry{
		User:   getUser(c),
		Action: "backup_restore",
	})
	c.JSON(200, gin.H{"status": "ok"})
}
