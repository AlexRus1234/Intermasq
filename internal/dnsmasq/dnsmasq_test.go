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
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"intermask/internal/models"
)

// newTestDir creates a temp dir, points *ConfigDir at it, and returns the
// dir. t.TempDir auto-cleans on test completion. Mirrors the same-named
// helper that lived in the main package's handlers_test.go before stage 4
// of the modularization; the call sites that scanned ConfigDir from
// internal/dnsmasq tests need an in-package equivalent.
func newTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	*ConfigDir = dir
	return dir
}

// ========== dhcp-host parsing / csv ==========
// (Migrated from the main package's dnsmasq_test.go / handlers_test.go —
// now exercising the exported parsers of this package.)

// TestReadAllHosts_EmptyFile confirms that an empty .conf file yields zero
// hosts without errors. This covers the "fresh install" scenario where the
// panel has just created a new config file but no hosts have been added.
func TestReadAllHosts_EmptyFile(t *testing.T) {
	dir := newTestDir(t)
	os.WriteFile(filepath.Join(dir, "empty.conf"), []byte(""), 0644)
	hosts := ReadAllHosts()
	if len(hosts) != 0 {
		t.Errorf("expected 0 hosts from empty file, got %d", len(hosts))
	}
}

// TestReadAllHosts_CommentsOnly verifies that a .conf file containing only
// comments (including commented-out dhcp-host lines) yields zero hosts.
func TestReadAllHosts_CommentsOnly(t *testing.T) {
	dir := newTestDir(t)
	content := "# header comment\n# another comment\n#dhcp-host=aa:bb:cc:dd:ee:ff,host,1.2.3.4\n"
	os.WriteFile(filepath.Join(dir, "comments.conf"), []byte(content), 0644)
	hosts := ReadAllHosts()
	if len(hosts) != 0 {
		t.Errorf("expected 0 hosts from comments-only file, got %d: %+v", len(hosts), hosts)
	}
}

// TestReadAllHosts_MultipleFiles confirms that hosts from multiple .conf
// files are aggregated correctly, and non-.conf files are ignored.
func TestReadAllHosts_MultipleFiles(t *testing.T) {
	dir := newTestDir(t)
	os.WriteFile(filepath.Join(dir, "10-hosts.conf"), []byte("dhcp-host=aa:bb:cc:dd:ee:01,h1,10.0.0.1\n"), 0644)
	os.WriteFile(filepath.Join(dir, "20-more.conf"), []byte("dhcp-host=aa:bb:cc:dd:ee:02,h2,10.0.0.2\n"), 0644)
	os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("dhcp-host=aa:bb:cc:dd:ee:03,h3,10.0.0.3\n"), 0644)
	os.WriteFile(filepath.Join(dir, "ignore.bak"), []byte("dhcp-host=aa:bb:cc:dd:ee:04,h4,10.0.0.4\n"), 0644)

	hosts := ReadAllHosts()
	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts (only .conf files), got %d: %+v", len(hosts), hosts)
	}
}

// TestParseDhcpHostLine_TrailingNewline confirms that a line with trailing
// \r\n (Windows CRLF) doesn't produce a phantom empty-field hostname.
func TestParseDhcpHostLine_TrailingNewline(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "test.conf")

	entry, ok := ParseDhcpHostLine("dhcp-host=aa:bb:cc:dd:ee:ff,host1,192.168.1.10\r", file)
	if !ok {
		t.Fatal("expected parse success")
	}
	if entry.Hostname != "host1" {
		t.Errorf("hostname should be 'host1', got %q (CR contamination?)", entry.Hostname)
	}
}

// TestParseDhcpHostLine_LeaseTime covers the regression where a trailing
// lease-time suffix (",12h") was mis-parsed as the hostname and then dropped
// on format round-trip, corrupting bulk move/edit. The suffix must land in
// LeaseTime and the real hostname must survive.
func TestParseDhcpHostLine_LeaseTime(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "test.conf")

	cases := []struct {
		name  string
		line  string
		host  string
		lease string
	}{
		{"hours", "dhcp-host=aa:bb:cc:dd:ee:ff,192.168.1.10,host1,12h", "host1", "12h"},
		{"seconds", "dhcp-host=aa:bb:cc:dd:ee:ff,host2,3600", "host2", "3600"},
		{"infinite", "dhcp-host=aa:bb:cc:dd:ee:ff,host3,10.0.0.3,infinite", "host3", "infinite"},
		{"host_before_ip", "dhcp-host=aa:bb:cc:dd:ee:ff,host4,10.0.0.4,7d", "host4", "7d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry, ok := ParseDhcpHostLine(tc.line, file)
			if !ok {
				t.Fatalf("expected parse success for %q", tc.line)
			}
			if entry.Hostname != tc.host {
				t.Errorf("hostname: got %q, want %q", entry.Hostname, tc.host)
			}
			if entry.LeaseTime != tc.lease {
				t.Errorf("lease_time: got %q, want %q", entry.LeaseTime, tc.lease)
			}
			// Round-trip: parsing the formatted output must yield the same
			// fields and the line must still carry the suffix.
			formatted := FormatDhcpHostLine(entry)
			if !strings.Contains(formatted, tc.lease) {
				t.Errorf("formatted line dropped lease-time: %q", formatted)
			}
			round, ok := ParseDhcpHostLine(formatted, file)
			if !ok {
				t.Fatalf("round-trip parse failed for %q", formatted)
			}
			if round.Hostname != tc.host || round.LeaseTime != tc.lease {
				t.Errorf("round-trip mismatch: got host=%q lease=%q, want host=%q lease=%q",
					round.Hostname, round.LeaseTime, tc.host, tc.lease)
			}
		})
	}
}

// TestParseDhcpHostLine_LeaseTimeWithTag ensures a lease-time coexists with a
// set: tag and that the tag survives the round-trip too.
func TestParseDhcpHostLine_LeaseTimeWithTag(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "test.conf")

	entry, ok := ParseDhcpHostLine("dhcp-host=aa:bb:cc:dd:ee:ff,host1,10.0.0.1,set:phone,12h", file)
	if !ok {
		t.Fatal("expected parse success")
	}
	if entry.Hostname != "host1" {
		t.Errorf("hostname: got %q, want host1", entry.Hostname)
	}
	if entry.LeaseTime != "12h" {
		t.Errorf("lease_time: got %q, want 12h", entry.LeaseTime)
	}
	if len(entry.Tags) != 1 || entry.Tags[0] != "set:phone" {
		t.Errorf("tags: got %+v, want [set:phone]", entry.Tags)
	}
	formatted := FormatDhcpHostLine(entry)
	// Tag must precede the trailing lease-time.
	if !strings.HasSuffix(formatted, ",set:phone,12h") {
		t.Errorf("unexpected format order: %q", formatted)
	}
}

// TestParseCSVHostsNormalizesDashMAC (A4 regression) — CSV import normalises
// dash-MACs the same way the JSON add path does.
func TestParseCSVHostsNormalizesDashMAC(t *testing.T) {
	csv := "mac,ip,hostname\naa-bb-cc-dd-ee-ff,10.0.0.5,x\n"
	hosts, err := ParseCSVHosts(strings.NewReader(csv), "/tmp/x.conf")
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
	hosts, err := ParseCSVHosts(strings.NewReader(csv), "/tmp/x.conf")
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
	hosts, err := ParseCSVHosts(strings.NewReader(csv), "/tmp/x.conf")
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

// TestParseCSVHostsLeaseTime — 4-я колонка lease_time подхватывается парсером.
func TestParseCSVHostsLeaseTime(t *testing.T) {
	csv := "mac,ip,hostname,lease_time\naa:bb:cc:dd:ee:ff,10.0.0.5,phone,12h\n"
	hosts, err := ParseCSVHosts(strings.NewReader(csv), "/tmp/x.conf")
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].LeaseTime != "12h" {
		t.Errorf("lease_time: got %q, want 12h", hosts[0].LeaseTime)
	}
}

// TestParseCSVHostsLegacy3ColNoLeaseTime — старые 3-колонные CSV (без
// lease_time) остаются совместимы: поле пустое.
func TestParseCSVHostsLegacy3ColNoLeaseTime(t *testing.T) {
	csv := "mac,ip,hostname\naa:bb:cc:dd:ee:ff,10.0.0.5,phone\n"
	hosts, err := ParseCSVHosts(strings.NewReader(csv), "/tmp/x.conf")
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].LeaseTime != "" {
		t.Errorf("legacy 3-col CSV should give empty lease_time, got %q", hosts[0].LeaseTime)
	}
}

// TestParseCSVHostsIgnoresInvalidLeaseTime — мусор в 4-й колонке не роняет
// строку, а молча игнорируется (хост импортируется без lease_time).
func TestParseCSVHostsIgnoresInvalidLeaseTime(t *testing.T) {
	csv := "mac,ip,hostname,lease_time\naa:bb:cc:dd:ee:ff,10.0.0.5,phone,garbage\n"
	hosts, err := ParseCSVHosts(strings.NewReader(csv), "/tmp/x.conf")
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].LeaseTime != "" {
		t.Errorf("invalid lease_time should be ignored, got %q", hosts[0].LeaseTime)
	}
}

// TestHostsToCSVIncludesLeaseTimeColumn — экспорт пишет заголовок из 4 колонок
// и lease_time в каждой строке (пустой — для хостов без него).
func TestHostsToCSVIncludesLeaseTimeColumn(t *testing.T) {
	hosts := []models.HostEntry{
		{Mac: "aa:bb:cc:dd:ee:01", Ip: "10.0.0.1", Hostname: "h1"},
		{Mac: "aa:bb:cc:dd:ee:02", Ip: "10.0.0.2", Hostname: "h2", LeaseTime: "12h"},
	}
	out := string(HostsToCSV(hosts))
	if !strings.HasPrefix(out, "mac,ip,hostname,lease_time\n") {
		t.Errorf("expected 4-col header, got: %s", out)
	}
	if !strings.Contains(out, "aa:bb:cc:dd:ee:02,10.0.0.2,h2,12h") {
		t.Errorf("expected lease_time in row, got: %s", out)
	}
	// empty lease_time renders as a trailing empty field on the row
	if !strings.Contains(out, "aa:bb:cc:dd:ee:01,10.0.0.1,h1,") {
		t.Errorf("expected empty lease_time field, got: %s", out)
	}
}

// TestHostsCSVRoundTripLeaseTime — экспорт → импорт сохраняет lease_time.
func TestHostsCSVRoundTripLeaseTime(t *testing.T) {
	original := []models.HostEntry{
		{Mac: "aa:bb:cc:dd:ee:02", Ip: "10.0.0.2", Hostname: "h2", LeaseTime: "infinite", File: "/tmp/x.conf"},
	}
	csvData := HostsToCSV(original)
	hosts, err := ParseCSVHosts(strings.NewReader(string(csvData)), "/tmp/x.conf")
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].LeaseTime != "infinite" {
		t.Errorf("round-trip lease_time: got %q, want infinite", hosts[0].LeaseTime)
	}
}

// ========== IP transforms (bulk-edit) ==========

// TestParseIPTransform covers every error branch and the two success modes
// (octet-prefix / CIDR / none) of ParseIPTransform.
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
			got, err := ParseIPTransform(tc.old, tc.nw)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ParseIPTransform(%q,%q) unexpected err: %v", tc.old, tc.nw, err)
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
	tr := &IPTransform{mode: ipTransformNone}
	got, err := tr.Apply("10.0.0.55")
	if err != nil || got != "10.0.0.55" {
		t.Fatalf("Apply(none) = %q, %v; want 10.0.0.55, nil", got, err)
	}
}

// TestIPTransform_Apply_InvalidIP covers net.ParseIP returning nil.
func TestIPTransform_Apply_InvalidIP(t *testing.T) {
	tr := &IPTransform{mode: ipTransformOctets, oldPref: "10.0.0", newPref: "10.0.1"}
	if _, err := tr.Apply("not-an-ip"); err == nil {
		t.Fatal("expected invalid_ip error")
	}
}

// TestIPTransform_Apply_Octets exercises octet-prefix substitution incl. the
// prefix_not_matched boundary checks.
func TestIPTransform_Apply_Octets(t *testing.T) {
	tr := &IPTransform{mode: ipTransformOctets, oldPref: "10.0.0", newPref: "10.0.1"}
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
			got, err := tr.Apply(tc.ip)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("Apply(%q) expected error, got %q", tc.ip, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Apply(%q) unexpected err: %v", tc.ip, err)
			}
			if got != tc.want {
				t.Errorf("Apply(%q) = %q, want %q", tc.ip, got, tc.want)
			}
		})
	}
}

// TestIPTransform_Apply_CIDR exercises the CIDR substitution path.
func TestIPTransform_Apply_CIDR(t *testing.T) {
	_, oldNet, _ := net.ParseCIDR("10.0.0.0/24")
	_, newNet, _ := net.ParseCIDR("10.0.1.0/24")
	tr := &IPTransform{mode: ipTransformCIDR, oldNet: oldNet, newNet: newNet}

	got, err := tr.Apply("10.0.0.55")
	if err != nil {
		t.Fatalf("Apply CIDR err: %v", err)
	}
	if got != "10.0.1.55" {
		t.Errorf("Apply CIDR = %q, want 10.0.1.55", got)
	}

	// prefix_not_matched
	if _, err := tr.Apply("192.168.0.55"); err == nil {
		t.Error("expected prefix_not_matched error for non-matching IP")
	}
	// ipv6_to4 returns nil
	if _, err := tr.Apply("::1"); err == nil {
		t.Error("expected invalid_ipv4 error for IPv6 under CIDR transform")
	}
}

// TestIPTransform_Apply_CIDRRoundTrip confirms a parse→apply happy path on a
// 16-bit prefix swap.
func TestIPTransform_Apply_CIDRRoundTrip(t *testing.T) {
	tr, err := ParseIPTransform("10.0.0.0/16", "172.16.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	got, err := tr.Apply("10.0.20.50")
	if err != nil {
		t.Fatal(err)
	}
	if got != "172.16.20.50" {
		t.Errorf("got %q, want 172.16.20.50", got)
	}
}

// Compile-time sanity: ensure the migrated tests reference package types the
// way the production code does, so a future API drift breaks compilation.
var _ = models.HostEntry{}
