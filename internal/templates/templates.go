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

package templates

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"intermask/internal/models"
)

var TemplatesPath = flag.String("templates", "/etc/intermasq/templates.json", "Path to templates file")
var templates = make(map[string]models.Template)

func Reset() { templates = make(map[string]models.Template) }

func Load() {
	if _, err := os.Stat(*TemplatesPath); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(*TemplatesPath), 0700)
		return
	}
	data, err := os.ReadFile(*TemplatesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] Cannot read templates file %s: %v\n", *TemplatesPath, err)
		os.Exit(1)
	}
	if err := json.Unmarshal(data, &templates); err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] Cannot parse templates file %s: %v\n", *TemplatesPath, err)
		os.Exit(1)
	}
}

func Save() error {
	data, _ := json.MarshalIndent(templates, "", "  ")
	dir := filepath.Dir(*TemplatesPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp := *TemplatesPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, *TemplatesPath)
}

func All() []models.Template {
	result := []models.Template{}
	for _, t := range templates {
		result = append(result, t)
	}
	return result
}

func Get(id string) (models.Template, bool) { t, ok := templates[id]; return t, ok }
func Set(id string, t models.Template)      { templates[id] = t }
func Delete(id string)                      { delete(templates, id) }
func GenHostnameFromPattern(pattern string, index int) string {
	return strings.ReplaceAll(pattern, "{NNN}", fmt.Sprintf("%03d", index))
}
func CountHostsInFile(file string) int {
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
