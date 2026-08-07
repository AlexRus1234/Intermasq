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

package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWriteAuditConcurrentAndRead(t *testing.T) {
	orig := *AuditLogPath
	*AuditLogPath = filepath.Join(t.TempDir(), "audit.log")
	t.Cleanup(func() { *AuditLogPath = orig })
	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			WriteAudit(AuditEntry{User: "user", Action: "test", Mac: "aa:bb:cc:dd:ee:ff"})
		}(i)
	}
	wg.Wait()
	lines := strings.Split(strings.TrimSpace(string(mustRead(t, *AuditLogPath))), "\n")
	if len(lines) != n {
		t.Fatalf("got %d audit lines, want %d", len(lines), n)
	}
	for _, line := range lines {
		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.Timestamp == "" {
			t.Fatalf("invalid audit entry: %q", line)
		}
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
