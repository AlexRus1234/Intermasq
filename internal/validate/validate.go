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

// Package validate centralises the syntactic validators for dhcp-host
// fields (MAC, IP, hostname) and dhcp-host tag qualifiers, plus the
// normalisers used on every user-input path before validation and before
// writing. The compiled regular expressions stay unexported inside this
// package; callers use the exported predicate wrappers.
package validate

import (
	"net"
	"regexp"
	"strings"
)

var (
	macRegex         = regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`)
	hostnameRegex    = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	aliasDomainRegex = regexp.MustCompile(`^[a-zA-Z0-9_]([a-zA-Z0-9-._]*[a-zA-Z0-9_])?$`)
	// dhcpTagRegex validates a single dhcp-host tag qualifier. dnsmasq
	// accepts "set:<name>" (assigns a tag to the host) and "tag:<name>"
	// (host matches only if that tag is already set by dhcp-match).
	// "id:..." (client-id) is intentionally out of scope for the UI.
	dhcpTagRegex = regexp.MustCompile(`^(set|tag):[a-zA-Z0-9_][a-zA-Z0-9_-]*$`)
)

// ValidMAC reports whether s is a syntactically valid MAC address in the
// colon- or dash-separated form (case-insensitive).
func ValidMAC(s string) bool { return macRegex.MatchString(s) }

// ValidAliasDomain reports whether s is an acceptable DNS owner name for
// an address=/ or cname= directive, including underscore-bearing names
// required by DMARC/DKIM/SRV/ACME.
func ValidAliasDomain(s string) bool { return aliasDomainRegex.MatchString(s) }

// ValidDhcpTag reports whether s is a recognised "set:<name>" / "tag:<name>"
// dhcp-host tag qualifier.
func ValidDhcpTag(s string) bool { return dhcpTagRegex.MatchString(s) }

// ValidHostname reports whether s is a syntactically valid DNS hostname
// per RFC 952 / RFC 1123 / RFC 1034: each dot-separated label is 1-63 chars,
// alphanumeric boundaries with hyphens allowed inside, total length <=253.
// Used for dhcp-host hostnames written by the panel.
func ValidHostname(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	return hostnameRegex.MatchString(s)
}

// NormalizeMAC converts the dash-separated MAC form (common when pasting
// from Windows `getmac` or some network tools) into the colon-separated
// form that dnsmasq accepts. dnsmasq --test rejects aa-bb-cc-dd-ee-ff, so
// every user input path normalises through this helper before validation
// and before writing. Case is preserved.
func NormalizeMAC(mac string) string {
	return strings.ReplaceAll(mac, "-", ":")
}

// ValidateHostFields проверяет поля dhcp-host записи без требования «все
// три обязательны». Контракт:
//   - MAC обязателен и валиден по macRegex.
//   - Если IP указан — должен парситься net.ParseIP.
//   - Если hostname указан — должен удовлетворять ValidHostname.
func ValidateHostFields(mac, ip, hostname string) bool {
	mac = NormalizeMAC(mac)
	if !macRegex.MatchString(mac) {
		return false
	}
	// Reject zero and broadcast MACs: dnsmasq either rejects them or, worse,
	// silently accepts and breaks DHCP for the whole segment.
	if strings.EqualFold(mac, "00:00:00:00:00:00") ||
		strings.EqualFold(mac, "ff:ff:ff:ff:ff:ff") {
		return false
	}
	if ip != "" && net.ParseIP(ip) == nil {
		return false
	}
	if hostname != "" && !ValidHostname(hostname) {
		return false
	}
	return true
}

// ValidateHostTags validates qualifiers for newly-created static hosts.
// A host assigns tags with "set:<name>"; "tag:<name>" is a matching
// condition for an already-defined tag and is not valid host input here.
// "id:<client-id>" remains accepted because it is a native dhcp-host
// qualifier.
func ValidateHostTags(tags []string) bool {
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, "set:") && !strings.HasPrefix(t, "id:") {
			return false
		}
		if strings.HasPrefix(t, "set:") && !dhcpTagRegex.MatchString(t) {
			return false
		}
	}
	return true
}

// NormalizeHostTags drops empties and de-duplicates case-insensitively while
// preserving the first-seen order.
func NormalizeHostTags(tags []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out
}
