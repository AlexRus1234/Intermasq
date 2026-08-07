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

// Package dnsmasq holds the pure (side-effect-free) dnsmasq configuration
// helpers: dhcp-host= parsing/formatting, host lookup across the configured
// directory, IP/CIDR utilities, IP transforms for bulk-edit, CSV import/export,
// the visual config-directive snapshot/serializer, the known config
// templates, and the alias (address=/cname=/ptr-record=/txt-record=)
// parsers. The flag ConfigDir (-conf-dir) lives here so these helpers can
// read from the configured directory without re-importing the host binary.
//
// The file-level manipulation (write, backup, history) lives in the main
// package (see dnsmasq.go / backup.go / history.go in the host binary) and
// is intentionally kept out of this package — stage 4 of the modularization
// extracts only the side-effect-free parsers.
package dnsmasq

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"intermask/internal/models"
	"intermask/internal/validate"
)

// ConfigDir is the directory containing the .conf files managed by dnsmasq.
// Registered on the default flag set at package init as -conf-dir with
// default "/etc/dnsmasq.d". The host binary doesn't redefine this flag —
// callers (main package, handlers, tests) refer to it via dnsmasq.ConfigDir.
var ConfigDir = flag.String("conf-dir", "/etc/dnsmasq.d", "Directory with dnsmasq configs")

// octetPrefixRegex matches 1-3 dot-separated octets (e.g. "10", "10.0",
// "10.0.0") used as an IP-prefix transform in bulk-edit.
var octetPrefixRegex = regexp.MustCompile(`^(\d{1,3}\.){0,2}\d{1,3}$`)

// ParseDhcpHostLine parses a single "dhcp-host=..." line into a HostEntry.
// This is the single canonical parser used everywhere in the codebase;
// previously the same logic was duplicated in 5 places. It recognises:
//   - MAC (any token passing validate.ValidMAC)
//   - IPv4/IPv6 (any token parseable by net.ParseIP)
//   - tag qualifiers "set:<name>" / "tag:<name>"  -> collected into Tags
//   - everything else                             -> Hostname
//
// "id:<client-id>" tokens are stored as Tags verbatim so they round-trip
// without loss, but the UI does not surface them.
func ParseDhcpHostLine(raw, file string) (models.HostEntry, bool) {
	line := strings.TrimSpace(raw)
	if !strings.HasPrefix(line, "dhcp-host=") && !strings.HasPrefix(line, "dhcp-host:") {
		return models.HostEntry{}, false
	}
	line = strings.TrimPrefix(line, "dhcp-host=")
	line = strings.TrimPrefix(line, "dhcp-host:")
	parts := strings.Split(line, ",")
	entry := models.HostEntry{File: file}
	var tags []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		switch {
		case validate.ValidMAC(p):
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
		return models.HostEntry{}, false
	}
	entry.Tags = tags
	return entry, true
}

// FormatDhcpHostLine renders a HostEntry back into the dnsmasq textual form.
// Order is fixed: mac[, hostname][, ip][, tags...]. Tags come last because
// that is the convention dnsmasq examples use and it keeps the human-readable
// part of the line at the front.
func FormatDhcpHostLine(h models.HostEntry) string {
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

// ReadHostByMac scans a single .conf file and returns the first dhcp-host=
// entry whose MAC matches mac (case-insensitive). Returns nil if the file
// is unreadable or contains no matching entry.
func ReadHostByMac(filePath, mac string) *models.HostEntry {
	macLower := strings.ToLower(mac)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	for _, raw := range strings.Split(string(content), "\n") {
		entry, ok := ParseDhcpHostLine(raw, filePath)
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

// IPTransform is a parsed old/new prefix pair (either 3-octet strings like
// "10.0.0" or CIDRs like "10.0.0.0/24") that can be applied to a list of
// host IPs in bulk-edit. Exposed as a named type so cross-package tests in
// the main package (and any future consumer) can construct transforms
// directly; its fields are unexported and populated only via ParseIPTransform.
type IPTransform struct {
	mode    ipTransformMode
	oldNet  *net.IPNet
	newNet  *net.IPNet
	oldPref string
	newPref string
}

// ParseIPTransform turns the user-supplied old/new prefix pair (either as
// 3-octet strings like "10.0.0" or as CIDRs like "10.0.0.0/24") into a
// transform that can be applied to a list of host IPs in bulk-edit.
func ParseIPTransform(oldStr, newStr string) (*IPTransform, error) {
	if oldStr == "" && newStr == "" {
		return &IPTransform{mode: ipTransformNone}, nil
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
		return &IPTransform{mode: ipTransformCIDR, oldNet: oldNet, newNet: newNet}, nil
	}

	if !octetPrefixRegex.MatchString(oldStr) || !octetPrefixRegex.MatchString(newStr) {
		return nil, fmt.Errorf("invalid_prefix_format")
	}
	if strings.Count(oldStr, ".") != strings.Count(newStr, ".") {
		return nil, fmt.Errorf("prefix_format_mismatch")
	}
	return &IPTransform{mode: ipTransformOctets, oldPref: oldStr, newPref: newStr}, nil
}

// Apply returns the transformed IP, or an error describing why ip cannot be
// remapped under this transform.
func (t *IPTransform) Apply(ip string) (string, error) {
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

// FindHostsByIP returns every dhcp-host= entry matching ip (excluding the
// one with excludeMac, case-insensitive) across all .conf files in ConfigDir.
func FindHostsByIP(ip, excludeMac string) []models.HostEntry {
	result := []models.HostEntry{}
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
			entry, ok := ParseDhcpHostLine(raw, fullPath)
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

// FindHostsByMac returns every dhcp-host= entry whose MAC matches mac
// (case-insensitive) across all .conf files in ConfigDir.
func FindHostsByMac(mac string) []models.HostEntry {
	result := []models.HostEntry{}
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
			entry, ok := ParseDhcpHostLine(raw, fullPath)
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

// ReadAllHosts scans every .conf file in ConfigDir and returns every
// dhcp-host= entry as a structured HostEntry, in file-then-line order.
func ReadAllHosts() []models.HostEntry {
	hosts := []models.HostEntry{}
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
			entry, ok := ParseDhcpHostLine(raw, fullPath)
			if !ok {
				continue
			}
			hosts = append(hosts, entry)
		}
	}
	return hosts
}

// HostsToCSV serialises a slice of HostEntry into a CSV with the header
// mac,ip,hostname (one row per host). Tags are not part of the CSV format.
func HostsToCSV(hosts []models.HostEntry) []byte {
	buf := new(bytes.Buffer)
	w := csv.NewWriter(buf)
	w.Write([]string{"mac", "ip", "hostname"})
	for _, h := range hosts {
		w.Write([]string{h.Mac, h.Ip, h.Hostname})
	}
	w.Flush()
	return buf.Bytes()
}

// validateHostFields / normalizeMAC live in internal/validate; the dhcp-host
// parsers below call them as validate.ValidateHostFields / validate.NormalizeMAC.

// ParseCSVHosts reads a CSV (with header row "mac,ip,hostname") and returns
// every row that normalises to a valid host (MAC normalised, fields pass
// validate.ValidateHostFields). targetFile is stamped onto every returned
// entry so the handler routes the add to the right .conf file.
func ParseCSVHosts(r io.Reader, targetFile string) ([]models.HostEntry, error) {
	reader := csv.NewReader(r)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	hosts := []models.HostEntry{}
	for i, row := range records {
		if i == 0 {
			continue
		}
		if len(row) < 3 {
			continue
		}
		mac := validate.NormalizeMAC(strings.TrimSpace(row[0]))
		ip := strings.TrimSpace(row[1])
		hostname := strings.TrimSpace(row[2])
		if validate.ValidateHostFields(mac, ip, hostname) {
			hosts = append(hosts, models.HostEntry{Mac: mac, Ip: ip, Hostname: hostname, File: targetFile})
		}
	}
	return hosts, nil
}

// FindFreeIP picks the first unallocated IPv4 inside cidr (network+1 …
// broadcast-1). Scans at most 256 candidates to keep latency bounded on
// wide ranges. Returns range_exhausted if nothing is available.
func FindFreeIP(cidr string) (string, error) {
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
		IncIP(candidate)
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

		if len(FindHostsByIP(candidate.String(), "")) == 0 {
			return candidate.String(), nil
		}
	}
}

// IncIP mutates ip in place, incrementing it as a little-endian integer.
func IncIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}
