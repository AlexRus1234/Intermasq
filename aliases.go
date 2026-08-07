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

// aliases.go — DNS alias file-level mutation (append/remove/ensure). The
// pure parsers and formatters (ParseAliasLine, AliasToLine, ReadAllAliases,
// FindAliasesByDomain, CleanAliasFile, AliasesToCSV, ParseCSVAliases,
// IsAliasDirective) moved to internal/dnsmasq in stage 4 of the
// modularization; the file-mutating half stays here because it depends on
// isSafePath / writeConfigWithTest / history, which the host binary still
// owns. Stage 5 will move these too.

package main

import (
	"os"
	"strings"

	"intermask/internal/dnsmasq"
	"intermask/internal/models"
)

// appendAliasLine appends a single alias directive to the file, preserving
// existing content. Does NOT validate; caller must do that.
func appendAliasLine(filePath string, entry models.DnsAliasEntry) error {
	content, _ := os.ReadFile(filePath)
	line := dnsmasq.AliasToLine(entry)
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
		if !removed && dnsmasq.IsAliasDirective(clean) {
			if entry, ok := dnsmasq.ParseAliasLine(clean, "", false); ok && entry.Type == aliasType && strings.ToLower(entry.Domain) == domainLower {
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
