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
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"intermask/internal/bins"
)

// makeTestZip builds an in-memory ZIP from the supplied entries.
// Migrated from main package's dnsmasq_test.go (stage 5).
func makeTestZip(entries map[string]string) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, content := range entries {
		fw, _ := zw.Create(name)
		fw.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

// ========== RestoreBackupZip ==========

func TestRestoreBackupZipValid(t *testing.T) {
	dir := newTestDir(t)
	zipData := makeTestZip(map[string]string{
		"hosts.conf": "dhcp-host=aa:bb:cc:dd:ee:ff,host1,1.2.3.4\n",
	})
	_ = RestoreBackupZip(zipData)
	_, err := os.ReadFile(filepath.Join(dir, "hosts.conf"))
	if err != nil {
		t.Error("file should have been written before dnsmasq test")
	}
}

func TestRestoreBackupZipCreatesRestoreBak(t *testing.T) {
	dir := newTestDir(t)
	os.WriteFile(filepath.Join(dir, "hosts.conf"), []byte("old content\n"), 0644)
	zipData := makeTestZip(map[string]string{
		"hosts.conf": "new content\n",
	})
	_ = RestoreBackupZip(zipData)
	bak, _ := os.ReadFile(filepath.Join(dir, "hosts.conf.restore.bak"))
	if string(bak) != "old content\n" {
		t.Errorf("bak mismatch: %q", bak)
	}
}

func TestRestoreBackupZipNoConfFiles(t *testing.T) {
	newTestDir(t)
	zipData := makeTestZip(map[string]string{
		"notes.txt": "hello\n",
	})
	err := RestoreBackupZip(zipData)
	if err == nil || !strings.Contains(err.Error(), "no_valid_conf_files") {
		t.Errorf("expected no_valid_conf_files error, got %v", err)
	}
}

func TestRestoreBackupZipInvalidData(t *testing.T) {
	newTestDir(t)
	err := RestoreBackupZip([]byte("not a zip file"))
	if err == nil || !strings.Contains(err.Error(), "invalid_zip") {
		t.Errorf("expected invalid_zip error, got %v", err)
	}
}

func TestRestoreBackupZipIgnoresUnsafeNames(t *testing.T) {
	dir := newTestDir(t)
	zipData := makeTestZip(map[string]string{
		"../evil.conf": "bad\n",
		"hosts.conf":   "good\n",
	})
	_ = RestoreBackupZip(zipData)
	_, err := os.ReadFile(filepath.Join(dir, "hosts.conf"))
	if err != nil {
		t.Fatal("hosts.conf should have been extracted")
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "evil.conf")); err == nil {
		t.Fatal("evil.conf should not have been extracted")
	}
}

// TestRestoreBackupZip_EmptyArchive verifies that a ZIP with no .conf files
// is rejected with "no_valid_conf_files".
// Migrated from main package's handlers_test.go (stage 5).
func TestRestoreBackupZip_EmptyArchive(t *testing.T) {
	newTestDir(t)

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	fw, _ := zw.Create("readme.txt")
	fw.Write([]byte("no conf files here"))
	zw.Close()

	err := RestoreBackupZip(buf.Bytes())
	if err == nil {
		t.Fatal("expected error for empty archive (no .conf files)")
	}
	if !strings.Contains(err.Error(), "no_valid_conf_files") {
		t.Errorf("expected 'no_valid_conf_files' error, got: %v", err)
	}
}

// TestRestoreBackupZip_ValidArchive confirms that a well-formed ZIP with
// .conf files restores correctly on Linux CI (dnsmasq --test passes). On
// non-Linux the dnsmasq binary is unavailable so the test skips.
// Migrated from main package's handlers_test.go (stage 5).
func TestRestoreBackupZip_ValidArchive(t *testing.T) {
	if bins.Dnsmasq() == "" {
		t.Skip("dnsmasq binary not found, skipping restore test")
	}

	dir := newTestDir(t)
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	fw, _ := zw.Create("restored.conf")
	fw.Write([]byte("domain-needed\nbogus-priv\n"))
	zw.Close()

	err := RestoreBackupZip(buf.Bytes())
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "restored.conf"))
	if err != nil {
		t.Fatalf("restored file should exist: %v", err)
	}
	if !strings.Contains(string(content), "domain-needed") {
		t.Error("restored content should contain 'domain-needed'")
	}
}

// ========== DeleteConfigFile ==========

// TestDeleteConfigFileRemovesFileAndBak checks that the backend function
// removes both the .conf file and its .bak sibling (a leftover .bak for a
// deleted file serves no purpose and confuses the UI's "show rollback
// button" logic).
// Migrated from main package's new_features_test.go (stage 5).
func TestDeleteConfigFileRemovesFileAndBak(t *testing.T) {
	dir := newTestDir(t)
	*HistoryDir = filepath.Join(dir, "history")
	*HistoryDepth = 5
	path := filepath.Join(dir, "old.conf")
	os.WriteFile(path, []byte("domain-needed\n"), 0644)
	os.WriteFile(path+".bak", []byte("domain-needed\n"), 0644)

	if err := DeleteConfigFile(path); err != nil {
		t.Fatalf("DeleteConfigFile failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error(".conf file should be removed")
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error(".bak sibling should also be removed")
	}
}

// TestDeleteConfigFileSavesHistory verifies that the file's content is
// snapshotted into versioned history BEFORE the physical removal, so the
// operator can recover via the history modal.
// Migrated from main package's new_features_test.go (stage 5).
func TestDeleteConfigFileSavesHistory(t *testing.T) {
	dir := newTestDir(t)
	*HistoryDir = filepath.Join(dir, "history")
	*HistoryDepth = 5
	path := filepath.Join(dir, "doomed.conf")
	content := []byte("# managed by Intermasq\ndomain-needed\nserver=1.1.1.1\n")
	os.WriteFile(path, content, 0644)

	if err := DeleteConfigFile(path); err != nil {
		t.Fatalf("DeleteConfigFile failed: %v", err)
	}

	versions, err := ListHistory(path)
	if err != nil {
		t.Fatalf("ListHistory after delete: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 history version saved before deletion, got %d", len(versions))
	}
	saved, err := ReadHistoryVersion(path, versions[0].Version)
	if err != nil {
		t.Fatalf("ReadHistoryVersion: %v", err)
	}
	if string(saved) != string(content) {
		t.Errorf("saved history content does not match pre-deletion file:\nwant: %q\ngot:  %q", content, saved)
	}
}

// TestDeleteConfigFileRejectsUnsafePath ensures path-traversal defence
// holds: a path outside ConfigDir must be refused with os.ErrPermission.
// Migrated from main package's new_features_test.go (stage 5).
func TestDeleteConfigFileRejectsUnsafePath(t *testing.T) {
	dir := newTestDir(t)
	*HistoryDir = filepath.Join(dir, "history")
	outside := filepath.Join(dir, "..", "escape.conf")
	err := DeleteConfigFile(outside)
	if err != os.ErrPermission {
		t.Errorf("expected os.ErrPermission for path outside ConfigDir, got %v", err)
	}
}

// TestDeleteConfigFileMissingReturnsNotExist — deleting a file that
// doesn't exist should bubble up os.ErrNotExist so the handler can
// produce a 404.
// Migrated from main package's new_features_test.go (stage 5).
func TestDeleteConfigFileMissingReturnsNotExist(t *testing.T) {
	dir := newTestDir(t)
	*HistoryDir = filepath.Join(dir, "history")
	path := filepath.Join(dir, "ghost.conf")
	err := DeleteConfigFile(path)
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}
