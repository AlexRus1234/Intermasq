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
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"intermask/internal/bins"
)

// TestKnownConfigTemplateIDsSorted — контракт: список отсортирован, чтобы
// UI и проверочные тесты могли полагаться на стабильный порядок.
func TestKnownConfigTemplateIDsSorted(t *testing.T) {
	ids := KnownConfigTemplateIDs()
	if !sort.StringsAreSorted(ids) {
		t.Errorf("KnownConfigTemplateIDs() must be sorted: %v", ids)
	}
	if len(ids) != len(ConfigTemplates) {
		t.Errorf("len mismatch: ids=%d map=%d", len(ids), len(ConfigTemplates))
	}
}

// TestKnownConfigTemplateIDsContainsEmpty — "empty" обязан всегда быть в
// списке: это дефолтный template при отсутствии поля в запросе.
func TestKnownConfigTemplateIDsContainsEmpty(t *testing.T) {
	if _, ok := ConfigTemplates["empty"]; !ok {
		t.Fatal(`"empty" template must always exist in ConfigTemplates`)
	}
	ids := KnownConfigTemplateIDs()
	found := false
	for _, id := range ids {
		if id == "empty" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal(`"empty" missing from KnownConfigTemplateIDs()`)
	}
}

// TestConfigTemplatesAllStartWithManagedHeader — каждый шаблон должен
// начинаться с маркера "# === Managed by Intermasq ===", чтобы было видно
// при чтении raw-файла, что он был создан через панель.
func TestConfigTemplatesAllStartWithManagedHeader(t *testing.T) {
	const marker = "# === Managed by Intermasq ==="
	for id, content := range ConfigTemplates {
		if !strings.HasPrefix(content, marker) {
			t.Errorf("template %q must start with %q", id, marker)
		}
	}
}

// TestConfigTemplatesValidForDnsmasqSyntax — каждый шаблон должен проходить
// `dnsmasq --test`, чтобы последующий PUT /api/config не падал на первой
// операции. Если dnsmasq не установлен — тест пропускается (CI без dnsmasq).
func TestConfigTemplatesValidForDnsmasqSyntax(t *testing.T) {
	if bins.Dnsmasq() == "" {
		t.Skip("dnsmasq binary not installed — skipping syntax validation")
	}
	for id, content := range ConfigTemplates {
		t.Run(id, func(t *testing.T) {
			tmp := filepath.Join(t.TempDir(), "x.conf")
			if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bins.Dnsmasq(), "--test", "--conf-file="+tmp)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("template %q failed dnsmasq --test:\n%s\noutput:\n%s", id, err, out)
			}
		})
	}
}
