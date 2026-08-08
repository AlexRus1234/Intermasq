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

package dnsmasq

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"intermask/internal/bins"
	"intermask/internal/models"
	"intermask/internal/stats"
)

// DefaultAliasesFileName is the file created on first alias add when no
// explicit target file is provided. Relative to ConfigDir.
const DefaultAliasesFileName = "10-dns-aliases.conf"

// Mu serialises every config-write critical section across the process.
// Handlers in package main take it (as dnsmasq.Mu) around read+write
// sequences so concurrent HTTP requests cannot interleave reads, decisions
// and writes of the same .conf file. Kept as an exported value (not a
// pointer) so callers use dnsmasq.Mu.Lock() / Unlock() verbatim.
var Mu sync.RWMutex

// IsSafePath reports whether path is the configured ConfigDir itself or a
// file inside it. Used as the single chokepoint for path-traversal defence
// across all subsystems that accept user-supplied paths.
func IsSafePath(path string) bool {
	cleanPath := filepath.Clean(path)
	cleanDir := filepath.Clean(*ConfigDir)
	return strings.HasPrefix(cleanPath, cleanDir+string(os.PathSeparator)) || cleanPath == cleanDir
}

// ReadFileRaw reads the contents of a .conf file. Refuses paths outside
// ConfigDir so the editor cannot be used to read arbitrary system files.
func ReadFileRaw(path string) ([]byte, error) {
	if !IsSafePath(path) {
		return nil, os.ErrPermission
	}
	return os.ReadFile(path)
}

// WriteFileRaw writes content to a .conf file after taking a backup, then
// runs `dnsmasq --test`. On test failure the file is rolled back from .bak
// so dnsmasq never sees an invalid config and the next reload succeeds.
func WriteFileRaw(path string, content []byte) error {
	if !IsSafePath(path) {
		return os.ErrPermission
	}
	CreateLocalBackup(path)
	if err := os.WriteFile(path, content, 0644); err != nil {
		return err
	}
	testCmd := exec.Command(bins.Dnsmasq(), "--test", "--conf-file="+path)
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		stats.Counters.TestFailures.Add(1)
		if restoreErr := restoreLocalBackup(path); restoreErr != nil {
			return fmt.Errorf("dnsmasq_test_failed: %s; rollback failed: %w", testOut, restoreErr)
		}
		return fmt.Errorf("dnsmasq_test_failed: %s", testOut)
	}
	return nil
}

// WriteConfigWithTest is the same shape as WriteFileRaw but is used by the
// visual config editor (handlers_config.go in main) which already has the
// freshly serialised content in hand. Kept separate for clarity at call
// sites.
func WriteConfigWithTest(path string, content []byte) error {
	if !IsSafePath(path) {
		return os.ErrPermission
	}
	CreateLocalBackup(path)
	if err := os.WriteFile(path, content, 0644); err != nil {
		return err
	}
	testCmd := exec.Command(bins.Dnsmasq(), "--test", "--conf-file="+path)
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		stats.Counters.TestFailures.Add(1)
		if restoreErr := restoreLocalBackup(path); restoreErr != nil {
			return fmt.Errorf("dnsmasq_test_failed: %s; rollback failed: %w", testOut, restoreErr)
		}
		return fmt.Errorf("dnsmasq_test_failed: %s", testOut)
	}
	return nil
}

// restoreLocalBackup restores the pre-write content without re-running
// dnsmasq --test. The failed validation already established that the new
// content is invalid; validating the backup with the same failing command
// would prevent the original content from being restored.
func restoreLocalBackup(path string) error {
	content, err := os.ReadFile(path + ".bak")
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}

// RemoveHostLine deletes every dhcp-host= line in filePath whose body
// mentions mac (case-insensitive substring match, mirroring the historical
// behaviour) and rewrites the file. Empty lines are dropped on the way.
func RemoveHostLine(filePath, mac string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	newLines := []string{}
	macLower := strings.ToLower(mac)
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if strings.HasPrefix(clean, "dhcp-host=") && strings.Contains(strings.ToLower(clean), macLower) {
			continue
		}
		if clean != "" {
			newLines = append(newLines, line)
		}
	}
	return os.WriteFile(filePath, []byte(strings.Join(newLines, "\n")+"\n"), 0644)
}

// AppendHostLine appends a single dhcp-host= line to filePath, preserving
// existing content. The line is built via FormatDhcpHostLine.
func AppendHostLine(filePath string, h models.HostEntry) error {
	content, _ := os.ReadFile(filePath)
	line := FormatDhcpHostLine(h)
	out := strings.TrimRight(string(content), "\n")
	if out != "" {
		out += "\n"
	}
	out += line + "\n"
	return os.WriteFile(filePath, []byte(out), 0644)
}

// AppendAliasLine appends a single alias directive to the file, preserving
// existing content. Does NOT validate; caller must do that.
func AppendAliasLine(filePath string, entry models.DnsAliasEntry) error {
	content, _ := os.ReadFile(filePath)
	line := AliasToLine(entry)
	out := strings.TrimRight(string(content), "\n")
	if out != "" {
		out += "\n"
	}
	out += line + "\n"
	return os.WriteFile(filePath, []byte(out), 0644)
}

// RemoveAliasLine removes the first alias directive matching the given
// type+domain from the file. Returns true if a line was removed.
func RemoveAliasLine(filePath, aliasType, domain string) (bool, error) {
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
		if !removed && IsAliasDirective(clean) {
			if entry, ok := ParseAliasLine(clean, "", false); ok && entry.Type == aliasType && strings.ToLower(entry.Domain) == domainLower {
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

// EnsureAliasesFile creates the default aliases file if it does not exist,
// with a small header comment. Used as a fallback when req.File is empty.
func EnsureAliasesFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if !IsSafePath(path) {
		return os.ErrPermission
	}
	header := "# DNS aliases managed by Intermasq\n# Format: address=/domain/IP  or  cname=alias,target\n"
	return os.WriteFile(path, []byte(header), 0644)
}
