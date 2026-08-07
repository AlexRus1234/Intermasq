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

func TestParseAliasLineA(t *testing.T) {
	e, ok := ParseAliasLine("address=/nas.lan/192.168.1.10", "/etc/dnsmasq.d/x.conf", false)
	if !ok {
		t.Fatal("expected parse success")
	}
	if e.Type != "A" || e.Domain != "nas.lan" || e.Target != "192.168.1.10" {
		t.Errorf("unexpected entry: %+v", e)
	}
}

func TestParseAliasLineCNAME(t *testing.T) {
	e, ok := ParseAliasLine("cname=wiki,nas.lan", "/etc/dnsmasq.d/x.conf", false)
	if !ok {
		t.Fatal("expected parse success")
	}
	if e.Type != "CNAME" || e.Domain != "wiki" || e.Target != "nas.lan" {
		t.Errorf("unexpected entry: %+v", e)
	}
}

func TestParseAliasLineCNAMEWithTag(t *testing.T) {
	e, ok := ParseAliasLine("cname=wiki,nas.lan,tag:lan", "/etc/dnsmasq.d/x.conf", false)
	if !ok {
		t.Fatal("expected parse success for tagged cname")
	}
	if e.Type != "CNAME" || e.Domain != "wiki" || e.Target != "nas.lan" {
		t.Errorf("unexpected entry: %+v", e)
	}
}

func TestParseAliasLineRejectsWildcard(t *testing.T) {
	if _, ok := ParseAliasLine("address=/#/10.0.0.1", "", false); ok {
		t.Error("wildcard # should be rejected")
	}
	if _, ok := ParseAliasLine("address=/*.evil/10.0.0.1", "", false); ok {
		t.Error("wildcard *.evil should be rejected")
	}
}

func TestParseAliasLineRejectsMalformed(t *testing.T) {
	if _, ok := ParseAliasLine("address=/nas.lan", "", false); ok {
		t.Error("missing closing slash should fail")
	}
	if _, ok := ParseAliasLine("address=/nas.lan/", "", false); ok {
		t.Error("empty target should fail")
	}
	if _, ok := ParseAliasLine("cname=onlyalias", "", false); ok {
		t.Error("cname without target should fail")
	}
}

func TestAliasToLineRoundTrip(t *testing.T) {
	cases := []models.DnsAliasEntry{
		{Type: "A", Domain: "nas.lan", Target: "192.168.1.10"},
		{Type: "CNAME", Domain: "wiki", Target: "nas.lan"},
	}
	for _, in := range cases {
		line := AliasToLine(in)
		out, ok := ParseAliasLine(line, "", false)
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
	dir := newTestDir(t)
	path := filepath.Join(dir, "dns.conf")
	content := []byte("address=/nas.lan/192.168.1.10\ncname=wiki,nas.lan\nserver=8.8.8.8\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	aliases := ReadAllAliases()
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
	dir := newTestDir(t)
	path := filepath.Join(dir, "dns.conf")
	if err := os.WriteFile(path, []byte("address=/a.b/1.2.3.4\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	aliases := ReadAllAliases()
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}
	if !strings.HasSuffix(aliases[0].File, "|has_bak") {
		t.Errorf("expected |has_bak marker, got %q", aliases[0].File)
	}
	if CleanAliasFile(aliases[0].File) != path {
		t.Errorf("CleanAliasFile wrong: got %q want %q", CleanAliasFile(aliases[0].File), path)
	}
}

// ========== Feature 4+5: PTR and TXT aliases ==========

func TestParseAliasLinePTR(t *testing.T) {
	e, ok := ParseAliasLine("ptr-record=10.1.168.192.in-addr.arpa,nas.lan", "/etc/dnsmasq.d/x.conf", false)
	if !ok {
		t.Fatal("expected parse success for PTR")
	}
	if e.Type != "PTR" || e.Domain != "10.1.168.192.in-addr.arpa" || e.Target != "nas.lan" {
		t.Errorf("unexpected PTR entry: %+v", e)
	}
}

func TestParseAliasLineTXT(t *testing.T) {
	e, ok := ParseAliasLine("txt-record=wiki.lan,v=spf1 -all", "/etc/dnsmasq.d/x.conf", false)
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
	e, ok := ParseAliasLine("txt-record=dkim._domainkey,k=rsa; p=MIGfMA0,a=test", "/etc/dnsmasq.d/x.conf", false)
	if !ok {
		t.Fatal("expected parse success for TXT with multiple commas")
	}
	if e.Target != "k=rsa; p=MIGfMA0,a=test" {
		t.Errorf("TXT value split on wrong comma: %q", e.Target)
	}
}

func TestAliasToLinePTR(t *testing.T) {
	got := AliasToLine(models.DnsAliasEntry{Type: "PTR", Domain: "10.1.168.192.in-addr.arpa", Target: "nas.lan"})
	if got != "ptr-record=10.1.168.192.in-addr.arpa,nas.lan" {
		t.Errorf("PTR serialization wrong: %q", got)
	}
}

func TestAliasToLineTXT(t *testing.T) {
	got := AliasToLine(models.DnsAliasEntry{Type: "TXT", Domain: "wiki.lan", Target: "v=spf1 -all"})
	if got != "txt-record=wiki.lan,v=spf1 -all" {
		t.Errorf("TXT serialization wrong: %q", got)
	}
}

func TestAliasRoundTripPTR(t *testing.T) {
	in := models.DnsAliasEntry{Type: "PTR", Domain: "5.0.168.192.in-addr.arpa", Target: "host.lan"}
	out, ok := ParseAliasLine(AliasToLine(in), "", false)
	if !ok {
		t.Fatal("PTR round-trip failed")
	}
	out.File = in.File
	if out != in {
		t.Errorf("PTR round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestAliasRoundTripTXT(t *testing.T) {
	in := models.DnsAliasEntry{Type: "TXT", Domain: "host.lan", Target: "some text value"}
	out, ok := ParseAliasLine(AliasToLine(in), "", false)
	if !ok {
		t.Fatal("TXT round-trip failed")
	}
	out.File = in.File
	if out != in {
		t.Errorf("TXT round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestIsAliasDirectiveRecognizesNewTypes(t *testing.T) {
	if !IsAliasDirective("ptr-record=foo,bar") {
		t.Error("ptr-record= not recognized as alias directive")
	}
	if !IsAliasDirective("txt-record=foo,bar") {
		t.Error("txt-record= not recognized as alias directive")
	}
}

func TestReadAllAliasesIncludesPTRAndTXT(t *testing.T) {
	dir := newTestDir(t)
	content := []byte("address=/nas.lan/192.168.1.10\n" +
		"cname=wiki,nas.lan\n" +
		"ptr-record=10.1.168.192.in-addr.arpa,nas.lan\n" +
		"txt-record=nas.lan,v=spf1 -all\n" +
		"server=8.8.8.8\n")
	if err := os.WriteFile(filepath.Join(dir, "dns.conf"), content, 0644); err != nil {
		t.Fatal(err)
	}
	aliases := ReadAllAliases()
	if len(aliases) != 4 {
		t.Fatalf("expected 4 aliases (A, CNAME, PTR, TXT), got %d: %+v", len(aliases), aliases)
	}
	types := map[string]bool{}
	for _, a := range aliases {
		types[a.Type] = true
	}
	if !types["A"] || !types["CNAME"] || !types["PTR"] || !types["TXT"] {
		t.Errorf("missing types in ReadAllAliases result: %+v", types)
	}
}

func TestParseCSVAliasesIncludesPTRAndTXT(t *testing.T) {
	csv := "type,domain,target\n" +
		"A,nas.lan,192.168.1.10\n" +
		"PTR,10.in-addr.arpa,nas.lan\n" +
		"TXT,nas.lan,v=spf1 -all\n"
	aliases, err := ParseCSVAliases(strings.NewReader(csv), "/tmp/x.conf")
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
