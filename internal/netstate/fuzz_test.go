// Intermasq - Web panel for dnsmasq
// Copyright (C) 2026 AlexRus1234
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package netstate

import (
	"bufio"
	"strings"
	"testing"
)

func FuzzParseArpContent(f *testing.F) {
	for _, s := range []string{"", "IP address HW type Flags HW address Mask Device\n", "IP address HW type Flags HW address Mask Device\n192.168.1.1 0x1 0x2 aa:bb:cc:dd:ee:ff * eth0\n", strings.Repeat("a b c d\n", 500)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, content string) {
		for mac := range parseArpContent(content) {
			if mac == "" || mac != strings.ToLower(mac) {
				t.Errorf("invalid MAC key %q", mac)
			}
		}
	})
}

func FuzzParseLeasesContent(f *testing.F) {
	for _, s := range []string{"", "0 aa:bb:cc:dd:ee:ff 192.168.1.1 *", "a b c", strings.Repeat("0 aa:bb:cc:dd:ee:ff 10.0.0.1 host\n", 300)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, content string) {
		leases := parseLeasesContent(content)
		expected := 0
		sc := bufio.NewScanner(strings.NewReader(content))
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 3 {
				continue
			}
			if expected >= len(leases) {
				t.Fatalf("missing lease for line %q", sc.Text())
			}
			if leases[expected].Mac != fields[1] || leases[expected].Ip != fields[2] {
				t.Errorf("lease field mismatch")
			}
			expected++
		}
		if expected != len(leases) {
			t.Errorf("entry count mismatch: %d != %d", expected, len(leases))
		}
	})
}
