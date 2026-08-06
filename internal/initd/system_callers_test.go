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

// Coverage sweep block D — system.go init-callers via fake binaries
// (логи/Coverage_sweep.md §2.D + §3.T-D).
//
// VANITY-покрытие: эти тесты лишь проверяют exec-wiring против фейковых
// shell-скриптов, а не против реального systemd/openrc/runit/sysvinit.
// Цифра coverage растёт, доверие — нет. Реальная проверка init-перезагрузки
// остаётся Gap 4 (L5 VM nightly), см. §6 промта.
//
// All tests here are Linux-gated (shebang trick via `fakeBin` skips on
// Windows). No t.Parallel(): we mutate the global *BinPath package vars
// (sudo/systemctl/service/rc-service/sv); cleanup restores via t.Cleanup.

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"intermask/internal/bins"
)

// fakeBin writes a shell-script binary `name` (with `script` body, no
// shebang) into a temp dir, installs it under the matching bin-path var in
// internal/bins for the duration of the test (via bins.SetPathForTest), and
// registers cleanup to restore the previous value. Recognised names:
// "dnsmasq", "sudo", "systemctl", "service", "rc-service", "sv". Windows is
// skipped because the shebang trick is not honoured by os/exec there.
//
// Inlined mirror of the helper that lives in main-package linux_test.go:
// the cross-package seam (bins.SetPathForTest) lets both copies stay in
// sync without one package reaching into the other's test files.
func fakeBin(t *testing.T, name, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script binary unsupported on Windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, name)
	full := "#!/bin/sh\n" + script + "\n"
	if err := os.WriteFile(bin, []byte(full), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
	bins.SetPathForTest(t, name, bin)
}

// sudoDispatch is the body of a fake `sudo`: it strips any leading
// dash-flags (notably `-n` from SystemdSystemCaller.IsActive) and then
// execs the rest of argv verbatim. This mirrors real sudo's argv handling
// closely enough for the UseSudo=true branches to reach the same fake
// wrapped binary (systemctl/rc-service/sv/service) as the UseSudo=false
// branches. Without the flag-strip (a naive `shift`) `Restart`/`RestartSelf`
// — which call `sudo <bin> restart <svc>` with no `-n` — would drop the
// wrapped binary path itself and exec `restart <svc>` (exit 127).
const sudoDispatch = `while [ $# -gt 0 ]; do
	case "$1" in
		-*) shift ;;
		*) break ;;
	esac
done
exec "$@"`

type binScript struct {
	name   string
	script string
}

// ===== SystemdSystemCaller =====

func TestSystemdSystemCaller_IsActive_Fakes(t *testing.T) {
	cases := []struct {
		name    string
		bins    []binScript
		useSudo bool
		want    bool
	}{
		{
			name:    "root_active",
			bins:    []binScript{{"systemctl", "echo active"}},
			useSudo: false,
			want:    true,
		},
		{
			name:    "root_inactive",
			bins:    []binScript{{"systemctl", "echo inactive"}},
			useSudo: false,
			want:    false,
		},
		{
			name: "sudo_active",
			bins: []binScript{
				{"sudo", sudoDispatch},
				{"systemctl", "echo active"},
			},
			useSudo: true,
			want:    true,
		},
		{
			name: "sudo_inactive",
			bins: []binScript{
				{"sudo", sudoDispatch},
				{"systemctl", "echo inactive"},
			},
			useSudo: true,
			want:    false,
		},
		{
			name:    "root_empty_output",
			bins:    []binScript{{"systemctl", "exit 0"}},
			useSudo: false,
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, b := range tc.bins {
				fakeBin(t, b.name, b.script)
			}
			c := &SystemdSystemCaller{UseSudo: tc.useSudo}
			if got := c.IsActive("dnsmasq"); got != tc.want {
				t.Errorf("IsActive = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSystemdSystemCaller_Restart_Fakes(t *testing.T) {
	cases := []struct {
		name    string
		bins    []binScript
		useSudo bool
		wantErr error
	}{
		{
			name:    "root_ok",
			bins:    []binScript{{"systemctl", "exit 0"}},
			useSudo: false,
			wantErr: nil,
		},
		{
			name:    "root_fail",
			bins:    []binScript{{"systemctl", "exit 7"}},
			useSudo: false,
			wantErr: errAny,
		},
		{
			name:    "sudo_ok",
			bins:    []binScript{{"sudo", sudoDispatch}, {"systemctl", "exit 0"}},
			useSudo: true,
			wantErr: nil,
		},
		{
			name:    "sudo_fail",
			bins:    []binScript{{"sudo", sudoDispatch}, {"systemctl", "exit 7"}},
			useSudo: true,
			wantErr: errAny,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, b := range tc.bins {
				fakeBin(t, b.name, b.script)
			}
			c := &SystemdSystemCaller{UseSudo: tc.useSudo}
			err := c.Restart("dnsmasq")
			checkErr(t, err, tc.wantErr)
		})
	}
}

func TestSystemdSystemCaller_RestartSelf_Fakes(t *testing.T) {
	cases := []struct {
		name    string
		bins    []binScript
		useSudo bool
		wantErr error
	}{
		{
			name:    "root_ok",
			bins:    []binScript{{"systemctl", "exit 0"}},
			useSudo: false,
			wantErr: nil,
		},
		{
			name:    "sudo_ok",
			bins:    []binScript{{"sudo", sudoDispatch}, {"systemctl", "exit 0"}},
			useSudo: true,
			wantErr: nil,
		},
		{
			name:    "root_fail",
			bins:    []binScript{{"systemctl", "exit 1"}},
			useSudo: false,
			wantErr: errAny,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, b := range tc.bins {
				fakeBin(t, b.name, b.script)
			}
			c := &SystemdSystemCaller{UseSudo: tc.useSudo}
			err := c.RestartSelf()
			checkErr(t, err, tc.wantErr)
		})
	}
}

// ===== SystemdUserCaller (no UseSudo field) =====

func TestSystemdUserCaller_Fakes(t *testing.T) {
	t.Run("IsActive_active", func(t *testing.T) {
		fakeBin(t, "systemctl", "echo active")
		c := &SystemdUserCaller{}
		if !c.IsActive("dnsmasq") {
			t.Error("expected IsActive=true on 'active' output")
		}
	})
	t.Run("IsActive_inactive", func(t *testing.T) {
		fakeBin(t, "systemctl", "echo inactive")
		c := &SystemdUserCaller{}
		if c.IsActive("dnsmasq") {
			t.Error("expected IsActive=false on non-active output")
		}
	})
	t.Run("Restart_ok", func(t *testing.T) {
		fakeBin(t, "systemctl", "exit 0")
		c := &SystemdUserCaller{}
		if err := c.Restart("dnsmasq"); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("Restart_fail", func(t *testing.T) {
		fakeBin(t, "systemctl", "exit 5")
		c := &SystemdUserCaller{}
		if err := c.Restart("dnsmasq"); err == nil {
			t.Error("expected error on failing fake systemctl")
		}
	})
	t.Run("RestartSelf_ok", func(t *testing.T) {
		fakeBin(t, "systemctl", "exit 0")
		c := &SystemdUserCaller{}
		if err := c.RestartSelf(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("RestartSelf_fail", func(t *testing.T) {
		fakeBin(t, "systemctl", "exit 3")
		c := &SystemdUserCaller{}
		if err := c.RestartSelf(); err == nil {
			t.Error("expected error on failing fake systemctl")
		}
	})
}

// ===== OpenRCCaller =====

func TestOpenRCCaller_Fakes(t *testing.T) {
	t.Run("IsActive_root_started", func(t *testing.T) {
		fakeBin(t, "rc-service", "echo 'started'")
		c := &OpenRCCaller{}
		if !c.IsActive("dnsmasq") {
			t.Error("expected IsActive=true on 'started' output")
		}
	})
	t.Run("IsActive_root_stopped", func(t *testing.T) {
		fakeBin(t, "rc-service", "echo 'stopped'")
		c := &OpenRCCaller{}
		if c.IsActive("dnsmasq") {
			t.Error("expected IsActive=false on 'stopped' output")
		}
	})
	t.Run("IsActive_sudo_started", func(t *testing.T) {
		fakeBin(t, "sudo", sudoDispatch)
		fakeBin(t, "rc-service", "echo '* status: started'")
		c := &OpenRCCaller{UseSudo: true}
		if !c.IsActive("dnsmasq") {
			t.Error("expected IsActive=true via sudo with 'started' substring")
		}
	})
	t.Run("Restart_root_ok", func(t *testing.T) {
		fakeBin(t, "rc-service", "exit 0")
		c := &OpenRCCaller{}
		if err := c.Restart("dnsmasq"); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("Restart_root_fail", func(t *testing.T) {
		fakeBin(t, "rc-service", "exit 1")
		c := &OpenRCCaller{}
		if err := c.Restart("dnsmasq"); err == nil {
			t.Error("expected error on failing fake rc-service")
		}
	})
	t.Run("Restart_sudo_ok", func(t *testing.T) {
		fakeBin(t, "sudo", sudoDispatch)
		fakeBin(t, "rc-service", "exit 0")
		c := &OpenRCCaller{UseSudo: true}
		if err := c.Restart("dnsmasq"); err != nil {
			t.Errorf("expected no error via sudo, got %v", err)
		}
	})
	t.Run("RestartSelf_root_ok", func(t *testing.T) {
		fakeBin(t, "rc-service", "exit 0")
		c := &OpenRCCaller{}
		if err := c.RestartSelf(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("RestartSelf_sudo_fail", func(t *testing.T) {
		fakeBin(t, "sudo", sudoDispatch)
		fakeBin(t, "rc-service", "exit 2")
		c := &OpenRCCaller{UseSudo: true}
		if err := c.RestartSelf(); err == nil {
			t.Error("expected error via sudo on failing fake rc-service")
		}
	})
}

// ===== RunitCaller =====

func TestRunitCaller_Fakes(t *testing.T) {
	t.Run("IsActive_root_run", func(t *testing.T) {
		fakeBin(t, "sv", "echo 'run: /etc/service/dnsmasq'")
		c := &RunitCaller{ServiceDir: "/etc/service"}
		if !c.IsActive("dnsmasq") {
			t.Error("expected IsActive=true on 'run' substring")
		}
	})
	t.Run("IsActive_root_down", func(t *testing.T) {
		fakeBin(t, "sv", "echo 'down: /etc/service/dnsmasq'")
		c := &RunitCaller{ServiceDir: "/etc/service"}
		if c.IsActive("dnsmasq") {
			t.Error("expected IsActive=false on 'down' output")
		}
	})
	t.Run("IsActive_sudo_run", func(t *testing.T) {
		fakeBin(t, "sudo", sudoDispatch)
		fakeBin(t, "sv", "echo 'run'")
		c := &RunitCaller{UseSudo: true, ServiceDir: "/svc"}
		if !c.IsActive("dnsmasq") {
			t.Error("expected IsActive=true via sudo with 'run' substring")
		}
	})
	t.Run("Restart_root_ok", func(t *testing.T) {
		fakeBin(t, "sv", "exit 0")
		c := &RunitCaller{ServiceDir: "/etc/service"}
		if err := c.Restart("dnsmasq"); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("Restart_sudo_fail", func(t *testing.T) {
		fakeBin(t, "sudo", sudoDispatch)
		fakeBin(t, "sv", "exit 1")
		c := &RunitCaller{UseSudo: true, ServiceDir: "/svc"}
		if err := c.Restart("dnsmasq"); err == nil {
			t.Error("expected error via sudo on failing fake sv")
		}
	})
	t.Run("RestartSelf_root_ok", func(t *testing.T) {
		fakeBin(t, "sv", "exit 0")
		c := &RunitCaller{}
		if err := c.RestartSelf(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("RestartSelf_sudo_ok", func(t *testing.T) {
		fakeBin(t, "sudo", sudoDispatch)
		fakeBin(t, "sv", "exit 0")
		c := &RunitCaller{UseSudo: true, ServiceDir: "/svc"}
		if err := c.RestartSelf(); err != nil {
			t.Errorf("expected no error via sudo, got %v", err)
		}
	})
}

// ===== SysVinitCaller =====

func TestSysVinitCaller_Fakes(t *testing.T) {
	t.Run("IsActive_root_ok", func(t *testing.T) {
		fakeBin(t, "service", "exit 0")
		c := &SysVinitCaller{}
		if !c.IsActive("dnsmasq") {
			t.Error("expected IsActive=true when `service ... status` exits 0")
		}
	})
	t.Run("IsActive_root_fail", func(t *testing.T) {
		fakeBin(t, "service", "exit 4")
		c := &SysVinitCaller{}
		if c.IsActive("dnsmasq") {
			t.Error("expected IsActive=false when `service ... status` exits non-zero")
		}
	})
	t.Run("IsActive_sudo_ok", func(t *testing.T) {
		fakeBin(t, "sudo", sudoDispatch)
		fakeBin(t, "service", "exit 0")
		c := &SysVinitCaller{UseSudo: true}
		if !c.IsActive("dnsmasq") {
			t.Error("expected IsActive=true via sudo when wrapped service exits 0")
		}
	})
	t.Run("Restart_root_ok", func(t *testing.T) {
		fakeBin(t, "service", "exit 0")
		c := &SysVinitCaller{}
		if err := c.Restart("dnsmasq"); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("Restart_root_fail", func(t *testing.T) {
		fakeBin(t, "service", "exit 1")
		c := &SysVinitCaller{}
		if err := c.Restart("dnsmasq"); err == nil {
			t.Error("expected error on failing fake service")
		}
	})
	t.Run("Restart_sudo_ok", func(t *testing.T) {
		fakeBin(t, "sudo", sudoDispatch)
		fakeBin(t, "service", "exit 0")
		c := &SysVinitCaller{UseSudo: true}
		if err := c.Restart("dnsmasq"); err != nil {
			t.Errorf("expected no error via sudo, got %v", err)
		}
	})
	t.Run("RestartSelf_root_ok", func(t *testing.T) {
		fakeBin(t, "service", "exit 0")
		c := &SysVinitCaller{}
		if err := c.RestartSelf(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("RestartSelf_root_fail", func(t *testing.T) {
		fakeBin(t, "service", "exit 1")
		c := &SysVinitCaller{}
		if err := c.RestartSelf(); err == nil {
			t.Error("expected error on failing fake service")
		}
	})
	t.Run("RestartSelf_sudo_fail", func(t *testing.T) {
		fakeBin(t, "sudo", sudoDispatch)
		fakeBin(t, "service", "exit 2")
		c := &SysVinitCaller{UseSudo: true}
		if err := c.RestartSelf(); err == nil {
			t.Error("expected error via sudo on failing fake service")
		}
	})
}

// errAny is a sentinel meaning "expect any non-nil error" — used by checkErr.
var errAny = errors.New("any")

// checkErr asserts that `err` matches the expectation:
//   - wantErr == nil → err must be nil,
//   - wantErr == errAny → err must be non-nil,
//   - otherwise → errors.Is(err, wantErr) must be true.
func checkErr(t *testing.T, err, wantErr error) {
	t.Helper()
	switch {
	case wantErr == nil && err != nil:
		t.Errorf("expected no error, got %v", err)
	case wantErr == errAny && err == nil:
		t.Error("expected a non-nil error, got nil")
	case wantErr != nil && wantErr != errAny && !errors.Is(err, wantErr):
		t.Errorf("expected error %v, got %v", wantErr, err)
	}
}
