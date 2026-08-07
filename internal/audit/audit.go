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

package audit

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

var AuditLogPath = flag.String("audit-log", "/etc/intermasq/audit.log", "Path to audit log file")

type AuditEntry struct {
	Timestamp string `json:"timestamp"`
	User      string `json:"user"`
	Action    string `json:"action"`
	Mac       string `json:"mac"`
	Hostname  string `json:"hostname"`
	Ip        string `json:"ip"`
	File      string `json:"file"`
	Version   string `json:"version,omitempty"`
	Template  string `json:"template,omitempty"`
}

func WriteAudit(entry AuditEntry) {
	entry.Timestamp = time.Now().Format(time.RFC3339)
	dir := filepath.Dir(*AuditLogPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("[AUDIT] Failed to create dir %s: %v\n", dir, err)
		return
	}
	f, err := os.OpenFile(*AuditLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("[AUDIT] Failed to open log %s: %v\n", *AuditLogPath, err)
		return
	}
	defer f.Close()
	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Printf("[AUDIT] Failed to marshal entry: %v\n", err)
		return
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		fmt.Printf("[AUDIT] Failed to write log: %v\n", err)
	}
}

func Handler(c *gin.Context) {
	entries := []AuditEntry{}
	f, err := os.Open(*AuditLogPath)
	if err != nil {
		c.JSON(200, entries)
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry AuditEntry
		if json.Unmarshal([]byte(line), &entry) == nil {
			entries = append(entries, entry)
		}
	}
	c.JSON(200, entries)
}
