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
	"os"
	"path/filepath"
	"sync"
	"testing"

	"intermask/internal/models"
)

func withTemplatesPath(t *testing.T, path string) {
	t.Helper()
	origPath, origMap := *TemplatesPath, templates
	*TemplatesPath, templates = path, make(map[string]models.Template)
	t.Cleanup(func() { *TemplatesPath, templates = origPath, origMap })
}

func TestTemplateConcurrentReadWrite(t *testing.T) {
	Reset()
	const workers = 8
	const iterations = 50
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				id := "template-" + string(rune('a'+worker))
				Set(id, models.Template{ID: id, Name: "name"})
				Get(id)
				All()
			}
		}(worker)
	}
	wg.Wait()
}

func TestLoadAndSaveTemplates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "templates.json")
	withTemplatesPath(t, path)
	templates["t1"] = models.Template{ID: "t1", Name: "Template 1"}
	if err := Save(); err != nil {
		t.Fatal(err)
	}
	Reset()
	Load()
	if _, ok := templates["t1"]; !ok {
		t.Fatal("template was not loaded")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]models.Template
	if err := json.Unmarshal(data, &got); err != nil || got["t1"].Name != "Template 1" {
		t.Fatalf("bad saved templates: %s", data)
	}
}

func TestTemplateHelpers(t *testing.T) {
	if got := GenHostnameFromPattern("host-{NNN}", 42); got != "host-042" {
		t.Fatal(got)
	}
	f := filepath.Join(t.TempDir(), "hosts.conf")
	if err := os.WriteFile(f, []byte("dhcp-host=a\n# dhcp-host=b\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := CountHostsInFile(f); got != 1 {
		t.Fatalf("got %d hosts", got)
	}
}
