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

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArpContent(t *testing.T) {
	content := `IP address       HW type     Flags       HW address            Mask Device
192.168.1.1      0x1         0x2         aa:bb:cc:dd:ee:ff     *    eth0
192.168.1.2      0x1         0x2         11:22:33:44:55:66     *    eth0
192.168.1.3      0x1         0x0         77:88:99:aa:bb:cc     *    eth0
`
	result := parseArpContent(content)
	if len(result) != 2 {
		t.Fatalf("expected 2 active MACs, got %d", len(result))
	}
	if !result["aa:bb:cc:dd:ee:ff"] {
		t.Error("expected aa:bb:cc:dd:ee:ff to be present")
	}
	if !result["11:22:33:44:55:66"] {
		t.Error("expected 11:22:33:44:55:66 to be present")
	}
	if result["77:88:99:aa:bb:cc"] {
		t.Error("expected 77:88:99:aa:bb:cc to be absent (flag 0x0)")
	}
}

func TestParseArpContentEmpty(t *testing.T) {
	content := `IP address       HW type     Flags       HW address            Mask Device
`
	result := parseArpContent(content)
	if len(result) != 0 {
		t.Fatalf("expected 0 MACs, got %d", len(result))
	}
}

func TestParseArpContentZeroMac(t *testing.T) {
	content := `IP address       HW type     Flags       HW address            Mask Device
192.168.1.1      0x1         0x2         00:00:00:00:00:00     *    eth0
`
	result := parseArpContent(content)
	if len(result) != 0 {
		t.Fatalf("expected 0 MACs (zero MAC filtered), got %d", len(result))
	}
}

func TestParseArpContentUppercaseMac(t *testing.T) {
	content := `IP address       HW type     Flags       HW address            Mask Device
192.168.1.1      0x1         0x2         AA:BB:CC:DD:EE:FF     *    eth0
`
	result := parseArpContent(content)
	if !result["aa:bb:cc:dd:ee:ff"] {
		t.Error("expected MAC to be lowercased")
	}
}

func TestIsSafePath(t *testing.T) {
	*ConfigDir = "/etc/dnsmasq.d"
	tests := []struct {
		path     string
		expected bool
	}{
		{"/etc/dnsmasq.d/host.conf", true},
		{"/etc/dnsmasq.d/sub/host.conf", true},
		{"/etc/dnsmasq.d", true},
		{"/etc/passwd", false},
		{"/etc/dnsmasq.d_evil/host.conf", false},
		{"../etc/passwd", false},
	}

	for _, tt := range tests {
		result := isSafePath(tt.path)
		if result != tt.expected {
			t.Errorf("isSafePath(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestResolveSystemCaller(t *testing.T) {
	tests := []struct {
		input   string
		wantStr string
	}{
		{"none", "none"},
		{"systemd-user", "systemd-user"},
		{"systemd", "systemd"},
		{"openrc", "openrc"},
		{"runit", "runit"},
		{"sysvinit", "sysvinit"},
	}

	for _, tt := range tests {
		caller := resolveSystemCaller(tt.input)
		if !strings.Contains(caller.String(), tt.wantStr) {
			t.Errorf("resolveSystemCaller(%q) = %q, want containing %q", tt.input, caller.String(), tt.wantStr)
		}
	}
}

func TestResolveSystemCallerLegacy(t *testing.T) {
	caller := resolveSystemCaller("system")
	if _, ok := caller.(*SystemdSystemCaller); !ok {
		t.Error("expected SystemdSystemCaller for legacy scope 'system'")
	}

	caller = resolveSystemCaller("user")
	if _, ok := caller.(*SystemdUserCaller); !ok {
		t.Error("expected SystemdUserCaller for legacy scope 'user'")
	}
}

func TestMapLegacyScope(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"system", "systemd"},
		{"user", "systemd-user"},
		{"none", "none"},
		{"auto", "auto"},
		{"openrc", "openrc"},
	}
	for _, tt := range tests {
		result := mapLegacyScope(tt.input)
		if result != tt.expect {
			t.Errorf("mapLegacyScope(%q) = %q, want %q", tt.input, result, tt.expect)
		}
	}
}

func TestNoneCaller(t *testing.T) {
	caller := &NoneCaller{}
	if !caller.IsActive("anything") {
		t.Error("NoneCaller.IsActive should always return true")
	}
	if caller.Restart("anything") != nil {
		t.Error("NoneCaller.Restart should always return nil")
	}
	if caller.RestartSelf() == nil {
		t.Error("NoneCaller.RestartSelf should return error")
	}
}

func TestOpenRCCaller(t *testing.T) {
	caller := &OpenRCCaller{UseSudo: false}
	if caller.String() != "openrc (root)" {
		t.Errorf("OpenRC String() = %q", caller.String())
	}
	callerSudo := &OpenRCCaller{UseSudo: true}
	if callerSudo.String() != "openrc (via sudo)" {
		t.Errorf("OpenRC sudo String() = %q", callerSudo.String())
	}
}

func TestRunitCaller(t *testing.T) {
	caller := &RunitCaller{UseSudo: false, ServiceDir: "/etc/service"}
	if !strings.Contains(caller.String(), "runit") {
		t.Errorf("Runit String() = %q", caller.String())
	}
	if !strings.Contains(caller.String(), "/etc/service") {
		t.Errorf("Runit String() should contain service dir, got %q", caller.String())
	}
}

func TestSysVinitCaller(t *testing.T) {
	caller := &SysVinitCaller{UseSudo: false}
	if caller.String() != "sysvinit (root)" {
		t.Errorf("SysVinit String() = %q", caller.String())
	}
	callerSudo := &SysVinitCaller{UseSudo: true}
	if callerSudo.String() != "sysvinit (via sudo)" {
		t.Errorf("SysVinit sudo String() = %q", callerSudo.String())
	}
}

func TestSystemdCallerRestartSelf(t *testing.T) {
	caller := &SystemdSystemCaller{UseSudo: false}
	_ = caller
	callerUser := &SystemdUserCaller{}
	_ = callerUser
}

func TestParseDhcpRangeClassic(t *testing.T) {
	r := parseDhcpRange("192.168.1.50,192.168.1.150,255.255.255.0,12h", "/etc/dnsmasq.d/x.conf", 1)
	if r.Start != "192.168.1.50" || r.End != "192.168.1.150" || r.Mask != "255.255.255.0" || r.LeaseTime != "12h" {
		t.Errorf("unexpected range: %+v", r)
	}
	if r.CIDR != "192.168.1.0/24" {
		t.Errorf("CIDR = %q, want 192.168.1.0/24", r.CIDR)
	}
}

func TestParseDhcpRangeCIDRForm(t *testing.T) {
	r := parseDhcpRange("192.168.0.0/24,1h", "/etc/dnsmasq.d/x.conf", 1)
	if r.Mask != "192.168.0.0/24" || r.LeaseTime != "1h" {
		t.Errorf("unexpected range: %+v", r)
	}
	if r.CIDR != "192.168.0.0/24" {
		t.Errorf("CIDR = %q, want 192.168.0.0/24", r.CIDR)
	}
}

func TestParseDhcpRangeTagged(t *testing.T) {
	r := parseDhcpRange("set:corp,192.168.1.10,192.168.1.100,255.255.255.0,2h", "/etc/dnsmasq.d/x.conf", 1)
	if r.Tag != "corp" || r.Start != "192.168.1.10" || r.End != "192.168.1.100" {
		t.Errorf("unexpected range: %+v", r)
	}
	if r.CIDR != "192.168.1.0/24" {
		t.Errorf("CIDR = %q, want 192.168.1.0/24", r.CIDR)
	}
}

func TestParseDhcpRangeNoMask(t *testing.T) {
	r := parseDhcpRange("10.0.0.5,10.0.0.20,6h", "/etc/dnsmasq.d/x.conf", 1)
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
	r := DhcpRange{Start: "::1", Mask: "ffff:ffff::"}
	if c := dhcpRangeToCIDR(r); c != "" {
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
	out, err := serializeConfigFile(path, []Directive{
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
	out, err := serializeConfigFile(path, []Directive{
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
	dir := t.TempDir()
	*ConfigDir = dir
	path := filepath.Join(dir, "net.conf")
	content := []byte("dhcp-host=11:22:33:44:55:66,h,10.0.0.1\nserver=8.8.8.8\nno-resolv\n#domain-needed\n# plain comment\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	snap := readConfigSnapshot()
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
	dir := t.TempDir()
	*ConfigDir = dir
	path := filepath.Join(dir, "net.conf")
	content := []byte("dhcp-range=192.168.1.50,192.168.1.150,255.255.255.0,12h\ndhcp-range=set:guest,10.0.0.10,10.0.0.50,255.255.255.0,2h\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	snap := readConfigSnapshot()
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
	dir := t.TempDir()
	*ConfigDir = dir
	path := filepath.Join(dir, "net.conf")
	content := []byte("dhcp-range=192.168.1.50,192.168.1.150,255.255.255.0,12h\ndhcp-range=192.168.1.200,192.168.1.250,255.255.255.0,1h\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	cidrs := detectDhcpRangesCIDR()
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
	out, err := serializeConfigFile(path, []Directive{
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
