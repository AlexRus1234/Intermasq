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

package initd

// Coverage sweep block C — system.go bootstrap tests (логи/Coverage_sweep.md
// §2.C). detectInitSystem is tested by pointing the package var
// procOneCommPath at a temp file with a candidate init name, then asserting
// the returned string. The fallback-branch cases (where /proc/1/comm is
// unreadable) are exercised by pointing at a non-existent path and relying
// on the installed-bin accessors — those depend on bins.RcService()/bins.Sv()/…,
// which resolve via $PATH, so we only assert the result is one of the known
// init names or "none".

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"intermask/internal/bins"
)

// withCommPath swaps procOneCommPath for the test and restores it on cleanup.
func withCommPath(t *testing.T, path string) {
	t.Helper()
	orig := procOneCommPath
	procOneCommPath = path
	t.Cleanup(func() { procOneCommPath = orig })
}

func TestDetectInitSystem_Systemd(t *testing.T) {
	if runtime.GOOS == "windows" {
		// bins.RcService() in the init-read-fail branch calls bins.Resolve()
		// which on Windows may return "" — that's fine, but the systemd
		// success branch doesn't touch bins.RcService(), so this case is
		// actually portable. Kept gated only for consistency with the suite.
	}
	tmp := t.TempDir()
	comm := filepath.Join(tmp, "comm")
	if err := os.WriteFile(comm, []byte("systemd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCommPath(t, comm)
	if got := detectInitSystem(); got != "systemd" {
		t.Errorf("detectInitSystem(systemd) = %q, want systemd", got)
	}
}

func TestDetectInitSystem_Runit(t *testing.T) {
	tmp := t.TempDir()
	comm := filepath.Join(tmp, "comm")
	if err := os.WriteFile(comm, []byte("runit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCommPath(t, comm)
	if got := detectInitSystem(); got != "runit" {
		t.Errorf("detectInitSystem(runit) = %q, want runit", got)
	}
}

func TestDetectInitSystem_InitOpenRC(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bins.RcService() lookup behaviour is Linux-specific")
	}
	// Need rc-service on $PATH for the openrc branch. The CI Fedora image
	// doesn't ship openrc, so this is unpredictable — we only run it on
	// hosts where bins.RcService() != "". If not, skip rather than assert a
	// false negative.
	if bins.RcService() == "" {
		t.Skip("rc-service not installed; openrc detection branch not exercisable")
	}
	tmp := t.TempDir()
	comm := filepath.Join(tmp, "comm")
	if err := os.WriteFile(comm, []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCommPath(t, comm)
	if got := detectInitSystem(); got != "openrc" {
		t.Errorf("detectInitSystem(init+rc-service) = %q, want openrc", got)
	}
}

func TestDetectInitSystem_InitSysVinit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bins.Service() lookup behaviour is Linux-specific")
	}
	// sysvinit branch fires when comm=="init" and rc-service is NOT found.
	// Force rcServiceBinPath to empty so the openrc branch is skipped.
	bins.SetPathForTest(t, "rc-service", "")
	// Need `service` on $PATH for the sysvinit branch. Skip if unavailable.
	if bins.Service() == "" {
		t.Skip("sysvinit `service` binary not installed")
	}
	tmp := t.TempDir()
	comm := filepath.Join(tmp, "comm")
	if err := os.WriteFile(comm, []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCommPath(t, comm)
	if got := detectInitSystem(); got != "sysvinit" {
		t.Errorf("detectInitSystem(init, no rc-service) = %q, want sysvinit", got)
	}
}

func TestDetectInitSystem_UnreadableComm_Fallback(t *testing.T) {
	// Point at a non-existent path so os.ReadFile fails; the function
	// must then fall through to the *Bin()-based heuristics and return
	// one of the known init names or "none".
	withCommPath(t, "/path/that/does/not/exist/comm-12345")
	got := detectInitSystem()
	switch got {
	case "systemd", "systemd-user", "openrc", "runit", "sysvinit", "none":
		// acceptable — depends on what's installed on the test host
	default:
		t.Errorf("detectInitSystem(unreadable) = %q, want a known init name or none", got)
	}
}

func TestDetectInitSystem_UnknownComm_Fallback(t *testing.T) {
	// A comm name that is not one of the recognised init processes must
	// also fall through to the bin-based heuristics.
	tmp := t.TempDir()
	comm := filepath.Join(tmp, "comm")
	if err := os.WriteFile(comm, []byte("not-an-init-name\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCommPath(t, comm)
	got := detectInitSystem()
	switch got {
	case "systemd", "systemd-user", "openrc", "runit", "sysvinit", "none":
	default:
		t.Errorf("detectInitSystem(unknown comm) = %q, want a known init name or none", got)
	}
}

// ===== String() methods of every caller (pure, no fakes) =====

func TestCallerStrings(t *testing.T) {
	cases := []struct {
		name string
		c    SystemCaller
		want string
	}{
		{"systemd-root", &SystemdSystemCaller{UseSudo: false}, "systemd (root)"},
		{"systemd-sudo", &SystemdSystemCaller{UseSudo: true}, "systemd (via sudo)"},
		{"systemd-user", &SystemdUserCaller{}, "systemd-user"},
		{"openrc-root", &OpenRCCaller{UseSudo: false}, "openrc (root)"},
		{"openrc-sudo", &OpenRCCaller{UseSudo: true}, "openrc (via sudo)"},
		{"runit-root", &RunitCaller{UseSudo: false, ServiceDir: "/svc"}, "runit (dir=/svc)"},
		{"runit-sudo", &RunitCaller{UseSudo: true, ServiceDir: "/svc"}, "runit (via sudo, dir=/svc)"},
		{"sysvinit-root", &SysVinitCaller{UseSudo: false}, "sysvinit (root)"},
		{"sysvinit-sudo", &SysVinitCaller{UseSudo: true}, "sysvinit (via sudo)"},
		{"none", &NoneCaller{}, "none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ===== ResolveSystemCaller (pure given inputs) =====
// (TestMapLegacyScope and TestResolveSystemCaller already live in
// dnsmasq_test.go in package main — not duplicated here.)

func TestResolveSystemCaller_Systemd(t *testing.T) {
	// The UseSudo field depends on os.Getuid(); we only assert the type
	// via String() prefix, not the sudo/root suffix, to stay portable.
	c := ResolveSystemCaller("systemd")
	s := c.String()
	if !startsWith(s, "systemd") {
		t.Errorf("ResolveSystemCaller(systemd).String() = %q, want systemd* prefix", s)
	}
}

func TestResolveSystemCaller_OpenRC(t *testing.T) {
	c := ResolveSystemCaller("openrc")
	s := c.String()
	if !startsWith(s, "openrc") {
		t.Errorf("ResolveSystemCaller(openrc).String() = %q, want openrc* prefix", s)
	}
}

func TestResolveSystemCaller_Runit(t *testing.T) {
	c := ResolveSystemCaller("runit")
	s := c.String()
	if !startsWith(s, "runit") {
		t.Errorf("ResolveSystemCaller(runit).String() = %q, want runit* prefix", s)
	}
}

func TestResolveSystemCaller_SysVinit(t *testing.T) {
	c := ResolveSystemCaller("sysvinit")
	s := c.String()
	if !startsWith(s, "sysvinit") {
		t.Errorf("ResolveSystemCaller(sysvinit).String() = %q, want sysvinit* prefix", s)
	}
}

func TestResolveSystemCaller_Unknown_FallsBackToDetect(t *testing.T) {
	// An unrecognised string falls into the default arm, which calls
	// detectSystemCaller. We only assert it returns a non-nil caller with
	// a non-empty String().
	c := ResolveSystemCaller("totally-unknown-init")
	if c == nil {
		t.Fatal("expected non-nil caller")
	}
	if c.String() == "" {
		t.Error("expected non-empty String()")
	}
}

// startsWith is a tiny helper to avoid importing strings just here.
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
