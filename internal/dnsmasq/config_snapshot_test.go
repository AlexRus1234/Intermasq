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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"intermask/internal/models"
)

func TestParseDhcpRangeClassic(t *testing.T) {
	r := ParseDhcpRange("192.168.1.50,192.168.1.150,255.255.255.0,12h", "/etc/dnsmasq.d/x.conf", 1)
	if r.Start != "192.168.1.50" || r.End != "192.168.1.150" || r.Mask != "255.255.255.0" || r.LeaseTime != "12h" {
		t.Errorf("unexpected range: %+v", r)
	}
	if r.CIDR != "192.168.1.0/24" {
		t.Errorf("CIDR = %q, want 192.168.1.0/24", r.CIDR)
	}
}

func TestParseDhcpRangeCIDRForm(t *testing.T) {
	r := ParseDhcpRange("192.168.0.0/24,1h", "/etc/dnsmasq.d/x.conf", 1)
	if r.Mask != "192.168.0.0/24" || r.LeaseTime != "1h" {
		t.Errorf("unexpected range: %+v", r)
	}
	if r.CIDR != "192.168.0.0/24" {
		t.Errorf("CIDR = %q, want 192.168.0.0/24", r.CIDR)
	}
}

func TestParseDhcpRangeTagged(t *testing.T) {
	r := ParseDhcpRange("set:corp,192.168.1.10,192.168.1.100,255.255.255.0,2h", "/etc/dnsmasq.d/x.conf", 1)
	if r.Tag != "corp" || r.Start != "192.168.1.10" || r.End != "192.168.1.100" {
		t.Errorf("unexpected range: %+v", r)
	}
	if r.CIDR != "192.168.1.0/24" {
		t.Errorf("CIDR = %q, want 192.168.1.0/24", r.CIDR)
	}
}

func TestParseDhcpRangeNoMask(t *testing.T) {
	r := ParseDhcpRange("10.0.0.5,10.0.0.20,6h", "/etc/dnsmasq.d/x.conf", 1)
	if r.Start != "10.0.0.5" || r.End != "10.0.0.20" || r.LeaseTime != "6h" {
		t.Errorf("unexpected range: %+v", r)
	}
	if r.Mask != "" {
		t.Errorf("Mask should be empty, got %q", r.Mask)
	}
	if r.CIDR != "" {
		t.Errorf("CIDR should be empty when mask missing, got %q", r.CIDR)
	}
}

func TestDhcpRangeToCIDRIPv6Rejected(t *testing.T) {
	r := models.DhcpRange{Start: "::1", Mask: "ffff:ffff::"}
	if c := DhcpRangeToCIDR(r); c != "" {
		t.Errorf("ipv6 should yield empty CIDR, got %q", c)
	}
}

func TestSerializeConfigFilePreservesDhcpHosts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.conf")
	initial := []byte("# header comment\n\ndhcp-host=aa:bb:cc:dd:ee:ff,host1,192.168.1.10\nserver=8.8.8.8\n")
	if err := os.WriteFile(path, initial, 0644); err != nil {
		t.Fatal(err)
	}
	out, err := SerializeConfigFile(path, []models.Directive{
		{Key: "domain", Value: "lan", Active: true},
		{Key: "server", Value: "1.1.1.1", Active: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "dhcp-host=aa:bb:cc:dd:ee:ff,host1,192.168.1.10") {
		t.Errorf("dhcp-host line lost:\n%s", s)
	}
	if !strings.Contains(s, "# header comment") {
		t.Errorf("header comment lost:\n%s", s)
	}
	if !strings.Contains(s, "domain=lan") {
		t.Errorf("new directive missing:\n%s", s)
	}
	if !strings.Contains(s, "server=1.1.1.1") {
		t.Errorf("server override missing:\n%s", s)
	}
	if strings.Contains(s, "server=8.8.8.8") {
		t.Errorf("old server should be replaced:\n%s", s)
	}
}

func TestSerializeConfigFileInactiveDirective(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.conf")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	out, err := SerializeConfigFile(path, []models.Directive{
		{Key: "no-resolv", Value: "", Active: false},
		{Key: "domain", Value: "lan", Active: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "#no-resolv") {
		t.Errorf("inactive directive should be prefixed with #:\n%s", s)
	}
	if strings.Contains(s, "\nno-resolv\n") {
		t.Errorf("inactive directive should not be active:\n%s", s)
	}
	if !strings.Contains(s, "domain=lan") {
		t.Errorf("active directive missing:\n%s", s)
	}
}

func TestReadConfigSnapshotFiltersDhcpHost(t *testing.T) {
	dir := newTestDir(t)
	path := filepath.Join(dir, "net.conf")
	content := []byte("dhcp-host=11:22:33:44:55:66,h,10.0.0.1\nserver=8.8.8.8\nno-resolv\n#domain-needed\n# plain comment\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	snap := ReadConfigSnapshot()
	if len(snap.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(snap.Files))
	}
	for _, d := range snap.Files[0].Directives {
		if d.Key == "dhcp-host" {
			t.Errorf("dhcp-host should be filtered out: %+v", d)
		}
	}
	hasServer := false
	hasNoResolv := false
	hasDomainNeededInactive := false
	for _, d := range snap.Files[0].Directives {
		if d.Key == "server" && d.Value == "8.8.8.8" && d.Active {
			hasServer = true
		}
		if d.Key == "no-resolv" && d.Active {
			hasNoResolv = true
		}
		if d.Key == "domain-needed" && !d.Active {
			hasDomainNeededInactive = true
		}
	}
	if !hasServer {
		t.Error("active server directive missing")
	}
	if !hasNoResolv {
		t.Error("active no-resolv directive missing")
	}
	if !hasDomainNeededInactive {
		t.Error("inactive domain-needed directive missing")
	}
}

func TestReadConfigSnapshotDhcpRanges(t *testing.T) {
	dir := newTestDir(t)
	path := filepath.Join(dir, "net.conf")
	content := []byte("dhcp-range=192.168.1.50,192.168.1.150,255.255.255.0,12h\ndhcp-range=set:guest,10.0.0.10,10.0.0.50,255.255.255.0,2h\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	snap := ReadConfigSnapshot()
	if len(snap.DhcpRanges) != 2 {
		t.Fatalf("expected 2 dhcp ranges, got %d", len(snap.DhcpRanges))
	}
	if snap.DhcpRanges[0].CIDR != "192.168.1.0/24" {
		t.Errorf("first CIDR = %q", snap.DhcpRanges[0].CIDR)
	}
	if snap.DhcpRanges[1].Tag != "guest" || snap.DhcpRanges[1].CIDR != "10.0.0.0/24" {
		t.Errorf("second range wrong: %+v", snap.DhcpRanges[1])
	}
}

func TestDetectDhcpRangesCIDRDedup(t *testing.T) {
	dir := newTestDir(t)
	path := filepath.Join(dir, "net.conf")
	content := []byte("dhcp-range=192.168.1.50,192.168.1.150,255.255.255.0,12h\ndhcp-range=192.168.1.200,192.168.1.250,255.255.255.0,1h\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	cidrs := DetectDhcpRangesCIDR()
	if len(cidrs) != 1 || cidrs[0] != "192.168.1.0/24" {
		t.Errorf("expected deduped [192.168.1.0/24], got %v", cidrs)
	}
}

func TestSerializeConfigFileGroupOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.conf")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	out, err := SerializeConfigFile(path, []models.Directive{
		{Key: "log-queries", Value: "", Active: true},
		{Key: "dhcp-option", Value: "3,192.168.1.1", Active: true},
		{Key: "domain", Value: "lan", Active: true},
		{Key: "server", Value: "8.8.8.8", Active: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	domainIdx := strings.Index(s, "domain=lan")
	serverIdx := strings.Index(s, "server=8.8.8.8")
	dhcpOptIdx := strings.Index(s, "dhcp-option=3,192.168.1.1")
	logIdx := strings.Index(s, "log-queries")
	if !(domainIdx < serverIdx && serverIdx < dhcpOptIdx && dhcpOptIdx < logIdx) {
		t.Errorf("directives not grouped in dns<dhcp<log order:\n%s", s)
	}
}

func TestSerializeConfigFilePreservesAliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.conf")
	initial := []byte("# header\n\naddress=/nas.lan/192.168.1.10\ncname=wiki,nas.lan\nserver=8.8.8.8\n")
	if err := os.WriteFile(path, initial, 0644); err != nil {
		t.Fatal(err)
	}
	out, err := SerializeConfigFile(path, []models.Directive{
		{Key: "domain", Value: "lan", Active: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "address=/nas.lan/192.168.1.10") {
		t.Errorf("alias A lost:\n%s", s)
	}
	if !strings.Contains(s, "cname=wiki,nas.lan") {
		t.Errorf("cname lost:\n%s", s)
	}
	if !strings.Contains(s, "domain=lan") {
		t.Errorf("new directive missing:\n%s", s)
	}
}

func TestReadConfigSnapshotFiltersAliases(t *testing.T) {
	dir := newTestDir(t)
	path := filepath.Join(dir, "net.conf")
	content := []byte("address=/nas.lan/192.168.1.10\ncname=wiki,nas.lan\nserver=8.8.8.8\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	snap := ReadConfigSnapshot()
	if len(snap.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(snap.Files))
	}
	for _, d := range snap.Files[0].Directives {
		if d.Key == "address" || d.Key == "cname" {
			t.Errorf("alias directive should be filtered out: %+v", d)
		}
	}
}

// TestIsLeaseTime covers the dnsmasq lease-time acceptor.
func TestIsLeaseTime(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"infinite", true},
		{"12", true},
		{"12s", true},
		{"12m", true},
		{"12h", true},
		{"12d", true},
		{"12w", true},
		{"1s", true},   // single digit + unit
		{"x", false},   // non-digit
		{"", false},    // too short
		{"a", false},   // too short + non-digit
		{"1", false},   // too short (len<2)
		{"12y", false}, // wrong unit
		{"12x", false}, // wrong unit
	}
	for _, tc := range cases {
		if got := IsLeaseTime(tc.s); got != tc.want {
			t.Errorf("IsLeaseTime(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

// TestDirectiveGroup covers every group id returned by DirectiveGroup.
func TestDirectiveGroup(t *testing.T) {
	cases := []struct {
		key  string
		want int
	}{
		// Group 0 — dns.
		{"domain", 0}, {"domain-needed", 0}, {"bogus-priv", 0}, {"no-resolv", 0},
		{"no-hosts", 0}, {"listen-address", 0}, {"bind-interfaces", 0},
		{"except-interface", 0}, {"interface", 0}, {"server", 0}, {"address", 0},
		{"local", 0}, {"expand-hosts", 0}, {"no-poll", 0}, {"resolv-file", 0},
		{"strict-order", 0}, {"all-servers", 0}, {"clear-on-reload", 0},
		// Group 1 — dhcp.
		{"dhcp-range", 1}, {"dhcp-option", 1}, {"dhcp-lease-max", 1},
		{"dhcp-authoritative", 1}, {"dhcp-no-override", 1}, {"dhcp-hostsfile", 1},
		{"dhcp-leasefile", 1}, {"no-dhcp-interface", 1},
		// Group 2 — log.
		{"log-queries", 2}, {"log-dhcp", 2}, {"log-facility", 2}, {"log-async", 2},
		// Group 3 — unknown.
		{"something-else", 3}, {"", 3}, {"conf-file", 3},
	}
	for _, tc := range cases {
		if got := DirectiveGroup(tc.key); got != tc.want {
			t.Errorf("DirectiveGroup(%q) = %d, want %d", tc.key, got, tc.want)
		}
	}
}
