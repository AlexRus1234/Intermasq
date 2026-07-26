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

// dnsmasq.go — core dhcp-host= subsystem: parsing, formatting, validation,
// host lookup, file-level manipulation, IP/CIDR helpers, IP-transforms for
// bulk-edit, CSV import/export, and low-level raw read/write helpers used
// by the file-editor endpoints. Other concerns live in sibling files:
//
//   - aliases.go          DNS alias directives (address=/cname=/…)
//   - config_snapshot.go  visual editor for all other dnsmasq directives
//   - arp_leases.go       ARP table, leases, "new devices" discovery
//   - history.go          versioned history + .bak rollback
//   - backup.go           ZIP backup/restore, file deletion
//   - sse.go              SSE broker + dnsmasq status/reload

package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// octetPrefixRegex matches 1-3 dot-separated octets (e.g. "10", "10.0",
// "10.0.0") used as an IP-prefix transform in bulk-edit.
var octetPrefixRegex = regexp.MustCompile(`^(\d{1,3}\.){0,2}\d{1,3}$`)

// isSafePath reports whether path is the configured ConfigDir itself or a
// file inside it. Used as the single chokepoint for path-traversal defence
// across all subsystems that accept user-supplied paths.
func isSafePath(path string) bool {
	cleanPath := filepath.Clean(path)
	cleanDir := filepath.Clean(*ConfigDir)
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
	testCmd := exec.Command(dnsmasqBin(), "--test", "--conf-file="+path)
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		counters.TestFailures.Add(1)
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
	testCmd := exec.Command(dnsmasqBin(), "--test", "--conf-file="+path)
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		counters.TestFailures.Add(1)
		_ = rollbackFile(path)
		return fmt.Errorf("dnsmasq_test_failed: %s", testOut)
	}
	return nil
}

// parseDhcpHostLine parses a single "dhcp-host=..." line into a HostEntry.
// This is the single canonical parser used everywhere in the codebase;
// previously the same logic was duplicated in 5 places. It recognises:
//   - MAC (any token matching macRegex)
//   - IPv4/IPv6 (any token parseable by net.ParseIP)
//   - tag qualifiers "set:<name>" / "tag:<name>"  -> collected into Tags
//   - everything else                             -> Hostname
//
// "id:<client-id>" tokens are stored as Tags verbatim so they round-trip
// without loss, but the UI does not surface them.
func parseDhcpHostLine(raw, file string) (HostEntry, bool) {
	line := strings.TrimSpace(raw)
	if !strings.HasPrefix(line, "dhcp-host=") && !strings.HasPrefix(line, "dhcp-host:") {
		return HostEntry{}, false
	}
	line = strings.TrimPrefix(line, "dhcp-host=")
	line = strings.TrimPrefix(line, "dhcp-host:")
	parts := strings.Split(line, ",")
	entry := HostEntry{File: file}
	var tags []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		switch {
		case macRegex.MatchString(p):
			entry.Mac = p
		case net.ParseIP(p) != nil:
			entry.Ip = p
		case strings.HasPrefix(p, "set:"), strings.HasPrefix(p, "tag:"), strings.HasPrefix(p, "id:"):
			tags = append(tags, p)
		default:
			entry.Hostname = p
		}
	}
	if entry.Mac == "" {
		return HostEntry{}, false
	}
	entry.Tags = tags
	return entry, true
}

// formatDhcpHostLine renders a HostEntry back into the dnsmasq textual form.
// Order is fixed: mac[, hostname][, ip][, tags...]. Tags come last because
// that is the convention dnsmasq examples use and it keeps the human-readable
// part of the line at the front.
func formatDhcpHostLine(h HostEntry) string {
	parts := make([]string, 0, 4+len(h.Tags))
	parts = append(parts, h.Mac)
	if h.Hostname != "" {
		parts = append(parts, h.Hostname)
	}
	if h.Ip != "" {
		parts = append(parts, h.Ip)
	}
	parts = append(parts, h.Tags...)
	return "dhcp-host=" + strings.Join(parts, ",")
}

func readHostByMac(filePath, mac string) *HostEntry {
	macLower := strings.ToLower(mac)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	for _, raw := range strings.Split(string(content), "\n") {
		entry, ok := parseDhcpHostLine(raw, filePath)
		if !ok {
			continue
		}
		if strings.ToLower(entry.Mac) == macLower {
			return &entry
		}
	}
	return nil
}

type ipTransformMode int

const (
	ipTransformNone ipTransformMode = iota
	ipTransformOctets
	ipTransformCIDR
)

type ipTransform struct {
	mode    ipTransformMode
	oldNet  *net.IPNet
	newNet  *net.IPNet
	oldPref string
	newPref string
}

// parseIPTransform turns the user-supplied old/new prefix pair (either as
// 3-octet strings like "10.0.0" or as CIDRs like "10.0.0.0/24") into a
// transform that can be applied to a list of host IPs in bulk-edit.
func parseIPTransform(oldStr, newStr string) (*ipTransform, error) {
	if oldStr == "" && newStr == "" {
		return &ipTransform{mode: ipTransformNone}, nil
	}
	if oldStr == "" || newStr == "" {
		return nil, fmt.Errorf("both_prefixes_required")
	}

	if strings.Contains(oldStr, "/") || strings.Contains(newStr, "/") {
		if !strings.Contains(oldStr, "/") || !strings.Contains(newStr, "/") {
			return nil, fmt.Errorf("prefix_format_mismatch")
		}
		_, oldNet, err1 := net.ParseCIDR(oldStr)
		_, newNet, err2 := net.ParseCIDR(newStr)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("invalid_cidr")
		}
		onesOld, _ := oldNet.Mask.Size()
		onesNew, _ := newNet.Mask.Size()
		if onesOld != onesNew {
			return nil, fmt.Errorf("prefix_mismatch")
		}
		if oldNet.IP.To4() == nil || newNet.IP.To4() == nil {
			return nil, fmt.Errorf("ipv6_not_supported")
		}
		return &ipTransform{mode: ipTransformCIDR, oldNet: oldNet, newNet: newNet}, nil
	}

	if !octetPrefixRegex.MatchString(oldStr) || !octetPrefixRegex.MatchString(newStr) {
		return nil, fmt.Errorf("invalid_prefix_format")
	}
	if strings.Count(oldStr, ".") != strings.Count(newStr, ".") {
		return nil, fmt.Errorf("prefix_format_mismatch")
	}
	return &ipTransform{mode: ipTransformOctets, oldPref: oldStr, newPref: newStr}, nil
}

func (t *ipTransform) apply(ip string) (string, error) {
	if t.mode == ipTransformNone {
		return ip, nil
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", fmt.Errorf("invalid_ip")
	}

	switch t.mode {
	case ipTransformOctets:
		if !strings.HasPrefix(ip, t.oldPref) {
			return "", fmt.Errorf("prefix_not_matched")
		}
		boundary := len(t.oldPref)
		if boundary < len(ip) && ip[boundary] != '.' {
			return "", fmt.Errorf("prefix_not_matched")
		}
		return t.newPref + ip[boundary:], nil
	case ipTransformCIDR:
		oldIP := parsed.To4()
		if oldIP == nil {
			return "", fmt.Errorf("invalid_ipv4")
		}
		mask := t.oldNet.Mask
		if !oldIP.Mask(mask).Equal(t.oldNet.IP.Mask(mask)) {
			return "", fmt.Errorf("prefix_not_matched")
		}
		newIP := make(net.IP, 4)
		for i := 0; i < 4; i++ {
			newIP[i] = (oldIP[i] & ^mask[i]) | t.newNet.IP[i]
		}
		return newIP.String(), nil
	}
	return ip, nil
}

func findHostsByIP(ip, excludeMac string) []HostEntry {
	result := []HostEntry{}
	excludeMacLower := strings.ToLower(excludeMac)

	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return result
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, f.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		for _, raw := range strings.Split(string(content), "\n") {
			entry, ok := parseDhcpHostLine(raw, fullPath)
			if !ok {
				continue
			}
			if entry.Ip == ip && strings.ToLower(entry.Mac) != excludeMacLower {
				result = append(result, entry)
			}
		}
	}
	return result
}

func findHostsByMac(mac string) []HostEntry {
	result := []HostEntry{}
	macLower := strings.ToLower(mac)

	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return result
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, f.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		for _, raw := range strings.Split(string(content), "\n") {
			entry, ok := parseDhcpHostLine(raw, fullPath)
			if !ok {
				continue
			}
			if strings.ToLower(entry.Mac) == macLower {
				result = append(result, entry)
			}
		}
	}
	return result
}

func readAllHosts() []HostEntry {
	hosts := []HostEntry{}
	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return hosts
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, f.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		for _, raw := range strings.Split(string(content), "\n") {
			entry, ok := parseDhcpHostLine(raw, fullPath)
			if !ok {
				continue
			}
			hosts = append(hosts, entry)
		}
	}
	return hosts
}

func hostsToCSV(hosts []HostEntry) []byte {
	buf := new(bytes.Buffer)
	w := csv.NewWriter(buf)
	w.Write([]string{"mac", "ip", "hostname"})
	for _, h := range hosts {
		w.Write([]string{h.Mac, h.Ip, h.Hostname})
	}
	w.Flush()
	return buf.Bytes()
}

// validateHostFields проверяет поля dhcp-host записи без требования «все
// три обязательны». Контракт:
//   - MAC обязателен и валиден по macRegex.
//   - Если IP указан — должен парситься net.ParseIP.
//   - Если hostname указан — должен удовлетворять validHostname.
func validateHostFields(mac, ip, hostname string) bool {
	if !macRegex.MatchString(mac) {
		return false
	}
	if ip != "" && net.ParseIP(ip) == nil {
		return false
	}
	if hostname != "" && !validHostname(hostname) {
		return false
	}
	return true
}

func parseCSVHosts(r io.Reader, targetFile string) ([]HostEntry, error) {
	reader := csv.NewReader(r)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	hosts := []HostEntry{}
	for i, row := range records {
		if i == 0 {
			continue
		}
		if len(row) < 3 {
			continue
		}
		mac := strings.TrimSpace(row[0])
		ip := strings.TrimSpace(row[1])
		hostname := strings.TrimSpace(row[2])
		if validateHostFields(mac, ip, hostname) {
			hosts = append(hosts, HostEntry{Mac: mac, Ip: ip, Hostname: hostname, File: targetFile})
		}
	}
	return hosts, nil
}

// findFreeIP picks the first unallocated IPv4 inside cidr (network+1 …
// broadcast-1). Scans at most 256 candidates to keep latency bounded on
// wide ranges. Returns range_exhausted if nothing is available.
func findFreeIP(cidr string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid_cidr")
	}

	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return "", fmt.Errorf("ipv6_not_supported")
	}
	if ones >= 31 {
		return "", fmt.Errorf("range_too_small")
	}

	network := ipNet.IP.To4()
	if network == nil {
		return "", fmt.Errorf("invalid_ipv4")
	}

	mask := ipNet.Mask
	broadcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		broadcast[i] = network[i] | ^mask[i]
	}

	candidate := make(net.IP, 4)
	copy(candidate, network)

	scanned := 0
	const scanLimit = 256

	for {
		incIP(candidate)
		scanned++
		if scanned > scanLimit {
			return "", fmt.Errorf("range_exhausted")
		}

		if candidate.Equal(broadcast) {
			return "", fmt.Errorf("range_exhausted")
		}

		if candidate[3] == 0 {
			continue
		}

		if len(findHostsByIP(candidate.String(), "")) == 0 {
			return candidate.String(), nil
		}
	}
}

// incIP mutates ip in place, incrementing it as a little-endian integer.
func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
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

func appendHostLine(filePath string, h HostEntry) error {
	content, _ := os.ReadFile(filePath)
	line := formatDhcpHostLine(h)
	out := strings.TrimRight(string(content), "\n")
	if out != "" {
		out += "\n"
	}
	out += line + "\n"
	return os.WriteFile(filePath, []byte(out), 0644)
}
