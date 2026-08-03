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

// audit_test.go — P2.8 (predrel-test-remediation). writeAudit (audit.go:26)
// previously had no direct test: it was exercised only indirectly through
// handlers, and never under concurrency. The auditHandler read path is
// already covered in handlers_test.go (TestAuditHandler_ReturnsEntries /
// _NoLogFile), so this file fills the two real gaps: (1) concurrent writes
// must not lose or corrupt JSON lines, and (2) a write -> auditHandler read
// round-trip returns exactly what was written.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestWriteAudit_Concurrent fires N concurrent writeAudit calls and asserts
// every entry lands as a well-formed JSON line with the expected fields.
// Relies on O_APPEND + a single f.Write per entry (audit.go:35,48) being
// atomic for small payloads. Under -race this also guards the file I/O path.
func TestWriteAudit_Concurrent(t *testing.T) {
	withSandboxFlags(t)
	origAudit := *AuditLogPath
	*AuditLogPath = filepath.Join(t.TempDir(), "audit.log")
	t.Cleanup(func() { *AuditLogPath = origAudit })

	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			writeAudit(AuditEntry{
				User:   fmt.Sprintf("user%d", idx),
				Action: "test_action",
				Mac:    "aa:bb:cc:dd:ee:00",
			})
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(*AuditLogPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		t.Fatalf("audit log empty after %d concurrent writes", N)
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) != N {
		t.Fatalf("expected %d audit lines, got %d (first/last=%q.../%q...)", N, len(lines), lines[0], lines[len(lines)-1])
	}
	for i, line := range lines {
		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d: invalid JSON: %v (line=%q)", i, err, line)
			continue
		}
		if entry.Action != "test_action" {
			t.Errorf("line %d: expected action %q, got %q", i, "test_action", entry.Action)
		}
		if entry.User == "" {
			t.Errorf("line %d: empty user (line=%q)", i, line)
		}
		if entry.Timestamp == "" {
			t.Errorf("line %d: writeAudit did not stamp Timestamp (line=%q)", i, line)
		}
	}
}

// TestWriteAudit_ReadRoundtrip pins the write -> read contract end to end:
// a single writeAudit must be visible to a subsequent auditHandler GET with
// no field loss. This is the round-trip that handlers_test.go's pre-seeded
// fixture does NOT exercise (it writes the JSON line itself, bypassing
// writeAudit's timestamping / marshalling).
func TestWriteAudit_ReadRoundtrip(t *testing.T) {
	withSandboxFlags(t)
	origAudit := *AuditLogPath
	*AuditLogPath = filepath.Join(t.TempDir(), "audit.log")
	t.Cleanup(func() { *AuditLogPath = origAudit })

	want := AuditEntry{User: "admin", Action: "add_host", Mac: "aa:bb:cc:dd:ee:01"}
	writeAudit(want)

	w, c := newJSONContext("GET", "/api/audit", "")
	auditHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200 from auditHandler, got %d (body=%s)", w.Code, w.Body.String())
	}
	var got []AuditEntry
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON response: %v (body=%q)", err, w.Body.String())
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 entry in round-trip, got %d (body=%s)", len(got), w.Body.String())
	}
	if got[0].Action != want.Action || got[0].User != want.User || got[0].Mac != want.Mac {
		t.Errorf("round-trip mismatch: got %+v, want action/user/mac = %q/%q/%q",
			got[0], want.Action, want.User, want.Mac)
	}
	if got[0].Timestamp == "" {
		t.Error("writeAudit did not populate Timestamp before persisting")
	}
}
