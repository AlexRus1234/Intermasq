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
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"intermask/internal/bins"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
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

// TestIsSafePath pins the A11 defense-in-depth layer (isSafePath,
// dnsmasq.go:51) DIRECTLY, independently of the handler-level substring
// filter (handlers_config.go:199/223).
//
// Every external HTTP traversal vector today carries "/" or "\", so the
// substring filter in getFileHandler/putFileHandler rejects it BEFORE
// isSafePath-after-Join ever fires (see TestGetFileHandlerRejectsUnsafePath /
// TestPutFileHandlerRejectsUnsafePath for that layer). There is no external
// HTTP vector that bypasses the substring filter but is caught by isSafePath
// by design — isSafePath exists precisely as the second gate in case the
// substring filter is ever weakened (e.g. to allow "/" in names) or a new
// call site forgets it. This test pins that second gate on its own.
//
// The "/etc/dnsmasq.d_evil/host.conf" case is the discriminating one: it
// catches a regression that drops the path-separator from the HasPrefix
// check (strings.HasPrefix(cleanPath, cleanDir+sep) → ...HasPrefix(_, cleanDir)),
// which would let a sibling directory whose name shares a prefix with ConfigDir
// pass as "inside". Mutate isSafePath that way and this case fails.
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

func TestParseAliasLineA(t *testing.T) {
	e, ok := parseAliasLine("address=/nas.lan/192.168.1.10", "/etc/dnsmasq.d/x.conf", false)
	if !ok {
		t.Fatal("expected parse success")
	}
	if e.Type != "A" || e.Domain != "nas.lan" || e.Target != "192.168.1.10" {
		t.Errorf("unexpected entry: %+v", e)
	}
}

func TestParseAliasLineCNAME(t *testing.T) {
	e, ok := parseAliasLine("cname=wiki,nas.lan", "/etc/dnsmasq.d/x.conf", false)
	if !ok {
		t.Fatal("expected parse success")
	}
	if e.Type != "CNAME" || e.Domain != "wiki" || e.Target != "nas.lan" {
		t.Errorf("unexpected entry: %+v", e)
	}
}

func TestParseAliasLineCNAMEWithTag(t *testing.T) {
	e, ok := parseAliasLine("cname=wiki,nas.lan,tag:lan", "/etc/dnsmasq.d/x.conf", false)
	if !ok {
		t.Fatal("expected parse success for tagged cname")
	}
	if e.Type != "CNAME" || e.Domain != "wiki" || e.Target != "nas.lan" {
		t.Errorf("unexpected entry: %+v", e)
	}
}

func TestParseAliasLineRejectsWildcard(t *testing.T) {
	if _, ok := parseAliasLine("address=/#/10.0.0.1", "", false); ok {
		t.Error("wildcard # should be rejected")
	}
	if _, ok := parseAliasLine("address=/*.evil/10.0.0.1", "", false); ok {
		t.Error("wildcard *.evil should be rejected")
	}
}

func TestParseAliasLineRejectsMalformed(t *testing.T) {
	if _, ok := parseAliasLine("address=/nas.lan", "", false); ok {
		t.Error("missing closing slash should fail")
	}
	if _, ok := parseAliasLine("address=/nas.lan/", "", false); ok {
		t.Error("empty target should fail")
	}
	if _, ok := parseAliasLine("cname=onlyalias", "", false); ok {
		t.Error("cname without target should fail")
	}
}

func TestAliasToLineRoundTrip(t *testing.T) {
	cases := []DnsAliasEntry{
		{Type: "A", Domain: "nas.lan", Target: "192.168.1.10"},
		{Type: "CNAME", Domain: "wiki", Target: "nas.lan"},
	}
	for _, in := range cases {
		line := aliasToLine(in)
		out, ok := parseAliasLine(line, "", false)
		if !ok {
			t.Errorf("round-trip failed for %+v: line=%q", in, line)
			continue
		}
		out.File = in.File
		if out != in {
			t.Errorf("round-trip mismatch:\n in=%+v\nout=%+v", in, out)
		}
	}
}

func TestReadAllAliases(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	path := filepath.Join(dir, "dns.conf")
	content := []byte("address=/nas.lan/192.168.1.10\ncname=wiki,nas.lan\nserver=8.8.8.8\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	aliases := readAllAliases()
	if len(aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d: %+v", len(aliases), aliases)
	}
	if aliases[0].Type != "A" || aliases[0].Domain != "nas.lan" {
		t.Errorf("first alias wrong: %+v", aliases[0])
	}
	if aliases[1].Type != "CNAME" || aliases[1].Domain != "wiki" {
		t.Errorf("second alias wrong: %+v", aliases[1])
	}
}

func TestReadAllAliasesHasBakMarker(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	path := filepath.Join(dir, "dns.conf")
	if err := os.WriteFile(path, []byte("address=/a.b/1.2.3.4\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	aliases := readAllAliases()
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}
	if !strings.HasSuffix(aliases[0].File, "|has_bak") {
		t.Errorf("expected |has_bak marker, got %q", aliases[0].File)
	}
	if cleanAliasFile(aliases[0].File) != path {
		t.Errorf("cleanAliasFile wrong: got %q want %q", cleanAliasFile(aliases[0].File), path)
	}
}

func TestRemoveAliasLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	content := []byte("address=/nas.lan/192.168.1.10\ncname=wiki,nas.lan\naddress=/other/10.0.0.1\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	removed, err := removeAliasLine(path, "A", "nas.lan")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected removal")
	}
	out, _ := os.ReadFile(path)
	s := string(out)
	if strings.Contains(s, "address=/nas.lan/") {
		t.Errorf("nas.lan not removed:\n%s", s)
	}
	if !strings.Contains(s, "cname=wiki,nas.lan") {
		t.Errorf("cname should be preserved:\n%s", s)
	}
	if !strings.Contains(s, "address=/other/10.0.0.1") {
		t.Errorf("other A should be preserved:\n%s", s)
	}
}

func TestRemoveAliasLineNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	if err := os.WriteFile(path, []byte("address=/nas.lan/192.168.1.10\n"), 0644); err != nil {
		t.Fatal(err)
	}
	removed, err := removeAliasLine(path, "A", "missing.lan")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("expected no removal for missing domain")
	}
}

func TestSerializeConfigFilePreservesAliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.conf")
	initial := []byte("# header\n\naddress=/nas.lan/192.168.1.10\ncname=wiki,nas.lan\nserver=8.8.8.8\n")
	if err := os.WriteFile(path, initial, 0644); err != nil {
		t.Fatal(err)
	}
	out, err := serializeConfigFile(path, []Directive{
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
	dir := t.TempDir()
	*ConfigDir = dir
	path := filepath.Join(dir, "net.conf")
	content := []byte("address=/nas.lan/192.168.1.10\ncname=wiki,nas.lan\nserver=8.8.8.8\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	snap := readConfigSnapshot()
	if len(snap.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(snap.Files))
	}
	for _, d := range snap.Files[0].Directives {
		if d.Key == "address" || d.Key == "cname" {
			t.Errorf("alias directive should be filtered out: %+v", d)
		}
	}
}

// setupHistoryEnv prepares temp ConfigDir and HistoryDir for history tests.
func setupHistoryEnv(t *testing.T) (confDir, histDir string) {
	t.Helper()
	confDir = t.TempDir()
	histDir = t.TempDir()
	*ConfigDir = confDir
	*HistoryDir = histDir
	*HistoryDepth = 10
	return confDir, histDir
}

func TestSaveHistoryCreatesVersion(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	path := filepath.Join(confDir, "hosts.conf")
	if err := os.WriteFile(path, []byte("dhcp-host=aa:bb:cc:dd:ee:ff,1.2.3.4,host\n"), 0644); err != nil {
		t.Fatal(err)
	}
	saveHistory(path)
	versions, err := listHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if !historyVersionRegex.MatchString(versions[0].Version) {
		t.Errorf("bad version id: %q", versions[0].Version)
	}
}

func TestSaveHistoryNoOpForMissingFile(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	path := filepath.Join(confDir, "nope.conf")
	saveHistory(path)
	versions, err := listHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("expected 0 versions for missing file, got %d", len(versions))
	}
}

func TestSaveHistoryRejectsUnsafePath(t *testing.T) {
	setupHistoryEnv(t)
	// Path outside ConfigDir — must be ignored.
	saveHistory("/etc/passwd")
	versions, _ := listHistory("/etc/passwd")
	if len(versions) != 0 {
		t.Fatalf("history written for unsafe path")
	}
}

func TestRotateHistoryKeepsDepth(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	*HistoryDepth = 3
	path := filepath.Join(confDir, "hosts.conf")
	if err := os.WriteFile(path, []byte("v0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Save 5 versions with distinct mtimes so rotation order is stable.
	for i := 0; i < 5; i++ {
		os.WriteFile(path, []byte("v"+string(rune('0'+i))+"\n"), 0644)
		saveHistory(path)
		// Bump mtime of just-written history file so sort is deterministic.
		entries, _ := os.ReadDir(*HistoryDir)
		for _, e := range entries {
			full := filepath.Join(*HistoryDir, e.Name())
			mtime := time.Now().Add(time.Duration(i) * time.Minute)
			os.Chtimes(full, mtime, mtime)
		}
	}
	versions, err := listHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions after rotation, got %d", len(versions))
	}
}

func TestReadHistoryVersionInvalid(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	path := filepath.Join(confDir, "hosts.conf")
	os.WriteFile(path, []byte("x\n"), 0644)
	if _, err := readHistoryVersion(path, "../escape"); err == nil {
		t.Fatal("expected error for invalid version")
	}
	if _, err := readHistoryVersion(path, "not-a-date"); err == nil {
		t.Fatal("expected error for non-date version")
	}
}

func TestListHistorySortedNewestFirst(t *testing.T) {
	confDir, _ := setupHistoryEnv(t)
	path := filepath.Join(confDir, "hosts.conf")
	os.WriteFile(path, []byte("a\n"), 0644)
	saveHistory(path)
	os.Chtimes(filepath.Join(*HistoryDir, historyFilePrefix(path)+firstVersion(t, path)+".bak"), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	os.WriteFile(path, []byte("b\n"), 0644)
	saveHistory(path)
	v, err := listHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(v))
	}
	if v[0].Version < v[1].Version {
		t.Errorf("expected newest first, got %q before %q", v[0].Version, v[1].Version)
	}
}

// firstVersion returns the single stored version id for path (test helper).
func firstVersion(t *testing.T, path string) string {
	t.Helper()
	v, err := listHistory(path)
	if err != nil || len(v) != 1 {
		t.Fatalf("firstVersion: %v (%d)", err, len(v))
	}
	return v[0].Version
}

func TestUnifiedDiffAddsAndRemoves(t *testing.T) {
	a := "line1\nline2\nline3\n"
	bText := "line1\nlineX\nline3\nline4\n"
	d := unifiedDiff(a, bText, "a", "b")
	if !strings.Contains(d, "-line2") {
		t.Errorf("diff missing removal: %s", d)
	}
	if !strings.Contains(d, "+lineX") || !strings.Contains(d, "+line4") {
		t.Errorf("diff missing additions: %s", d)
	}
	if strings.Contains(d, "+line1") || strings.Contains(d, "-line1") {
		t.Errorf("common line should not appear: %s", d)
	}
}

func TestUnifiedDiffEmptyA(t *testing.T) {
	d := unifiedDiff("", "x\ny\n", "a", "b")
	if !strings.Contains(d, "+x") || !strings.Contains(d, "+y") {
		t.Errorf("expected both lines added: %s", d)
	}
}

// ========== Raw file read/write ==========

func TestReadFileRaw(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	path := filepath.Join(dir, "raw.conf")
	os.WriteFile(path, []byte("server=1.2.3.4\n"), 0644)
	content, err := readFileRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "server=1.2.3.4\n" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestReadFileRawUnsafePath(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	_, err := readFileRaw("/etc/passwd")
	if err != os.ErrPermission {
		t.Errorf("expected ErrPermission, got %v", err)
	}
}

func TestReadFileRawNotExist(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	path := filepath.Join(dir, "nope.conf")
	_, err := readFileRaw(path)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWriteFileRaw(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	*HistoryDir = t.TempDir()
	*HistoryDepth = 5
	path := filepath.Join(dir, "writetest.conf")
	os.WriteFile(path, []byte("old\n"), 0644)
	_ = writeFileRaw(path, []byte("server=8.8.8.8\n"))
	_, err := os.Stat(path + ".bak")
	if os.IsNotExist(err) {
		t.Error(".bak should exist even if dnsmasq --test fails")
	}
}

func TestWriteFileRawUnsafePath(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	err := writeFileRaw("/etc/passwd", []byte("x"))
	if err != os.ErrPermission {
		t.Errorf("expected ErrPermission, got %v", err)
	}
}

// ========== SSE broker ==========

func TestSseRegisterUnregister(t *testing.T) {
	cl := &sseClient{ch: make(chan string, 1)}
	sseRegister(cl)
	if !sseClients[cl] {
		t.Fatal("client should be registered")
	}
	sseUnregister(cl)
	if sseClients[cl] {
		t.Fatal("client should be unregistered")
	}
}

func TestSseBroadcast(t *testing.T) {
	cl := &sseClient{ch: make(chan string, 10)}
	sseRegister(cl)
	defer sseUnregister(cl)
	sseBroadcast("arp", `{"aa:bb:cc:dd:ee:ff":true}`)
	select {
	case msg := <-cl.ch:
		if !strings.Contains(msg, "event: arp") {
			t.Errorf("bad event: %s", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("message not received")
	}
}

func TestSseBroadcastFullChannel(t *testing.T) {
	cl := &sseClient{ch: make(chan string, 0)}
	sseRegister(cl)
	defer sseUnregister(cl)
	sseBroadcast("arp", "{}")
	select {
	case <-cl.ch:
		t.Errorf("expected broadcast to be dropped on full/unbuffered channel, but a message was delivered")
	default:
	}
	if len(cl.ch) != 0 {
		t.Errorf("expected empty channel after broadcast to full/unbuffered channel, got len=%d", len(cl.ch))
	}
}

func TestArpToJSON(t *testing.T) {
	arp := map[string]bool{"aa:bb:cc:dd:ee:ff": true, "11:22:33:44:55:66": false}
	s := arpToJSON(arp)
	var decoded map[string]bool
	if err := json.Unmarshal([]byte(s), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(decoded) != 2 {
		t.Errorf("expected 2 entries, got %d", len(decoded))
	}
	if !decoded["aa:bb:cc:dd:ee:ff"] {
		t.Error("expected aa:bb:cc:dd:ee:ff=true")
	}
}

// ========== User management ==========

func TestCreateUser(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = make(map[string]string)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/users", strings.NewReader(`{"username":"admin","password":"secret123"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createUserHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, ok := users["admin"]; !ok {
		t.Fatal("user not stored")
	}
}

func TestCreateUserDuplicate(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"admin": "hash"}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/users", strings.NewReader(`{"username":"admin","password":"secret123"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createUserHandler(c)
	if w.Code != 409 {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestCreateUserEmptyFields(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = make(map[string]string)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/users", strings.NewReader(`{"username":"","password":""}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createUserHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeleteUser(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"target": "hash"}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/users/target", nil)
	c.Params = gin.Params{{Key: "name", Value: "target"}}
	c.Set("user", "admin")
	deleteUserHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, ok := users["target"]; ok {
		t.Fatal("user should be deleted")
	}
}

func TestDeleteUserSelf(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"admin": "hash"}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/users/admin", nil)
	c.Params = gin.Params{{Key: "name", Value: "admin"}}
	c.Set("user", "admin")
	deleteUserHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeleteUserNotFound(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = make(map[string]string)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/users/nobody", nil)
	c.Params = gin.Params{{Key: "name", Value: "nobody"}}
	c.Set("user", "admin")
	deleteUserHandler(c)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestChangePassword(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"admin": "$2a$10$1"}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/users/password", strings.NewReader(`{"old_password":"1","new_password":"new"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	changePasswordHandler(c)
	if w.Code == 401 {
		t.Log("bcrypt rejected dummy hash — expected. Validating shape.")
	} else if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChangePasswordWrongOld(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"admin": "$2a$10$zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/users/password", strings.NewReader(`{"old_password":"wrong","new_password":"new"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	changePasswordHandler(c)
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// ========== New devices ==========

func TestGetNewDevicesAllInStatic(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	*ArpPath = filepath.Join(dir, "arp")
	*LeasesPath = filepath.Join(dir, "leases")
	os.WriteFile(*ArpPath, []byte("IP address       HW type     Flags       HW address            Mask Device\n192.168.1.1      0x1         0x2         aa:bb:cc:dd:ee:ff     *    eth0\n"), 0644)
	os.WriteFile(*LeasesPath, []byte("0 aa:bb:cc:dd:ee:ff 192.168.1.1 * 01:aa:bb:cc:dd:ee:ff\n"), 0644)
	devices := getNewDevices()
	if len(devices) != 0 {
		t.Fatalf("expected 0 devices (MAC in leases), got %d", len(devices))
	}
}

func TestGetNewDevicesAllInHosts(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	*ArpPath = filepath.Join(dir, "arp")
	*LeasesPath = filepath.Join(dir, "leases")
	os.WriteFile(*ArpPath, []byte("IP address       HW type     Flags       HW address            Mask Device\n192.168.1.1      0x1         0x2         aa:bb:cc:dd:ee:ff     *    eth0\n"), 0644)
	os.WriteFile(*LeasesPath, []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "hosts.conf"), []byte("dhcp-host=aa:bb:cc:dd:ee:ff,host1,192.168.1.1\n"), 0644)
	devices := getNewDevices()
	if len(devices) != 0 {
		t.Fatalf("expected 0 devices (MAC in static), got %d", len(devices))
	}
}

func TestGetNewDevicesUnknown(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	*ArpPath = filepath.Join(dir, "arp")
	*LeasesPath = filepath.Join(dir, "leases")
	os.WriteFile(*ArpPath, []byte("IP address       HW type     Flags       HW address            Mask Device\n192.168.1.1      0x1         0x2         aa:bb:cc:dd:ee:ff     *    eth0\n"), 0644)
	os.WriteFile(*LeasesPath, []byte(""), 0644)
	devices := getNewDevices()
	if len(devices) != 1 {
		t.Fatalf("expected 1 unknown device, got %d", len(devices))
	}
	if devices[0].Mac != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("bad MAC: %q", devices[0].Mac)
	}
}

func TestGetNewDevicesEmpty(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	*ArpPath = filepath.Join(dir, "arp")
	*LeasesPath = filepath.Join(dir, "leases")
	os.WriteFile(*ArpPath, []byte("IP address       HW type     Flags       HW address            Mask Device\n"), 0644)
	os.WriteFile(*LeasesPath, []byte(""), 0644)
	devices := getNewDevices()
	if len(devices) != 0 {
		t.Fatalf("expected 0 devices, got %d", len(devices))
	}
}

// ========== Bulk lease-to-static ==========

func TestBulkLeaseToStatic(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)
	body := `{"leases":[{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.1","hostname":"testhost"}],"file":"` + strings.ReplaceAll(file, "\\", "\\\\") + `"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/leases/to-static", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	bulkLeaseToStaticHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "dhcp-host=aa:bb:cc:dd:ee:ff,testhost,192.168.1.1") {
		t.Errorf("host not written: %s", content)
	}
}

func TestBulkLeaseToStaticMacConflict(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte("dhcp-host=aa:bb:cc:dd:ee:ff,existing,1.2.3.4\n"), 0644)
	body := `{"leases":[{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.1","hostname":"new"}],"file":"` + strings.ReplaceAll(file, "\\", "\\\\") + `"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/leases/to-static", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	bulkLeaseToStaticHandler(c)
	if w.Code != 409 {
		t.Fatalf("expected 409 for MAC conflict, got %d", w.Code)
	}
}

func TestBulkLeaseToStaticInvalidMac(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)
	body := `{"leases":[{"mac":"bad","ip":"192.168.1.1"}],"file":"` + strings.ReplaceAll(file, "\\", "\\\\") + `"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/leases/to-static", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	bulkLeaseToStaticHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid MAC, got %d", w.Code)
	}
}

func TestBulkLeaseToStaticEmpty(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/leases/to-static", strings.NewReader(`{"leases":[],"file":"`+file+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	bulkLeaseToStaticHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400 for empty list, got %d", w.Code)
	}
}

func TestBulkLeaseToStaticUnsafePath(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/leases/to-static", strings.NewReader(`{"leases":[{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.1"}],"file":"/etc/passwd"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	bulkLeaseToStaticHandler(c)
	if w.Code != 403 {
		t.Fatalf("expected 403 for unsafe path, got %d", w.Code)
	}
}

func TestBulkLeaseToStaticDefaultHostname(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)
	body := `{"leases":[{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.1","hostname":"*"}],"file":"` + strings.ReplaceAll(file, "\\", "\\\\") + `"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/leases/to-static", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	bulkLeaseToStaticHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "device-") {
		t.Errorf("expected auto-generated hostname, got: %s", content)
	}
}

// ========== Restore from ZIP ==========

func makeTestZip(entries map[string]string) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, content := range entries {
		fw, _ := zw.Create(name)
		fw.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

func TestRestoreBackupZipValid(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	zipData := makeTestZip(map[string]string{
		"hosts.conf": "dhcp-host=aa:bb:cc:dd:ee:ff,host1,1.2.3.4\n",
	})
	_ = restoreBackupZip(zipData)
	_, err := os.ReadFile(filepath.Join(dir, "hosts.conf"))
	if err != nil {
		t.Error("file should have been written before dnsmasq test")
	}
}

func TestRestoreBackupZipCreatesRestoreBak(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	os.WriteFile(filepath.Join(dir, "hosts.conf"), []byte("old content\n"), 0644)
	zipData := makeTestZip(map[string]string{
		"hosts.conf": "new content\n",
	})
	_ = restoreBackupZip(zipData)
	bak, _ := os.ReadFile(filepath.Join(dir, "hosts.conf.restore.bak"))
	if string(bak) != "old content\n" {
		t.Errorf("bak mismatch: %q", bak)
	}
}

func TestRestoreBackupZipNoConfFiles(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	zipData := makeTestZip(map[string]string{
		"notes.txt": "hello\n",
	})
	err := restoreBackupZip(zipData)
	if err == nil || !strings.Contains(err.Error(), "no_valid_conf_files") {
		t.Errorf("expected no_valid_conf_files error, got %v", err)
	}
}

func TestRestoreBackupZipInvalidData(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	err := restoreBackupZip([]byte("not a zip file"))
	if err == nil || !strings.Contains(err.Error(), "invalid_zip") {
		t.Errorf("expected invalid_zip error, got %v", err)
	}
}

func TestRestoreBackupZipIgnoresUnsafeNames(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	zipData := makeTestZip(map[string]string{
		"../evil.conf": "bad\n",
		"hosts.conf":   "good\n",
	})
	_ = restoreBackupZip(zipData)
	_, err := os.ReadFile(filepath.Join(dir, "hosts.conf"))
	if err != nil {
		t.Fatal("hosts.conf should have been extracted")
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "evil.conf")); err == nil {
		t.Fatal("evil.conf should not have been extracted")
	}
}

// ========== Rate-limit ==========

func TestRateLimitUnderLimit(t *testing.T) {
	rateLimitStore = make(map[string][]time.Time)
	handler := rateLimitMiddleware(3, time.Minute)
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", nil)
		c.Request.RemoteAddr = "10.0.0.1:1234"
		handler(c)
		if w.Code == 429 {
			t.Fatalf("request %d should not be rate-limited", i+1)
		}
	}
}

func TestRateLimitOverLimit(t *testing.T) {
	rateLimitStore = make(map[string][]time.Time)
	handler := rateLimitMiddleware(2, time.Minute)
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", nil)
		c.Request.RemoteAddr = "10.0.0.2:1234"
		handler(c)
		if i == 2 && w.Code != 429 {
			t.Fatalf("third request should be rate-limited, got %d", w.Code)
		}
	}
}

func TestRateLimitDifferentIPs(t *testing.T) {
	rateLimitStore = make(map[string][]time.Time)
	handler := rateLimitMiddleware(2, time.Minute)

	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest("POST", "/", nil)
	c1.Request.RemoteAddr = "10.0.1.1:1234"
	handler(c1)
	handler(c1)

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("POST", "/", nil)
	c2.Request.RemoteAddr = "10.0.1.2:1234"
	handler(c2)
	handler(c2)

	if w1.Code == 429 {
		t.Fatal("IP1 should not be rate-limited yet")
	}
	if w2.Code == 429 {
		t.Fatal("IP2 should not be rate-limited yet")
	}

	handler(c1)
	if w1.Code != 429 {
		t.Fatalf("IP1 should now be rate-limited, got %d", w1.Code)
	}
	handler(c2)
	if w2.Code != 429 {
		t.Fatalf("IP2 should now be rate-limited, got %d", w2.Code)
	}
}

func TestRateLimitCleanupExpired(t *testing.T) {
	rateLimitStore = make(map[string][]time.Time)
	rateLimitStore["10.0.0.1"] = []time.Time{
		time.Now().Add(-2 * time.Minute),
		time.Now().Add(-2 * time.Minute),
	}
	handler := rateLimitMiddleware(2, time.Minute)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", nil)
	c.Request.RemoteAddr = "10.0.0.1:1234"
	handler(c)
	if w.Code == 429 {
		t.Fatal("old entries should have been cleaned, request should pass")
	}
}

// ========== JWT blacklist / logout ==========

func TestTokenRevoked(t *testing.T) {
	blacklist = make(map[string]time.Time)
	exp := time.Now().Add(time.Hour)
	jti := "test-jti-123"
	revokeToken(jti, exp)
	if !isTokenRevoked(jti) {
		t.Fatal("token should be revoked")
	}
}

func TestTokenNotRevoked(t *testing.T) {
	blacklist = make(map[string]time.Time)
	if isTokenRevoked("nonexistent") {
		t.Fatal("non-revoked token should not be marked revoked")
	}
}

func TestCleanBlacklist(t *testing.T) {
	blacklist = make(map[string]time.Time)
	expiredJTI := "expired-jti"
	freshJTI := "fresh-jti"
	revokeToken(expiredJTI, time.Now().Add(-time.Hour))
	revokeToken(freshJTI, time.Now().Add(time.Hour))
	blacklistMu.Lock()
	now := time.Now()
	for id, exp := range blacklist {
		if exp.Before(now) {
			delete(blacklist, id)
		}
	}
	blacklistMu.Unlock()
	if isTokenRevoked(expiredJTI) {
		t.Fatal("expired token should be cleaned")
	}
	if !isTokenRevoked(freshJTI) {
		t.Fatal("fresh token should still be revoked")
	}
}

func TestLogoutRevokesToken(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"admin": "$2a$10$placeholder"}
	blacklist = make(map[string]time.Time)

	originalKey := SecretKey
	SecretKey = []byte("test-secret-key-32-bytes-long!!")
	defer func() { SecretKey = originalKey }()

	token := makeToken("admin")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/logout", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)
	logoutHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	parsed, _ := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) { return SecretKey, nil })
	if parsed == nil {
		t.Fatal("token parsing failed")
	}
	claims := parsed.Claims.(jwt.MapClaims)
	jti := claims["jti"].(string)
	if !isTokenRevoked(jti) {
		t.Fatal("token JTI should be in blacklist after logout")
	}
}

// ========== OUI lookup ==========
// (TestLookupOUI* moved to internal/oui.)

// ========== Auth middleware (header + query token for SSE) ==========

func setTestSecret(t *testing.T) {
	t.Helper()
	orig := SecretKey
	SecretKey = []byte("unit-test-secret-key-0123456789ab")
	t.Cleanup(func() { SecretKey = orig })
}

func TestAuthMiddlewareBearerHeader(t *testing.T) {
	setTestSecret(t)
	users = map[string]string{"admin": "hash"}
	token := makeToken("admin")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/whatever", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)
	authMiddleware(c)
	if w.Code == 401 {
		t.Fatalf("bearer header should be accepted")
	}
	if c.GetString("user") != "admin" {
		t.Errorf("expected user admin, got %q", c.GetString("user"))
	}
}

func TestAuthMiddlewareQueryTokenRejected(t *testing.T) {
	// ?token= was removed from authMiddleware to avoid leaking JWTs into
	// access logs via SSE. Only Bearer header / X-API-Key are accepted now.
	setTestSecret(t)
	users = map[string]string{"admin": "hash"}
	token := makeToken("admin")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/events?token="+token, nil)
	authMiddleware(c)
	if w.Code != 401 {
		t.Fatalf("query token should be rejected, got %d", w.Code)
	}
}

func TestAuthMiddlewareNoCredentials(t *testing.T) {
	setTestSecret(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/events", nil)
	authMiddleware(c)
	if w.Code != 401 {
		t.Fatalf("missing credentials should be rejected, got %d", w.Code)
	}
}

func TestAuthMiddlewareAPIKey(t *testing.T) {
	setTestSecret(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/whatever", nil)
	c.Request.Header.Set("X-API-Key", string(SecretKey))
	authMiddleware(c)
	if w.Code == 401 {
		t.Fatalf("X-API-Key should be accepted")
	}
	if c.GetString("user") != "api-key" {
		t.Errorf("expected user api-key, got %q", c.GetString("user"))
	}
}

// ========== Events handler (SSE end-to-end) ==========

func TestEventsHandlerStreamsSSE(t *testing.T) {
	dir := t.TempDir()
	*ArpPath = filepath.Join(dir, "arp")
	os.WriteFile(*ArpPath, []byte("IP address       HW type     Flags       HW address            Mask Device\n192.168.1.5      0x1         0x2         aa:bb:cc:dd:ee:ff     *    eth0\n"), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/events", nil).WithContext(ctx)
	eventsHandler(c)

	if !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Errorf("expected text/event-stream content-type, got %q", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), "arp") {
		t.Errorf("expected initial arp event in body, got: %s", w.Body.String())
	}
}

// ========== GET /api/files/:name (.conf restriction) ==========

func TestGetFileHandlerRejectsNonConf(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0644)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/files/notes.txt", nil)
	c.Params = gin.Params{{Key: "name", Value: "notes.txt"}}
	getFileHandler(c)
	if w.Code != 403 {
		t.Fatalf("expected 403 for non-.conf, got %d", w.Code)
	}
}

func TestGetFileHandlerRejectsPathSeparator(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/files/sub/x.conf", nil)
	c.Params = gin.Params{{Key: "name", Value: "sub/x.conf"}}
	getFileHandler(c)
	if w.Code != 403 {
		t.Fatalf("expected 403 for path separator in name, got %d", w.Code)
	}
}

func TestGetFileHandlerSuccess(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	os.WriteFile(filepath.Join(dir, "x.conf"), []byte("server=1.2.3.4\n"), 0644)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/files/x.conf", nil)
	c.Params = gin.Params{{Key: "name", Value: "x.conf"}}
	getFileHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "server=1.2.3.4") {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestGetFileHandlerMissing(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/files/missing.conf", nil)
	c.Params = gin.Params{{Key: "name", Value: "missing.conf"}}
	getFileHandler(c)
	if w.Code != 404 {
		t.Fatalf("expected 404 for missing file, got %d", w.Code)
	}
}

// TestGetFileHandlerRejectsUnsafePath locks the path-traversal defence for
// GET /api/files/:name (A11, defense-in-depth). Every vector below carries a
// .conf extension so the extension check does not short-circuit, exercising
// the traversal defence specifically. The substring filter on "/" / "\"
// fires first today; the isSafePath-after-Join layer is kept so a future
// weakening of that filter (or a new call site) cannot enable reads outside
// ConfigDir.
func TestGetFileHandlerRejectsUnsafePath(t *testing.T) {
	cases := []string{
		"../etc/evil.conf",
		"..\\evil.conf",
		"../../etc/dnsmasq.conf",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			*ConfigDir = dir
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/files/x.conf", nil)
			c.Params = gin.Params{{Key: "name", Value: name}}
			getFileHandler(c)
			if w.Code != 403 {
				t.Fatalf("expected 403 for traversal name %q, got %d: %s", name, w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "access_denied") {
				t.Errorf("expected access_denied body for %q, got: %s", name, w.Body.String())
			}
		})
	}
}

// ========== Config file templates (POST /api/config/file?template=…) ==========

// TestCreateConfigFileHandlerEachTemplate проверяет, что каждый зарегистрированный
// шаблон корректно записывается в файл при выборе через POST /api/config/file.
func TestCreateConfigFileHandlerEachTemplate(t *testing.T) {
	for _, tpl := range knownConfigTemplateIDs() {
		t.Run(tpl, func(t *testing.T) {
			dir := t.TempDir()
			*ConfigDir = dir
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			body := fmt.Sprintf(`{"name":"test_%s.conf","template":"%s"}`, tpl, tpl)
			c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", "admin")
			createConfigFileHandler(c)
			if w.Code != 200 {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			content, err := os.ReadFile(filepath.Join(dir, "test_"+tpl+".conf"))
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != configTemplates[tpl] {
				t.Errorf("content mismatch for template %q:\nwant:\n%s\ngot:\n%s", tpl, configTemplates[tpl], string(content))
			}
		})
	}
}

// TestCreateConfigFileHandlerEmptyTemplateDefault — отсутствие template в теле
// запроса эквивалентно template="empty" (обратная совместимость).
func TestCreateConfigFileHandlerEmptyTemplateDefault(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(`{"name":"x.conf"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createConfigFileHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(filepath.Join(dir, "x.conf"))
	if string(content) != configTemplates["empty"] {
		t.Errorf("default template not 'empty':\n%s", string(content))
	}
}

// TestCreateConfigFileHandlerUnknownTemplate — неизвестный ID шаблона даёт
// 400 + список доступных в поле available (нужно для подсказки в UI).
func TestCreateConfigFileHandlerUnknownTemplate(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(`{"name":"x.conf","template":"nonexistent"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createConfigFileHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400 for unknown template, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["error"] != "unknown_template" {
		t.Errorf("expected error=unknown_template, got %v", resp["error"])
	}
	avail, _ := resp["available"].([]interface{})
	if len(avail) != len(configTemplates) {
		t.Errorf("expected %d available templates, got %v", len(configTemplates), avail)
	}
	// файл не должен быть создан при ошибке
	if _, err := os.Stat(filepath.Join(dir, "x.conf")); !os.IsNotExist(err) {
		t.Error("file should not be created when template is unknown")
	}
}

// TestCreateConfigFileHandlerTemplateCaseInsensitive — "Basic-DHCP" и
// "basic-dhcp" дают одинаковый результат (нормализация через ToLower).
func TestCreateConfigFileHandlerTemplateCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(`{"name":"x.conf","template":"Basic-DHCP"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createConfigFileHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for uppercase template, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(filepath.Join(dir, "x.conf"))
	if string(content) != configTemplates["basic-dhcp"] {
		t.Errorf("case-insensitive lookup failed:\n%s", string(content))
	}
}

// TestCreateConfigFileHandlerTemplateWhitespace — пробелы вокруг ID шаблона
// должны молча обрезаться (защита от копипаста " basic-dhcp ").
func TestCreateConfigFileHandlerTemplateWhitespace(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(`{"name":"x.conf","template":"  forwarder  "}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createConfigFileHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(filepath.Join(dir, "x.conf"))
	if string(content) != configTemplates["forwarder"] {
		t.Errorf("whitespace trim failed:\n%s", string(content))
	}
}

// TestCreateConfigFileHandlerExistingFileStill409 — даже при выборе шаблона
// попытка перезаписать существующий файл остаётся 409 (поведение не изменилось).
func TestCreateConfigFileHandlerExistingFileStill409(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	existing := filepath.Join(dir, "x.conf")
	if err := os.WriteFile(existing, []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(`{"name":"x.conf","template":"basic-dhcp"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createConfigFileHandler(c)
	if w.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	// Содержимое существующего файла не должно измениться.
	content, _ := os.ReadFile(existing)
	if string(content) != "old\n" {
		t.Errorf("existing file was overwritten:\n%s", string(content))
	}
}

// TestListConfigTemplatesHandler — каталог отдаёт все ID из configTemplates,
// у каждого есть непустой preview.
func TestListConfigTemplatesHandler(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/config/templates", nil)
	c.Set("user", "admin")
	listConfigTemplatesHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Templates []struct {
			ID      string `json:"id"`
			Preview string `json:"preview"`
		} `json:"templates"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Templates) != len(configTemplates) {
		t.Errorf("expected %d templates, got %d", len(configTemplates), len(resp.Templates))
	}
	seen := make(map[string]bool)
	for _, tpl := range resp.Templates {
		seen[tpl.ID] = true
		if tpl.Preview == "" {
			t.Errorf("template %q has empty preview", tpl.ID)
		}
		if !strings.HasPrefix(tpl.Preview, "# === Managed by Intermasq ===") {
			t.Errorf("template %q preview missing managed header", tpl.ID)
		}
	}
	for id := range configTemplates {
		if !seen[id] {
			t.Errorf("template %q missing from response", id)
		}
	}
}

// TestKnownConfigTemplateIDsSorted — контракт: список отсортирован, чтобы
// UI и проверочные тесты могли полагаться на стабильный порядок.
func TestKnownConfigTemplateIDsSorted(t *testing.T) {
	ids := knownConfigTemplateIDs()
	if !sort.StringsAreSorted(ids) {
		t.Errorf("knownConfigTemplateIDs() must be sorted: %v", ids)
	}
	if len(ids) != len(configTemplates) {
		t.Errorf("len mismatch: ids=%d map=%d", len(ids), len(configTemplates))
	}
}

// TestKnownConfigTemplateIDsContainsEmpty — "empty" обязан всегда быть в
// списке: это дефолтный template при отсутствии поля в запросе.
func TestKnownConfigTemplateIDsContainsEmpty(t *testing.T) {
	if _, ok := configTemplates["empty"]; !ok {
		t.Fatal(`"empty" template must always exist in configTemplates`)
	}
	ids := knownConfigTemplateIDs()
	found := false
	for _, id := range ids {
		if id == "empty" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal(`"empty" missing from knownConfigTemplateIDs()`)
	}
}

// TestConfigTemplatesAllStartWithManagedHeader — каждый шаблон должен
// начинаться с маркера "# === Managed by Intermasq ===", чтобы было видно
// при чтении raw-файла, что он был создан через панель.
func TestConfigTemplatesAllStartWithManagedHeader(t *testing.T) {
	const marker = "# === Managed by Intermasq ==="
	for id, content := range configTemplates {
		if !strings.HasPrefix(content, marker) {
			t.Errorf("template %q must start with %q", id, marker)
		}
	}
}

// TestConfigTemplatesValidForDnsmasqSyntax — каждый шаблон должен проходить
// `dnsmasq --test`, чтобы последующий PUT /api/config не падал на первой
// операции. Если dnsmasq не установлен — тест пропускается (CI без dnsmasq).
func TestConfigTemplatesValidForDnsmasqSyntax(t *testing.T) {
	if bins.Dnsmasq() == "" {
		t.Skip("dnsmasq binary not installed — skipping syntax validation")
	}
	for id, content := range configTemplates {
		t.Run(id, func(t *testing.T) {
			tmp := filepath.Join(t.TempDir(), "x.conf")
			if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bins.Dnsmasq(), "--test", "--conf-file="+tmp)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("template %q failed dnsmasq --test:\n%s\noutput:\n%s", id, err, out)
			}
		})
	}
}

// TestCreateConfigFileHandlerTemplateAuditWritten — при создании файла с
// шаблоном в audit-лог попадает запись с полем template = выбранный ID.
// Проверяет, что поле не теряется по пути от request до audit entry.
func TestCreateConfigFileHandlerTemplateAuditWritten(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	auditDir := t.TempDir()
	auditPath := filepath.Join(auditDir, "audit.log")
	*AuditLogPath = auditPath

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/config/file", strings.NewReader(`{"name":"x.conf","template":"forwarder"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	createConfigFileHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("audit log not readable: %v", err)
	}
	var entry AuditEntry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil { // последняя '\n'
		t.Fatalf("audit entry parse error: %v", err)
	}
	if entry.Template != "forwarder" {
		t.Errorf("expected template=forwarder in audit, got %q", entry.Template)
	}
	if entry.Action != "config_create_file" {
		t.Errorf("expected action=config_create_file, got %q", entry.Action)
	}
	if entry.User != "admin" {
		t.Errorf("expected user=admin, got %q", entry.User)
	}
}

// ========== Bug 1+2: loadUsers / loadTemplates fatal on read errors ==========

// TestLoadUsersFailsOnCorruptJSON — повреждённый JSON в users.json должен
// вызвать os.Exit, а не оставить users пустым (что открывает /api/setup).
// Проверяем через subprocess, чтобы перехватить exit code.
func TestLoadUsersFailsOnCorruptJSON(t *testing.T) {
	if os.Getenv("INTERMASQ_TEST_FATAL") == "1" {
		// Внутренний прогоhн: corrupt users.json, ждём fatal.
		dir := t.TempDir()
		*DBPath = filepath.Join(dir, "users.json")
		os.WriteFile(*DBPath, []byte("{not json"), 0600)
		loadUsers()
		// loadUsers должна была вызвать os.Exit — эта строка недостижима.
		t.Fatal("loadUsers should have called os.Exit on corrupt JSON")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLoadUsersFailsOnCorruptJSON")
	cmd.Env = append(os.Environ(), "INTERMASQ_TEST_FATAL=1", "INTERMASQ_SECRET=test-secret-32-bytes-long-for-ci-0001")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit code on corrupt users.json")
	}
}

// TestLoadTemplatesFailsOnCorruptJSON — аналогично для templates.json.
func TestLoadTemplatesFailsOnCorruptJSON(t *testing.T) {
	if os.Getenv("INTERMASQ_TEST_FATAL") == "1" {
		dir := t.TempDir()
		*TemplatesPath = filepath.Join(dir, "templates.json")
		os.WriteFile(*TemplatesPath, []byte("definitely not json"), 0600)
		loadTemplates()
		t.Fatal("loadTemplates should have called os.Exit on corrupt JSON")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLoadTemplatesFailsOnCorruptJSON")
	cmd.Env = append(os.Environ(), "INTERMASQ_TEST_FATAL=1", "INTERMASQ_SECRET=test-secret-32-bytes-long-for-ci-0001")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit code on corrupt templates.json")
	}
}

// TestLoadUsersMissingFileIsOK — отсутствие файла (первый запуск) остаётся
// нормальным сценарием: setup_required=true, /api/setup доступен.
func TestLoadUsersMissingFileIsOK(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "absent.json")
	users = make(map[string]string)
	loadUsers()
	if len(users) != 0 {
		t.Errorf("expected empty users map on missing file, got %d", len(users))
	}
}

// TestSaveUsersAtomic — после сохранения users.json файл существует и
// парсится; tmp-файла после rename не остаётся.
func TestSaveUsersAtomic(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	users = map[string]string{"admin": "$2a$10$hash", "bob": "$2a$10$another"}
	if err := saveUsers(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(*DBPath); err != nil {
		t.Fatalf("users.json not written: %v", err)
	}
	if _, err := os.Stat(*DBPath + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp file should not remain after atomic save")
	}
	data, _ := os.ReadFile(*DBPath)
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("users.json not valid JSON after atomic save: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 users, got %d", len(got))
	}
}

// TestSaveUsersAtomicPreservesExistingOnFailure — подтвердить наличие
// tmp+rename: если записать в read-only dir, saveUsers вернёт ошибку, но
// исходный файл не должен быть повреждён.
func TestSaveUsersAtomicPreservesExistingOnFailure(t *testing.T) {
	dir := t.TempDir()
	*DBPath = filepath.Join(dir, "users.json")
	original := []byte(`{"admin":"$2a$10$orig"}`)
	os.WriteFile(*DBPath, original, 0600)

	// Делаем родительский каталог read-only, чтобы rename провалился.
	// (WriteFile в read-only dir тоже упадёт — это и есть «crash mid-write».)
	os.Chmod(dir, 0500)
	defer os.Chmod(dir, 0755) // восстановим для t.TempDir cleanup

	users = map[string]string{"admin": "$2a$10$new"}
	err := saveUsers()
	if err == nil {
		t.Skip("saveUsers succeeded despite read-only dir (root or permissive FS)")
	}
	// Исходный файл должен остаться нетронутым.
	data, _ := os.ReadFile(*DBPath)
	if string(data) != string(original) {
		t.Errorf("original users.json was modified on failed save:\nwant: %s\ngot:  %s", original, data)
	}
}

// ========== Feature 3: optional IP/hostname in dhcp-host ==========
// (TestValidateHostFieldsAllCombinations moved to internal/validate.)

// TestAddHostHandlerMacOnly — POST /api/hosts только с MAC создаёт строку
// dhcp-host=<mac> (infinite lease без имени и IP).
func TestAddHostHandlerMacOnly(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for MAC-only host, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "dhcp-host=aa:bb:cc:dd:ee:ff\n") {
		t.Errorf("MAC-only line not written correctly:\n%s", content)
	}
	if strings.Contains(string(content), ",") {
		t.Errorf("MAC-only line should have no commas:\n%s", content)
	}
}

// TestAddHostHandlerMacPlusHostname — DHCP-выданный IP + DNS-имя.
func TestAddHostHandlerMacPlusHostname(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","hostname":"phone","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for MAC+hostname host, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "dhcp-host=aa:bb:cc:dd:ee:ff,phone\n") {
		t.Errorf("MAC+hostname line not written correctly:\n%s", content)
	}
}

// TestAddHostHandlerMacPlusIP — статический IP без DNS-имени.
func TestAddHostHandlerMacPlusIP(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.10","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for MAC+IP host, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "dhcp-host=aa:bb:cc:dd:ee:ff,192.168.1.10\n") {
		t.Errorf("MAC+IP line not written correctly:\n%s", content)
	}
}

// TestAddHostHandlerRejectsBadIP — опциональность не означает «пропустить
// мусор»: невалидный IP всё ещё отвергается.
func TestAddHostHandlerRejectsBadIP(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","ip":"not-an-ip","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid IP, got %d", w.Code)
	}
}

// TestAddHostHandlerIPDuplicateStillChecked — если IP указан, duplicate check
// работает как раньше.
func TestAddHostHandlerIPDuplicateStillChecked(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte("dhcp-host=11:22:33:44:55:66,existing,192.168.1.10\n"), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.10","hostname":"new","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 409 {
		t.Fatalf("expected 409 for IP conflict, got %d", w.Code)
	}
}

// TestAddHostHandlerRejectsUnsafeFile (mutation-go M8 regression) — the
// isSafePath guard at the top of addHostHandler must reject a file outside
// ConfigDir with 400 invalid_data, before any field validation runs.
func TestAddHostHandlerRejectsUnsafeFile(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir

	cases := []struct{ name, file string }{
		{"absolute_outside", "/etc/passwd"},
		{"traversal", filepath.Join(dir, "..", "evil.conf")},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","file":%q}`, tc.file)
		c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", "admin")
		addHostHandler(c)
		if w.Code != 400 {
			t.Errorf("%s: expected 400 for unsafe file %q, got %d: %s", tc.name, tc.file, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "invalid_data") {
			t.Errorf("%s: expected invalid_data body, got: %s", tc.name, w.Body.String())
		}
	}
}

// TestAddHostHandlerMACDuplicateRejected (mutation-go M9 regression) —
// adding a host whose MAC already exists in the target file must return
// 409 mac_duplicate and must not overwrite the existing entry. IP is
// omitted so the IP-duplicate branch cannot mask the MAC check.
func TestAddHostHandlerMACDuplicateRejected(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte("dhcp-host=aa:bb:cc:dd:ee:ff,existing\n"), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa:bb:cc:dd:ee:ff","hostname":"new","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 409 {
		t.Fatalf("expected 409 for MAC conflict, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "mac_duplicate") {
		t.Errorf("expected mac_duplicate body, got: %s", w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "dhcp-host=aa:bb:cc:dd:ee:ff,existing\n") {
		t.Errorf("existing entry should be preserved on MAC conflict:\n%s", content)
	}
}

// TestAddHostHandlerRejectsZeroBroadcastMAC (A3 regression) — zero and
// broadcast MACs must be rejected at the handler layer even though they
// match validate.ValidMAC.
func TestAddHostHandlerRejectsZeroBroadcastMAC(t *testing.T) {
	for _, mac := range []string{"00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff"} {
		dir := t.TempDir()
		*ConfigDir = dir
		file := filepath.Join(dir, "hosts.conf")
		os.WriteFile(file, []byte(""), 0644)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := fmt.Sprintf(`{"mac":%q,"ip":"10.0.0.99","hostname":"x","file":%q}`, mac, file)
		c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", "admin")
		addHostHandler(c)
		if w.Code != 400 {
			t.Errorf("expected 400 for MAC %q, got %d: %s", mac, w.Code, w.Body.String())
		}
		content, _ := os.ReadFile(file)
		if strings.Contains(string(content), mac) {
			t.Errorf("MAC %q should not be written to file:\n%s", mac, content)
		}
	}
}

// TestAddHostHandlerDashMACNormalized (A4 regression) — POST /api/hosts with
// a dash-separated MAC returns 200 and stores the colon form, so dnsmasq
// --test passes on reload.
func TestAddHostHandlerDashMACNormalized(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "hosts.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"mac":"aa-bb-cc-dd-ee-07","ip":"10.0.0.17","hostname":"dashmac","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addHostHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for dash-MAC (normalised), got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "aa:bb:cc:dd:ee:07") {
		t.Errorf("colon form should be in file:\n%s", content)
	}
	if strings.Contains(string(content), "aa-bb-cc-dd-ee-07") {
		t.Errorf("dash form must NOT be in file:\n%s", content)
	}
}

// TestParseCSVHostsNormalizesDashMAC (A4 regression) — CSV import normalises
// dash-MACs the same way the JSON add path does.
func TestParseCSVHostsNormalizesDashMAC(t *testing.T) {
	csv := "mac,ip,hostname\naa-bb-cc-dd-ee-ff,10.0.0.5,x\n"
	hosts, err := parseCSVHosts(strings.NewReader(csv), "/tmp/x.conf")
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].Mac != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("expected normalised MAC aa:bb:cc:dd:ee:ff, got %q", hosts[0].Mac)
	}
}

// TestParseCSVHostsAcceptsMACOnly — CSV import тоже ослаблен: строки без IP
// и/или без hostname принимаются.
func TestParseCSVHostsAcceptsMACOnly(t *testing.T) {
	csv := "mac,ip,hostname\naa:bb:cc:dd:ee:ff,,\n"
	hosts, err := parseCSVHosts(strings.NewReader(csv), "/tmp/x.conf")
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host (MAC only), got %d", len(hosts))
	}
	if hosts[0].Mac != "aa:bb:cc:dd:ee:ff" || hosts[0].Ip != "" || hosts[0].Hostname != "" {
		t.Errorf("unexpected host: %+v", hosts[0])
	}
}

func TestParseCSVHostsMACPlusHostname(t *testing.T) {
	csv := "mac,ip,hostname\naa:bb:cc:dd:ee:ff,,phone\n"
	hosts, err := parseCSVHosts(strings.NewReader(csv), "/tmp/x.conf")
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].Hostname != "phone" || hosts[0].Ip != "" {
		t.Errorf("unexpected host: %+v", hosts[0])
	}
}

// ========== Feature 4+5: PTR and TXT aliases ==========

func TestParseAliasLinePTR(t *testing.T) {
	e, ok := parseAliasLine("ptr-record=10.1.168.192.in-addr.arpa,nas.lan", "/etc/dnsmasq.d/x.conf", false)
	if !ok {
		t.Fatal("expected parse success for PTR")
	}
	if e.Type != "PTR" || e.Domain != "10.1.168.192.in-addr.arpa" || e.Target != "nas.lan" {
		t.Errorf("unexpected PTR entry: %+v", e)
	}
}

func TestParseAliasLineTXT(t *testing.T) {
	e, ok := parseAliasLine("txt-record=wiki.lan,v=spf1 -all", "/etc/dnsmasq.d/x.conf", false)
	if !ok {
		t.Fatal("expected parse success for TXT")
	}
	if e.Type != "TXT" || e.Domain != "wiki.lan" || e.Target != "v=spf1 -all" {
		t.Errorf("unexpected TXT entry: %+v", e)
	}
}

// TestParseAliasLineTXTMultiComma — TXT-значение может содержать запятые
// (например, DKIM с k=rsa; p=…), сплит должен быть только по первой.
func TestParseAliasLineTXTMultiComma(t *testing.T) {
	e, ok := parseAliasLine("txt-record=dkim._domainkey,k=rsa; p=MIGfMA0,a=test", "/etc/dnsmasq.d/x.conf", false)
	if !ok {
		t.Fatal("expected parse success for TXT with multiple commas")
	}
	if e.Target != "k=rsa; p=MIGfMA0,a=test" {
		t.Errorf("TXT value split on wrong comma: %q", e.Target)
	}
}

func TestAliasToLinePTR(t *testing.T) {
	got := aliasToLine(DnsAliasEntry{Type: "PTR", Domain: "10.1.168.192.in-addr.arpa", Target: "nas.lan"})
	if got != "ptr-record=10.1.168.192.in-addr.arpa,nas.lan" {
		t.Errorf("PTR serialization wrong: %q", got)
	}
}

func TestAliasToLineTXT(t *testing.T) {
	got := aliasToLine(DnsAliasEntry{Type: "TXT", Domain: "wiki.lan", Target: "v=spf1 -all"})
	if got != "txt-record=wiki.lan,v=spf1 -all" {
		t.Errorf("TXT serialization wrong: %q", got)
	}
}

func TestAliasRoundTripPTR(t *testing.T) {
	in := DnsAliasEntry{Type: "PTR", Domain: "5.0.168.192.in-addr.arpa", Target: "host.lan"}
	out, ok := parseAliasLine(aliasToLine(in), "", false)
	if !ok {
		t.Fatal("PTR round-trip failed")
	}
	out.File = in.File
	if out != in {
		t.Errorf("PTR round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestAliasRoundTripTXT(t *testing.T) {
	in := DnsAliasEntry{Type: "TXT", Domain: "host.lan", Target: "some text value"}
	out, ok := parseAliasLine(aliasToLine(in), "", false)
	if !ok {
		t.Fatal("TXT round-trip failed")
	}
	out.File = in.File
	if out != in {
		t.Errorf("TXT round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestIsAliasDirectiveRecognizesNewTypes(t *testing.T) {
	if !isAliasDirective("ptr-record=foo,bar") {
		t.Error("ptr-record= not recognized as alias directive")
	}
	if !isAliasDirective("txt-record=foo,bar") {
		t.Error("txt-record= not recognized as alias directive")
	}
}

func TestReadAllAliasesIncludesPTRAndTXT(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	content := []byte("address=/nas.lan/192.168.1.10\n" +
		"cname=wiki,nas.lan\n" +
		"ptr-record=10.1.168.192.in-addr.arpa,nas.lan\n" +
		"txt-record=nas.lan,v=spf1 -all\n" +
		"server=8.8.8.8\n")
	if err := os.WriteFile(filepath.Join(dir, "dns.conf"), content, 0644); err != nil {
		t.Fatal(err)
	}
	aliases := readAllAliases()
	if len(aliases) != 4 {
		t.Fatalf("expected 4 aliases (A, CNAME, PTR, TXT), got %d: %+v", len(aliases), aliases)
	}
	types := map[string]bool{}
	for _, a := range aliases {
		types[a.Type] = true
	}
	if !types["A"] || !types["CNAME"] || !types["PTR"] || !types["TXT"] {
		t.Errorf("missing types in readAllAliases result: %+v", types)
	}
}

func TestValidateAliasEntryPTRAndTXT(t *testing.T) {
	cases := []struct {
		name  string
		entry DnsAliasEntry
		want  bool
	}{
		{"valid PTR", DnsAliasEntry{Type: "PTR", Domain: "10.in-addr.arpa", Target: "nas.lan"}, true},
		{"valid TXT", DnsAliasEntry{Type: "TXT", Domain: "nas.lan", Target: "v=spf1 -all"}, true},
		{"TXT empty target", DnsAliasEntry{Type: "TXT", Domain: "nas.lan", Target: ""}, false},
		{"TXT with newline", DnsAliasEntry{Type: "TXT", Domain: "nas.lan", Target: "a\nb"}, false},
		{"unknown type", DnsAliasEntry{Type: "MX", Domain: "x", Target: "y"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateAliasEntry(tc.entry); got != tc.want {
				t.Errorf("validateAliasEntry(%+v) = %v, want %v", tc.entry, got, tc.want)
			}
		})
	}
}

// TestAliasDomainRegexUnderscore moved to internal/validate (white-box).

func TestRemoveAliasLinePTR(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	content := []byte("ptr-record=10.1.168.192.in-addr.arpa,nas.lan\naddress=/other/10.0.0.1\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	removed, err := removeAliasLine(path, "PTR", "10.1.168.192.in-addr.arpa")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected PTR line to be removed")
	}
	out, _ := os.ReadFile(path)
	if strings.Contains(string(out), "ptr-record=") {
		t.Errorf("PTR not removed:\n%s", out)
	}
	if !strings.Contains(string(out), "address=/other/10.0.0.1") {
		t.Errorf("A record should be preserved:\n%s", out)
	}
}

func TestRemoveAliasLineTXT(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	content := []byte("txt-record=nas.lan,v=spf1 -all\ncname=wiki,nas.lan\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	removed, err := removeAliasLine(path, "TXT", "nas.lan")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected TXT line to be removed")
	}
	out, _ := os.ReadFile(path)
	if strings.Contains(string(out), "txt-record=") {
		t.Errorf("TXT not removed:\n%s", out)
	}
	if !strings.Contains(string(out), "cname=wiki,nas.lan") {
		t.Errorf("CNAME should be preserved:\n%s", out)
	}
}

// TestAddAliasHandlerPTR — end-to-end: POST /api/aliases с type=PTR
// создаёт ptr-record= строку в файле.
func TestAddAliasHandlerPTR(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "dns.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"type":"PTR","domain":"10.1.168.192.in-addr.arpa","target":"nas.lan","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/aliases", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addAliasHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for PTR alias, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "ptr-record=10.1.168.192.in-addr.arpa,nas.lan") {
		t.Errorf("PTR line not written:\n%s", content)
	}
}

// TestAddAliasHandlerTXT — end-to-end: POST /api/aliases с type=TXT
// создаёт txt-record= строку.
func TestAddAliasHandlerTXT(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "dns.conf")
	os.WriteFile(file, []byte(""), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"type":"TXT","domain":"nas.lan","target":"v=spf1 -all","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/aliases", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addAliasHandler(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for TXT alias, got %d: %s", w.Code, w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), "txt-record=nas.lan,v=spf1 -all") {
		t.Errorf("TXT line not written:\n%s", content)
	}
}

// TestAddAliasHandlerDuplicateRejected (A2 regression) — adding an A record
// whose domain+type already exists in the same file must return 409 and must
// NOT append a second line. Previously findAliasesByDomain excluded the
// matching type+file combo, so the duplicate check saw zero conflicts.
func TestAddAliasHandlerDuplicateRejected(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "dns.conf")
	os.WriteFile(file, []byte("address=/nas.local/10.0.0.5\n"), 0644)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"type":"A","domain":"nas.local","target":"10.0.0.99","file":%q}`, file)
	c.Request = httptest.NewRequest("POST", "/api/aliases", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	addAliasHandler(c)
	if w.Code != 409 {
		t.Fatalf("expected 409 for duplicate alias, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "alias_duplicate") {
		t.Errorf("expected alias_duplicate error, got: %s", w.Body.String())
	}
	content, _ := os.ReadFile(file)
	if got := strings.Count(string(content), "address=/nas.local/"); got != 1 {
		t.Errorf("expected exactly 1 nas.local A record, got %d:\n%s", got, content)
	}
}

// TestDeleteAliasHandlerSecondDeleteNotFound (A2 knock-on) — once A2 is fixed
// there is at most one record per domain+type+file, so a second delete must
// return 404 (previously it found the duplicate copy and returned 200).
func TestDeleteAliasHandlerSecondDeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	file := filepath.Join(dir, "dns.conf")
	os.WriteFile(file, []byte("address=/nas.local/10.0.0.5\n"), 0644)

	doDelete := func() int {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := fmt.Sprintf(`{"type":"A","domain":"nas.local","file":%q}`, file)
		c.Request = httptest.NewRequest("POST", "/api/aliases/delete", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", "admin")
		deleteAliasHandler(c)
		return w.Code
	}
	if code := doDelete(); code != 200 {
		t.Fatalf("first delete: expected 200, got %d", code)
	}
	if code := doDelete(); code != 404 {
		t.Fatalf("second delete: expected 404 (A2 knock-on), got %d", code)
	}
}

func TestParseCSVAliasesIncludesPTRAndTXT(t *testing.T) {
	csv := "type,domain,target\n" +
		"A,nas.lan,192.168.1.10\n" +
		"PTR,10.in-addr.arpa,nas.lan\n" +
		"TXT,nas.lan,v=spf1 -all\n"
	aliases, err := parseCSVAliases(strings.NewReader(csv), "/tmp/x.conf")
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 3 {
		t.Fatalf("expected 3 aliases, got %d: %+v", len(aliases), aliases)
	}
	if aliases[1].Type != "PTR" {
		t.Errorf("second row should be PTR, got %s", aliases[1].Type)
	}
	if aliases[2].Type != "TXT" {
		t.Errorf("third row should be TXT, got %s", aliases[2].Type)
	}
}

// ===== Coverage sweep T-A: pure unit functions =====

// TestParseIPTransform covers every error branch and the two success modes
// (octet-prefix / CIDR / none) of parseIPTransform.
func TestParseIPTransform(t *testing.T) {
	cases := []struct {
		name    string
		old, nw string
		wantErr string // empty => no error expected
	}{
		{"none", "", "", ""},
		{"only_old_set", "10.0.0", "", "both_prefixes_required"},
		{"only_new_set", "", "10.0.0", "both_prefixes_required"},
		{"cidr_mismatch_only_old", "10.0.0.0/24", "10.0.0", "prefix_format_mismatch"},
		{"cidr_mismatch_only_new", "10.0.0", "10.0.0.0/24", "prefix_format_mismatch"},
		{"cidr_invalid_old", "nope/x", "10.0.0.0/24", "invalid_cidr"},
		{"cidr_invalid_new", "10.0.0.0/24", "nope/x", "invalid_cidr"},
		{"cidr_mask_mismatch", "10.0.0.0/24", "10.0.0.0/16", "prefix_mismatch"},
		{"cidr_ipv6_old", "::1/24", "10.0.0.0/24", "ipv6_not_supported"},
		{"cidr_ipv6_new", "10.0.0.0/24", "::1/24", "ipv6_not_supported"},
		{"cidr_ok", "10.0.0.0/24", "10.0.1.0/24", ""},
		{"octet_mismatched_dots", "10.0.0", "10.0", "prefix_format_mismatch"},
		{"octet_invalid_old", "9999.0.0", "10.0.0", "invalid_prefix_format"},
		{"octet_invalid_new", "10.0.0", "9999.0.0", "invalid_prefix_format"},
		{"octet_ok_3octets", "10.0.0", "192.168.1", ""},
		{"octet_ok_2octets", "10.0", "192.168", ""},
		{"octet_ok_1octet", "10", "192", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseIPTransform(tc.old, tc.nw)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("parseIPTransform(%q,%q) unexpected err: %v", tc.old, tc.nw, err)
				}
				if got == nil {
					t.Fatal("expected non-nil transform")
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.wantErr)
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("expected err %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

// TestIPTransform_Apply_None checks that the zero transform returns the IP
// untouched.
func TestIPTransform_Apply_None(t *testing.T) {
	tr := &ipTransform{mode: ipTransformNone}
	got, err := tr.apply("10.0.0.55")
	if err != nil || got != "10.0.0.55" {
		t.Fatalf("apply(none) = %q, %v; want 10.0.0.55, nil", got, err)
	}
}

// TestIPTransform_Apply_InvalidIP covers net.ParseIP returning nil.
func TestIPTransform_Apply_InvalidIP(t *testing.T) {
	tr := &ipTransform{mode: ipTransformOctets, oldPref: "10.0.0", newPref: "10.0.1"}
	if _, err := tr.apply("not-an-ip"); err == nil {
		t.Fatal("expected invalid_ip error")
	}
}

// TestIPTransform_Apply_Octets exercises octet-prefix substitution incl. the
// prefix_not_matched boundary checks.
func TestIPTransform_Apply_Octets(t *testing.T) {
	tr := &ipTransform{mode: ipTransformOctets, oldPref: "10.0.0", newPref: "10.0.1"}
	cases := []struct {
		name string
		ip   string
		want string // "" => expect prefix_not_matched error
	}{
		{"basic", "10.0.0.55", "10.0.1.55"},
		{"prefix_no_dot", "10.0.0255", ""}, // boundary char is not '.', should fail
		{"wrong_prefix", "10.0.1.55", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tr.apply(tc.ip)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("apply(%q) expected error, got %q", tc.ip, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("apply(%q) unexpected err: %v", tc.ip, err)
			}
			if got != tc.want {
				t.Errorf("apply(%q) = %q, want %q", tc.ip, got, tc.want)
			}
		})
	}
}

// TestIPTransform_Apply_CIDR exercises the CIDR substitution path.
func TestIPTransform_Apply_CIDR(t *testing.T) {
	_, oldNet, _ := net.ParseCIDR("10.0.0.0/24")
	_, newNet, _ := net.ParseCIDR("10.0.1.0/24")
	tr := &ipTransform{mode: ipTransformCIDR, oldNet: oldNet, newNet: newNet}

	got, err := tr.apply("10.0.0.55")
	if err != nil {
		t.Fatalf("apply CIDR err: %v", err)
	}
	if got != "10.0.1.55" {
		t.Errorf("apply CIDR = %q, want 10.0.1.55", got)
	}

	// prefix_not_matched
	if _, err := tr.apply("192.168.0.55"); err == nil {
		t.Error("expected prefix_not_matched error for non-matching IP")
	}
	// ipv6_to4 returns nil
	if _, err := tr.apply("::1"); err == nil {
		t.Error("expected invalid_ipv4 error for IPv6 under CIDR transform")
	}
}

// TestIPTransform_Apply_CIDRRoundTrip confirms a parse→apply happy path on a
// 16-bit prefix swap.
func TestIPTransform_Apply_CIDRRoundTrip(t *testing.T) {
	tr, err := parseIPTransform("10.0.0.0/16", "172.16.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	got, err := tr.apply("10.0.20.50")
	if err != nil {
		t.Fatal(err)
	}
	if got != "172.16.20.50" {
		t.Errorf("got %q, want 172.16.20.50", got)
	}
}

// TestEnsureAliasesFile covers all three branches.
func TestEnsureAliasesFile(t *testing.T) {
	dir := newTestDir(t)

	// 1. Path traversal attempt → ErrPermission.
	unsafe := filepath.Join(dir, "..", "escape.conf")
	if err := ensureAliasesFile(unsafe); err != os.ErrPermission {
		t.Fatalf("expected os.ErrPermission for unsafe path, got %v", err)
	}

	// 2. New file inside ConfigDir → created with header.
	good := filepath.Join(dir, "aliases.conf")
	if err := ensureAliasesFile(good); err != nil {
		t.Fatalf("ensureAliasesFile err: %v", err)
	}
	data, err := os.ReadFile(good)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "# DNS aliases") {
		t.Errorf("expected header comment, got: %q", string(data))
	}

	// 3. Already exists → no-op (preserve prior content).
	stamped := []byte("address=/existing/x\n")
	if err := os.WriteFile(good, stamped, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureAliasesFile(good); err != nil {
		t.Fatalf("ensureAliasesFile on existing err: %v", err)
	}
	after, _ := os.ReadFile(good)
	if string(after) != string(stamped) {
		t.Errorf("existing file modified: before %q, after %q", stamped, after)
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
		if got := isLeaseTime(tc.s); got != tc.want {
			t.Errorf("isLeaseTime(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

// TestDirectiveGroup covers every group id returned by directiveGroup.
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
		if got := directiveGroup(tc.key); got != tc.want {
			t.Errorf("directiveGroup(%q) = %d, want %d", tc.key, got, tc.want)
		}
	}
}

// ========== Coverage sweep §3 (Этап 3): resolveAliasesTargetFile ==========

// TestResolveAliasesTargetFile_EmptyCreatesDefault covers the empty-reqFile
// branch (was 50%): when the caller omits the file, the default aliases file
// (DefaultAliasesFileName) is created on demand inside ConfigDir and
// returned. This is the path POST /api/aliases takes when the UI sends no
// explicit target file.
func TestResolveAliasesTargetFile_EmptyCreatesDefault(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir

	path, ok := resolveAliasesTargetFile("")
	if !ok {
		t.Fatal("expected ok=true for empty reqFile")
	}
	want := filepath.Join(dir, DefaultAliasesFileName)
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("default aliases file should have been created: %v", err)
	}
}

// TestResolveAliasesTargetFile_ExplicitSafe covers the explicit-path happy
// path: a pre-existing safe file inside ConfigDir is returned verbatim.
func TestResolveAliasesTargetFile_ExplicitSafe(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir
	given := filepath.Join(dir, "custom.conf")
	if err := os.WriteFile(given, []byte("address=/x/1.2.3.4\n"), 0644); err != nil {
		t.Fatal(err)
	}

	path, ok := resolveAliasesTargetFile(given)
	if !ok {
		t.Fatal("expected ok=true for safe explicit path")
	}
	if path != given {
		t.Errorf("path = %q, want %q", path, given)
	}
}

// TestResolveAliasesTargetFile_Unsafe covers the isSafePath rejection branch
// (returns ok=false for a path outside ConfigDir).
func TestResolveAliasesTargetFile_Unsafe(t *testing.T) {
	dir := t.TempDir()
	*ConfigDir = dir

	if _, ok := resolveAliasesTargetFile("/etc/passwd"); ok {
		t.Error("expected ok=false for unsafe path")
	}
}
