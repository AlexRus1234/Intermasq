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
)

type SystemCaller interface {
	IsActive(service string) bool
	Restart(service string) error
}

type SystemdSystemCaller struct {
	UseSudo bool
}

func (s *SystemdSystemCaller) IsActive(service string) bool {
	var cmd *exec.Cmd
	if s.UseSudo {
		cmd = exec.Command("/usr/bin/sudo", "-n", "/usr/bin/systemctl", "is-active", service)
	} else {
		cmd = exec.Command("/usr/bin/systemctl", "is-active", service)
	}
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out)) == "active"
}

func (s *SystemdSystemCaller) Restart(service string) error {
	var cmd *exec.Cmd
	if s.UseSudo {
		cmd = exec.Command("/usr/bin/sudo", "/usr/bin/systemctl", "restart", service)
	} else {
		cmd = exec.Command("/usr/bin/systemctl", "restart", service)
	}
	return cmd.Run()
}

func (s *SystemdSystemCaller) String() string {
	if s.UseSudo {
		return "system (via sudo)"
	}
	return "system (root)"
}

type SystemdUserCaller struct{}

func (s *SystemdUserCaller) IsActive(service string) bool {
	cmd := exec.Command("/usr/bin/systemctl", "--user", "is-active", service)
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out)) == "active"
}

func (s *SystemdUserCaller) Restart(service string) error {
	cmd := exec.Command("/usr/bin/systemctl", "--user", "restart", service)
	return cmd.Run()
}

func (s *SystemdUserCaller) String() string {
	return "user"
}

type NoneCaller struct{}

func (s *NoneCaller) IsActive(service string) bool {
	return true
}

func (s *NoneCaller) Restart(service string) error {
	return nil
}

func (s *NoneCaller) String() string {
	return "none"
}

func detectSystemCaller() SystemCaller {
	if os.Getuid() == 0 {
		return &SystemdSystemCaller{UseSudo: false}
	}

	cmd := exec.Command("/usr/bin/sudo", "-n", "/usr/bin/systemctl", "is-active", "dnsmasq")
	if err := cmd.Run(); err == nil {
		return &SystemdSystemCaller{UseSudo: true}
	}

	cmd = exec.Command("/usr/bin/systemctl", "--user", "is-active", "default.target")
	if err := cmd.Run(); err == nil {
		return &SystemdUserCaller{}
	}

	return &NoneCaller{}
}

func resolveSystemCaller(scope string) SystemCaller {
	switch scope {
	case "system":
		return &SystemdSystemCaller{UseSudo: os.Getuid() != 0}
	case "user":
		return &SystemdUserCaller{}
	case "none":
		return &NoneCaller{}
	default:
		return detectSystemCaller()
	}
}

var sysCaller SystemCaller

func initSystemCaller(scope string) {
	sysCaller = resolveSystemCaller(scope)
	fmt.Printf("[SYSTEMD] Scope: %s\n", sysCaller)
}
