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

package validate

import (
	"strings"
	"testing"
)

func TestValidHostname(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
		reason   string
	}{
		{"host1", true, "simple"},
		{"my-host", true, "single label with hyphen"},
		{"a", true, "single char"},
		{"a-b-c", true, "multiple hyphens"},
		{"host.example.com", true, "fqdn"},
		{"1host", true, "leading digit allowed by RFC 1123"},
		{"h1-h2.h3-h4", true, "hyphens in multi-label"},

		{"", false, "empty"},
		{"-host", false, "leading hyphen"},
		{"host-", false, "trailing hyphen"},
		{".host", false, "leading dot"},
		{"host.", false, "trailing dot"},
		{"host..name", false, "consecutive dots"},
		{"host name", false, "space"},
		{"host_name", false, "underscore"},
		{"host.name-", false, "trailing hyphen in label"},
		{"-a.b", false, "leading hyphen in first label"},
		{strings.Repeat("a", 254), false, "too long (>253)"},
		{strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 60), true, "max total length (253)"},
	}
	for _, tt := range tests {
		got := ValidHostname(tt.host)
		if got != tt.expected {
			t.Errorf("ValidHostname(%q) = %v, want %v (%s)", tt.host, got, tt.expected, tt.reason)
		}
	}
}

// TestValidHostname_Unicode verifies that non-ASCII hostnames are rejected.
// hostnameRegex is [a-zA-Z0-9]... which deliberately excludes UTF-8 multibyte.
func TestValidHostname_Unicode(t *testing.T) {
	cases := []struct {
		hostname string
		want     bool
	}{
		{"host", true},    // ASCII baseline
		{"höst", false},   // Latin Extended
		{"сервер", false}, // Cyrillic
		{"サーバ", false},    // Japanese
		{"hōst", false},   // Maori macron
		{"host₀₁", false}, // Unicode subscripts
	}
	for _, tc := range cases {
		got := ValidHostname(tc.hostname)
		if got != tc.want {
			t.Errorf("ValidHostname(%q) = %v, want %v", tc.hostname, got, tc.want)
		}
	}
}

// ========== Feature 3: optional IP/hostname in dhcp-host ==========

func TestValidateHostFieldsAllCombinations(t *testing.T) {
	cases := []struct {
		name     string
		mac      string
		ip       string
		hostname string
		want     bool
	}{
		{"full valid", "aa:bb:cc:dd:ee:ff", "192.168.1.10", "nas", true},
		{"mac only", "aa:bb:cc:dd:ee:ff", "", "", true},
		{"mac + hostname", "aa:bb:cc:dd:ee:ff", "", "phone", true},
		{"mac + ip", "aa:bb:cc:dd:ee:ff", "192.168.1.10", "", true},
		{"mac + bad ip", "aa:bb:cc:dd:ee:ff", "not-an-ip", "", false},
		{"mac + bad hostname", "aa:bb:cc:dd:ee:ff", "", "with space", false},
		{"bad mac", "not-a-mac", "1.2.3.4", "host", false},
		{"empty mac", "", "1.2.3.4", "host", false},
		{"empty all", "", "", "", false},
		// A3: zero/broadcast MACs must be rejected even though they match macRegex.
		{"zero mac", "00:00:00:00:00:00", "", "", false},
		{"broadcast mac", "ff:ff:ff:ff:ff:ff", "", "", false},
		{"zero mac upper", "FF:FF:FF:FF:FF:FF", "", "", false},
		// A4: dash-separated MAC is normalised inside ValidateHostFields, so the
		// validator accepts it (the entry point then writes the colon form).
		{"dash mac", "aa-bb-cc-dd-ee-ff", "", "", true},
		{"dash mac + ip", "aa-bb-cc-dd-ee-ff", "10.0.0.5", "x", true},
		{"dash zero mac", "00-00-00-00-00-00", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateHostFields(tc.mac, tc.ip, tc.hostname)
			if got != tc.want {
				t.Errorf("ValidateHostFields(%q,%q,%q) = %v, want %v", tc.mac, tc.ip, tc.hostname, got, tc.want)
			}
		})
	}
}

// TestValidateHostFields_IPv6 confirms that net.ParseIP in ValidateHostFields
// accepts IPv6 addresses. dnsmasq itself supports IPv6 in dhcp-host, so the
// panel should not reject them at the validation layer.
func TestValidateHostFields_IPv6(t *testing.T) {
	cases := []struct {
		name string
		mac  string
		ip   string
		want bool
	}{
		{"ipv6 loopback", "aa:bb:cc:dd:ee:ff", "::1", true},
		{"ipv6 full", "aa:bb:cc:dd:ee:ff", "2001:db8::1", true},
		{"ipv6 link-local", "aa:bb:cc:dd:ee:ff", "fe80::1", true},
		{"ipv6 invalid", "aa:bb:cc:dd:ee:ff", "not-ipv6", false},
		{"ipv6 empty", "aa:bb:cc:dd:ee:ff", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateHostFields(tc.mac, tc.ip, "")
			if got != tc.want {
				t.Errorf("ValidateHostFields(%q,%q,...) = %v, want %v", tc.mac, tc.ip, got, tc.want)
			}
		})
	}
}

// TestNormalizeMAC (A4) confirms dash-separated MACs become colon-separated.
func TestNormalizeMAC(t *testing.T) {
	cases := []struct{ in, want string }{
		{"aa-bb-cc-dd-ee-ff", "aa:bb:cc:dd:ee:ff"},
		{"aa:bb:cc:dd:ee:ff", "aa:bb:cc:dd:ee:ff"},
		{"AA-BB-CC-DD-EE-FF", "AA:BB:CC:DD:EE:FF"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := NormalizeMAC(tc.in); got != tc.want {
			t.Errorf("NormalizeMAC(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestValidateHostTags covers every branch: empty (skip), set:/tag: ok,
// id:-prefixed accepted for round-trip, garbage rejected.
func TestValidateHostTags(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		want bool
	}{
		{"empty slice", []string{}, true},
		{"only empties", []string{"", "  ", ""}, true},
		{"set tag", []string{"set:foo"}, true},
		{"tag tag", []string{"tag:bar"}, true},
		{"id accepted for round-trip", []string{"id:abc"}, true},
		{"mixed valid", []string{"set:foo", "tag:bar", "id:xyz"}, true},
		{"whitespace trimmed valid", []string{"  set:foo  "}, true},
		{"invalid bareword", []string{"foo"}, false},
		{"invalid prefix unknown", []string{"foo:bar"}, false},
		{"one invalid in mix", []string{"set:foo", "BAD"}, false},
		{"empty inside still ok", []string{"set:foo", "", "tag:bar"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidateHostTags(tc.tags); got != tc.want {
				t.Errorf("ValidateHostTags(%v) = %v, want %v", tc.tags, got, tc.want)
			}
		})
	}
}

// TestNormalizeHostTags covers trim, lowercased dedup and first-seen order.
func TestNormalizeHostTags(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", []string{}, []string{}},
		{"all-empty dropped", []string{"", "  ", ""}, []string{}},
		{"simple", []string{"set:foo"}, []string{"set:foo"}},
		{"preserves order", []string{"tag:z", "set:a", "tag:b"}, []string{"tag:z", "set:a", "tag:b"}},
		{"dedup case-insensitive", []string{"set:FOO", "set:foo", "SET:Foo"}, []string{"set:FOO"}},
		{"trim whitespace", []string{"  set:foo  ", "tag:bar"}, []string{"set:foo", "tag:bar"}},
		{"dedup after trim", []string{"  set:foo", "set:foo"}, []string{"set:foo"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeHostTags(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("NormalizeHostTags(%v) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("idx %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestAliasDomainRegexUnderscore (A12 regression) confirms that the domain
// regex accepts underscore-prefixed/suffixed owner names required for DMARC
// (_dmarc), DKIM (default._domainkey), SRV (_sip._tcp) and ACME DNS-01
// (_acme-challenge), while still rejecting malformed domains.
func TestAliasDomainRegexUnderscore(t *testing.T) {
	accept := []string{
		"_dmarc.local",
		"_sip._tcp.example.com",
		"default._domainkey.example.com",
		"_acme-challenge.example.com",
		"nas.local",
		"a",
		"_",
	}
	reject := []string{
		"",
		"-leading.example.com",
		".leadingdot.example.com",
		"with space.example.com",
		"bad!char.example.com",
	}
	for _, d := range accept {
		if !aliasDomainRegex.MatchString(d) {
			t.Errorf("aliasDomainRegex should accept %q (A12)", d)
		}
	}
	for _, d := range reject {
		if aliasDomainRegex.MatchString(d) {
			t.Errorf("aliasDomainRegex should reject %q", d)
		}
	}
}
