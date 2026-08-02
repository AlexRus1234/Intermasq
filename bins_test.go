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

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
// package var is empty they trigger resolveBins() (which is sync.Once and
// likely already primed by main/init); the accessor must simply return the
// stored string without panicking. We exercise every accessor so the 0%-rows
// in coverage flip to covered (the var is already non-empty on a real test
// run because main_test/setup ran resolveBins elsewhere; the empty-branch is
// not practical to force — see T-D for the *BinPath assignment seam).
func TestLazyAccessors_CallResolve(t *testing.T) {
	// Each accessor must return a stable string (resolved path or "" if the
	// binary is not installed on the host — both are valid). Two consecutive
	// calls must agree (idempotent lazy init), and the returned value must
	// match the underlying package var (cache consistency — accessor must
	// not fabricate a value unrelated to the cached path).
	accessors := []struct {
		name string
		fn   func() string
		ptr  *string
	}{
		{"dnsmasqBin", dnsmasqBin, &dnsmasqBinPath},
		{"sudoBin", sudoBin, &sudoBinPath},
		{"systemctlBin", systemctlBin, &systemctlBinPath},
		{"serviceBin", serviceBin, &serviceBinPath},
		{"rcServiceBin", rcServiceBin, &rcServiceBinPath},
		{"svBin", svBin, &svBinPath},
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
