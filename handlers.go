package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
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
		fmt.Printf("[VALIDATION] IP duplicate detected: %d conflicts for IP %s\n", len(conflicts), req.Ip)
		c.JSON(409, gin.H{"error": "ip_duplicate", "conflicts": conflicts})
		return
	}

	macConflicts := findHostsByMac(req.Mac)
	if len(macConflicts) > 0 {
		fmt.Printf("[VALIDATION] MAC duplicate detected: %d for MAC %s\n", len(macConflicts), req.Mac)
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

// historyListHandler returns the list of stored versions for a config file.
// Query: GET /api/history?file=<absolute path inside ConfigDir>
func historyListHandler(c *gin.Context) {
	file := c.Query("file")
	if file == "" {
		c.JSON(400, gin.H{"error": "file_required"})
		return
	}
	if !isSafePath(file) {
		c.JSON(400, gin.H{"error": "invalid_path"})
		return
	}
	versions, err := listHistory(file)
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
	if !isSafePath(file) {
		c.JSON(400, gin.H{"error": "invalid_path"})
		return
	}
	fromBytes, err := readHistoryVersion(file, from)
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
		toBytes, err = readHistoryVersion(file, to)
		if err != nil {
			c.JSON(404, gin.H{"error": "version_not_found"})
			return
		}
	}
	diff := unifiedDiff(string(fromBytes), string(toBytes), file+" (@"+from+")", file+" (@"+coalesce(to, "current")+")")
	c.JSON(200, gin.H{"diff": diff})
}

// historyRestoreHandler restores a config file to a stored version.
func historyRestoreHandler(c *gin.Context) {
	var req HistoryRestoreReq
	if err := c.BindJSON(&req); err != nil {
		return
	}
	if req.File == "" || req.Version == "" {
		c.JSON(400, gin.H{"error": "params_required"})
		return
	}
	if !isSafePath(req.File) {
		c.JSON(400, gin.H{"error": "invalid_path"})
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if err := restoreHistoryVersion(req.File, req.Version); err != nil {
		c.JSON(500, gin.H{"error": "restore_error", "detail": err.Error()})
		return
	}
	writeAudit(AuditEntry{
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
	directiveKeyValidator := regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
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
	if err := os.WriteFile(fullPath, []byte("# === Managed by Intermasq ===\n"), 0644); err != nil {
		c.JSON(500, gin.H{"error": "write_error"})
		return
	}

	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "config_create_file",
		File:   fullPath,
	})

	snap := readConfigSnapshot()
	c.JSON(200, snap)
}

// resolveAliasesTargetFile returns the absolute path to the aliases target
// file. If reqFile is empty, the default file (10-dns-aliases.conf under
// ConfigDir) is created on demand and returned. Returns ok=false if the
// supplied path is unsafe.
func resolveAliasesTargetFile(reqFile string) (string, bool) {
	if reqFile == "" {
		path := filepath.Join(*ConfigDir, DefaultAliasesFileName)
		if err := ensureAliasesFile(path); err != nil {
			return "", false
		}
		return path, true
	}
	if !isSafePath(reqFile) {
		return "", false
	}
	return reqFile, true
}

func validateAliasEntry(a DnsAliasEntry) bool {
	if a.Type != "A" && a.Type != "CNAME" {
		return false
	}
	if !aliasDomainRegex.MatchString(a.Domain) {
		return false
	}
	if a.Type == "A" {
		return net.ParseIP(a.Target) != nil
	}
	return aliasDomainRegex.MatchString(a.Target)
}

func getAliasesHandler(c *gin.Context) {
	c.JSON(200, readAllAliases())
}

func addAliasHandler(c *gin.Context) {
	var req DnsAliasEntry
	if err := c.BindJSON(&req); err != nil {
		return
	}
	target, ok := resolveAliasesTargetFile(req.File)
	if !ok {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	req.File = target
	if !validateAliasEntry(req) {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}

	conflicts := findAliasesByDomain(req.Domain, req.Type, req.File)
	if len(conflicts) > 0 {
		c.JSON(409, gin.H{"error": "alias_duplicate", "conflicts": conflicts})
		return
	}

	mu.Lock()
	defer mu.Unlock()

	createLocalBackup(req.File)
	if err := appendAliasLine(req.File, req); err != nil {
		c.JSON(500, gin.H{"error": "file_write_error"})
		return
	}

	writeAudit(AuditEntry{
		User:     getUser(c),
		Action:   "alias_add",
		Mac:      req.Type,
		Hostname: req.Domain,
		Ip:       req.Target,
		File:     req.File,
	})

	c.JSON(200, gin.H{"status": "ok"})
}

func bulkAddAliasesHandler(c *gin.Context) {
	var req BulkAliasReq
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}
	target, ok := resolveAliasesTargetFile(req.File)
	if !ok {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	req.File = target

	valid := []DnsAliasEntry{}
	for _, a := range req.Aliases {
		a.File = req.File
		if !validateAliasEntry(a) {
			continue
		}
		valid = append(valid, a)
	}
	if len(valid) == 0 {
		c.JSON(400, gin.H{"error": "no_valid_entries"})
		return
	}

	// Intra-batch duplicate check (same domain across rows).
	for i, a1 := range valid {
		for j, a2 := range valid {
			if i != j && strings.ToLower(a1.Domain) == strings.ToLower(a2.Domain) {
				c.JSON(409, gin.H{"error": "alias_duplicate_bulk", "domain": a1.Domain})
				return
			}
		}
	}

	// Cross-config duplicate check.
	for _, a := range valid {
		conflicts := findAliasesByDomain(a.Domain, "", "")
		if len(conflicts) > 0 {
			c.JSON(409, gin.H{"error": "alias_duplicate", "conflicts": conflicts})
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

	newDomains := make(map[string]bool)
	for _, a := range valid {
		newDomains[strings.ToLower(a.Type+":"+a.Domain)] = true
	}

	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if isAliasDirective(clean) {
			if entry, ok := parseAliasLine(clean, "", false); ok {
				if newDomains[strings.ToLower(entry.Type+":"+entry.Domain)] {
					continue
				}
			}
		}
		if strings.TrimSpace(line) != "" {
			newLines = append(newLines, line)
		}
	}

	for _, a := range valid {
		newLines = append(newLines, aliasToLine(a))
	}

	if err := os.WriteFile(req.File, []byte(strings.Join(newLines, "\n")+"\n"), 0644); err != nil {
		c.JSON(500, gin.H{"error": "write_error"})
		return
	}

	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "alias_bulk_add",
		File:   req.File,
		Mac:    fmt.Sprintf("%d aliases", len(valid)),
	})

	c.JSON(200, gin.H{"status": "ok", "count": len(valid)})
}

func deleteAliasHandler(c *gin.Context) {
	var req DeleteAliasReq
	if err := c.BindJSON(&req); err != nil {
		return
	}
	if req.Type != "A" && req.Type != "CNAME" {
		c.JSON(400, gin.H{"error": "bad_request"})
		return
	}
	if !isSafePath(req.File) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}

	mu.Lock()
	defer mu.Unlock()

	createLocalBackup(req.File)
	removed, err := removeAliasLine(req.File, req.Type, req.Domain)
	if err != nil {
		c.JSON(500, gin.H{"error": "file_write_error"})
		return
	}
	if !removed {
		c.JSON(404, gin.H{"error": "alias_not_found"})
		return
	}

	writeAudit(AuditEntry{
		User:     getUser(c),
		Action:   "alias_delete",
		Mac:      req.Type,
		Hostname: req.Domain,
		File:     req.File,
	})

	c.JSON(200, gin.H{"status": "deleted"})
}

func exportAliasesCSVHandler(c *gin.Context) {
	aliases := readAllAliases()
	// Strip the |has_bak marker before exporting.
	for i := range aliases {
		aliases[i].File = cleanAliasFile(aliases[i].File)
	}
	csvData := aliasesToCSV(aliases)
	c.Header("Content-Disposition", "attachment; filename=intermasq_aliases.csv")
	c.Data(200, "text/csv", csvData)
}

func importAliasesCSVHandler(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "no_file"})
		return
	}
	targetFile := c.PostForm("target_file")
	target, ok := resolveAliasesTargetFile(targetFile)
	if !ok {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	targetFile = target

	f, err := file.Open()
	if err != nil {
		c.JSON(500, gin.H{"error": "read_error"})
		return
	}
	defer f.Close()

	aliases, err := parseCSVAliases(f, targetFile)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid_csv"})
		return
	}
	if len(aliases) == 0 {
		c.JSON(400, gin.H{"error": "csv_empty"})
		return
	}

	for i, a1 := range aliases {
		for j, a2 := range aliases {
			if i != j && strings.ToLower(a1.Domain) == strings.ToLower(a2.Domain) {
				c.JSON(409, gin.H{"error": "alias_duplicate_bulk", "domain": a1.Domain})
				return
			}
		}
	}
	for _, a := range aliases {
		conflicts := findAliasesByDomain(a.Domain, "", "")
		if len(conflicts) > 0 {
			c.JSON(409, gin.H{"error": "alias_duplicate", "conflicts": conflicts})
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

	newKeys := make(map[string]bool)
	for _, a := range aliases {
		newKeys[strings.ToLower(a.Type+":"+a.Domain)] = true
	}

	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if isAliasDirective(clean) {
			if entry, ok := parseAliasLine(clean, "", false); ok {
				if newKeys[strings.ToLower(entry.Type+":"+entry.Domain)] {
					continue
				}
			}
		}
		if strings.TrimSpace(line) != "" {
			newLines = append(newLines, line)
		}
	}

	for _, a := range aliases {
		newLines = append(newLines, aliasToLine(a))
	}

	if err := os.WriteFile(targetFile, []byte(strings.Join(newLines, "\n")+"\n"), 0644); err != nil {
		c.JSON(500, gin.H{"error": "write_error"})
		return
	}

	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "alias_bulk_add",
		File:   targetFile,
		Mac:    fmt.Sprintf("%d aliases (csv)", len(aliases)),
	})

	c.JSON(200, gin.H{"status": "ok", "count": len(aliases)})
}

func getFileHandler(c *gin.Context) {
	name := c.Param("name")
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || filepath.Ext(name) != ".conf" {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	path := filepath.Join(*ConfigDir, name)
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

func createUserHandler(c *gin.Context) {
	var req AuthReq
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
	mu.Lock()
	defer mu.Unlock()
	if _, exists := users[req.Username]; exists {
		c.JSON(409, gin.H{"error": "user_exists"})
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	users[req.Username] = string(hash)
	saveUsers()
	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "user_create",
		Mac:    req.Username,
	})
	c.JSON(200, gin.H{"status": "ok"})
}

func deleteUserHandler(c *gin.Context) {
	name := c.Param("name")
	currentUser := getUser(c)
	if name == currentUser {
		c.JSON(400, gin.H{"error": "cannot_delete_self"})
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if _, exists := users[name]; !exists {
		c.JSON(404, gin.H{"error": "user_not_found"})
		return
	}
	delete(users, name)
	saveUsers()
	writeAudit(AuditEntry{
		User:   currentUser,
		Action: "user_delete",
		Mac:    name,
	})
	c.JSON(200, gin.H{"status": "deleted"})
}

func changePasswordHandler(c *gin.Context) {
	var req UserPasswordReq
	if err := c.BindJSON(&req); err != nil {
		return
	}
	if req.NewPassword == "" {
		c.JSON(400, gin.H{"error": "missing_fields"})
		return
	}
	currentUser := getUser(c)
	mu.Lock()
	defer mu.Unlock()
	hash, ok := users[currentUser]
	if !ok || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)) != nil {
		c.JSON(401, gin.H{"error": "invalid_credentials"})
		return
	}
	newHash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	users[currentUser] = string(newHash)
	saveUsers()
	writeAudit(AuditEntry{
		User:   currentUser,
		Action: "password_change",
	})
	c.JSON(200, gin.H{"status": "ok"})
}

func logoutHandler(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, _ := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) { return SecretKey, nil })
		if token != nil {
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if jti, ok := claims["jti"].(string); ok {
					if exp, ok := claims["exp"].(float64); ok {
						revokeToken(jti, time.Unix(int64(exp), 0))
					}
				}
			}
		}
	}
	c.JSON(200, gin.H{"status": "logged_out"})
}

func getNewDevicesHandler(c *gin.Context) {
	c.JSON(200, getNewDevices())
}

func bulkLeaseToStaticHandler(c *gin.Context) {
	var req BulkLeaseToStaticReq
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid_data"})
		return
	}
	if !isSafePath(req.File) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	if len(req.Leases) == 0 {
		c.JSON(400, gin.H{"error": "no_leases"})
		return
	}

	for _, l := range req.Leases {
		if !macRegex.MatchString(l.Mac) {
			c.JSON(400, gin.H{"error": "invalid_mac", "mac": l.Mac})
			return
		}
		macConflicts := findHostsByMac(l.Mac)
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
	for _, l := range req.Leases {
		if macRegex.MatchString(l.Mac) {
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
		if !macRegex.MatchString(l.Mac) {
			continue
		}
		hostname := l.Hostname
		if hostname == "*" || hostname == "" {
			hostname = "device-" + strings.ReplaceAll(strings.ToLower(l.Mac), ":", "")[:8]
		}
		newLines = append(newLines, fmt.Sprintf("dhcp-host=%s,%s,%s", l.Mac, hostname, l.Ip))
		count++
	}

	if err := os.WriteFile(req.File, []byte(strings.Join(newLines, "\n")+"\n"), 0644); err != nil {
		c.JSON(500, gin.H{"error": "write_error"})
		return
	}

	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "bulk_lease_to_static",
		File:   req.File,
		Mac:    fmt.Sprintf("%d leases", count),
	})

	c.JSON(200, gin.H{"status": "ok", "count": count})
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
	mu.Lock()
	defer mu.Unlock()
	if err := restoreBackupZip(zipData); err != nil {
		if strings.HasPrefix(err.Error(), "dnsmasq_test_failed") {
			c.JSON(400, gin.H{"error": "dnsmasq_test_failed", "detail": strings.TrimPrefix(err.Error(), "dnsmasq_test_failed: ")})
		} else {
			c.JSON(400, gin.H{"error": err.Error()})
		}
		return
	}
	writeAudit(AuditEntry{
		User:   getUser(c),
		Action: "backup_restore",
	})
	c.JSON(200, gin.H{"status": "ok"})
}

func getUsersHandler(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()
	names := make([]string, 0, len(users))
	for u := range users {
		names = append(names, u)
	}
	c.JSON(200, gin.H{"users": names})
}

func eventsHandler(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	client := &sseClient{ch: make(chan string, 10)}
	sseRegister(client)
	defer sseUnregister(client)

	arp := getArpTable()
	c.SSEvent("arp", arp)
	c.Writer.Flush()

	for {
		select {
		case msg := <-client.ch:
			c.Writer.Write([]byte(msg))
			c.Writer.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}
