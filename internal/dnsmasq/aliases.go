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

// aliases.go — pure (side-effect-free) DNS alias helpers: parsers,
// formatters, file scanning, duplicate detection, CSV import/export.
// The file-level manipulation (append/remove, EnsureAliasesFile, IsSafePath
// gating) lives in write.go in this package after stage 5.

package dnsmasq

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"intermask/internal/models"
	"intermask/internal/validate"
)

// IsAliasDirective returns true if the given trimmed line is a managed
// DNS alias directive (address= / cname= / ptr-record= / txt-record=).
func IsAliasDirective(line string) bool {
	return strings.HasPrefix(line, "address=") ||
		strings.HasPrefix(line, "cname=") ||
		strings.HasPrefix(line, "ptr-record=") ||
		strings.HasPrefix(line, "txt-record=")
}

// ParseAliasLine parses a single "address=", "cname=", "ptr-record=" or
// "txt-record=" line into a DnsAliasEntry. Returns ok=false if the line is
// malformed or unsupported (e.g. address=/#/IP wildcard is out of scope).
//
// Supported forms:
//
//	address=/domain/IP              → A
//	cname=alias,target              → CNAME
//	cname=alias,target,tag:…        → CNAME (extra fields ignored)
//	ptr-record=name,target          → PTR
//	txt-record=name,value           → TXT
//	txt-record=name,"multi word"    → TXT (quotes preserved as-is)
//
// When hasBak is true, the returned entry's File field is suffixed with
// "|has_bak" so callers can pair the in-memory entry with the corresponding
// .bak marker; CleanAliasFile strips the suffix to get back the raw path.
func ParseAliasLine(line, file string, hasBak bool) (models.DnsAliasEntry, bool) {
	entry := models.DnsAliasEntry{File: file}
	if hasBak {
		entry.File = file + "|has_bak"
	}
	if strings.HasPrefix(line, "address=") {
		val := strings.TrimPrefix(line, "address=")
		if !strings.HasPrefix(val, "/") {
			return models.DnsAliasEntry{}, false
		}
		rest := strings.TrimPrefix(val, "/")
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return models.DnsAliasEntry{}, false
		}
		entry.Type = "A"
		entry.Domain = rest[:slash]
		entry.Target = strings.TrimSpace(rest[slash+1:])
		if entry.Domain == "" || entry.Target == "" {
			return models.DnsAliasEntry{}, false
		}
		if entry.Domain == "#" || strings.HasPrefix(entry.Domain, "*") {
			return models.DnsAliasEntry{}, false
		}
		return entry, true
	}
	if strings.HasPrefix(line, "cname=") {
		val := strings.TrimPrefix(line, "cname=")
		parts := strings.Split(val, ",")
		if len(parts) < 2 {
			return models.DnsAliasEntry{}, false
		}
		entry.Type = "CNAME"
		entry.Domain = strings.TrimSpace(parts[0])
		entry.Target = strings.TrimSpace(parts[1])
		if entry.Domain == "" || entry.Target == "" {
			return models.DnsAliasEntry{}, false
		}
		return entry, true
	}
	if strings.HasPrefix(line, "ptr-record=") {
		val := strings.TrimPrefix(line, "ptr-record=")
		parts := strings.Split(val, ",")
		if len(parts) < 2 {
			return models.DnsAliasEntry{}, false
		}
		entry.Type = "PTR"
		entry.Domain = strings.TrimSpace(parts[0])
		entry.Target = strings.TrimSpace(parts[len(parts)-1])
		if entry.Domain == "" || entry.Target == "" {
			return models.DnsAliasEntry{}, false
		}
		return entry, true
	}
	if strings.HasPrefix(line, "txt-record=") {
		val := strings.TrimPrefix(line, "txt-record=")
		idx := strings.Index(val, ",")
		if idx < 0 {
			return models.DnsAliasEntry{}, false
		}
		entry.Type = "TXT"
		entry.Domain = strings.TrimSpace(val[:idx])
		entry.Target = strings.TrimSpace(val[idx+1:])
		if entry.Domain == "" || entry.Target == "" {
			return models.DnsAliasEntry{}, false
		}
		return entry, true
	}
	return models.DnsAliasEntry{}, false
}

// AliasToLine renders a DnsAliasEntry back into its dnsmasq textual form.
func AliasToLine(a models.DnsAliasEntry) string {
	switch a.Type {
	case "CNAME":
		return fmt.Sprintf("cname=%s,%s", a.Domain, a.Target)
	case "PTR":
		return fmt.Sprintf("ptr-record=%s,%s", a.Domain, a.Target)
	case "TXT":
		return fmt.Sprintf("txt-record=%s,%s", a.Domain, a.Target)
	}
	return fmt.Sprintf("address=/%s/%s", a.Domain, a.Target)
}

// ReadAllAliases scans all .conf files in ConfigDir and returns every
// address=/cname=/ptr-record=/txt-record= directive as a structured entry.
func ReadAllAliases() []models.DnsAliasEntry {
	Mu.RLock()
	defer Mu.RUnlock()
	return ReadAllAliasesLocked()
}

// ReadAllAliasesLocked is ReadAllAliases for callers holding Mu.Lock.
func ReadAllAliasesLocked() []models.DnsAliasEntry {
	aliases := []models.DnsAliasEntry{}
	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return aliases
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
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		for _, raw := range strings.Split(string(content), "\n") {
			line := strings.TrimSpace(raw)
			if !IsAliasDirective(line) {
				continue
			}
			if entry, ok := ParseAliasLine(line, fullPath, hasBak); ok {
				aliases = append(aliases, entry)
			}
		}
	}
	return aliases
}

// FindAliasesByDomain returns aliases whose Domain matches (case-insensitive)
// the given domain, excluding one with the provided file+type combination.
func FindAliasesByDomain(domain string, excludeType, excludeFile string) []models.DnsAliasEntry {
	Mu.RLock()
	defer Mu.RUnlock()
	return FindAliasesByDomainLocked(domain, excludeType, excludeFile)
}

// FindAliasesByDomainLocked is FindAliasesByDomain for callers holding Mu.Lock.
func FindAliasesByDomainLocked(domain string, excludeType, excludeFile string) []models.DnsAliasEntry {
	result := []models.DnsAliasEntry{}
	domainLower := strings.ToLower(domain)
	for _, a := range ReadAllAliasesLocked() {
		if strings.ToLower(a.Domain) != domainLower {
			continue
		}
		if a.Type == excludeType && CleanAliasFile(a.File) == excludeFile {
			continue
		}
		result = append(result, a)
	}
	return result
}

// CleanAliasFile strips the "|has_bak" marker appended by ReadAllAliases.
// Exported because the main-package aliases-handlers (handlers_aliases.go)
// also call it to canonicalise the alias File field before reporting
// conflicts back to the UI; the migration pulls it out of the main package
// so the function stays next to its only producer.
func CleanAliasFile(f string) string {
	if i := strings.Index(f, "|"); i >= 0 {
		return f[:i]
	}
	return f
}

// AliasesToCSV serialises a slice of DnsAliasEntry into a CSV with the
// header type,domain,target.
func AliasesToCSV(aliases []models.DnsAliasEntry) []byte {
	buf := new(bytes.Buffer)
	w := csv.NewWriter(buf)
	w.Write([]string{"type", "domain", "target"})
	for _, a := range aliases {
		w.Write([]string{a.Type, a.Domain, a.Target})
	}
	w.Flush()
	return buf.Bytes()
}

// ParseCSVAliases reads a CSV (with optional header row starting with "type")
// and returns every row that passes the per-type validation rules. Rows with
// unsupported types or invalid domains/targets are silently dropped, matching
// the historical import semantics.
func ParseCSVAliases(r io.Reader, targetFile string) ([]models.DnsAliasEntry, error) {
	reader := csv.NewReader(r)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	aliases := []models.DnsAliasEntry{}
	for i, row := range records {
		if i == 0 {
			if len(row) >= 1 && strings.EqualFold(row[0], "type") {
				continue
			}
		}
		if len(row) < 3 {
			continue
		}
		t := strings.ToUpper(strings.TrimSpace(row[0]))
		domain := strings.TrimSpace(row[1])
		target := strings.TrimSpace(row[2])
		if t != "A" && t != "CNAME" && t != "PTR" && t != "TXT" {
			continue
		}
		if !validate.ValidAliasDomain(domain) {
			continue
		}
		switch t {
		case "A":
			if net.ParseIP(target) == nil {
				continue
			}
		case "CNAME", "PTR":
			if !validate.ValidAliasDomain(target) {
				continue
			}
		case "TXT":
			if target == "" || strings.Contains(target, "\n") {
				continue
			}
		}
		aliases = append(aliases, models.DnsAliasEntry{Type: t, Domain: domain, Target: target, File: targetFile})
	}
	return aliases, nil
}
