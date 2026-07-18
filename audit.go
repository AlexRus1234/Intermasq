package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

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

func writeAudit(entry AuditEntry) {
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

func auditHandler(c *gin.Context) {
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
