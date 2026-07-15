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
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func isSafePath(path string) bool {
	cleanPath := filepath.Clean(path)
	cleanDir := filepath.Clean(*ConfigDir)
	return strings.HasPrefix(cleanPath, cleanDir+string(os.PathSeparator)) || cleanPath == cleanDir
}

func checkDnsmasqStatus() bool {
	return sysCaller.IsActive("dnsmasq")
}

func reloadDnsmasq() error {
	testCmd := exec.Command("/usr/bin/dnsmasq", "--test")
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		return fmt.Errorf("%s", testOut)
	}
	return sysCaller.Restart("dnsmasq")
}

func getArpTable() map[string]bool {
	content, err := os.ReadFile(*ArpPath)
	if err != nil {
		return make(map[string]bool)
	}
	return parseArpContent(string(content))
}

func parseArpContent(content string) map[string]bool {
	activeMacs := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Scan()
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 && fields[2] == "0x2" && fields[3] != "00:00:00:00:00:00" {
			activeMacs[strings.ToLower(fields[3])] = true
		}
	}
	return activeMacs
}

func createLocalBackup(filePath string) {
	if !isSafePath(filePath) {
		return
	}
	content, err := os.ReadFile(filePath)
	if err == nil {
		os.WriteFile(filePath+".bak", content, 0644)
	}
}

func rollbackFile(filePath string) error {
	if !isSafePath(filePath) {
		return os.ErrPermission
	}
	bakPath := filePath + ".bak"
	content, err := os.ReadFile(bakPath)
	if err != nil {
		return err
	}
	createLocalBackup(filePath)
	return os.WriteFile(filePath, content, 0644)
}

func createBackupZip() ([]byte, string, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return nil, "", err
	}

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, f.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		fWriter, err := zipWriter.Create(f.Name())
		if err != nil {
			continue
		}
		fWriter.Write(content)
	}
	zipWriter.Close()

	fileName := "intermasq_backup_" + time.Now().Format("2006-01-02_15-04") + ".zip"
	return buf.Bytes(), fileName, nil
}

func parseLeases() []LeaseEntry {
	leases := []LeaseEntry{}
	file, err := os.Open(*LeasesPath)
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 3 {
				l := LeaseEntry{Ip: fields[2], Mac: fields[1]}
				if len(fields) > 3 {
					l.Hostname = fields[3]
				}
				leases = append(leases, l)
			}
		}
	}
	return leases
}

func findFreeIP(cidr string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid_cidr")
	}

	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return "", fmt.Errorf("ipv6_not_supported")
	}
	if ones >= 31 {
		return "", fmt.Errorf("range_too_small")
	}

	network := ipNet.IP.To4()
	if network == nil {
		return "", fmt.Errorf("invalid_ipv4")
	}

	mask := ipNet.Mask
	broadcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		broadcast[i] = network[i] | ^mask[i]
	}

	candidate := make(net.IP, 4)
	copy(candidate, network)

	scanned := 0
	const scanLimit = 256

	for {
		incIP(candidate)
		scanned++
		if scanned > scanLimit {
			return "", fmt.Errorf("range_exhausted")
		}

		if candidate.Equal(broadcast) {
			return "", fmt.Errorf("range_exhausted")
		}

		if candidate[3] == 0 {
			continue
		}

		if len(findHostsByIP(candidate.String(), "")) == 0 {
			return candidate.String(), nil
		}
	}
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

func removeHostLine(filePath, mac string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	newLines := []string{}
	macLower := strings.ToLower(mac)
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if strings.HasPrefix(clean, "dhcp-host=") && strings.Contains(strings.ToLower(clean), macLower) {
			continue
		}
		if clean != "" {
			newLines = append(newLines, line)
		}
	}
	return os.WriteFile(filePath, []byte(strings.Join(newLines, "\n")+"\n"), 0644)
}

func appendHostLine(filePath, mac, hostname, ip string) error {
	content, _ := os.ReadFile(filePath)
	line := fmt.Sprintf("dhcp-host=%s,%s,%s", mac, hostname, ip)
	out := strings.TrimRight(string(content), "\n")
	if out != "" {
		out += "\n"
	}
	out += line + "\n"
	return os.WriteFile(filePath, []byte(out), 0644)
}

func readHostByMac(filePath, mac string) *HostEntry {
	macLower := strings.ToLower(mac)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "dhcp-host=") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(line, "dhcp-host="), ",")
		entry := HostEntry{File: filePath}
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if macRegex.MatchString(p) {
				entry.Mac = p
			} else if net.ParseIP(p) != nil {
				entry.Ip = p
			} else {
				entry.Hostname = p
			}
		}
		if strings.ToLower(entry.Mac) == macLower {
			return &entry
		}
	}
	return nil
}

type ipTransformMode int

const (
	ipTransformNone ipTransformMode = iota
	ipTransformOctets
	ipTransformCIDR
)

type ipTransform struct {
	mode     ipTransformMode
	oldNet   *net.IPNet
	newNet   *net.IPNet
	oldPref  string
	newPref  string
}

func parseIPTransform(oldStr, newStr string) (*ipTransform, error) {
	if oldStr == "" && newStr == "" {
		return &ipTransform{mode: ipTransformNone}, nil
	}
	if oldStr == "" || newStr == "" {
		return nil, fmt.Errorf("both_prefixes_required")
	}

	if strings.Contains(oldStr, "/") || strings.Contains(newStr, "/") {
		if !strings.Contains(oldStr, "/") || !strings.Contains(newStr, "/") {
			return nil, fmt.Errorf("prefix_format_mismatch")
		}
		_, oldNet, err1 := net.ParseCIDR(oldStr)
		_, newNet, err2 := net.ParseCIDR(newStr)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("invalid_cidr")
		}
		onesOld, _ := oldNet.Mask.Size()
		onesNew, _ := newNet.Mask.Size()
		if onesOld != onesNew {
			return nil, fmt.Errorf("prefix_mismatch")
		}
		if oldNet.IP.To4() == nil || newNet.IP.To4() == nil {
			return nil, fmt.Errorf("ipv6_not_supported")
		}
		return &ipTransform{mode: ipTransformCIDR, oldNet: oldNet, newNet: newNet}, nil
	}

	octetRe := regexp.MustCompile(`^(\d{1,3}\.){0,2}\d{1,3}$`)
	if !octetRe.MatchString(oldStr) || !octetRe.MatchString(newStr) {
		return nil, fmt.Errorf("invalid_prefix_format")
	}
	if strings.Count(oldStr, ".") != strings.Count(newStr, ".") {
		return nil, fmt.Errorf("prefix_format_mismatch")
	}
	return &ipTransform{mode: ipTransformOctets, oldPref: oldStr, newPref: newStr}, nil
}

func (t *ipTransform) apply(ip string) (string, error) {
	if t.mode == ipTransformNone {
		return ip, nil
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", fmt.Errorf("invalid_ip")
	}

	switch t.mode {
	case ipTransformOctets:
		if !strings.HasPrefix(ip, t.oldPref) {
			return "", fmt.Errorf("prefix_not_matched")
		}
		boundary := len(t.oldPref)
		if boundary < len(ip) && ip[boundary] != '.' {
			return "", fmt.Errorf("prefix_not_matched")
		}
		return t.newPref + ip[boundary:], nil
	case ipTransformCIDR:
		oldIP := parsed.To4()
		if oldIP == nil {
			return "", fmt.Errorf("invalid_ipv4")
		}
		mask := t.oldNet.Mask
		if !oldIP.Mask(mask).Equal(t.oldNet.IP.Mask(mask)) {
			return "", fmt.Errorf("prefix_not_matched")
		}
		newIP := make(net.IP, 4)
		for i := 0; i < 4; i++ {
			newIP[i] = (oldIP[i] & ^mask[i]) | t.newNet.IP[i]
		}
		return newIP.String(), nil
	}
	return ip, nil
}

func findHostsByIP(ip, excludeMac string) []HostEntry {
	result := []HostEntry{}
	excludeMacLower := strings.ToLower(excludeMac)

	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return result
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, f.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "dhcp-host=") {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(line, "dhcp-host="), ",")
			entry := HostEntry{File: fullPath}
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if macRegex.MatchString(p) {
					entry.Mac = p
				} else if net.ParseIP(p) != nil {
					entry.Ip = p
				} else {
					entry.Hostname = p
				}
			}
			if entry.Ip == ip && strings.ToLower(entry.Mac) != excludeMacLower {
				result = append(result, entry)
			}
		}
	}
	return result
}

func findHostsByMac(mac string) []HostEntry {
	result := []HostEntry{}
	macLower := strings.ToLower(mac)

	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return result
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, f.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "dhcp-host=") {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(line, "dhcp-host="), ",")
			entry := HostEntry{File: fullPath}
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if macRegex.MatchString(p) {
					entry.Mac = p
				} else if net.ParseIP(p) != nil {
					entry.Ip = p
				} else {
					entry.Hostname = p
				}
			}
			if strings.ToLower(entry.Mac) == macLower {
				result = append(result, entry)
			}
		}
	}
	return result
}

func readAllHosts() []HostEntry {
	hosts := []HostEntry{}
	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return hosts
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, f.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "dhcp-host=") {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(line, "dhcp-host="), ",")
			entry := HostEntry{File: fullPath}
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if macRegex.MatchString(p) {
					entry.Mac = p
				} else if net.ParseIP(p) != nil {
					entry.Ip = p
				} else {
					entry.Hostname = p
				}
			}
			if entry.Mac != "" {
				hosts = append(hosts, entry)
			}
		}
	}
	return hosts
}

func hostsToCSV(hosts []HostEntry) []byte {
	buf := new(bytes.Buffer)
	w := csv.NewWriter(buf)
	w.Write([]string{"mac", "ip", "hostname"})
	for _, h := range hosts {
		w.Write([]string{h.Mac, h.Ip, h.Hostname})
	}
	w.Flush()
	return buf.Bytes()
}

func parseCSVHosts(r io.Reader, targetFile string) ([]HostEntry, error) {
	reader := csv.NewReader(r)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	hosts := []HostEntry{}
	for i, row := range records {
		if i == 0 {
			continue
		}
		if len(row) < 3 {
			continue
		}
		mac := strings.TrimSpace(row[0])
		ip := strings.TrimSpace(row[1])
		hostname := strings.TrimSpace(row[2])

		if macRegex.MatchString(mac) && net.ParseIP(ip) != nil && hostnameRegex.MatchString(hostname) {
			hosts = append(hosts, HostEntry{Mac: mac, Ip: ip, Hostname: hostname, File: targetFile})
		}
	}
	return hosts, nil
}
