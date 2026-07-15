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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var templates = make(map[string]Template)

func loadTemplates() {
	if _, err := os.Stat(*TemplatesPath); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(*TemplatesPath), 0700)
		return
	}
	data, _ := os.ReadFile(*TemplatesPath)
	json.Unmarshal(data, &templates)
}

func saveTemplates() error {
	data, _ := json.MarshalIndent(templates, "", "  ")
	return os.WriteFile(*TemplatesPath, data, 0600)
}

func genHostnameFromPattern(pattern string, index int) string {
	padded := fmt.Sprintf("%03d", index)
	return strings.ReplaceAll(pattern, "{NNN}", padded)
}

func countHostsInFile(file string) int {
	content, err := os.ReadFile(file)
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "dhcp-host=") {
			count++
		}
	}
	return count
}
