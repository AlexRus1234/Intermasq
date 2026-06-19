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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	if !isSafePath(filePath) { return }
	content, err := os.ReadFile(filePath)
	if err == nil {
		os.WriteFile(filePath+".bak", content, 0644)
	}
}

func rollbackFile(filePath string) error {
	if !isSafePath(filePath) { return os.ErrPermission }
	bakPath := filePath + ".bak"
	content, err := os.ReadFile(bakPath)
	if err != nil { return err }
	createLocalBackup(filePath)
	return os.WriteFile(filePath, content, 0644)
}

func createBackupZip() ([]byte, string, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	files, err := os.ReadDir(*ConfigDir)
	if err != nil { return nil, "", err }

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".conf" { continue }
		fullPath := filepath.Join(*ConfigDir, f.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil { continue }

		fWriter, err := zipWriter.Create(f.Name())
		if err != nil { continue }
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
				if len(fields) > 3 { l.Hostname = fields[3] }
				leases = append(leases, l)
			}
		}
	}
	return leases
}
