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

package metrics

// Coverage sweep block C — DNS health-checker tests (логи/Coverage_sweep.md
// §2.C, T-C.2/3/4). Each test exercises runDNSHealthPass directly without
// spawning the background goroutine, so there is no sleeps/race-detector
// flakiness. Migrated from the main package's goroutines_test.go during
// stage 8 of the modularization.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"intermask/internal/dnsmasq"
)

// newTestDir creates a temp dir, points *dnsmasq.ConfigDir at it, and
// returns the dir. t.TempDir auto-cleans on test completion. Mirrors the
// same-named helper that lived in the main package's handlers_test.go
// before stage 8 of the modularization; the call sites that scanned
// ConfigDir from internal/metrics tests need an in-package equivalent.
func newTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	return dir
}

// withDNSResolver swaps the package-level dnsResolver for the test and
// restores it on cleanup.
func withDNSResolver(t *testing.T, fn func(ctx context.Context, domain string) ([]string, error)) {
	t.Helper()
	orig := dnsResolver
	dnsResolver = fn
	t.Cleanup(func() { dnsResolver = orig })
}

func TestRunDNSHealthPass_NoAliases(t *testing.T) {
	newTestDir(t) // empty ConfigDir → ReadAllAliases() returns []
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
