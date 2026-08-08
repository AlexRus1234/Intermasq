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

package dnsmasq

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"intermask/internal/bins"
)

// This file holds the in-package copies of the fake-dnsmasq test helpers
// that the migrated linux-only tests (TestWriteConfigWithTest_*,
// TestRestoreHistoryVersion_*) need to inject a stub binary via
// bins.SetPathForTest. The originals now live in internal/webapi's
// helpers_test.go because the handler-level wiring tests
// (TestPutFileHandler_*, TestUpdateConfigHandler_*,
// TestRestoreBackupHandler_*, TestReload*) moved there during stage 11 and
// also depend on them. The two copies cannot share code without polluting
// the production API of one package or the other, so the duplication is
// intentional.

// fakeDnsmasq writes a shell-script "dnsmasq" that exits with `exitCode`
// into a temp dir, points the cached dnsmasq path (internal/bins, via
// bins.SetPathForTest) at it, and registers cleanup to restore the previous
// value. The script honours the shebang convention; on non-Linux hosts
// callers must t.Skip() beforehand.
//
// If exitCode is 0, the script behaves like the real dnsmasq's `--test`
// success path. If exitCode != 0, the script prints a marker to stderr+stdout
// (so CombinedOutput captures a non-empty string) and exits with that code.
func fakeDnsmasq(t *testing.T, exitCode int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-dnsmasq shell-script unsupported on Windows")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "dnsmasq")
	var script string
	if exitCode == 0 {
		script = "#!/bin/sh\nexit 0\n"
	} else {
		script = "#!/bin/sh\necho 'fake dnsmasq: test failed'\nexit " + itoa(exitCode) + "\n"
	}
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake dnsmasq: %v", err)
	}
	bins.SetPathForTest(t, "dnsmasq", bin)
}

// fakeDnsmasqStrict installs a fake `dnsmasq` that ACTUALLY inspects the
// config file named by --conf-file=<path> and exits 1 if it contains the
// marker `# INVALID` (exit 0 otherwise). This lets a test assert that
// WriteConfigWithTest / WriteFileRaw genuinely ran `dnsmasq --test` against
// the just-written content AND that a content rejection surfaces as
// `dnsmasq_test_failed` — something the plain fakeDnsmasq (which is
// `#!/bin/sh\nexit 0` and accepts any garbage) cannot do.
func fakeDnsmasqStrict(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-dnsmasq shell-script unsupported on Windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "dnsmasq")
	// Parse --conf-file=<path> out of argv, read that file, reject on the
	// `# INVALID` marker. `grep -q` + `exit 1` mirrors a real `dnsmasq --test`
	// content rejection closely enough for the success/failure wiring under
	// test. Writes nothing to stdout/stderr on success; on rejection prints a
	// short message so CombinedOutput captures a non-empty body (matching the
	// real dnsmasq --test failure shape that becomes `dnsmasq_test_failed: ...`).
	script := `#!/bin/sh
conf=""
for arg in "$@"; do
    case "$arg" in
        --conf-file=*) conf="${arg#--conf-file=}" ;;
    esac
done
if [ -n "$conf" ] && [ -f "$conf" ]; then
    if grep -q '# INVALID' "$conf"; then
        echo "fake dnsmasq: invalid config"
        exit 1
    fi
fi
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write strict fake dnsmasq: %v", err)
	}
	bins.SetPathForTest(t, "dnsmasq", bin)
}

// itoa is a tiny stdlib-free itoa so we avoid importing strconv just for the
// script-assembly line above. Supports non-negative ints only.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
