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

package oui

import "testing"

func TestLookupOUIKnownVMware(t *testing.T) {
	v := LookupOUI("00:0c:29:aa:bb:cc")
	if v != "VMware" {
		t.Errorf("expected VMware, got %q", v)
	}
}

func TestLookupOUIKnownApple(t *testing.T) {
	v := LookupOUI("f0:18:98:11:22:33")
	if v != "Apple" {
		t.Errorf("expected Apple, got %q", v)
	}
}

func TestLookupOUIUnknown(t *testing.T) {
	v := LookupOUI("ff:ff:ff:aa:bb:cc")
	if v != "" {
		t.Errorf("expected empty for unknown OUI, got %q", v)
	}
}

func TestLookupOUIShort(t *testing.T) {
	v := LookupOUI("aa:bb")
	if v != "" {
		t.Errorf("expected empty for short MAC, got %q", v)
	}
}

func TestLookupOUICaseInsensitive(t *testing.T) {
	v1 := LookupOUI("00:0C:29:AA:BB:CC")
	v2 := LookupOUI("00:0c:29:11:22:33")
	if v1 != "VMware" || v2 != "VMware" {
		t.Errorf("case-insensitive lookup failed: v1=%q v2=%q", v1, v2)
	}
}

func TestLookupOUIKnownCisco(t *testing.T) {
	v := LookupOUI("f4:7a:c2:11:22:33")
	if v != "Cisco" {
		t.Errorf("expected Cisco, got %q", v)
	}
}

func TestLookupOUIKnownNetgear(t *testing.T) {
	v := LookupOUI("c0:3f:0e:11:22:33")
	if v != "Netgear" {
		t.Errorf("expected Netgear, got %q", v)
	}
}
