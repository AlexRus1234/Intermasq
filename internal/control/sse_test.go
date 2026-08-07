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

package control

// White-box unit tests for the SSE broker + dnsmasq reload helpers. migrated
// from the main package's dnsmasq_test.go (SSE broker block), goroutines_test.go
// (ssePollOnce block) and linux_test.go (TestReloadDnsmasq_* block) during
// stage 9 of the modularization. Being in package control gives the tests
// access to the unexported pollOnce helper and the clients map.
//
// The reload tests are Linux-gated: they inject a fake `dnsmasq` shell-script
// via bins.SetPathForTest (internal/bins). On Windows the shebang script is
// not executable by os/exec, so fakeDnsmasq skips. The shell-script + bin-path
// wiring is intentionally duplicated here as a minimal in-package helper
// rather than reaching across packages for the main package's linux_test.go
// fixtures.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"intermask/internal/bins"
	"intermask/internal/initd"
	"intermask/internal/netstate"
)

// ========== SSE broker ==========

func TestSseRegisterUnregister(t *testing.T) {
	cl := &Client{Ch: make(chan string, 1)}
	Register(cl)
	if !clients[cl] {
		t.Fatal("client should be registered")
	}
	Unregister(cl)
	if clients[cl] {
		t.Fatal("client should be unregistered")
	}
}

func TestSseBroadcast(t *testing.T) {
	cl := &Client{Ch: make(chan string, 10)}
	Register(cl)
	defer Unregister(cl)
	Broadcast("arp", `{"aa:bb:cc:dd:ee:ff":true}`)
	select {
	case msg := <-cl.Ch:
		if !strings.Contains(msg, "event: arp") {
			t.Errorf("bad event: %s", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("message not received")
	}
}

func TestSseBroadcastFullChannel(t *testing.T) {
	cl := &Client{Ch: make(chan string, 0)}
	Register(cl)
	defer Unregister(cl)
	Broadcast("arp", "{}")
	select {
	case <-cl.Ch:
		t.Errorf("expected broadcast to be dropped on full/unbuffered channel, but a message was delivered")
	default:
	}
	if len(cl.Ch) != 0 {
		t.Errorf("expected empty channel after broadcast to full/unbuffered channel, got len=%d", len(cl.Ch))
	}
}

func TestArpToJSON(t *testing.T) {
	arp := map[string]bool{"aa:bb:cc:dd:ee:ff": true, "11:22:33:44:55:66": false}
	s := ArpToJSON(arp)
	var decoded map[string]bool
	if err := json.Unmarshal([]byte(s), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(decoded) != 2 {
		t.Errorf("expected 2 entries, got %d", len(decoded))
	}
	if !decoded["aa:bb:cc:dd:ee:ff"] {
		t.Error("expected aa:bb:cc:dd:ee:ff=true")
	}
}

// ========== pollOnce ==========

func TestSsePollOnce(t *testing.T) {
	// Point ArpPath at a temp path so GetArpTable (which reads it) and
	// CheckDnsmasqStatus (which calls the init caller) are safe. The file is
	// absent → GetArpTable returns an empty map → ArpToJSON yields "{}".
	origArp := *netstate.ArpPath
	*netstate.ArpPath = filepath.Join(t.TempDir(), "no-arp")
	t.Cleanup(func() { *netstate.ArpPath = origArp })
	initd.SetCurrentForTest(t, &initd.NoneCaller{})

	arpJSON, status := pollOnce()
	if !status {
		// NoneCaller.IsActive returns true, so status must be true.
		t.Errorf("expected status=true (NoneCaller), got false")
	}
	if !strings.Contains(arpJSON, "{") {
		t.Errorf("expected JSON object in arpJSON, got %q", arpJSON)
	}
}

func TestSsePollOnce_BroadcastsOnDelta(t *testing.T) {
	origArp := *netstate.ArpPath
	*netstate.ArpPath = filepath.Join(t.TempDir(), "no-arp")
	t.Cleanup(func() { *netstate.ArpPath = origArp })
	initd.SetCurrentForTest(t, &initd.NoneCaller{})

	// Spin a fake client and run one broadcaster-iteration logic by hand.
	client := &Client{Ch: make(chan string, 1)}
	Register(client)
	t.Cleanup(func() { Unregister(client) })

	// First poll: lastArp differs from "" → should broadcast "arp".
	arpJSON, status := pollOnce()
	lastArp := ""
	lastStatus := false
	if arpJSON != lastArp {
		Broadcast("arp", arpJSON)
		lastArp = arpJSON
	}
	if status != lastStatus {
		Broadcast("dnsmasq_status", `{"active":true}`)
		lastStatus = status
	}
	select {
	case msg := <-client.Ch:
		if !strings.HasPrefix(msg, "event: arp") {
			t.Errorf("expected arp event first, got: %q", msg)
		}
	default:
		t.Error("expected first poll to broadcast an arp event")
	}
	// Second poll: identical values → no further broadcast.
	arpJSON2, status2 := pollOnce()
	if arpJSON2 != lastArp {
		Broadcast("arp", arpJSON2)
	}
	if status2 != lastStatus {
		Broadcast("dnsmasq_status", `{"active":true}`)
	}
	select {
	case <-client.Ch:
		t.Error("second identical poll should not broadcast")
	default:
	}
}

// ========== ReloadDnsmasq ==========

// fakeDnsmasq writes a shell-script "dnsmasq" that exits with `exitCode`
// into a temp dir, points the cached dnsmasq path (internal/bins) at it, and
// registers cleanup to restore the previous value. On non-Linux hosts it
// skips the test. Minimal in-package copy of the main package fixture so the
// control tests stay self-contained.
func fakeDnsmasq(t *testing.T, exitCode int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-dnsmasq shell-script unsupported on Windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "dnsmasq")
	var script string
	if exitCode == 0 {
		script = "#!/bin/sh\nexit 0\n"
	} else {
		script = "#!/bin/sh\necho 'fake dnsmasq: test failed'\nexit " + strconv.Itoa(exitCode) + "\n"
	}
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake dnsmasq: %v", err)
	}
	bins.SetPathForTest(t, "dnsmasq", bin)
}

// failCaller is a minimal SystemCaller whose Restart returns an error and
// IsActive returns false. Used to drive the post-test-failure branch of
// ReloadDnsmasq (where dnsmasq --test succeeded but the init caller failed).
// Mirrors the same-named type that lived in the main package's linux_test.go
// before stage 9 of the modularization.
type failCaller struct{}

func (f *failCaller) IsActive(service string) bool { return false }
func (f *failCaller) Restart(service string) error { return errFailCallerRestart }
func (f *failCaller) RestartSelf() error           { return errFailCallerRestart }
func (f *failCaller) String() string               { return "fail" }

var errFailCallerRestart = errors.New("caller restart failed")

func TestReloadDnsmasq_Success(t *testing.T) {
	fakeDnsmasq(t, 0)
	initd.SetCurrentForTest(t, &initd.NoneCaller{})
	if err := ReloadDnsmasq(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestReloadDnsmasq_TestFail(t *testing.T) {
	fakeDnsmasq(t, 1)
	initd.SetCurrentForTest(t, &initd.NoneCaller{})
	if err := ReloadDnsmasq(); err == nil || !strings.Contains(err.Error(), "fake dnsmasq") {
		t.Fatalf("expected dnsmasq-test failure propagated, got %v", err)
	}
}

func TestReloadDnsmasq_CallerFail(t *testing.T) {
	fakeDnsmasq(t, 0)
	initd.SetCurrentForTest(t, &failCaller{})
	if err := ReloadDnsmasq(); err == nil {
		t.Fatal("expected caller-restart error, got nil")
	}
}
