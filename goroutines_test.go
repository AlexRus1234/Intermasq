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
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

// ===== T-C.3 runDNSHealthPass =====

// withDNSResolver swaps the package-level dnsResolver for the test and
// restores it on cleanup.
func withDNSResolver(t *testing.T, fn func(ctx context.Context, domain string) ([]string, error)) {
	t.Helper()
	orig := dnsResolver
	dnsResolver = fn
	t.Cleanup(func() { dnsResolver = orig })
}

func TestRunDNSHealthPass_NoAliases(t *testing.T) {
	newTestDir(t) // empty ConfigDir → readAllAliases() returns []
	// Clear any prior dnsHealth entries so the assertion below is stable.
	dnsHealthMu.Lock()
	for k := range dnsHealth {
		delete(dnsHealth, k)
	}
	dnsHealthMu.Unlock()

	runDNSHealthPass()

	dnsHealthMu.RLock()
	defer dnsHealthMu.RUnlock()
	if len(dnsHealth) != 0 {
		t.Errorf("expected no health entries with 0 aliases, got %d", len(dnsHealth))
	}
}

func TestRunDNSHealthPass_HappyAndSadPaths(t *testing.T) {
	dir := newTestDir(t)
	// Seed one .conf with two A aliases and one TXT (which must be skipped).
	conf := []byte("address=/up.lan/10.0.0.1\naddress=/down.lan/10.0.0.2\ntxt-record=skip.lan,ignored\n")
	if err := os.WriteFile(filepath.Join(dir, "dns.conf"), conf, 0o644); err != nil {
		t.Fatal(err)
	}

	// Stub resolver: up.lan resolves, down.lan fails. Everything else errors.
	calls := make(map[string]int)
	var callsMu sync.Mutex
	withDNSResolver(t, func(_ context.Context, domain string) ([]string, error) {
		callsMu.Lock()
		calls[domain]++
		callsMu.Unlock()
		switch domain {
		case "up.lan":
			return []string{"10.0.0.1"}, nil
		case "down.lan":
			return nil, errors.New("no such host")
		default:
			return nil, errors.New("unexpected domain in stub: " + domain)
		}
	})

	// Clear prior health entries.
	dnsHealthMu.Lock()
	for k := range dnsHealth {
		delete(dnsHealth, k)
	}
	dnsHealthMu.Unlock()

	runDNSHealthPass()

	dnsHealthMu.RLock()
	defer dnsHealthMu.RUnlock()
	if len(dnsHealth) != 2 {
		t.Fatalf("expected 2 health entries, got %d: %+v", len(dnsHealth), dnsHealth)
	}
	if h, ok := dnsHealth["up.lan"]; !ok || !h.Up {
		t.Errorf("expected up.lan Up=true, got %+v", dnsHealth["up.lan"])
	}
	if h, ok := dnsHealth["down.lan"]; !ok || h.Up {
		t.Errorf("expected down.lan Up=false, got %+v", dnsHealth["down.lan"])
	}
	callsMu.Lock()
	if calls["up.lan"] != 1 || calls["down.lan"] != 1 {
		t.Errorf("stub call counts wrong: %+v", calls)
	}
	if calls["skip.lan"] != 0 {
		t.Errorf("TXT alias should be skipped, got calls=%d", calls["skip.lan"])
	}
	callsMu.Unlock()
}

// ===== T-C.4 cleanupBlacklistOnce =====

func TestCleanupBlacklistOnce_RemovesExpired(t *testing.T) {
	blacklistMu.Lock()
	// Reset to a known state.
	for k := range blacklist {
		delete(blacklist, k)
	}
	blacklist["expired"] = time.Now().Add(-time.Hour)
	blacklist["future"] = time.Now().Add(time.Hour)
	blacklistMu.Unlock()
	t.Cleanup(func() {
		blacklistMu.Lock()
		for k := range blacklist {
			delete(blacklist, k)
		}
		blacklistMu.Unlock()
	})

	cleanupBlacklistOnce(time.Now())

	blacklistMu.RLock()
	defer blacklistMu.RUnlock()
	if _, ok := blacklist["expired"]; ok {
		t.Error("expected expired entry to be removed")
	}
	if _, ok := blacklist["future"]; !ok {
		t.Error("expected future entry to be retained")
	}
}

func TestCleanupBlacklistOnce_EmptyMap(t *testing.T) {
	blacklistMu.Lock()
	for k := range blacklist {
		delete(blacklist, k)
	}
	blacklistMu.Unlock()

	// Must not panic on an empty map and must leave it empty (no spurious
	// inserts / no mutation of unrelated state).
	cleanupBlacklistOnce(time.Now())

	blacklistMu.RLock()
	n := len(blacklist)
	blacklistMu.RUnlock()
	if n != 0 {
		t.Errorf("cleanupBlacklistOnce on empty map left %d entries", n)
	}
}
