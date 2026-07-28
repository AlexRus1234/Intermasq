package main

// This file holds HTTP handlers for the static-host subsystem
// (dhcp-host=): single add/edit/delete, bulk add via JSON or CSV,
// bulk-move between files, bulk-edit (IP-prefix transform), and the
// template CRUD + apply flow used by the auto-next-IP button.
//
// See handlers.go for the root endpoints (status, setup, login, reload,
// arp, next-ip, leases, events, discovery) and other handler_*.go files
// for aliases / config / safety / users.

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func getHostsHandler(c *gin.Context) {
	hosts := []HostEntry{}
	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		c.JSON(500, gin.H{"error": "dir_read_error"})
		return
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, f.Name())

		hasBak := false
		if _, err := os.Stat(fullPath + ".bak"); err == nil {
			hasBak = true
		}

		content, _ := os.ReadFile(fullPath)
		for _, raw := range strings.Split(string(content), "\n") {
			entry, ok := parseDhcpHostLine(raw, fullPath)
			if !ok {
				continue
			}
			if hasBak {
				entry.File = fullPath + "|has_bak"
			}
			hosts = append(hosts, entry)
		}
	}
	c.JSON(200, hosts)
}

func addHostHandler(c *gin.Context) {
	var req HostEntry
	if err := c.BindJSON(&req); err != nil {
		return
	}
	if !isSafePath(req.File) {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}
	req.Mac = normalizeMAC(req.Mac)
	if !validateHostFields(req.Mac, req.Ip, req.Hostname) {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}
	if !validateHostTags(req.Tags) {
		c.JSON(400, gin.H{"error": "invalid_tag", "detail": "tags must look like set:iot or tag:guest"})
		return
	}
	req.Tags = normalizeHostTags(req.Tags)

	if req.Ip != "" {
		conflicts := findHostsByIP(req.Ip, req.Mac)
		if len(conflicts) > 0 {
			fmt.Printf("[VALIDATION] IP duplicate detected: %d conflicts for IP %s\n", len(conflicts), req.Ip)
			c.JSON(409, gin.H{"error": "ip_duplicate", "conflicts": conflicts})
			return
		}
	}

	macConflicts := findHostsByMac(req.Mac)
	if len(macConflicts) > 0 {
		fmt.Printf("[VALIDATION] MAC duplicate detected: %d for MAC %s\n", len(macConflicts), req.Mac)
		c.JSON(409, gin.H{"error": "mac_duplicate", "conflicts": macConflicts})
		return
	}
	fmt.Printf("[VALIDATION] MAC %s accepted (ip=%q hostname=%q)\n", req.Mac, req.Ip, req.Hostname)

	mu.Lock()
	defer mu.Unlock()

	createLocalBackup(req.File)

	content, err := os.ReadFile(req.File)
	if err != nil && !os.IsNotExist(err) {
		c.JSON(500, gin.H{"error": "file_read_error"})
		return
	}

	lines := strings.Split(string(content), "\n")
	newLines := []string{}
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "dhcp-host=") && strings.Contains(line, req.Mac) {
			continue
		}
		if strings.TrimSpace(line) != "" {
			newLines = append(newLines, line)
		}
	}
	newLines = append(newLines, formatDhcpHostLine(req))

	if err := os.WriteFile(req.File, []byte(strings.Join(newLines, "\n")+"\n"), 0644); err != nil {
		c.JSON(500, gin.H{"error": "file_write_error"})
		return
	}

	writeAudit(AuditEntry{
		User:     getUser(c),
		Action:   "add",
		Mac:      req.Mac,
		Hostname: req.Hostname,
		Ip:       req.Ip,
		File:     req.File,
	})

	c.JSON(200, gin.H{"status": "ok"})
}

// validateHostTags returns false if any tag is not a recognized dhcp-host
// qualifier ("set:<name>" / "tag:<name>"). "id:<client-id>" is also accepted
// so existing configs round-trip without being rejected on edit.
func validateHostTags(tags []string) bool {
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !dhcpTagRegex.MatchString(t) && !strings.HasPrefix(t, "id:") {
			return false
		}
	}
	return true
}

// normalizeHostTags drops empties and de-duplicates case-insensitively while
// preserving the first-seen order.
func normalizeHostTags(tags []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out
}

func bulkAddHostsHandler(c *gin.Context) {
	var req BulkHostReq
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}
	if !isSafePath(req.File) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}

	// Normalise MAC separators up-front so the in-batch cross-check and the
	// eventual write both see the canonical colon form.
	for i := range req.Hosts {
		req.Hosts[i].Mac = normalizeMAC(req.Hosts[i].Mac)
	}

	for i, h1 := range req.Hosts {
		if !macRegex.MatchString(h1.Mac) {
			c.JSON(400, gin.H{"error": "invalid_mac", "mac": h1.Mac})
			return
		}
		if h1.Ip != "" && net.ParseIP(h1.Ip) == nil {
			c.JSON(400, gin.H{"error": "invalid_ip", "mac": h1.Mac})
			return
		}
		if h1.Hostname != "" && !validHostname(h1.Hostname) {
			c.JSON(400, gin.H{"error": "invalid_hostname", "mac": h1.Mac})
			return
		}
		for j, h2 := range req.Hosts {
			if i != j && h1.Ip != "" && h2.Ip == h1.Ip && strings.ToLower(h2.Mac) != strings.ToLower(h1.Mac) {
				c.JSON(409, gin.H{"error": "ip_duplicate_bulk", "ip": h1.Ip, "mac1": h1.Mac, "mac2": h2.Mac})
				return
			}
		}
	}

	for _, h := range req.Hosts {
		if h.Ip != "" {
			conflicts := findHostsByIP(h.Ip, h.Mac)
			if len(conflicts) > 0 {
				c.JSON(409, gin.H{"error": "ip_duplicate", "conflicts": conflicts})
				return
			}
		}
		macConflicts := findHostsByMac(h.Mac)
		if len(macConflicts) > 0 {
			c.JSON(409, gin.H{"error": "mac_duplicate", "conflicts": macConflicts})
			return
		}
	}

	mu.Lock()
	defer mu.Unlock()

	createLocalBackup(req.File)

	content, err := os.ReadFile(req.File)
	if err != nil && !os.IsNotExist(err) {
		c.JSON(500, gin.H{"error": "read_error"})
		return
	}

	lines := strings.Split(string(content), "\n")
	newLines := []string{}

	newMacs := make(map[string]bool)
	for _, h := range req.Hosts {
		if validateHostFields(h.Mac, h.Ip, h.Hostname) {
			newMacs[strings.ToLower(h.Mac)] = true
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

	for _, h := range req.Hosts {
		if newMacs[strings.ToLower(h.Mac)] {
			newLines = append(newLines, formatDhcpHostLine(h))
		}
	}

	if err := os.WriteFile(req.File, []byte(strings.Join(newLines, "\n")+"\n"), 0644); err != nil {
		c.JSON(500, gin.H{"error": "write_error"})
		return
	}

	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "bulk_add",
		File:   req.File,
		Mac:    fmt.Sprintf("%d hosts", len(req.Hosts)),
	})

	c.JSON(200, gin.H{"status": "ok", "count": len(req.Hosts)})
}

func deleteHostHandler(c *gin.Context) {
	mac := c.Param("mac")
	file := c.Query("file")
	if !macRegex.MatchString(mac) || !isSafePath(file) {
		c.JSON(400, gin.H{"error": "bad_request"})
		return
	}

	macLower := strings.ToLower(mac)

	mu.Lock()
	defer mu.Unlock()

	createLocalBackup(file)

	content, err := os.ReadFile(file)
	if err != nil {
		c.JSON(500, gin.H{"error": "file_read_error"})
		return
	}

	lines := strings.Split(string(content), "\n")
	newLines := []string{}
	found := false
	var deletedHostname, deletedIP string

	for _, line := range lines {
		cleanLine := strings.TrimSpace(line)
		if strings.HasPrefix(cleanLine, "dhcp-host=") && strings.Contains(strings.ToLower(cleanLine), macLower) {
			found = true
			parts := strings.Split(strings.TrimPrefix(cleanLine, "dhcp-host="), ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if net.ParseIP(p) != nil {
					deletedIP = p
				} else if !macRegex.MatchString(p) && p != "" {
					deletedHostname = p
				}
			}
			continue
		}
		if cleanLine != "" {
			newLines = append(newLines, line)
		}
	}

	if found {
		if err := os.WriteFile(file, []byte(strings.Join(newLines, "\n")+"\n"), 0644); err != nil {
			c.JSON(500, gin.H{"error": "file_write_error"})
			return
		}

		writeAudit(AuditEntry{
			User:     getUser(c),
			Action:   "delete",
			Mac:      mac,
			Hostname: deletedHostname,
			Ip:       deletedIP,
			File:     file,
		})

		c.JSON(200, gin.H{"status": "deleted"})
	} else {
		c.JSON(404, gin.H{"error": "host_not_found"})
	}
}

func exportCSVHandler(c *gin.Context) {
	hosts := readAllHosts()
	csvData := hostsToCSV(hosts)
	c.Header("Content-Disposition", "attachment; filename=intermasq_hosts.csv")
	c.Data(200, "text/csv", csvData)
}

func importCSVHandler(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "no_file"})
		return
	}

	targetFile := c.PostForm("target_file")
	if !isSafePath(targetFile) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}

	f, err := file.Open()
	if err != nil {
		c.JSON(500, gin.H{"error": "read_error"})
		return
	}
	defer f.Close()

	hosts, err := parseCSVHosts(f, targetFile)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid_csv"})
		return
	}

	if len(hosts) == 0 {
		c.JSON(400, gin.H{"error": "csv_empty"})
		return
	}

	for i, h1 := range hosts {
		for j, h2 := range hosts {
			if i != j && h2.Ip == h1.Ip && strings.ToLower(h2.Mac) != strings.ToLower(h1.Mac) {
				c.JSON(409, gin.H{"error": "ip_duplicate_bulk", "ip": h1.Ip, "mac1": h1.Mac, "mac2": h2.Mac})
				return
			}
		}
	}

	for _, h := range hosts {
		conflicts := findHostsByIP(h.Ip, h.Mac)
		if len(conflicts) > 0 {
			c.JSON(409, gin.H{"error": "ip_duplicate", "conflicts": conflicts})
			return
		}
		macConflicts := findHostsByMac(h.Mac)
		if len(macConflicts) > 0 {
			c.JSON(409, gin.H{"error": "mac_duplicate", "conflicts": macConflicts})
			return
		}
	}

	mu.Lock()
	defer mu.Unlock()

	createLocalBackup(targetFile)

	content, err := os.ReadFile(targetFile)
	if err != nil && !os.IsNotExist(err) {
		c.JSON(500, gin.H{"error": "read_error"})
		return
	}

	lines := strings.Split(string(content), "\n")
	newLines := []string{}

	newMacs := make(map[string]bool)
	for _, h := range hosts {
		newMacs[strings.ToLower(h.Mac)] = true
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

	for _, h := range hosts {
		newLines = append(newLines, formatDhcpHostLine(h))
	}

	if err := os.WriteFile(targetFile, []byte(strings.Join(newLines, "\n")+"\n"), 0644); err != nil {
		c.JSON(500, gin.H{"error": "write_error"})
		return
	}

	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "bulk_add",
		File:   targetFile,
		Mac:    fmt.Sprintf("%d hosts (csv)", len(hosts)),
	})

	c.JSON(200, gin.H{"status": "ok", "count": len(hosts)})
}

func bulkMoveHandler(c *gin.Context) {
	var req BulkMoveReq
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}
	if !isSafePath(req.Target) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	if len(req.Hosts) == 0 {
		c.JSON(400, gin.H{"error": "no_hosts"})
		return
	}

	for _, h := range req.Hosts {
		if !macRegex.MatchString(h.Mac) || !isSafePath(h.File) {
			c.JSON(400, gin.H{"error": "invalid_data"})
			return
		}
		if h.File == req.Target {
			c.JSON(400, gin.H{"error": "same_file", "mac": h.Mac})
			return
		}
	}

	mu.Lock()
	defer mu.Unlock()

	moved := 0
	skipped := []string{}

	for _, h := range req.Hosts {
		existing := readHostByMac(h.File, h.Mac)
		if existing == nil {
			skipped = append(skipped, h.Mac)
			continue
		}

		ipConflicts := findHostsByIP(existing.Ip, existing.Mac)
		hasConflictInTarget := false
		for _, cf := range ipConflicts {
			if cf.File == req.Target {
				hasConflictInTarget = true
				break
			}
		}
		if hasConflictInTarget {
			skipped = append(skipped, h.Mac)
			continue
		}

		macConflicts := findHostsByMac(existing.Mac)
		hasMacInTarget := false
		for _, cf := range macConflicts {
			if cf.File == req.Target {
				hasMacInTarget = true
				break
			}
		}
		if hasMacInTarget {
			skipped = append(skipped, h.Mac)
			continue
		}

		createLocalBackup(h.File)
		createLocalBackup(req.Target)

		if err := removeHostLine(h.File, h.Mac); err != nil {
			c.JSON(500, gin.H{"error": "file_write_error", "mac": h.Mac})
			return
		}
		if err := appendHostLine(req.Target, *existing); err != nil {
			c.JSON(500, gin.H{"error": "file_write_error", "mac": h.Mac})
			return
		}
		moved++
	}

	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "bulk_move",
		File:   req.Target,
		Mac:    fmt.Sprintf("%d moved, %d skipped", moved, len(skipped)),
	})

	c.JSON(200, gin.H{"status": "ok", "moved": moved, "skipped": skipped})
}

func bulkEditHandler(c *gin.Context) {
	var req BulkEditReq
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}
	if len(req.Hosts) == 0 {
		c.JSON(400, gin.H{"error": "no_hosts"})
		return
	}

	transform, err := parseIPTransform(req.IPTransform.OldPrefix, req.IPTransform.NewPrefix)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	for _, h := range req.Hosts {
		if !macRegex.MatchString(h.Mac) || !isSafePath(h.File) {
			c.JSON(400, gin.H{"error": "invalid_data", "mac": h.Mac})
			return
		}
	}

	type plannedChange struct {
		mac      string
		file     string
		oldEntry *HostEntry
		newIP    string
		newHost  string
		newTags  []string
	}

	planned := []plannedChange{}
	seenNewIPs := make(map[string]string)

	for _, h := range req.Hosts {
		existing := readHostByMac(h.File, h.Mac)
		if existing == nil {
			c.JSON(404, gin.H{"error": "host_not_found", "mac": h.Mac})
			return
		}

		newIP, err := transform.apply(existing.Ip)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error(), "mac": h.Mac, "old_ip": existing.Ip})
			return
		}
		if net.ParseIP(newIP) == nil {
			c.JSON(400, gin.H{"error": "invalid_ip", "mac": h.Mac})
			return
		}

		newHostname := existing.Hostname
		if strip := req.HostnameTransform.StripOld; strip != "" {
			newHostname = strings.TrimSuffix(newHostname, strip)
		}
		if suffix := req.HostnameTransform.Suffix; suffix != "" {
			newHostname = newHostname + suffix
		}
		if !validHostname(newHostname) {
			c.JSON(400, gin.H{"error": "invalid_hostname", "mac": h.Mac, "hostname": newHostname})
			return
		}

		if otherMac, dup := seenNewIPs[newIP]; dup && otherMac != existing.Mac {
			c.JSON(409, gin.H{"error": "ip_duplicate_bulk", "ip": newIP, "mac1": existing.Mac, "mac2": otherMac})
			return
		}
		seenNewIPs[newIP] = existing.Mac

		conflicts := findHostsByIP(newIP, existing.Mac)
		if len(conflicts) > 0 {
			c.JSON(409, gin.H{"error": "ip_duplicate", "conflicts": conflicts})
			return
		}

		planned = append(planned, plannedChange{
			mac:      existing.Mac,
			file:     existing.File,
			oldEntry: existing,
			newIP:    newIP,
			newHost:  newHostname,
			newTags:  existing.Tags,
		})
	}

	mu.Lock()
	defer mu.Unlock()

	affectedFiles := make(map[string]bool)
	for _, p := range planned {
		affectedFiles[p.file] = true
	}
	for f := range affectedFiles {
		createLocalBackup(f)
	}

	for _, p := range planned {
		if err := removeHostLine(p.file, p.mac); err != nil {
			c.JSON(500, gin.H{"error": "file_write_error", "mac": p.mac})
			return
		}
		if err := appendHostLine(p.file, HostEntry{
			Mac: p.mac, Hostname: p.newHost, Ip: p.newIP, File: p.file, Tags: p.newTags,
		}); err != nil {
			c.JSON(500, gin.H{"error": "file_write_error", "mac": p.mac})
			return
		}
	}

	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "bulk_edit",
		Mac:    fmt.Sprintf("%d hosts", len(planned)),
		Ip:     fmt.Sprintf("%s -> %s", req.IPTransform.OldPrefix, req.IPTransform.NewPrefix),
	})

	c.JSON(200, gin.H{"status": "ok", "updated": len(planned)})
}

// ===== Templates (host add presets) =====

func getTemplatesHandler(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

	result := []Template{}
	for _, t := range templates {
		result = append(result, t)
	}
	c.JSON(200, result)
}

func createTemplateHandler(c *gin.Context) {
	var req Template
	if err := c.BindJSON(&req); err != nil {
		return
	}
	if req.Name == "" || req.HostnamePattern == "" || req.IPRange == "" {
		c.JSON(400, gin.H{"error": "missing_fields"})
		return
	}
	if !isSafePath(req.TargetFile) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}

	mu.Lock()
	defer mu.Unlock()

	req.ID = strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))
	if _, exists := templates[req.ID]; exists {
		c.JSON(409, gin.H{"error": "template_exists"})
		return
	}
	templates[req.ID] = req
	saveTemplates()
	c.JSON(200, req)
}

func deleteTemplateHandler(c *gin.Context) {
	id := c.Param("id")

	mu.Lock()
	defer mu.Unlock()

	if _, exists := templates[id]; !exists {
		c.JSON(404, gin.H{"error": "template_not_found"})
		return
	}
	delete(templates, id)
	saveTemplates()
	c.JSON(200, gin.H{"status": "deleted"})
}

func applyTemplateHandler(c *gin.Context) {
	var req ApplyTemplateReq
	if err := c.BindJSON(&req); err != nil {
		return
	}
	if !macRegex.MatchString(req.Mac) {
		c.JSON(400, gin.H{"error": "invalid_mac"})
		return
	}

	mu.Lock()
	tpl, exists := templates[req.TemplateID]
	mu.Unlock()
	if !exists {
		c.JSON(404, gin.H{"error": "template_not_found"})
		return
	}

	ip, err := findFreeIP(tpl.IPRange)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	index := countHostsInFile(tpl.TargetFile) + 1
	hostname := genHostnameFromPattern(tpl.HostnamePattern, index)

	c.JSON(200, gin.H{
		"mac":      req.Mac,
		"ip":       ip,
		"hostname": hostname,
		"file":     tpl.TargetFile,
	})
}
