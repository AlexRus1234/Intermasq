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
