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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
)

// Binary path overrides. Empty value means: resolve via $PATH, then fall
// back to well-known absolute paths. Needed for distros (Alpine, older
// Debian) where these binaries live under /bin or /sbin rather than /usr/bin
// /usr/sbin. Registered on the default flag set at package init (before
// main runs flag.Parse), so the flags keep their original -dnsmasq-bin /
// -sudo-bin / ... names and defaults.
var (
	DnsmasqBin   = flag.String("dnsmasq-bin", "", "Path to dnsmasq binary (auto-resolved via $PATH if empty)")
	SudoBin      = flag.String("sudo-bin", "", "Path to sudo binary (auto-resolved if empty)")
	SystemctlBin = flag.String("systemctl-bin", "", "Path to systemctl binary (auto-resolved if empty)")
	ServiceBin   = flag.String("service-bin", "", "Path to sysvinit service binary (auto-resolved if empty)")
	RcServiceBin = flag.String("rc-service-bin", "", "Path to OpenRC rc-service binary (auto-resolved if empty)")
	SvBin        = flag.String("sv-bin", "", "Path to runit sv binary (auto-resolved if empty)")
)

// Resolved absolute paths of external binaries. They are populated by
// Resolve() (called from main after flag.Parse) with a lazy fallback so
// tests that bypass main() still get sensible defaults via $PATH lookup.
var (
	dnsmasqBinPath   string
	sudoBinPath      string
	systemctlBinPath string
	serviceBinPath   string
	rcServiceBinPath string
	svBinPath        string

	binsOnce sync.Once
)

// resolveBin picks the binary path in priority order:
//  1. explicit flag value (operator override; must be executable & exist),
//  2. exec.LookPath over candidates (honours $PATH, works on Alpine/Debian
//     where binaries may live in /bin or /sbin instead of /usr/bin),
//  3. the first existing fallback absolute path,
//  4. "" if nothing was found (caller decides whether that is fatal).
func resolveBin(flagVal string, candidates, fallbacks []string) string {
	if flagVal != "" {
		if isExecutable(flagVal) {
			return flagVal
		}
		fmt.Fprintf(os.Stderr, "[BINS] flag override %q not executable; falling back to $PATH\n", flagVal)
	}
	for _, name := range candidates {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, p := range fallbacks {
		if isExecutable(p) {
			return p
		}
	}
	return ""
}

// isExecutable reports whether path exists and looks executable.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Mode()&0o111 != 0
}

// Resolve populates the *BinPath package vars. Idempotent and safe to call
// multiple times (tests may invoke after stubbing flags). The first caller
// wins: subsequent calls are no-ops so tests don't fight main().
func Resolve() {
	binsOnce.Do(func() {
		dnsmasqBinPath = resolveBin(*DnsmasqBin, []string{"dnsmasq"}, []string{"/usr/sbin/dnsmasq", "/usr/bin/dnsmasq", "/bin/dnsmasq", "/sbin/dnsmasq"})
		sudoBinPath = resolveBin(*SudoBin, []string{"sudo"}, []string{"/usr/bin/sudo", "/bin/sudo"})
		systemctlBinPath = resolveBin(*SystemctlBin, []string{"systemctl"}, []string{"/usr/bin/systemctl", "/bin/systemctl"})
		serviceBinPath = resolveBin(*ServiceBin, []string{"service"}, []string{"/usr/sbin/service", "/usr/bin/service", "/sbin/service", "/bin/service"})
		rcServiceBinPath = resolveBin(*RcServiceBin, []string{"rc-service"}, []string{"/usr/bin/rc-service", "/bin/rc-service", "/sbin/rc-service"})
		svBinPath = resolveBin(*SvBin, []string{"sv"}, []string{"/usr/bin/sv", "/bin/sv"})

		if dnsmasqBinPath == "" {
			fmt.Fprintln(os.Stderr, "[BINS] dnsmasq binary not found; `dnsmasq --test` pre-checks will fail")
		}
	})
}

// Dnsmasq returns the resolved dnsmasq path, resolving lazily if Resolve()
// has not run yet (e.g. in unit tests).
func Dnsmasq() string {
	if dnsmasqBinPath == "" {
		Resolve()
	}
	return dnsmasqBinPath
}

func Sudo() string {
	if sudoBinPath == "" {
		Resolve()
	}
	return sudoBinPath
}

func Systemctl() string {
	if systemctlBinPath == "" {
		Resolve()
	}
	return systemctlBinPath
}

func Service() string {
	if serviceBinPath == "" {
		Resolve()
	}
	return serviceBinPath
}

func RcService() string {
	if rcServiceBinPath == "" {
		Resolve()
	}
	return rcServiceBinPath
}

func Sv() string {
	if svBinPath == "" {
		Resolve()
	}
	return svBinPath
}

// SetPathForTest substitutes the *BinPath var matching name with p for the
// duration of the test. The previous value is restored and binsOnce is reset
// via t.Cleanup, so a subsequent lazy accessor call may trigger a fresh
// Resolve() (otherwise the sync.Once would be permanently primed). Recognised
// names: "dnsmasq", "sudo", "systemctl", "service", "rc-service", "sv".
//
// Exported for cross-package tests during modularization (fake-bin tests in
// package main); the in-package white-box tests reset binsOnce directly.
func SetPathForTest(t *testing.T, name string, p string) {
	var ptr *string
	switch name {
	case "dnsmasq":
		ptr = &dnsmasqBinPath
	case "sudo":
		ptr = &sudoBinPath
	case "systemctl":
		ptr = &systemctlBinPath
	case "service":
		ptr = &serviceBinPath
	case "rc-service":
		ptr = &rcServiceBinPath
	case "sv":
		ptr = &svBinPath
	default:
		t.Fatalf("SetPathForTest: unknown binary name %q", name)
		return
	}
	orig := *ptr
	*ptr = p
	t.Cleanup(func() {
		*ptr = orig
		binsOnce = sync.Once{}
	})
}
