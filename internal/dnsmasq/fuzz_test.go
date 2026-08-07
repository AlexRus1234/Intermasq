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

// fuzz_test.go — native Go 1.18+ fuzz targets for the two pure text parsers
// that live in this package: ParseDhcpHostLine and ParseAliasLine. The fuzz
// oracle is "does not panic + structural invariants hold on a successful
// parse" (see логи/Hardening_sweep.md §T1). The parsers deliberately accept
// garbage as legitimate input (returning ok=false or an empty collection),
// so we do NOT build a reference output — we hunt for panics, infinite
// loops, nil-derefs and broken round-trips.
//
// The ARP / leases fuzz targets (FuzzParseArpContent /
// FuzzParseLeasesContent) stay in the main package for now — those parsers
// migrate in stage 6 of the modularization.
//
// Seed corpus is provided via f.Add only. Go's fuzz engine auto-loads
// testdata/fuzz/<Name>/ (not testdata/corpus/); f.Add seeds run both in
// unit mode (`go test`, as free subtests) and as the initial corpus for
// `-fuzz`. Keeping every seed in f.Add makes the corpus compile-time
// type-checked — no risk of a malformed corpus file turning the default
// CI pipeline red.

package dnsmasq

import (
	"strings"
	"testing"

	"intermask/internal/validate"
)

// FuzzParseDhcpHostLine guarantees ParseDhcpHostLine never panics on
// arbitrary input and that a successfully parsed entry round-trips: the MAC
// stays valid (validate.ValidMAC), File is propagated, and
// FormatDhcpHostLine re-parses to an entry with the same MAC.
func FuzzParseDhcpHostLine(f *testing.F) {
	seeds := []struct {
		raw, file string
	}{
		{"dhcp-host=aa:bb:cc:dd:ee:ff,nas,10.0.0.1", "/etc/dnsmasq.d/x.conf"},
		{"dhcp-host=aa:bb:cc:dd:ee:ff", "/etc/dnsmasq.d/x.conf"},
		{"dhcp-host=aa-bb-cc-dd-ee-ff,10.0.0.1", "/x.conf"},
		{"dhcp-host:11:22:33:44:55:66,set:iot", "/x.conf"},
		{"dhcp-host=aa:bb:cc:dd:ee:ff,10.0.0.1,tag:lan,id:abc", "/x.conf"},
		{"dhcp-host=", ""},
		{"not-a-dhcp-host-line", "/x.conf"},
		{"", ""},
		{"dhcp-host=" + strings.Repeat(",", 1000), "/x.conf"},
	}
	for _, s := range seeds {
		f.Add(s.raw, s.file)
	}
	f.Fuzz(func(t *testing.T, raw, file string) {
		entry, ok := ParseDhcpHostLine(raw, file)
		if !ok {
			return
		}
		if !validate.ValidMAC(validate.NormalizeMAC(entry.Mac)) {
			t.Errorf("parsed MAC %q fails validate.ValidMAC (raw=%q)", entry.Mac, raw)
		}
		if entry.File != file {
			t.Errorf("File not propagated: got %q want %q", entry.File, file)
		}
		out := FormatDhcpHostLine(entry)
		if !strings.Contains(out, entry.Mac) {
			t.Errorf("FormatDhcpHostLine round-trip lost MAC: %q", out)
		}
		entry2, ok2 := ParseDhcpHostLine(out, file)
		if !ok2 || entry2.Mac != entry.Mac {
			t.Errorf("FormatDhcpHostLine round-trip mismatch: raw=%q -> %q (mac got %q want %q, ok=%v)", raw, out, entry2.Mac, entry.Mac, ok2)
		}
	})
}

// FuzzParseAliasLine guarantees ParseAliasLine never panics on arbitrary
// input and that a successfully parsed entry round-trips through
// AliasToLine: re-parsing the rendered line yields the same Type, Domain
// and Target. File propagation (incl. the |has_bak suffix) is also checked.
func FuzzParseAliasLine(f *testing.F) {
	seeds := []struct {
		line   string
		file   string
		hasBak bool
	}{
		{"address=/nas.lan/192.168.1.10", "/etc/dnsmasq.d/x.conf", false},
		{"cname=wiki,nas.lan", "/x.conf", false},
		{"cname=wiki,nas.lan,tag:lan", "/x.conf", true},
		{"ptr-record=10.1.168.192.in-addr.arpa,nas.lan", "/x.conf", false},
		{"txt-record=wiki.lan,v=spf1 -all", "/x.conf", false},
		{`txt-record=dkim,"multi word"`, "/x.conf", false},
		{"address=/#/", "", false},
		{"address=/*.evil/10.0.0.1", "", false},
		{"address=/nas.lan", "", false},
		{"", "", false},
		{"not-an-alias", "", false},
		{strings.Repeat("txt-record=a,", 200) + "b", "/x.conf", false},
	}
	for _, s := range seeds {
		f.Add(s.line, s.file, s.hasBak)
	}
	f.Fuzz(func(t *testing.T, line, file string, hasBak bool) {
		entry, ok := ParseAliasLine(line, file, hasBak)
		if !ok {
			return
		}
		wantFile := file
		if hasBak {
			wantFile = file + "|has_bak"
		}
		if entry.File != wantFile {
			t.Errorf("File not propagated: got %q want %q (hasBak=%v)", entry.File, wantFile, hasBak)
		}
		out := AliasToLine(entry)
		entry2, ok2 := ParseAliasLine(out, file, false)
		if !ok2 {
			t.Errorf("AliasToLine round-trip failed to parse: %q -> %q", line, out)
			return
		}
		if entry2.Type != entry.Type || entry2.Domain != entry.Domain || entry2.Target != entry.Target {
			t.Errorf("AliasToLine round-trip mismatch: %q -> %q\n  got  %+v\n  want %+v", line, out, entry2, entry)
		}
	})
}
