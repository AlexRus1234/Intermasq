package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func statusHandler(c *gin.Context) {
	isActive := checkDnsmasqStatus()
	mu.Lock()
	defer mu.Unlock()
	c.JSON(200, gin.H{
		"setup_required": len(users) == 0,
		"version":        "3.0",
		"dnsmasq_active": isActive,
	})
}

func setupHandler(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()
	if len(users) > 0 {
		c.JSON(403, gin.H{"error": "already_setup"})
		return
	}
	var req AuthReq
	if err := c.BindJSON(&req); err != nil {
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	users[req.Username] = string(hash)
	saveUsers()
	c.JSON(200, gin.H{"token": makeToken(req.Username)})
}

func loginHandler(c *gin.Context) {
	var req AuthReq
	if err := c.BindJSON(&req); err != nil {
		return
	}
	mu.Lock()
	hash, ok := users[req.Username]
	mu.Unlock()
	if !ok || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		c.JSON(401, gin.H{"error": "invalid_credentials"})
		return
	}
	c.JSON(200, gin.H{"token": makeToken(req.Username)})
}

func getArpHandler(c *gin.Context) {
	c.JSON(200, getArpTable())
}

func nextIPHandler(c *gin.Context) {
	cidr := c.Query("range")
	if cidr == "" {
		c.JSON(400, gin.H{"error": "range_required"})
		return
	}
	ip, err := findFreeIP(cidr)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ip": ip})
}

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
		if err := appendHostLine(req.Target, existing.Mac, existing.Hostname, existing.Ip); err != nil {
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
		if !hostnameRegex.MatchString(newHostname) {
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
		if err := appendHostLine(p.file, p.mac, p.newHost, p.newIP); err != nil {
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
		lines := strings.Split(string(content), "\n")

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "dhcp-host=") {
				parts := strings.Split(strings.TrimPrefix(line, "dhcp-host="), ",")
				entry := HostEntry{File: fullPath}
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if macRegex.MatchString(p) {
						entry.Mac = p
					} else if net.ParseIP(p) != nil {
						entry.Ip = p
					} else {
						entry.Hostname = p
					}
				}
				if entry.Mac != "" {
					if hasBak {
						entry.File = fullPath + "|has_bak"
					}
					hosts = append(hosts, entry)
				}
			}
		}
	}
	c.JSON(200, hosts)
}

func getLeasesHandler(c *gin.Context) {
	c.JSON(200, parseLeases())
}

func getUser(c *gin.Context) string {
	u, _ := c.Get("user")
	user, _ := u.(string)
	return user
}

func addHostHandler(c *gin.Context) {
	var req HostEntry
	if err := c.BindJSON(&req); err != nil {
		return
	}
	if !macRegex.MatchString(req.Mac) || net.ParseIP(req.Ip) == nil || !hostnameRegex.MatchString(req.Hostname) || !isSafePath(req.File) {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}

	conflicts := findHostsByIP(req.Ip, req.Mac)
	if len(conflicts) > 0 {
		fmt.Printf("[VALIDATION] IP duplicate detected: %s conflicts for IP %s\n", len(conflicts), req.Ip)
		c.JSON(409, gin.H{"error": "ip_duplicate", "conflicts": conflicts})
		return
	}

	macConflicts := findHostsByMac(req.Mac)
	if len(macConflicts) > 0 {
		fmt.Printf("[VALIDATION] MAC duplicate detected: %s for MAC %s\n", len(macConflicts), req.Mac)
		c.JSON(409, gin.H{"error": "mac_duplicate", "conflicts": macConflicts})
		return
	}
	fmt.Printf("[VALIDATION] IP %s and MAC %s are unique, proceeding\n", req.Ip, req.Mac)

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
	newLines = append(newLines, fmt.Sprintf("dhcp-host=%s,%s,%s", req.Mac, req.Hostname, req.Ip))

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

	for i, h1 := range req.Hosts {
		if !macRegex.MatchString(h1.Mac) || net.ParseIP(h1.Ip) == nil || !hostnameRegex.MatchString(h1.Hostname) {
			continue
		}
		for j, h2 := range req.Hosts {
			if i != j && h2.Ip == h1.Ip && strings.ToLower(h2.Mac) != strings.ToLower(h1.Mac) {
				c.JSON(409, gin.H{"error": "ip_duplicate_bulk", "ip": h1.Ip, "mac1": h1.Mac, "mac2": h2.Mac})
				return
			}
		}
	}

	for _, h := range req.Hosts {
		if !macRegex.MatchString(h.Mac) || net.ParseIP(h.Ip) == nil || !hostnameRegex.MatchString(h.Hostname) {
			continue
		}
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
		if macRegex.MatchString(h.Mac) && net.ParseIP(h.Ip) != nil && hostnameRegex.MatchString(h.Hostname) {
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
			newLines = append(newLines, fmt.Sprintf("dhcp-host=%s,%s,%s", h.Mac, h.Hostname, h.Ip))
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

	c.JSON(200, gin.H{"status": "ok"})
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

func rollbackHandler(c *gin.Context) {
	var req struct {
		File string `json:"file"`
	}
	if err := c.BindJSON(&req); err != nil {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if err := rollbackFile(req.File); err != nil {
		c.JSON(500, gin.H{"error": "rollback_error"})
		return
	}

	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "rollback",
		File:   req.File,
	})

	c.JSON(200, gin.H{"status": "rollback_ok"})
}

func backupHandler(c *gin.Context) {
	zipBytes, fileName, err := createBackupZip()
	if err != nil {
		c.JSON(500, gin.H{"error": "backup_error"})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	c.Data(200, "application/zip", zipBytes)
}

func reloadHandler(c *gin.Context) {
	if err := reloadDnsmasq(); err != nil {
		c.JSON(400, gin.H{"error": "reload_error"})
		return
	}

	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "reload",
	})

	c.JSON(200, gin.H{"status": "reloaded"})
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
		newLines = append(newLines, fmt.Sprintf("dhcp-host=%s,%s,%s", h.Mac, h.Hostname, h.Ip))
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
