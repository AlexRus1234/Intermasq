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

// fuzz_test.go — native Go 1.18+ fuzz targets (FuzzXxx) for the four pure
// text parsers: parseDhcpHostLine, parseArpContent, parseAliasLine,
// parseLeasesContent. The fuzz oracle is "does not panic + structural
// invariants hold on a successful parse" (see логи/Hardening_sweep.md §T1).
// The parsers deliberately accept garbage as legitimate input (returning
// ok=false or an empty collection), so we do NOT build a reference output —
// we hunt for panics, infinite loops, nil-derefs and broken round-trips.
//
// Seed corpus is provided via f.Add only. Go's fuzz engine auto-loads
// testdata/fuzz/<Name>/ (not testdata/corpus/); f.Add seeds run both in
// unit mode (`go test`, as free subtests) and as the initial corpus for
// `-fuzz`. Keeping every seed in f.Add makes the corpus compile-time
// type-checked — no risk of a malformed corpus file turning the default
// CI pipeline red.

package main

import (
	"bufio"
	"strings"
	"testing"
)

// FuzzParseDhcpHostLine guarantees parseDhcpHostLine never panics on
// arbitrary input and that a successfully parsed entry round-trips: the MAC
// stays macRegex-valid, File is propagated, and formatDhcpHostLine re-parses
// to an entry with the same MAC.
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
		entry, ok := parseDhcpHostLine(raw, file)
		if !ok {
			return
		}
		if !macRegex.MatchString(normalizeMAC(entry.Mac)) {
			t.Errorf("parsed MAC %q fails macRegex (raw=%q)", entry.Mac, raw)
		}
		if entry.File != file {
			t.Errorf("File not propagated: got %q want %q", entry.File, file)
		}
		out := formatDhcpHostLine(entry)
		if !strings.Contains(out, entry.Mac) {
			t.Errorf("formatDhcpHostLine round-trip lost MAC: %q", out)
		}
		entry2, ok2 := parseDhcpHostLine(out, file)
		if !ok2 || entry2.Mac != entry.Mac {
			t.Errorf("formatDhcpHostLine round-trip mismatch: raw=%q -> %q (mac got %q want %q, ok=%v)", raw, out, entry2.Mac, entry.Mac, ok2)
		}
	})
}

// FuzzParseArpContent guarantees parseArpContent never panics on arbitrary
// input and that every returned key is non-empty and lowercased.
func FuzzParseArpContent(f *testing.F) {
	seeds := []string{
		"",
		"IP address       HW type     Flags       HW address            Mask Device\n",
		"IP address       HW type     Flags       HW address            Mask Device\n" +
			"192.168.1.1      0x1         0x2         aa:bb:cc:dd:ee:ff     *    eth0\n",
		"192.168.1.1      0x1         0x2         aa:bb:cc:dd:ee:ff     *    eth0\n",
		"junk\n\n   \n0x2 0x2 0x2 0x2 0x2\n",
		"\x00\x01 bad row\n",
		strings.Repeat("a b c d\n", 500),
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, content string) {
		result := parseArpContent(content)
		for mac := range result {
			if mac == "" {
				t.Errorf("empty MAC key in result")
			}
			if mac != strings.ToLower(mac) {
				t.Errorf("MAC key not lowercased: %q", mac)
			}
		}
	})
}

// FuzzParseAliasLine guarantees parseAliasLine never panics on arbitrary
// input and that a successfully parsed entry round-trips through
// aliasToLine: re-parsing the rendered line yields the same Type, Domain
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
		entry, ok := parseAliasLine(line, file, hasBak)
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
		out := aliasToLine(entry)
		entry2, ok2 := parseAliasLine(out, file, false)
		if !ok2 {
			t.Errorf("aliasToLine round-trip failed to parse: %q -> %q", line, out)
			return
		}
		if entry2.Type != entry.Type || entry2.Domain != entry.Domain || entry2.Target != entry.Target {
			t.Errorf("aliasToLine round-trip mismatch: %q -> %q\n  got  %+v\n  want %+v", line, out, entry2, entry)
		}
	})
}

// FuzzParseLeasesContent guarantees parseLeasesContent never panics on
// arbitrary input and that the field-index mapping (fields[1]→Mac,
// fields[2]→Ip, fields[3]→Hostname) plus entry count hold for every line
// with >= 3 whitespace-separated fields.
func FuzzParseLeasesContent(f *testing.F) {
	seeds := []string{
		"",
		"0 aa:bb:cc:dd:ee:ff 192.168.1.1 * 01:aa:bb:cc:dd:ee:ff\n",
		"1700000000 11:22:33:44:55:01 10.0.0.1 phone\n" +
			"1700000001 11:22:33:44:55:02 10.0.0.2\n",
		"   \n\n garbage line with no fields\n",
		"a b\n",
		"a b c\n",
		"t mac ip host clientid extra fields here\n",
		// Regression seed: a 3-token line whose Mac token is a bare ":"
		// (contains a MAC separator but is not a MAC). Pins that the parser
		// accepts this as a valid (if nonsensical) entry — an over-eager
		// format assertion on Mac would false-fire here.
		"0 : 0\n",
		strings.Repeat("0 aa:bb:cc:dd:ee:ff 10.0.0.1 host\n", 300),
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, content string) {
		leases := parseLeasesContent(content)
		// The parser makes no format guarantee — it returns one entry per
		// line with >= 3 whitespace-separated tokens, assigning fields[1]→Mac,
		// fields[2]→Ip, fields[3]→Hostname. The original empty-string checks
		// (l.Ip == "", l.Mac == "") were unreachable (strings.Fields yields
		// no empty tokens), and an unconditional IP/MAC format check
		// false-fires on the documented garbage inputs the parser accepts
		// (e.g. "0 : 0" — Mac=":" is separator-shaped but not a MAC). The
		// real, always-true contract is the field-index mapping: re-split
		// the same source lines the parser consumed and verify each entry's
		// fields match the expected indices and count. This catches
		// field-swap regressions (e.g. Ip=fields[0]) and entry-count drift
		// with no format assumption that garbage inputs would violate.
		// Both the parser and this check use bufio.Scanner with the default
		// buffer, so over-long lines truncate identically and stay
		// consistent.
		expected := 0
		sc := bufio.NewScanner(strings.NewReader(content))
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 3 {
				continue
			}
			if expected >= len(leases) {
				t.Errorf("FuzzParseLeasesContent: parser returned fewer entries than parseable lines: missing entry for line %d %q (input=%q)", expected, sc.Text(), content)
				return
			}
			l := leases[expected]
			if l.Mac != fields[1] {
				t.Errorf("FuzzParseLeasesContent: lease[%d].Mac = %q, want fields[1]=%q (line=%q, input=%q)", expected, l.Mac, fields[1], sc.Text(), content)
			}
			if l.Ip != fields[2] {
				t.Errorf("FuzzParseLeasesContent: lease[%d].Ip = %q, want fields[2]=%q (line=%q, input=%q)", expected, l.Ip, fields[2], sc.Text(), content)
			}
			wantHost := ""
			if len(fields) > 3 {
				wantHost = fields[3]
			}
			if l.Hostname != wantHost {
				t.Errorf("FuzzParseLeasesContent: lease[%d].Hostname = %q, want %q (line=%q, input=%q)", expected, l.Hostname, wantHost, sc.Text(), content)
			}
			expected++
		}
		if expected != len(leases) {
			t.Errorf("FuzzParseLeasesContent: entry-count mismatch: %d parseable lines, parser returned %d entries (input=%q)", expected, len(leases), content)
		}
	})
}
