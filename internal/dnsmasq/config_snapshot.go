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

// config_snapshot.go — structured reading/writing of dnsmasq directives
// (everything except dhcp-host= and alias directives, which have their own
// subsystems). Implements the visual "dnsmasq config" tab's backend:
// ReadConfigSnapshot, ParseDhcpRange, SerializeConfigFile, directive
// grouping/sorting. dhcp-range directives are also surfaced in a structured
// form so the UI can build dropdowns of CIDRs.

package dnsmasq

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"intermask/internal/models"
)

// DirectiveKeyRegex matches a dnsmasq directive name: lowercase letters,
// digits and hyphens. Used to distinguish "real" comments from commented-out
// directives (e.g. "#no-resolv"). Exported so the main package's PUT /api/config
// handler can re-use the same compiled pattern when validating user-supplied
// directive keys.
var DirectiveKeyRegex = directiveKeyRegex

// directiveKeyRegex matches a dnsmasq directive name: lowercase letters,
// digits and hyphens. Used to distinguish "real" comments from commented-out
// directives (e.g. "#no-resolv").
var directiveKeyRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// leaseTimeRegex matches a dnsmasq dhcp-range lease-time suffix: digits
// optionally followed by s/m/h/d/w, or the literal "infinite".
var leaseTimeRegex = regexp.MustCompile(`^\d+[smhdw]?$`)

// SplitDirective splits "key=value" into key and value. dnsmasq conf
// files use '=' as the canonical separator. Keys may contain hyphens
// (e.g. "no-resolv", "domain-needed"), so '-' is NOT treated as a separator.
func SplitDirective(line string) (key, value string, ok bool) {
	if idx := strings.Index(line, "="); idx > 0 {
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		if directiveKeyRegex.MatchString(k) {
			return k, v, true
		}
	}
	k := strings.TrimSpace(line)
	if directiveKeyRegex.MatchString(k) {
		return k, "", true
	}
	return "", "", false
}

// ReadConfigSnapshot walks all .conf files in ConfigDir and extracts every
// directive except dhcp-host and alias directives. Commented-out directives
// (e.g. "#no-resolv") are returned with Active=false. dhcp-range directives
// are additionally returned in a structured form via DhcpRanges slice.
func ReadConfigSnapshot() models.ConfigSnapshot {
	snap := models.ConfigSnapshot{Files: []models.ConfigFile{}, DhcpRanges: []models.DhcpRange{}}

	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return snap
	}

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, f.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		cf := models.ConfigFile{
			Path:       fullPath,
			Name:       f.Name(),
			Directives: []models.Directive{},
		}
		if _, err := os.Stat(fullPath + ".bak"); err == nil {
			cf.HasBak = true
		}

		lines := strings.Split(string(content), "\n")
		for i, raw := range lines {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}

			active := true
			if strings.HasPrefix(line, "#") {
				active = false
				line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
				if line == "" {
					continue
				}
			}

			if strings.HasPrefix(line, "dhcp-host=") || strings.HasPrefix(line, "dhcp-host:") {
				continue
			}
			if IsAliasDirective(line) {
				continue
			}

			key, value, ok := SplitDirective(line)
			if !ok {
				continue
			}

			d := models.Directive{
				Key:    key,
				Value:  value,
				Active: active,
				File:   fullPath,
				LineNo: i + 1,
			}
			cf.Directives = append(cf.Directives, d)

			if key == "dhcp-range" && active {
				r := ParseDhcpRange(value, fullPath, i+1)
				snap.DhcpRanges = append(snap.DhcpRanges, r)
			}
		}

		snap.Files = append(snap.Files, cf)
	}

	return snap
}

// ParseDhcpRange parses a dhcp-range value into a structured form.
func ParseDhcpRange(raw, file string, lineNo int) models.DhcpRange {
	r := models.DhcpRange{Raw: raw, File: file, LineNo: lineNo}
	parts := strings.Split(raw, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	rest := parts
	if len(rest) > 0 {
		first := rest[0]
		if strings.HasPrefix(first, "set:") {
			r.Tag = strings.TrimPrefix(first, "set:")
			rest = rest[1:]
		} else if strings.HasPrefix(first, "tag:") {
			r.Tag = strings.TrimPrefix(first, "tag:")
			rest = rest[1:]
		}
	}

	if len(rest) == 0 {
		return r
	}

	if strings.Contains(rest[0], "/") {
		r.Mask = rest[0]
		r.CIDR = rest[0]
		if _, _, err := net.ParseCIDR(rest[0]); err == nil {
			r.Start = strings.Split(rest[0], "/")[0]
		}
		if len(rest) > 1 {
			r.LeaseTime = rest[1]
		}
		return r
	}

	r.Start = rest[0]
	if len(rest) > 1 {
		r.End = rest[1]
	}
	if len(rest) > 2 {
		third := rest[2]
		if IsLeaseTime(third) {
			r.LeaseTime = third
		} else {
			r.Mask = third
			if len(rest) > 3 {
				r.LeaseTime = rest[3]
			}
		}
	}

	r.CIDR = DhcpRangeToCIDR(r)
	return r
}

// IsLeaseTime reports whether s looks like a dnsmasq lease duration.
func IsLeaseTime(s string) bool {
	if s == "infinite" {
		return true
	}
	if len(s) < 2 {
		return false
	}
	return leaseTimeRegex.MatchString(s)
}

// DhcpRangeToCIDR computes a CIDR string (network/prefix) from a DhcpRange
// using start address and netmask. Returns "" if it cannot be determined.
func DhcpRangeToCIDR(r models.DhcpRange) string {
	if r.Mask == "" || r.Start == "" {
		return ""
	}
	if strings.Contains(r.Mask, "/") {
		return r.Mask
	}
	ip := net.ParseIP(r.Start)
	mask := net.ParseIP(r.Mask)
	if ip == nil || mask == nil {
		return ""
	}
	ip4 := ip.To4()
	mask4 := mask.To4()
	if ip4 == nil || mask4 == nil {
		return ""
	}
	maskIP := net.IPv4Mask(mask4[0], mask4[1], mask4[2], mask4[3])
	ones, _ := maskIP.Size()
	if ones == 0 {
		return ""
	}
	network := ip4.Mask(maskIP)
	return fmt.Sprintf("%s/%d", network.String(), ones)
}

// DetectDhcpRangesCIDR returns the list of CIDR strings derived from all
// active dhcp-range directives. Used to populate the IP-range dropdown.
func DetectDhcpRangesCIDR() []string {
	snap := ReadConfigSnapshot()
	out := []string{}
	seen := make(map[string]bool)
	for _, r := range snap.DhcpRanges {
		if r.CIDR != "" && !seen[r.CIDR] {
			seen[r.CIDR] = true
			out = append(out, r.CIDR)
		}
	}
	return out
}

// SerializeConfigFile rebuilds a .conf file preserving:
//   - leading plain comments (lines starting with # that are not directives)
//   - all dhcp-host= lines in their original order
//   - the supplied directives, sorted by group then key for readability
//
// Inactive directives are written with a "#" prefix. Boolean directives
// (empty Value) are written as bare "key"; valued directives as "key=value".
func SerializeConfigFile(path string, directives []models.Directive) ([]byte, error) {
	content, err := os.ReadFile(path)
	existing := ""
	if err == nil {
		existing = string(content)
	}

	headerComments := []string{}
	dhcpHostLines := []string{}
	aliasLines := []string{}
	if existing != "" {
		for _, raw := range strings.Split(existing, "\n") {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "dhcp-host=") || strings.HasPrefix(line, "dhcp-host:") {
				dhcpHostLines = append(dhcpHostLines, raw)
				continue
			}
			if IsAliasDirective(line) {
				aliasLines = append(aliasLines, raw)
				continue
			}
			if strings.HasPrefix(line, "#") {
				stripped := strings.TrimSpace(strings.TrimPrefix(line, "#"))
				if stripped == "" {
					headerComments = append(headerComments, raw)
					continue
				}
				if _, _, ok := SplitDirective(stripped); ok {
					continue
				}
				headerComments = append(headerComments, raw)
				continue
			}
		}
	}

	sorted := make([]models.Directive, len(directives))
	copy(sorted, directives)
	sort.SliceStable(sorted, func(i, j int) bool {
		gi, gj := DirectiveGroup(sorted[i].Key), DirectiveGroup(sorted[j].Key)
		if gi != gj {
			return gi < gj
		}
		if sorted[i].Key != sorted[j].Key {
			return sorted[i].Key < sorted[j].Key
		}
		return false
	})

	var b strings.Builder
	if len(headerComments) > 0 {
		b.WriteString(strings.Join(headerComments, "\n"))
		b.WriteString("\n")
	}
	if len(dhcpHostLines) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.Join(dhcpHostLines, "\n"))
		b.WriteString("\n")
	}
	if len(aliasLines) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.Join(aliasLines, "\n"))
		b.WriteString("\n")
	}
	if len(sorted) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("# === Managed by Intermasq ===\n")
		for _, d := range sorted {
			line := d.Key
			if d.Value != "" {
				line = d.Key + "=" + d.Value
			}
			if !d.Active {
				line = "#" + line
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return []byte(b.String()), nil
}

// DirectiveGroup returns a numeric group id for sorting directives in the
// serialized file. Lower id = earlier in file. Order: dns, dhcp, log, other.
func DirectiveGroup(key string) int {
	switch key {
	case "domain", "domain-needed", "bogus-priv", "no-resolv", "no-hosts",
		"listen-address", "bind-interfaces", "except-interface", "interface",
		"server", "address", "local", "expand-hosts", "no-poll", "resolv-file",
		"strict-order", "all-servers", "clear-on-reload":
		return 0
	case "dhcp-range", "dhcp-option", "dhcp-lease-max", "dhcp-authoritative",
		"dhcp-no-override", "dhcp-hostsfile", "dhcp-leasefile", "no-dhcp-interface":
		return 1
	case "log-queries", "log-dhcp", "log-facility", "log-async":
		return 2
	}
	return 3
}
