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
	"strings"

	"intermask/internal/bins"
)

type SystemCaller interface {
	IsActive(service string) bool
	Restart(service string) error
	RestartSelf() error
	String() string
}

type SystemdSystemCaller struct {
	UseSudo bool
}

func (s *SystemdSystemCaller) IsActive(service string) bool {
	var cmd *exec.Cmd
	if s.UseSudo {
		cmd = exec.Command(bins.Sudo(), "-n", bins.Systemctl(), "is-active", service)
	} else {
		cmd = exec.Command(bins.Systemctl(), "is-active", service)
	}
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out)) == "active"
}

func (s *SystemdSystemCaller) Restart(service string) error {
	var cmd *exec.Cmd
	if s.UseSudo {
		cmd = exec.Command(bins.Sudo(), bins.Systemctl(), "restart", service)
	} else {
		cmd = exec.Command(bins.Systemctl(), "restart", service)
	}
	return cmd.Run()
}

func (s *SystemdSystemCaller) RestartSelf() error {
	var cmd *exec.Cmd
	if s.UseSudo {
		cmd = exec.Command(bins.Sudo(), bins.Systemctl(), "restart", "intermasq")
	} else {
		cmd = exec.Command(bins.Systemctl(), "restart", "intermasq")
	}
	return cmd.Run()
}

func (s *SystemdSystemCaller) String() string {
	if s.UseSudo {
		return "systemd (via sudo)"
	}
	return "systemd (root)"
}

type SystemdUserCaller struct{}

func (s *SystemdUserCaller) IsActive(service string) bool {
	cmd := exec.Command(bins.Systemctl(), "--user", "is-active", service)
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out)) == "active"
}

func (s *SystemdUserCaller) Restart(service string) error {
	cmd := exec.Command(bins.Systemctl(), "--user", "restart", service)
	return cmd.Run()
}

func (s *SystemdUserCaller) RestartSelf() error {
	cmd := exec.Command(bins.Systemctl(), "--user", "restart", "intermasq")
	return cmd.Run()
}

func (s *SystemdUserCaller) String() string {
	return "systemd-user"
}

type OpenRCCaller struct {
	UseSudo bool
}

func (s *OpenRCCaller) IsActive(service string) bool {
	var cmd *exec.Cmd
	if s.UseSudo {
		cmd = exec.Command(bins.Sudo(), "-n", bins.RcService(), service, "status")
	} else {
		cmd = exec.Command(bins.RcService(), service, "status")
	}
	out, _ := cmd.Output()
	return strings.Contains(strings.TrimSpace(string(out)), "started")
}

func (s *OpenRCCaller) Restart(service string) error {
	var cmd *exec.Cmd
	if s.UseSudo {
		cmd = exec.Command(bins.Sudo(), bins.RcService(), service, "restart")
	} else {
		cmd = exec.Command(bins.RcService(), service, "restart")
	}
	return cmd.Run()
}

func (s *OpenRCCaller) RestartSelf() error {
	var cmd *exec.Cmd
	if s.UseSudo {
		cmd = exec.Command(bins.Sudo(), bins.RcService(), "intermasq", "restart")
	} else {
		cmd = exec.Command(bins.RcService(), "intermasq", "restart")
	}
	return cmd.Run()
}

func (s *OpenRCCaller) String() string {
	if s.UseSudo {
		return "openrc (via sudo)"
	}
	return "openrc (root)"
}

type RunitCaller struct {
	UseSudo    bool
	ServiceDir string
}

func (s *RunitCaller) IsActive(service string) bool {
	var cmd *exec.Cmd
	svcPath := s.ServiceDir + "/" + service
	if s.UseSudo {
		cmd = exec.Command(bins.Sudo(), "-n", bins.Sv(), "status", svcPath)
	} else {
		cmd = exec.Command(bins.Sv(), "status", svcPath)
	}
	out, _ := cmd.Output()
	return strings.Contains(strings.TrimSpace(string(out)), "run")
}

func (s *RunitCaller) Restart(service string) error {
	var cmd *exec.Cmd
	svcPath := s.ServiceDir + "/" + service
	if s.UseSudo {
		cmd = exec.Command(bins.Sudo(), bins.Sv(), "restart", svcPath)
	} else {
		cmd = exec.Command(bins.Sv(), "restart", svcPath)
	}
	return cmd.Run()
}

func (s *RunitCaller) RestartSelf() error {
	var cmd *exec.Cmd
	svcPath := s.ServiceDir + "/intermasq"
	if s.UseSudo {
		cmd = exec.Command(bins.Sudo(), bins.Sv(), "restart", svcPath)
	} else {
		cmd = exec.Command(bins.Sv(), "restart", svcPath)
	}
	return cmd.Run()
}

func (s *RunitCaller) String() string {
	if s.UseSudo {
		return fmt.Sprintf("runit (via sudo, dir=%s)", s.ServiceDir)
	}
	return fmt.Sprintf("runit (dir=%s)", s.ServiceDir)
}

type SysVinitCaller struct {
	UseSudo bool
}

func (s *SysVinitCaller) IsActive(service string) bool {
	var cmd *exec.Cmd
	if s.UseSudo {
		cmd = exec.Command(bins.Sudo(), "-n", bins.Service(), service, "status")
	} else {
		cmd = exec.Command(bins.Service(), service, "status")
	}
	return cmd.Run() == nil
}

func (s *SysVinitCaller) Restart(service string) error {
	var cmd *exec.Cmd
	if s.UseSudo {
		cmd = exec.Command(bins.Sudo(), bins.Service(), service, "restart")
	} else {
		cmd = exec.Command(bins.Service(), service, "restart")
	}
	return cmd.Run()
}

func (s *SysVinitCaller) RestartSelf() error {
	var cmd *exec.Cmd
	if s.UseSudo {
		cmd = exec.Command(bins.Sudo(), bins.Service(), "intermasq", "restart")
	} else {
		cmd = exec.Command(bins.Service(), "intermasq", "restart")
	}
	return cmd.Run()
}

func (s *SysVinitCaller) String() string {
	if s.UseSudo {
		return "sysvinit (via sudo)"
	}
	return "sysvinit (root)"
}

type NoneCaller struct{}

func (s *NoneCaller) IsActive(service string) bool {
	return true
}

func (s *NoneCaller) Restart(service string) error {
	return nil
}

func (s *NoneCaller) RestartSelf() error {
	return fmt.Errorf("self-restart not supported without init system")
}

func (s *NoneCaller) String() string {
	return "none"
}

// procOneCommPath is the path read by detectInitSystem to identify the
// init process. It is a package var (not a hard-coded literal) so tests
// can point it at a temp file on any platform without touching /proc.
var procOneCommPath = "/proc/1/comm"

func detectInitSystem() string {
	comm, err := os.ReadFile(procOneCommPath)
	if err == nil {
		name := strings.TrimSpace(string(comm))
		switch name {
		case "systemd":
			return "systemd"
		case "runit":
			return "runit"
		case "init":
			if bins.RcService() != "" {
				return "openrc"
			}
			return "sysvinit"
		}
	}

	if bins.Systemctl() != "" {
		return "systemd"
	}
	if bins.RcService() != "" {
		return "openrc"
	}
	if bins.Sv() != "" {
		return "runit"
	}
	if bins.Service() != "" {
		return "sysvinit"
	}

	return "none"
}

func detectSystemCaller() SystemCaller {
	initSystem := detectInitSystem()

	switch initSystem {
	case "systemd":
		if os.Getuid() == 0 {
			return &SystemdSystemCaller{UseSudo: false}
		}
		cmd := exec.Command(bins.Sudo(), "-n", bins.Systemctl(), "is-active", "dnsmasq")
		if err := cmd.Run(); err == nil {
			return &SystemdSystemCaller{UseSudo: true}
		}
		cmd = exec.Command(bins.Systemctl(), "--user", "is-active", "default.target")
		if err := cmd.Run(); err == nil {
			return &SystemdUserCaller{}
		}
		return &SystemdSystemCaller{UseSudo: os.Getuid() != 0}
	case "openrc":
		return &OpenRCCaller{UseSudo: os.Getuid() != 0}
	case "runit":
		return &RunitCaller{UseSudo: os.Getuid() != 0, ServiceDir: "/etc/service"}
	case "sysvinit":
		return &SysVinitCaller{UseSudo: os.Getuid() != 0}
	default:
		return &NoneCaller{}
	}
}

func resolveSystemCaller(initSystem string) SystemCaller {
	initSystem = mapLegacyScope(initSystem)
	useSudo := os.Getuid() != 0

	switch initSystem {
	case "systemd":
		return &SystemdSystemCaller{UseSudo: useSudo}
	case "systemd-user":
		return &SystemdUserCaller{}
	case "openrc":
		return &OpenRCCaller{UseSudo: useSudo}
	case "runit":
		return &RunitCaller{UseSudo: useSudo, ServiceDir: "/etc/service"}
	case "sysvinit":
		return &SysVinitCaller{UseSudo: useSudo}
	case "none":
		return &NoneCaller{}
	default:
		return detectSystemCaller()
	}
}

func mapLegacyScope(scope string) string {
	switch scope {
	case "system":
		return "systemd"
	case "user":
		return "systemd-user"
	case "none":
		return "none"
	default:
		return scope
	}
}

var sysCaller SystemCaller

func initSystemCaller(initSystem string) {
	sysCaller = resolveSystemCaller(initSystem)
	fmt.Printf("[INIT] System: %s\n", sysCaller)
}
