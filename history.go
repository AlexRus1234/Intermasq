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

// history.go — multi-level versioned history of dnsmasq config files.
// Each time createLocalBackup is called (i.e. before every write to a
// .conf file), the current content is snapshotted into -history-dir under
// a name derived from sha256(path)+UTC timestamp. Up to -history-depth
// versions are kept per file. REST endpoints allow listing, diffing and
// restoring any stored version. The legacy single-shot .bak file is still
// maintained in parallel — it powers the quick "rollback" button, while
// history supports the more deliberate "pick a version" flow.

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"intermask/internal/stats"
)

// historyVersionRegex matches a version id used by the history subsystem.
// Format: YYYYMMDD-HHMMSS with an optional numeric suffix (-2, -3, ...)
// used when multiple snapshots are taken within the same second.
var historyVersionRegex = regexp.MustCompile(`^\d{8}-\d{6}(-\d+)?$`)

// historyFilePrefix returns the stable prefix used for all history files
// related to the given config file. The original absolute path is hashed
// (sha256, first 16 hex chars) so files from different directories never
// collide and the original path cannot be reverse-engineered from history.
func historyFilePrefix(filePath string) string {
	h := sha256.Sum256([]byte(filepath.Clean(filePath)))
	return fmt.Sprintf("%x", h[:8]) + "_"
}

// historyFileName builds the on-disk filename for a given path+version.
func historyFileName(filePath, version string) string {
	return historyFilePrefix(filePath) + version + ".bak"
}

// nextHistoryVersion returns a version id for filePath that does not
// collide with an existing history file.
func nextHistoryVersion(filePath string) string {
	base := time.Now().UTC().Format("20060102-150405")
	candidate := base
	for n := 2; ; n++ {
		full := filepath.Join(*HistoryDir, historyFileName(filePath, candidate))
		if _, err := os.Stat(full); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, n)
		if n > 9999 {
			return candidate
		}
	}
}

// isSafeHistoryPath reports whether filePath is inside the configured
// ConfigDir (same policy as isSafePath).
func isSafeHistoryPath(filePath string) bool {
	return isSafePath(filePath)
}

// ensureHistoryDir creates HistoryDir if it does not exist.
func ensureHistoryDir() error {
	if *HistoryDir == "" {
		return nil
	}
	return os.MkdirAll(*HistoryDir, 0750)
}

// saveHistory copies the current content of filePath into HistoryDir under
// a name derived from a hash of the path + current UTC timestamp. After
// writing, older versions beyond HistoryDepth are deleted (oldest first).
// No-op if filePath does not exist or is not inside ConfigDir. Errors are
// logged but not returned — history is best-effort.
func saveHistory(filePath string) {
	if !isSafeHistoryPath(filePath) {
		return
	}
	if *HistoryDir == "" || *HistoryDepth <= 0 {
		return
	}
	if err := ensureHistoryDir(); err != nil {
		fmt.Printf("[HISTORY] mkdir %s: %v\n", *HistoryDir, err)
		return
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	if len(content) == 0 {
		return
	}
	stamp := nextHistoryVersion(filePath)
	name := historyFileName(filePath, stamp)
	full := filepath.Join(*HistoryDir, name)
	if err := os.WriteFile(full, content, 0640); err != nil {
		fmt.Printf("[HISTORY] write %s: %v\n", full, err)
		return
	}
	rotateHistory(filePath)
}

// rotateHistory deletes the oldest history files for filePath until at
// most HistoryDepth remain.
func rotateHistory(filePath string) {
	entries, err := os.ReadDir(*HistoryDir)
	if err != nil {
		return
	}
	prefix := historyFilePrefix(filePath)
	type fi struct {
		name  string
		mtime time.Time
	}
	var versions []fi
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasPrefix(n, prefix) || !strings.HasSuffix(n, ".bak") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		versions = append(versions, fi{name: n, mtime: info.ModTime()})
	}
	if len(versions) <= *HistoryDepth {
		return
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].mtime.Before(versions[j].mtime)
	})
	excess := len(versions) - *HistoryDepth
	for i := 0; i < excess; i++ {
		_ = os.Remove(filepath.Join(*HistoryDir, versions[i].name))
	}
}

// HistoryEntry describes one saved version of a config file.
type HistoryEntry struct {
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
	Size      int    `json:"size"`
}

// listHistory returns all stored versions for filePath, newest first.
func listHistory(filePath string) ([]HistoryEntry, error) {
	if !isSafeHistoryPath(filePath) {
		return nil, os.ErrPermission
	}
	if *HistoryDir == "" {
		return []HistoryEntry{}, nil
	}
	entries, err := os.ReadDir(*HistoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []HistoryEntry{}, nil
		}
		return nil, err
	}
	prefix := historyFilePrefix(filePath)
	out := []HistoryEntry{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasPrefix(n, prefix) || !strings.HasSuffix(n, ".bak") {
			continue
		}
		stamp := strings.TrimSuffix(strings.TrimPrefix(n, prefix), ".bak")
		if !historyVersionRegex.MatchString(stamp) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, HistoryEntry{
			Version:   stamp,
			Timestamp: stamp,
			Size:      int(info.Size()),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Version > out[j].Version
	})
	return out, nil
}

// readHistoryVersion returns the raw bytes of a stored version.
func readHistoryVersion(filePath, version string) ([]byte, error) {
	if !isSafeHistoryPath(filePath) {
		return nil, os.ErrPermission
	}
	if !historyVersionRegex.MatchString(version) {
		return nil, fmt.Errorf("invalid_version")
	}
	full := filepath.Join(*HistoryDir, historyFilePrefix(filePath)+version+".bak")
	return os.ReadFile(full)
}

// restoreHistoryVersion overwrites filePath with the content of the given
// version, but only after saving the current state to history and running
// `dnsmasq --test`. If the test fails the previous content is restored.
func restoreHistoryVersion(filePath, version string) error {
	if !isSafeHistoryPath(filePath) {
		return os.ErrPermission
	}
	if !historyVersionRegex.MatchString(version) {
		return fmt.Errorf("invalid_version")
	}
	content, err := readHistoryVersion(filePath, version)
	if err != nil {
		return err
	}
	prev, _ := os.ReadFile(filePath)
	saveHistory(filePath)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return err
	}
	testCmd := exec.Command(dnsmasqBin(), "--test", "--conf-file="+filePath)
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		stats.Counters.TestFailures.Add(1)
		if prev != nil {
			_ = os.WriteFile(filePath, prev, 0644)
		}
		return fmt.Errorf("dnsmasq_test_failed: %s", testOut)
	}
	return nil
}

// createLocalBackup creates a single-shot .bak copy of filePath (overwriting
// any previous .bak) and also persists a versioned snapshot to history.
// Called before every write to a .conf file. No-op for unsafe paths.
func createLocalBackup(filePath string) {
	if !isSafePath(filePath) {
		return
	}
	saveHistory(filePath)
	content, err := os.ReadFile(filePath)
	if err == nil {
		os.WriteFile(filePath+".bak", content, 0644)
	}
}

// rollbackFile restores filePath from its .bak sibling. A fresh .bak is
// taken first so the rollback itself can be rolled back.
func rollbackFile(filePath string) error {
	if !isSafePath(filePath) {
		return os.ErrPermission
	}
	bakPath := filePath + ".bak"
	content, err := os.ReadFile(bakPath)
	if err != nil {
		return err
	}
	createLocalBackup(filePath)
	return os.WriteFile(filePath, content, 0644)
}

// unifiedDiff produces a minimal unified-style line diff between a and b.
// LCS-based — sufficient for short config files and avoids external deps.
func unifiedDiff(a, bText, headerA, headerB string) string {
	aLines := strings.Split(strings.TrimRight(a, "\n"), "\n")
	bLines := strings.Split(strings.TrimRight(bText, "\n"), "\n")
	if a == "" {
		aLines = []string{}
	}
	if bText == "" {
		bLines = []string{}
	}

	n, m := len(aLines), len(bLines)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if aLines[i] == bLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var b strings.Builder
	b.WriteString("--- " + headerA + "\n")
	b.WriteString("+++ " + headerB + "\n")
	i, j := 0, 0
	for i < n || j < m {
		if i < n && j < m && aLines[i] == bLines[j] {
			i++
			j++
			continue
		}
		for i < n && (j >= m || aLines[i] != bLines[j]) {
			if j < m && dp[i][j+1] > dp[i+1][j] {
				break
			}
			b.WriteString("-" + aLines[i] + "\n")
			i++
		}
		for j < m && (i >= n || aLines[i] != bLines[j]) {
			if i < n && dp[i+1][j] > dp[i][j+1] {
				break
			}
			b.WriteString("+" + bLines[j] + "\n")
			j++
		}
	}
	return b.String()
}
