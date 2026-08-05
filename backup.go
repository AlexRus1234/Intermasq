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

// backup.go — ZIP backup/restore of all .conf files in ConfigDir. Mirrors
// the audit-trail and .bak logic used by the live editor: existing files
// are stashed as .restore.bak before being overwritten, and on test failure
// every change is rolled back. This is the disaster-recovery fallback for
// the more granular history subsystem (history.go).

package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"intermask/internal/stats"
)

// createBackupZip archives every .conf file in ConfigDir into a ZIP and
// returns the bytes plus a timestamped filename.
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

// restoreBackupZip unpacks a ZIP into ConfigDir. For every .conf file in
// the archive the existing on-disk file is preserved as <name>.restore.bak
// before being overwritten. After all writes, dnsmasq --test --conf-file=<path>
// runs for each restored file; on the first failure every changed file is
// rolled back from its .restore.bak.
func restoreBackupZip(zipData []byte) error {
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("invalid_zip: %v", err)
	}

	var restoredFiles []string

	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		if filepath.Ext(name) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, name)
		if !isSafePath(fullPath) {
			continue
		}

		src, err := f.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			continue
		}

		if existing, err := os.ReadFile(fullPath); err == nil {
			bakPath := fullPath + ".restore.bak"
			os.WriteFile(bakPath, existing, 0644)
		}

		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			continue
		}
		restoredFiles = append(restoredFiles, name)
	}

	if len(restoredFiles) == 0 {
		return fmt.Errorf("no_valid_conf_files")
	}

	for _, name := range restoredFiles {
		fullPath := filepath.Join(*ConfigDir, name)
		testCmd := exec.Command(dnsmasqBin(), "--test", "--conf-file="+fullPath)
		testOut, testErr := testCmd.CombinedOutput()
		if testErr != nil {
			stats.Counters.TestFailures.Add(1)
			for _, rb := range restoredFiles {
				rbPath := filepath.Join(*ConfigDir, rb)
				bakPath := rbPath + ".restore.bak"
				if bakContent, err := os.ReadFile(bakPath); err == nil {
					os.WriteFile(rbPath, bakContent, 0644)
				}
			}
			return fmt.Errorf("dnsmasq_test_failed: %s (file: %s)", testOut, name)
		}
	}

	return nil
}

// deleteConfigFile removes a .conf file and its .bak sibling from ConfigDir.
// The .bak is taken first into the history subsystem so the deletion can be
// undone via the versioned-history UI. dnsmasq --test is NOT run — an absent
// file is functionally equivalent to an empty one for dnsmasq (the file is
// simply not loaded), and the user is expected to click "Apply" afterwards
// when ready. Returns os.ErrPermission for paths outside ConfigDir,
// os.ErrNotExist when the file is missing.
func deleteConfigFile(path string) error {
	if !isSafePath(path) {
		return os.ErrPermission
	}
	if _, err := os.Stat(path); err != nil {
		return err
	}
	// Snapshot current state to history so the file can be restored via
	// the versioned-history modal even after physical deletion.
	saveHistory(path)
	if err := os.Remove(path); err != nil {
		return err
	}
	// Best-effort cleanup of the .bak sibling; not a hard failure if it
	// cannot be removed (e.g. permission issue on the parent dir).
	_ = os.Remove(path + ".bak")
	return nil
}
