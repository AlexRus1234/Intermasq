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

package webapi

// Shared test helpers for the webapi handler-level tests (white-box). Every
// helper that was previously duplicated across the main-package test files
// (newTestDir / newJSONContext / jsonPath, the fake-bin dnsmasq harness, the
// multipart writer, the users/templates shims) lives here so the per-feature
// test files hold only Test* functions.

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"intermask/internal/auth"
	"intermask/internal/bins"
	"intermask/internal/dnsmasq"
	"intermask/internal/models"
	templatepkg "intermask/internal/templates"
)

// DBPath re-exports the auth user-DB flag so handler tests can point it at a
// temp file without qualifying every call site.
var DBPath = auth.DBPath

// setUsers clears the in-memory user map and seeds it with the given
// name→bcrypt-hash pairs (no disk write).
func setUsers(values map[string]string) {
	auth.ClearUsers()
	for name, hash := range values {
		auth.SetUser(name, hash)
	}
}

// newTestDir creates a temp dir, points *dnsmasq.ConfigDir at it, and returns
// the dir. t.TempDir auto-cleans on test completion.
func newTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	*dnsmasq.ConfigDir = dir
	return dir
}

// newJSONContext builds a gin test context with a JSON body and admin user.
func newJSONContext(method, target, body string) (*httptest.ResponseRecorder, *gin.Context) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", "admin")
	return w, c
}

// jsonPath escapes a file path for embedding in a JSON string literal.
func jsonPath(p string) string {
	return strings.ReplaceAll(p, "\\", "\\\\")
}

// multipartWriter fills `dst` with a multipart/form-data body carrying a
// single field named "file" with the given filename and contents, and
// returns the writer so the caller can read FormDataContentType().
func multipartWriter(t *testing.T, dst *bytes.Buffer, filename string, content []byte) *multipart.Writer {
	t.Helper()
	mw := multipart.NewWriter(dst)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return mw
}

// ===== fake-bin dnsmasq harness (Linux-gated) =====
//
// These helpers install throwaway shell-script binaries under temp dirs and
// point the cached bin paths (internal/bins) at them for the duration of the
// test. They skip on Windows because os/exec does not honour the shebang
// convention there.

func fakeDnsmasq(t *testing.T, exitCode int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-dnsmasq shell-script unsupported on Windows")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "dnsmasq")
	var script string
	if exitCode == 0 {
		script = "#!/bin/sh\nexit 0\n"
	} else {
		script = "#!/bin/sh\necho 'fake dnsmasq: test failed'\nexit " + itoa(exitCode) + "\n"
	}
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake dnsmasq: %v", err)
	}
	setBinPath(t, "dnsmasq", bin)
}

func fakeDnsmasqArgvInspect(t *testing.T, exitCode int) (binPath, logPath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-dnsmasq shell-script unsupported on Windows")
	}
	dir := t.TempDir()
	binPath = filepath.Join(dir, "dnsmasq")
	logPath = filepath.Join(dir, "argv.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + logPath + "\"\nexit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake dnsmasq (argv-inspect): %v", err)
	}
	setBinPath(t, "dnsmasq", binPath)
	return binPath, logPath
}

func readArgvLog(t *testing.T, logPath string) string {
	t.Helper()
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read argv log %s: %v", logPath, err)
	}
	return string(b)
}

func fakeDnsmasqStrict(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-dnsmasq shell-script unsupported on Windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "dnsmasq")
	script := `#!/bin/sh
conf=""
for arg in "$@"; do
    case "$arg" in
        --conf-file=*) conf="${arg#--conf-file=}" ;;
    esac
done
if [ -n "$conf" ] && [ -f "$conf" ]; then
    if grep -q '# INVALID' "$conf"; then
        echo "fake dnsmasq: invalid config"
        exit 1
    fi
fi
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write strict fake dnsmasq: %v", err)
	}
	setBinPath(t, "dnsmasq", bin)
}

func fakeBin(t *testing.T, name, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script binary unsupported on Windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, name)
	full := "#!/bin/sh\n" + script + "\n"
	if err := os.WriteFile(bin, []byte(full), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
	setBinPath(t, name, bin)
}

func setBinPath(t *testing.T, name, bin string) {
	t.Helper()
	bins.SetPathForTest(t, name, bin)
}

// itoa is a tiny stdlib-free itoa so we avoid importing strconv just for the
// script-assembly lines above. Supports non-negative ints only.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// ===== history helpers =====

func withHistoryDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := *dnsmasq.HistoryDir
	*dnsmasq.HistoryDir = dir
	t.Cleanup(func() { *dnsmasq.HistoryDir = orig })
	return dir
}

func newestHistoryVersion(t *testing.T, filePath string) string {
	t.Helper()
	versions, err := dnsmasq.ListHistory(filePath)
	if err != nil {
		t.Fatalf("listHistory: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("no history versions produced")
	}
	return versions[0].Version
}

func firstVersion(t *testing.T, filePath string) string {
	t.Helper()
	versions, err := dnsmasq.ListHistory(filePath)
	if err != nil || len(versions) != 1 {
		t.Fatalf("firstVersion: %v (%d)", err, len(versions))
	}
	return versions[0].Version
}

// ===== template test shims =====
//
// Thin wrappers over internal/templates so the handler tests stay readable.
// The template store itself is package-qualified everywhere else in webapi.

func resetTemplates()                          { templatepkg.Reset() }
func setTemplate(id string, t models.Template) { templatepkg.Set(id, t) }
func hasTemplate(id string) bool               { _, ok := templatepkg.Get(id); return ok }
