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

package main

// Coverage sweep block C — goroutine-extract tests (логи/Coverage_sweep.md
// §2.C, T-C.2/3/4). Each extracted helper has a deterministic, side-effect
// scoped test that does NOT spawn the background goroutine, so there is no
// sleeps/race-detector flakiness.

import (
	"path/filepath"
	"strings"
	"testing"

	"intermask/internal/initd"
)

// ===== T-C.2 ssePollOnce =====

func TestSsePollOnce(t *testing.T) {
	// Point ConfigDir at a temp dir so getArpTable (which reads *ArpPath)
	// and checkDnsmasqStatus (which calls sysCaller.IsActive) are safe.
	newTestDir(t)
	origArp := *ArpPath
	*ArpPath = filepath.Join(t.TempDir(), "no-arp")
	t.Cleanup(func() { *ArpPath = origArp })
	initd.SetCurrentForTest(t, &initd.NoneCaller{})

	// Register a throwaway client so we can observe the broadcast side effect
	// is NOT produced here (ssePollOnce just returns values; the broadcaster
	// loop decides whether to send). We only assert the return values.
	arpJSON, status := ssePollOnce()
	if !status {
		// NoneCaller.IsActive returns true, so status must be true.
		t.Errorf("expected status=true (NoneCaller), got false")
	}
	// arpJSON is a marshalled empty map ("{}") since /tmp/.../no-arp is absent.
	if !strings.Contains(arpJSON, "{") {
		t.Errorf("expected JSON object in arpJSON, got %q", arpJSON)
	}
}

func TestSsePollOnce_BroadcastsOnDelta(t *testing.T) {
	newTestDir(t)
	origArp := *ArpPath
	*ArpPath = filepath.Join(t.TempDir(), "no-arp")
	t.Cleanup(func() { *ArpPath = origArp })
	initd.SetCurrentForTest(t, &initd.NoneCaller{})

	// Spin a fake client and run one broadcaster-iteration logic by hand.
	client := &sseClient{ch: make(chan string, 1)}
	sseRegister(client)
	t.Cleanup(func() { sseUnregister(client) })

	// First poll: lastArp differs from "" → should broadcast "arp".
	arpJSON, status := ssePollOnce()
	lastArp := ""
	lastStatus := false
	if arpJSON != lastArp {
		sseBroadcast("arp", arpJSON)
		lastArp = arpJSON
	}
	if status != lastStatus {
		sseBroadcast("dnsmasq_status", `{"active":true}`)
		lastStatus = status
	}
	select {
	case msg := <-client.ch:
		if !strings.HasPrefix(msg, "event: arp") {
			t.Errorf("expected arp event first, got: %q", msg)
		}
	default:
		t.Error("expected first poll to broadcast an arp event")
	}
	// Second poll: identical values → no further broadcast.
	arpJSON2, status2 := ssePollOnce()
	if arpJSON2 != lastArp {
		sseBroadcast("arp", arpJSON2)
	}
	if status2 != lastStatus {
		sseBroadcast("dnsmasq_status", `{"active":true}`)
	}
	select {
	case <-client.ch:
		t.Error("second identical poll should not broadcast")
	default:
	}
}
