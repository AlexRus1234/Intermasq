package main

// HTTP handlers for the DNS alias subsystem (address= / cname= /
// ptr-record= / txt-record=). CRUD, bulk add, CSV import/export, plus the
// shared helpers (target-file resolution, validation) used by every alias
// endpoint. Backend logic lives in aliases.go.

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"intermask/internal/dnsmasq"
	"intermask/internal/validate"
)

// resolveAliasesTargetFile returns the absolute path to the aliases target
// file. If reqFile is empty, the default file (10-dns-aliases.conf under
// ConfigDir) is created on demand and returned. Returns ok=false if the
// supplied path is unsafe.
func resolveAliasesTargetFile(reqFile string) (string, bool) {
	if reqFile == "" {
		path := filepath.Join(*dnsmasq.ConfigDir, dnsmasq.DefaultAliasesFileName)
		if err := dnsmasq.EnsureAliasesFile(path); err != nil {
			return "", false
		}
		return path, true
	}
	if !dnsmasq.IsSafePath(reqFile) {
		return "", false
	}
	return reqFile, true
}

// validateAliasEntry enforces the per-type rules for A/CNAME/PTR/TXT records.
// Used on every add/bulk/CSV path so the rules cannot drift between them.
func validateAliasEntry(a DnsAliasEntry) bool {
	if a.Type != "A" && a.Type != "CNAME" && a.Type != "PTR" && a.Type != "TXT" {
		return false
	}
	if !validate.ValidAliasDomain(a.Domain) {
		return false
	}
	switch a.Type {
	case "A":
		return net.ParseIP(a.Target) != nil
	case "CNAME", "PTR":
		return validate.ValidAliasDomain(a.Target)
	case "TXT":
		return a.Target != "" && !strings.Contains(a.Target, "\n")
	}
	return false
}

func getAliasesHandler(c *gin.Context) {
	c.JSON(200, dnsmasq.ReadAllAliases())
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

	conflicts := dnsmasq.FindAliasesByDomain(req.Domain, "", "")
	if len(conflicts) > 0 {
		c.JSON(409, gin.H{"error": "alias_duplicate", "conflicts": conflicts})
		return
	}

	dnsmasq.Mu.Lock()
	defer dnsmasq.Mu.Unlock()

	dnsmasq.CreateLocalBackup(req.File)
	if err := dnsmasq.AppendAliasLine(req.File, req); err != nil {
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

	for i, a1 := range valid {
		for j, a2 := range valid {
			if i != j && strings.ToLower(a1.Domain) == strings.ToLower(a2.Domain) {
				c.JSON(409, gin.H{"error": "alias_duplicate_bulk", "domain": a1.Domain})
				return
			}
		}
	}

	for _, a := range valid {
		conflicts := dnsmasq.FindAliasesByDomain(a.Domain, "", "")
		if len(conflicts) > 0 {
			c.JSON(409, gin.H{"error": "alias_duplicate", "conflicts": conflicts})
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

	newDomains := make(map[string]bool)
	for _, a := range valid {
		newDomains[strings.ToLower(a.Type+":"+a.Domain)] = true
	}

	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if dnsmasq.IsAliasDirective(clean) {
			if entry, ok := dnsmasq.ParseAliasLine(clean, "", false); ok {
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
		newLines = append(newLines, dnsmasq.AliasToLine(a))
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
	if !dnsmasq.IsSafePath(req.File) {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}

	dnsmasq.Mu.Lock()
	defer dnsmasq.Mu.Unlock()

	dnsmasq.CreateLocalBackup(req.File)
	removed, err := dnsmasq.RemoveAliasLine(req.File, req.Type, req.Domain)
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
	aliases := dnsmasq.ReadAllAliases()
	for i := range aliases {
		aliases[i].File = dnsmasq.CleanAliasFile(aliases[i].File)
	}
	csvData := dnsmasq.AliasesToCSV(aliases)
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

	aliases, err := dnsmasq.ParseCSVAliases(f, targetFile)
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
		conflicts := dnsmasq.FindAliasesByDomain(a.Domain, "", "")
		if len(conflicts) > 0 {
			c.JSON(409, gin.H{"error": "alias_duplicate", "conflicts": conflicts})
			return
		}
	}

	dnsmasq.Mu.Lock()
	defer dnsmasq.Mu.Unlock()

	dnsmasq.CreateLocalBackup(targetFile)

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
		if dnsmasq.IsAliasDirective(clean) {
			if entry, ok := dnsmasq.ParseAliasLine(clean, "", false); ok {
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
		newLines = append(newLines, dnsmasq.AliasToLine(a))
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
