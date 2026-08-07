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

// dnsmasq.go — low-level file mutation for the dnsmasq config subtree.
// The pure parsers (ParseDhcpHostLine, ReadAllHosts, …) and the visual
// config-editor snapshot (ReadConfigSnapshot, ParseDhcpRange, …) moved to
// internal/dnsmasq in stage 4 of the modularization. What remains here is
// the unsafe-path gated raw read/write layer and the dhcp-host line
// append/remove helpers that the host-binary handlers depend on.
//
// Other concerns live in sibling files:
//
//   - aliases.go          DNS alias file-mutation (append/remove/ensure)
//   - arp_leases.go       ARP table, leases, "new devices" discovery
//   - history.go          versioned history + .bak rollback
//   - backup.go           ZIP backup/restore, file deletion
//   - sse.go              SSE broker + dnsmasq status/reload

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"intermask/internal/bins"
	"intermask/internal/dnsmasq"
	"intermask/internal/models"
	"intermask/internal/stats"
)

// isSafePath reports whether path is the configured ConfigDir itself or a
// file inside it. Used as the single chokepoint for path-traversal defence
// across all subsystems that accept user-supplied paths.
func isSafePath(path string) bool {
	cleanPath := filepath.Clean(path)
	cleanDir := filepath.Clean(*dnsmasq.ConfigDir)
	return strings.HasPrefix(cleanPath, cleanDir+string(os.PathSeparator)) || cleanPath == cleanDir
}

// readFileRaw reads the contents of a .conf file. Refuses paths outside
// ConfigDir so the editor cannot be used to read arbitrary system files.
func readFileRaw(path string) ([]byte, error) {
	if !isSafePath(path) {
		return nil, os.ErrPermission
	}
	return os.ReadFile(path)
}

// writeFileRaw writes content to a .conf file after taking a backup, then
// runs `dnsmasq --test`. On test failure the file is rolled back from .bak
// so dnsmasq never sees an invalid config and the next reload succeeds.
func writeFileRaw(path string, content []byte) error {
	if !isSafePath(path) {
		return os.ErrPermission
	}
	createLocalBackup(path)
	if err := os.WriteFile(path, content, 0644); err != nil {
		return err
	}
	testCmd := exec.Command(bins.Dnsmasq(), "--test", "--conf-file="+path)
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		stats.Counters.TestFailures.Add(1)
		_ = rollbackFile(path)
		return fmt.Errorf("dnsmasq_test_failed: %s", testOut)
	}
	return nil
}

// writeConfigWithTest is the same shape as writeFileRaw but is used by the
// visual config editor (handlers_config.go) which already has the freshly
// serialised content in hand. Kept separate for clarity at call sites.
func writeConfigWithTest(path string, content []byte) error {
	if !isSafePath(path) {
		return os.ErrPermission
	}
	createLocalBackup(path)
	if err := os.WriteFile(path, content, 0644); err != nil {
		return err
	}
	testCmd := exec.Command(bins.Dnsmasq(), "--test", "--conf-file="+path)
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		stats.Counters.TestFailures.Add(1)
		_ = rollbackFile(path)
		return fmt.Errorf("dnsmasq_test_failed: %s", testOut)
	}
	return nil
}

func removeHostLine(filePath, mac string) error {
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

func appendHostLine(filePath string, h models.HostEntry) error {
	content, _ := os.ReadFile(filePath)
	line := dnsmasq.FormatDhcpHostLine(h)
	out := strings.TrimRight(string(content), "\n")
	if out != "" {
		out += "\n"
	}
	out += line + "\n"
	return os.WriteFile(filePath, []byte(out), 0644)
}
