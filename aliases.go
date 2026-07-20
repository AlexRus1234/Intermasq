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

// aliases.go — DNS alias directives (address= / cname= / ptr-record= /
// txt-record=). Parsing, formatting, file-level operations (append/remove),
// duplicate detection, CSV import/export. Lives separately from
// dnsmasq.go because the alias subsystem is independent from dhcp-host=
// handling and has its own validation/serialisation rules.

package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// isAliasDirective returns true if the given trimmed line is a managed
// DNS alias directive (address= / cname= / ptr-record= / txt-record=).
func isAliasDirective(line string) bool {
	return strings.HasPrefix(line, "address=") ||
		strings.HasPrefix(line, "cname=") ||
		strings.HasPrefix(line, "ptr-record=") ||
		strings.HasPrefix(line, "txt-record=")
}

// parseAliasLine parses a single "address=", "cname=", "ptr-record=" or
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
func parseAliasLine(line, file string, hasBak bool) (DnsAliasEntry, bool) {
	entry := DnsAliasEntry{File: file}
	if hasBak {
		entry.File = file + "|has_bak"
	}
	if strings.HasPrefix(line, "address=") {
		val := strings.TrimPrefix(line, "address=")
		if !strings.HasPrefix(val, "/") {
			return DnsAliasEntry{}, false
		}
		rest := strings.TrimPrefix(val, "/")
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return DnsAliasEntry{}, false
		}
		entry.Type = "A"
		entry.Domain = rest[:slash]
		entry.Target = strings.TrimSpace(rest[slash+1:])
		if entry.Domain == "" || entry.Target == "" {
			return DnsAliasEntry{}, false
		}
		if entry.Domain == "#" || strings.HasPrefix(entry.Domain, "*") {
			return DnsAliasEntry{}, false
		}
		return entry, true
	}
	if strings.HasPrefix(line, "cname=") {
		val := strings.TrimPrefix(line, "cname=")
		parts := strings.Split(val, ",")
		if len(parts) < 2 {
			return DnsAliasEntry{}, false
		}
		entry.Type = "CNAME"
		entry.Domain = strings.TrimSpace(parts[0])
		entry.Target = strings.TrimSpace(parts[1])
		if entry.Domain == "" || entry.Target == "" {
			return DnsAliasEntry{}, false
		}
		return entry, true
	}
	if strings.HasPrefix(line, "ptr-record=") {
		val := strings.TrimPrefix(line, "ptr-record=")
		parts := strings.Split(val, ",")
		if len(parts) < 2 {
			return DnsAliasEntry{}, false
		}
		entry.Type = "PTR"
		entry.Domain = strings.TrimSpace(parts[0])
		entry.Target = strings.TrimSpace(parts[len(parts)-1])
		if entry.Domain == "" || entry.Target == "" {
			return DnsAliasEntry{}, false
		}
		return entry, true
	}
	if strings.HasPrefix(line, "txt-record=") {
		val := strings.TrimPrefix(line, "txt-record=")
		idx := strings.Index(val, ",")
		if idx < 0 {
			return DnsAliasEntry{}, false
		}
		entry.Type = "TXT"
		entry.Domain = strings.TrimSpace(val[:idx])
		entry.Target = strings.TrimSpace(val[idx+1:])
		if entry.Domain == "" || entry.Target == "" {
			return DnsAliasEntry{}, false
		}
		return entry, true
	}
	return DnsAliasEntry{}, false
}

func aliasToLine(a DnsAliasEntry) string {
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

// readAllAliases scans all .conf files in ConfigDir and returns every
// address=/cname=/ptr-record=/txt-record= directive as a structured entry.
func readAllAliases() []DnsAliasEntry {
	aliases := []DnsAliasEntry{}
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
			if !isAliasDirective(line) {
				continue
			}
			if entry, ok := parseAliasLine(line, fullPath, hasBak); ok {
				aliases = append(aliases, entry)
			}
		}
	}
	return aliases
}

// findAliasesByDomain returns aliases whose Domain matches (case-insensitive)
// the given domain, excluding one with the provided file+type combination.
func findAliasesByDomain(domain string, excludeType, excludeFile string) []DnsAliasEntry {
	result := []DnsAliasEntry{}
	domainLower := strings.ToLower(domain)
	for _, a := range readAllAliases() {
		if strings.ToLower(a.Domain) != domainLower {
			continue
		}
		if a.Type == excludeType && cleanAliasFile(a.File) == excludeFile {
			continue
		}
		result = append(result, a)
	}
	return result
}

// cleanAliasFile strips the "|has_bak" marker appended by readAllAliases.
func cleanAliasFile(f string) string {
	if i := strings.Index(f, "|"); i >= 0 {
		return f[:i]
	}
	return f
}

// appendAliasLine appends a single alias directive to the file, preserving
// existing content. Does NOT validate; caller must do that.
func appendAliasLine(filePath string, entry DnsAliasEntry) error {
	content, _ := os.ReadFile(filePath)
	line := aliasToLine(entry)
	out := strings.TrimRight(string(content), "\n")
	if out != "" {
		out += "\n"
	}
	out += line + "\n"
	return os.WriteFile(filePath, []byte(out), 0644)
}

// removeAliasLine removes the first alias directive matching the given
// type+domain from the file. Returns true if a line was removed.
func removeAliasLine(filePath, aliasType, domain string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(content), "\n")
	newLines := []string{}
	removed := false
	domainLower := strings.ToLower(domain)
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if !removed && isAliasDirective(clean) {
			if entry, ok := parseAliasLine(clean, "", false); ok && entry.Type == aliasType && strings.ToLower(entry.Domain) == domainLower {
				removed = true
				continue
			}
		}
		if clean != "" {
			newLines = append(newLines, line)
		}
	}
	if !removed {
		return false, nil
	}
	return true, os.WriteFile(filePath, []byte(strings.Join(newLines, "\n")+"\n"), 0644)
}

func aliasesToCSV(aliases []DnsAliasEntry) []byte {
	buf := new(bytes.Buffer)
	w := csv.NewWriter(buf)
	w.Write([]string{"type", "domain", "target"})
	for _, a := range aliases {
		w.Write([]string{a.Type, a.Domain, a.Target})
	}
	w.Flush()
	return buf.Bytes()
}

func parseCSVAliases(r io.Reader, targetFile string) ([]DnsAliasEntry, error) {
	reader := csv.NewReader(r)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	aliases := []DnsAliasEntry{}
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
		if !aliasDomainRegex.MatchString(domain) {
			continue
		}
		switch t {
		case "A":
			if net.ParseIP(target) == nil {
				continue
			}
		case "CNAME", "PTR":
			if !aliasDomainRegex.MatchString(target) {
				continue
			}
		case "TXT":
			if target == "" || strings.Contains(target, "\n") {
				continue
			}
		}
		aliases = append(aliases, DnsAliasEntry{Type: t, Domain: domain, Target: target, File: targetFile})
	}
	return aliases, nil
}

// ensureAliasesFile creates the default aliases file if it does not exist,
// with a small header comment. Used as a fallback when req.File is empty.
func ensureAliasesFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if !isSafePath(path) {
		return os.ErrPermission
	}
	header := "# DNS aliases managed by Intermasq\n# Format: address=/domain/IP  or  cname=alias,target\n"
	return os.WriteFile(path, []byte(header), 0644)
}
