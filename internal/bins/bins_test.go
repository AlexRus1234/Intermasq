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

package bins

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// TestResolveBin_EmptyInputs covers the "nothing found" tail: flag empty,
// no candidate on $PATH, no existing fallback → "".
func TestResolveBin_EmptyInputs(t *testing.T) {
	got := resolveBin("", []string{"nonexistent-bin-xyz-12345"}, []string{"/nonexistent/path/bin-xyz-12345"})
	if got != "" {
		t.Fatalf("expected empty path, got %q", got)
	}
}

// TestResolveBin_FlagDir covers the Fall-through path: a flag value that
// points to a directory must NOT be returned (isExecutable==false), and the
// function proceeds to candidates/fallbacks.
func TestResolveBin_FlagDir(t *testing.T) {
	tmp := t.TempDir()
	got := resolveBin(tmp, []string{"nonexistent-bin-xyz-12345"}, []string{"/nonexistent/path/bin-xyz-12345"})
	if got != "" {
		t.Fatalf("expected empty path when flag is dir, got %q", got)
	}
}

// TestResolveBin_FlagNonexistent covers the Fprintf-side branch when the
// flag override file does not exist.
func TestResolveBin_FlagNonexistent(t *testing.T) {
	got := resolveBin("/path/that/does/not/exist/12345", []string{"nonexistent-bin-xyz-12345"}, nil)
	if got != "" {
		t.Fatalf("expected empty path, got %q", got)
	}
}

// TestResolveBin_FlagExecutable executes the success path on platforms that
// honor the Unix exec bit. On Windows os.Stat never reports 0o111 for a
// regular file, so the flag-success branch is CI-Linux only.
func TestResolveBin_FlagExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exec-bit not honored on Windows")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "fakebin")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := resolveBin(bin, []string{"nonexistent-bin-xyz-12345"}, nil)
	if got != bin {
		t.Fatalf("expected flag path %q, got %q", bin, got)
	}
}

// TestResolveBin_FallbackExecutable covers the fallback-success path
// (Linux-only, see above rationale).
func TestResolveBin_FallbackExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exec-bit not honored on Windows")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "fakebin")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := resolveBin("", []string{"nonexistent-bin-xyz-12345"}, []string{bin})
	if got != bin {
		t.Fatalf("expected fallback path %q, got %q", bin, got)
	}
}

// TestResolveBin_CandidateFound covers the exec.LookPath success path. We
// create a temp dir, place an executable script in it with a unique name,
// prepend it to $PATH, and expect LookPath to resolve it.
func TestResolveBin_CandidateFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("$PATH manipulation not portable on Windows test runner")
	}
	tmp := t.TempDir()
	name := "fakebin-unique-12345"
	bin := filepath.Join(tmp, name)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", tmp+string(os.PathListSeparator)+origPath)

	got := resolveBin("", []string{name}, nil)
	if got == "" {
		t.Fatalf("expected LookPath to resolve %q, got empty", name)
	}
	// LookPath may return an absolute or PATH-relative result; just verify the
	// resolved file is executable and equals our script (resolve it).
	resolved, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("LookPath second call failed: %v", err)
	}
	if got != resolved {
		t.Fatalf("expected %q, got %q", resolved, got)
	}
}

// TestIsExecutable covers all three return paths of isExecutable.
func TestIsExecutable(t *testing.T) {
	// Nonexistent path → false (os.Stat error).
	if isExecutable("/path/that/does/not/exist/12345") {
		t.Error("expected false for nonexistent path")
	}
	// Directory → false (IsDir==true).
	dir := t.TempDir()
	if isExecutable(dir) {
		t.Error("expected false for directory")
	}
	// Regular file without exec bit.
	tmp := t.TempDir()
	f := filepath.Join(tmp, "noexec")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if isExecutable(f) {
			t.Error("expected false for file without exec bit")
		}
		// Regular file with exec bit → true.
		f2 := filepath.Join(tmp, "exec")
		if err := os.WriteFile(f2, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
		if !isExecutable(f2) {
			t.Error("expected true for file with exec bit")
		}
	}
}

// TestLazyAccessors_CallResolve covers the lazy-init accessors: when the
// package var is empty they trigger Resolve() (which is sync.Once and likely
// already primed by main/init); the accessor must simply return the stored
// string without panicking. We exercise every accessor so the 0%-rows in
// coverage flip to covered. Two consecutive calls must agree (idempotent
// lazy init), and the returned value must match the underlying package var
// (cache consistency — accessor must not fabricate a value unrelated to the
// cached path).
func TestLazyAccessors_CallResolve(t *testing.T) {
	accessors := []struct {
		name string
		fn   func() string
		ptr  *string
	}{
		{"Dnsmasq", Dnsmasq, &dnsmasqBinPath},
		{"Sudo", Sudo, &sudoBinPath},
		{"Systemctl", Systemctl, &systemctlBinPath},
		{"Service", Service, &serviceBinPath},
		{"RcService", RcService, &rcServiceBinPath},
		{"Sv", Sv, &svBinPath},
	}
	for _, a := range accessors {
		got1, got2 := a.fn(), a.fn()
		if got1 != got2 {
			t.Errorf("%s: idempotency broken — first call %q, second call %q", a.name, got1, got2)
		}
		if got1 != *a.ptr {
			t.Errorf("%s: returned %q but underlying var is %q (cache mismatch)", a.name, got1, *a.ptr)
		}
	}
}

// TestLazyAccessors_ResolveBranch forces the empty-var branch of a lazy
// accessor by resetting binsOnce and clearing the cached path (in-package
// reset — only possible because this test lives in package bins). The
// accessor must invoke Resolve() and repopulate the var; the returned value
// must again match the var. This covers the resolve-on-empty branch that
// TestLazyAccessors_CallResolve cannot force once the Once is primed.
func TestLazyAccessors_ResolveBranch(t *testing.T) {
	// Fresh guard + cleared cache → next accessor call must re-resolve.
	// (Assigning a brand-new sync.Once value is fine; copying an existing
	// one is not, so we never snapshot binsOnce by value.)
	binsOnce = sync.Once{}
	dnsmasqBinPath = ""
	got := Dnsmasq()
	// Resolve() ran inside Dnsmasq(); the var must match the return value
	// (resolved path or "" when dnsmasq is not installed on the host). The
	// Once is now primed again — consistent steady state for later tests.
	if got != dnsmasqBinPath {
		t.Fatalf("Dnsmasq()=%q but dnsmasqBinPath=%q after lazy resolve", got, dnsmasqBinPath)
	}
}

// TestSetPathForTest covers the cross-package seam: it substitutes the var
// for the duration of the (sub)test and, on cleanup, restores the original
// value while resetting binsOnce so a later accessor call can re-resolve.
func TestSetPathForTest(t *testing.T) {
	fake := "/tmp/fake-bin-SetPathForTest-12345"

	t.Run("substitutes_and_restores", func(t *testing.T) {
		SetPathForTest(t, "sudo", fake)
		if sudoBinPath != fake {
			t.Fatalf("SetPathForTest did not set sudoBinPath: got %q, want %q", sudoBinPath, fake)
		}
		if got := Sudo(); got != fake {
			t.Fatalf("Sudo()=%q, want %q", got, fake)
		}
		// Inner cleanup registered by SetPathForTest runs at subtest end.
	})
	// After the subtest, sudoBinPath is restored by the cleanup. readCached
	// (no accessor call) just reads the raw var, so the once-reset performed
	// by cleanup does not perturb what we observe here.
	if sudoBinPath == fake {
		t.Errorf("sudoBinPath not restored after subtest cleanup: still %q", fake)
	}

	// Every recognised name must be accepted (round-trip through each arm of
	// the switch without hitting the Fatalf default).
	for _, name := range []string{"dnsmasq", "sudo", "systemctl", "service", "rc-service", "sv"} {
		t.Run(name, func(t *testing.T) {
			SetPathForTest(t, name, fake)
			if got := readCached(t, name); got != fake {
				t.Errorf("%s: SetPathForTest set %q, want %q", name, got, fake)
			}
		})
	}
}

// readCached returns the current cached path for name (test-only helper).
func readCached(t *testing.T, name string) string {
	t.Helper()
	switch name {
	case "dnsmasq":
		return dnsmasqBinPath
	case "sudo":
		return sudoBinPath
	case "systemctl":
		return systemctlBinPath
	case "service":
		return serviceBinPath
	case "rc-service":
		return rcServiceBinPath
	case "sv":
		return svBinPath
	}
	t.Fatalf("readCached: unknown name %q", name)
	return ""
}
