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

package main

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// Resolved absolute paths of external binaries. They are populated by
// resolveBins() (called from main after flag.Parse) with a lazy fallback so
// tests that bypass main() still get sensible defaults via $PATH lookup.
var (
	dnsmasqBinPath string
	sudoBinPath    string
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

// resolveBins populates the *BinPath package vars. Idempotent and safe to
// call multiple times (tests may invoke after stubbing flags). The first
// caller wins: subsequent calls are no-ops so tests don't fight main().
func resolveBins() {
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

// dnsmasqBin returns the resolved dnsmasq path, resolving lazily if main()
// has not run yet (e.g. in unit tests).
func dnsmasqBin() string {
	if dnsmasqBinPath == "" {
		resolveBins()
	}
	return dnsmasqBinPath
}
func sudoBin() string {
	if sudoBinPath == "" {
		resolveBins()
	}
	return sudoBinPath
}
func systemctlBin() string {
	if systemctlBinPath == "" {
		resolveBins()
	}
	return systemctlBinPath
}
func serviceBin() string {
	if serviceBinPath == "" {
		resolveBins()
	}
	return serviceBinPath
}
func rcServiceBin() string {
	if rcServiceBinPath == "" {
		resolveBins()
	}
	return rcServiceBinPath
}
func svBin() string {
	if svBinPath == "" {
		resolveBins()
	}
	return svBinPath
}
