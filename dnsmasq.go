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
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

func isSafePath(path string) bool {
	cleanPath := filepath.Clean(path)
	cleanDir := filepath.Clean(*ConfigDir)
	return strings.HasPrefix(cleanPath, cleanDir+string(os.PathSeparator)) || cleanPath == cleanDir
}

type sseClient struct {
	ch chan string
}

var (
	sseClients   = make(map[*sseClient]bool)
	sseClientsMu sync.Mutex
)

func sseRegister(client *sseClient) {
	sseClientsMu.Lock()
	sseClients[client] = true
	sseClientsMu.Unlock()
}

func sseUnregister(client *sseClient) {
	sseClientsMu.Lock()
	delete(sseClients, client)
	sseClientsMu.Unlock()
}

func sseBroadcast(event, data string) {
	sseClientsMu.Lock()
	defer sseClientsMu.Unlock()
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
	for c := range sseClients {
		select {
		case c.ch <- msg:
		default:
		}
	}
}

func startSSEBroadcaster() {
	go func() {
		lastArp := ""
		lastStatus := false
		for {
			time.Sleep(5 * time.Second)
			arp := getArpTable()
			arpJSON := arpToJSON(arp)
			status := checkDnsmasqStatus()
			if arpJSON != lastArp {
				sseBroadcast("arp", arpJSON)
				lastArp = arpJSON
			}
			if status != lastStatus {
				sseBroadcast("dnsmasq_status", fmt.Sprintf(`{"active":%v}`, status))
				lastStatus = status
			}
		}
	}()
}

func arpToJSON(arp map[string]bool) string {
	b, _ := json.Marshal(arp)
	return string(b)
}

func checkDnsmasqStatus() bool {
	return sysCaller.IsActive("dnsmasq")
}

func reloadDnsmasq() error {
	testCmd := exec.Command("/usr/bin/dnsmasq", "--test")
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		return fmt.Errorf("%s", testOut)
	}
	return sysCaller.Restart("dnsmasq")
}

func getArpTable() map[string]bool {
	content, err := os.ReadFile(*ArpPath)
	if err != nil {
		return make(map[string]bool)
	}
	return parseArpContent(string(content))
}

func parseArpContent(content string) map[string]bool {
	activeMacs := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Scan()
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 && fields[2] == "0x2" && fields[3] != "00:00:00:00:00:00" {
			activeMacs[strings.ToLower(fields[3])] = true
		}
	}
	return activeMacs
}

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
// collide with an existing history file. Base format is YYYYMMDD-HHMMSS;
// a -N suffix is appended when a file with that base id already exists.
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
// ConfigDir (same policy as isSafePath — history is only stored for files
// we actually manage).
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
// logged but not returned — history is best-effort and must never block a
// write operation.
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
		// File does not exist yet — nothing to snapshot.
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
		// name = <prefix><YYYYMMDD-HHMMSS>.bak
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
	// Read the current on-disk content so we can undo on test failure.
	prev, _ := os.ReadFile(filePath)
	// Snapshot the current state so the user can undo the restore.
	saveHistory(filePath)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return err
	}
	testCmd := exec.Command("/usr/bin/dnsmasq", "--test")
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		// Restore the pre-restore content. Best-effort.
		if prev != nil {
			_ = os.WriteFile(filePath, prev, 0644)
		}
		return fmt.Errorf("dnsmasq_test_failed: %s", testOut)
	}
	return nil
}

func createLocalBackup(filePath string) {
	if !isSafePath(filePath) {
		return
	}
	// Persist a versioned snapshot BEFORE overwriting the .bak file so
	// history always reflects the on-disk state prior to this edit.
	saveHistory(filePath)
	content, err := os.ReadFile(filePath)
	if err == nil {
		os.WriteFile(filePath+".bak", content, 0644)
	}
}

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

func parseLeases() []LeaseEntry {
	leases := []LeaseEntry{}
	file, err := os.Open(*LeasesPath)
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 3 {
				l := LeaseEntry{Ip: fields[2], Mac: fields[1]}
				if len(fields) > 3 {
					l.Hostname = fields[3]
				}
				leases = append(leases, l)
			}
		}
	}
	return leases
}

func findFreeIP(cidr string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid_cidr")
	}

	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return "", fmt.Errorf("ipv6_not_supported")
	}
	if ones >= 31 {
		return "", fmt.Errorf("range_too_small")
	}

	network := ipNet.IP.To4()
	if network == nil {
		return "", fmt.Errorf("invalid_ipv4")
	}

	mask := ipNet.Mask
	broadcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		broadcast[i] = network[i] | ^mask[i]
	}

	candidate := make(net.IP, 4)
	copy(candidate, network)

	scanned := 0
	const scanLimit = 256

	for {
		incIP(candidate)
		scanned++
		if scanned > scanLimit {
			return "", fmt.Errorf("range_exhausted")
		}

		if candidate.Equal(broadcast) {
			return "", fmt.Errorf("range_exhausted")
		}

		if candidate[3] == 0 {
			continue
		}

		if len(findHostsByIP(candidate.String(), "")) == 0 {
			return candidate.String(), nil
		}
	}
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

func removeHostLine(filePath, mac string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	newLines := []string{}
	macLower := strings.ToLower(mac)
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if strings.HasPrefix(clean, "dhcp-host=") && strings.Contains(strings.ToLower(clean), macLower) {
			continue
		}
		if clean != "" {
			newLines = append(newLines, line)
		}
	}
	return os.WriteFile(filePath, []byte(strings.Join(newLines, "\n")+"\n"), 0644)
}

func appendHostLine(filePath, mac, hostname, ip string) error {
	content, _ := os.ReadFile(filePath)
	line := fmt.Sprintf("dhcp-host=%s,%s,%s", mac, hostname, ip)
	out := strings.TrimRight(string(content), "\n")
	if out != "" {
		out += "\n"
	}
	out += line + "\n"
	return os.WriteFile(filePath, []byte(out), 0644)
}

func readHostByMac(filePath, mac string) *HostEntry {
	macLower := strings.ToLower(mac)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "dhcp-host=") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(line, "dhcp-host="), ",")
		entry := HostEntry{File: filePath}
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if macRegex.MatchString(p) {
				entry.Mac = p
			} else if net.ParseIP(p) != nil {
				entry.Ip = p
			} else {
				entry.Hostname = p
			}
		}
		if strings.ToLower(entry.Mac) == macLower {
			return &entry
		}
	}
	return nil
}

type ipTransformMode int

const (
	ipTransformNone ipTransformMode = iota
	ipTransformOctets
	ipTransformCIDR
)

type ipTransform struct {
	mode    ipTransformMode
	oldNet  *net.IPNet
	newNet  *net.IPNet
	oldPref string
	newPref string
}

func parseIPTransform(oldStr, newStr string) (*ipTransform, error) {
	if oldStr == "" && newStr == "" {
		return &ipTransform{mode: ipTransformNone}, nil
	}
	if oldStr == "" || newStr == "" {
		return nil, fmt.Errorf("both_prefixes_required")
	}

	if strings.Contains(oldStr, "/") || strings.Contains(newStr, "/") {
		if !strings.Contains(oldStr, "/") || !strings.Contains(newStr, "/") {
			return nil, fmt.Errorf("prefix_format_mismatch")
		}
		_, oldNet, err1 := net.ParseCIDR(oldStr)
		_, newNet, err2 := net.ParseCIDR(newStr)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("invalid_cidr")
		}
		onesOld, _ := oldNet.Mask.Size()
		onesNew, _ := newNet.Mask.Size()
		if onesOld != onesNew {
			return nil, fmt.Errorf("prefix_mismatch")
		}
		if oldNet.IP.To4() == nil || newNet.IP.To4() == nil {
			return nil, fmt.Errorf("ipv6_not_supported")
		}
		return &ipTransform{mode: ipTransformCIDR, oldNet: oldNet, newNet: newNet}, nil
	}

	octetRe := regexp.MustCompile(`^(\d{1,3}\.){0,2}\d{1,3}$`)
	if !octetRe.MatchString(oldStr) || !octetRe.MatchString(newStr) {
		return nil, fmt.Errorf("invalid_prefix_format")
	}
	if strings.Count(oldStr, ".") != strings.Count(newStr, ".") {
		return nil, fmt.Errorf("prefix_format_mismatch")
	}
	return &ipTransform{mode: ipTransformOctets, oldPref: oldStr, newPref: newStr}, nil
}

func (t *ipTransform) apply(ip string) (string, error) {
	if t.mode == ipTransformNone {
		return ip, nil
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", fmt.Errorf("invalid_ip")
	}

	switch t.mode {
	case ipTransformOctets:
		if !strings.HasPrefix(ip, t.oldPref) {
			return "", fmt.Errorf("prefix_not_matched")
		}
		boundary := len(t.oldPref)
		if boundary < len(ip) && ip[boundary] != '.' {
			return "", fmt.Errorf("prefix_not_matched")
		}
		return t.newPref + ip[boundary:], nil
	case ipTransformCIDR:
		oldIP := parsed.To4()
		if oldIP == nil {
			return "", fmt.Errorf("invalid_ipv4")
		}
		mask := t.oldNet.Mask
		if !oldIP.Mask(mask).Equal(t.oldNet.IP.Mask(mask)) {
			return "", fmt.Errorf("prefix_not_matched")
		}
		newIP := make(net.IP, 4)
		for i := 0; i < 4; i++ {
			newIP[i] = (oldIP[i] & ^mask[i]) | t.newNet.IP[i]
		}
		return newIP.String(), nil
	}
	return ip, nil
}

func findHostsByIP(ip, excludeMac string) []HostEntry {
	result := []HostEntry{}
	excludeMacLower := strings.ToLower(excludeMac)

	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return result
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, f.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "dhcp-host=") {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(line, "dhcp-host="), ",")
			entry := HostEntry{File: fullPath}
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if macRegex.MatchString(p) {
					entry.Mac = p
				} else if net.ParseIP(p) != nil {
					entry.Ip = p
				} else {
					entry.Hostname = p
				}
			}
			if entry.Ip == ip && strings.ToLower(entry.Mac) != excludeMacLower {
				result = append(result, entry)
			}
		}
	}
	return result
}

func findHostsByMac(mac string) []HostEntry {
	result := []HostEntry{}
	macLower := strings.ToLower(mac)

	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return result
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, f.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "dhcp-host=") {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(line, "dhcp-host="), ",")
			entry := HostEntry{File: fullPath}
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if macRegex.MatchString(p) {
					entry.Mac = p
				} else if net.ParseIP(p) != nil {
					entry.Ip = p
				} else {
					entry.Hostname = p
				}
			}
			if strings.ToLower(entry.Mac) == macLower {
				result = append(result, entry)
			}
		}
	}
	return result
}

func readAllHosts() []HostEntry {
	hosts := []HostEntry{}
	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return hosts
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, f.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "dhcp-host=") {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(line, "dhcp-host="), ",")
			entry := HostEntry{File: fullPath}
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if macRegex.MatchString(p) {
					entry.Mac = p
				} else if net.ParseIP(p) != nil {
					entry.Ip = p
				} else {
					entry.Hostname = p
				}
			}
			if entry.Mac != "" {
				hosts = append(hosts, entry)
			}
		}
	}
	return hosts
}

func hostsToCSV(hosts []HostEntry) []byte {
	buf := new(bytes.Buffer)
	w := csv.NewWriter(buf)
	w.Write([]string{"mac", "ip", "hostname"})
	for _, h := range hosts {
		w.Write([]string{h.Mac, h.Ip, h.Hostname})
	}
	w.Flush()
	return buf.Bytes()
}

func parseCSVHosts(r io.Reader, targetFile string) ([]HostEntry, error) {
	reader := csv.NewReader(r)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	hosts := []HostEntry{}
	for i, row := range records {
		if i == 0 {
			continue
		}
		if len(row) < 3 {
			continue
		}
		mac := strings.TrimSpace(row[0])
		ip := strings.TrimSpace(row[1])
		hostname := strings.TrimSpace(row[2])

		if macRegex.MatchString(mac) && net.ParseIP(ip) != nil && hostnameRegex.MatchString(hostname) {
			hosts = append(hosts, HostEntry{Mac: mac, Ip: ip, Hostname: hostname, File: targetFile})
		}
	}
	return hosts, nil
}

// directiveKeyRegex matches a dnsmasq directive name: lowercase letters,
// digits and hyphens. Used to distinguish "real" comments from commented-out
// directives (e.g. "#no-resolv").
var directiveKeyRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// directiveValueSeparator splits "key=value" into key and value. dnsmasq conf
// files use '=' as the canonical separator. Keys may contain hyphens
// (e.g. "no-resolv", "domain-needed"), so '-' is NOT treated as a separator.
func splitDirective(line string) (key, value string, ok bool) {
	if idx := strings.Index(line, "="); idx > 0 {
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		if directiveKeyRegex.MatchString(k) {
			return k, v, true
		}
	}
	k := strings.TrimSpace(line)
	if directiveKeyRegex.MatchString(k) {
		return k, "", true
	}
	return "", "", false
}

// readConfigSnapshot walks all .conf files in ConfigDir and extracts every
// directive except dhcp-host. Commented-out directives (e.g. "#no-resolv")
// are returned with Active=false. Plain comments (non-directive) and empty
// lines are skipped. dhcp-range directives are additionally returned in a
// structured form via DhcpRange slice.
func readConfigSnapshot() ConfigSnapshot {
	snap := ConfigSnapshot{Files: []ConfigFile{}, DhcpRanges: []DhcpRange{}}

	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return snap
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

		cf := ConfigFile{
			Path:       fullPath,
			Name:       f.Name(),
			Directives: []Directive{},
		}
		if _, err := os.Stat(fullPath + ".bak"); err == nil {
			cf.HasBak = true
		}

		lines := strings.Split(string(content), "\n")
		for i, raw := range lines {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}

			active := true
			if strings.HasPrefix(line, "#") {
				active = false
				line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
				if line == "" {
					continue
				}
			}

			if strings.HasPrefix(line, "dhcp-host=") || strings.HasPrefix(line, "dhcp-host:") {
				continue
			}
			if isAliasDirective(line) {
				continue
			}

			key, value, ok := splitDirective(line)
			if !ok {
				continue
			}

			d := Directive{
				Key:    key,
				Value:  value,
				Active: active,
				File:   fullPath,
				LineNo: i + 1,
			}
			cf.Directives = append(cf.Directives, d)

			if key == "dhcp-range" && active {
				r := parseDhcpRange(value, fullPath, i+1)
				snap.DhcpRanges = append(snap.DhcpRanges, r)
			}
		}

		snap.Files = append(snap.Files, cf)
	}

	return snap
}

// parseDhcpRange parses a dhcp-range value into a structured form.
// Supported formats:
//   - start,end,netmask,lease          (e.g. 192.168.1.50,192.168.1.150,255.255.255.0,12h)
//   - start,end,lease                  (netmask inferred)
//   - prefix/len,lease                 (CIDR form, e.g. 192.168.0.0/24,1h)
//   - set:tag,start,end,netmask,lease  (tagged)
//   - tag:tag,...                      (tagged)
func parseDhcpRange(raw, file string, lineNo int) DhcpRange {
	r := DhcpRange{Raw: raw, File: file, LineNo: lineNo}
	parts := strings.Split(raw, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	rest := parts
	if len(rest) > 0 {
		first := rest[0]
		if strings.HasPrefix(first, "set:") {
			r.Tag = strings.TrimPrefix(first, "set:")
			rest = rest[1:]
		} else if strings.HasPrefix(first, "tag:") {
			r.Tag = strings.TrimPrefix(first, "tag:")
			rest = rest[1:]
		}
	}

	if len(rest) == 0 {
		return r
	}

	if strings.Contains(rest[0], "/") {
		r.Mask = rest[0]
		r.CIDR = rest[0]
		if _, _, err := net.ParseCIDR(rest[0]); err == nil {
			r.Start = strings.Split(rest[0], "/")[0]
		}
		if len(rest) > 1 {
			r.LeaseTime = rest[1]
		}
		return r
	}

	r.Start = rest[0]
	if len(rest) > 1 {
		r.End = rest[1]
	}
	if len(rest) > 2 {
		third := rest[2]
		if isLeaseTime(third) {
			r.LeaseTime = third
		} else {
			r.Mask = third
			if len(rest) > 3 {
				r.LeaseTime = rest[3]
			}
		}
	}

	r.CIDR = dhcpRangeToCIDR(r)
	return r
}

// isLeaseTime reports whether s looks like a dnsmasq lease duration
// (e.g. "12h", "30m", "1d", "infinite", "3600s", or a plain seconds number).
func isLeaseTime(s string) bool {
	if s == "infinite" {
		return true
	}
	if len(s) < 2 {
		return false
	}
	leaseRe := regexp.MustCompile(`^\d+[smhdw]?$`)
	return leaseRe.MatchString(s)
}

// dhcpRangeToCIDR computes a CIDR string (network/prefix) from a DhcpRange
// using start address and netmask. Returns "" if it cannot be determined.
func dhcpRangeToCIDR(r DhcpRange) string {
	if r.Mask == "" || r.Start == "" {
		return ""
	}
	if strings.Contains(r.Mask, "/") {
		return r.Mask
	}
	ip := net.ParseIP(r.Start)
	mask := net.ParseIP(r.Mask)
	if ip == nil || mask == nil {
		return ""
	}
	ip4 := ip.To4()
	mask4 := mask.To4()
	if ip4 == nil || mask4 == nil {
		return ""
	}
	maskIP := net.IPv4Mask(mask4[0], mask4[1], mask4[2], mask4[3])
	ones, _ := maskIP.Size()
	if ones == 0 {
		return ""
	}
	network := ip4.Mask(maskIP)
	return fmt.Sprintf("%s/%d", network.String(), ones)
}

// detectDhcpRangesCIDR returns the list of CIDR strings derived from all
// active dhcp-range directives. Used to populate the IP-range dropdown in
// templates and the random-IP button.
func detectDhcpRangesCIDR() []string {
	snap := readConfigSnapshot()
	out := []string{}
	seen := make(map[string]bool)
	for _, r := range snap.DhcpRanges {
		if r.CIDR != "" && !seen[r.CIDR] {
			seen[r.CIDR] = true
			out = append(out, r.CIDR)
		}
	}
	return out
}

// serializeConfigFile rebuilds a .conf file preserving:
//   - leading plain comments (lines starting with # that are not directives)
//   - all dhcp-host= lines in their original order
//   - the supplied directives, sorted by group then key for readability
//
// Inactive directives are written with a "#" prefix. Boolean directives
// (empty Value) are written as bare "key"; valued directives as "key=value".
func serializeConfigFile(path string, directives []Directive) ([]byte, error) {
	content, err := os.ReadFile(path)
	existing := ""
	if err == nil {
		existing = string(content)
	}

	headerComments := []string{}
	dhcpHostLines := []string{}
	aliasLines := []string{}
	if existing != "" {
		for _, raw := range strings.Split(existing, "\n") {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "dhcp-host=") || strings.HasPrefix(line, "dhcp-host:") {
				dhcpHostLines = append(dhcpHostLines, raw)
				continue
			}
			if isAliasDirective(line) {
				aliasLines = append(aliasLines, raw)
				continue
			}
			if strings.HasPrefix(line, "#") {
				stripped := strings.TrimSpace(strings.TrimPrefix(line, "#"))
				if stripped == "" {
					headerComments = append(headerComments, raw)
					continue
				}
				if _, _, ok := splitDirective(stripped); ok {
					continue
				}
				headerComments = append(headerComments, raw)
				continue
			}
		}
	}

	sorted := make([]Directive, len(directives))
	copy(sorted, directives)
	sort.SliceStable(sorted, func(i, j int) bool {
		gi, gj := directiveGroup(sorted[i].Key), directiveGroup(sorted[j].Key)
		if gi != gj {
			return gi < gj
		}
		if sorted[i].Key != sorted[j].Key {
			return sorted[i].Key < sorted[j].Key
		}
		return false
	})

	var b strings.Builder
	if len(headerComments) > 0 {
		b.WriteString(strings.Join(headerComments, "\n"))
		b.WriteString("\n")
	}
	if len(dhcpHostLines) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.Join(dhcpHostLines, "\n"))
		b.WriteString("\n")
	}
	if len(aliasLines) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.Join(aliasLines, "\n"))
		b.WriteString("\n")
	}
	if len(sorted) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("# === Managed by Intermasq ===\n")
		for _, d := range sorted {
			line := d.Key
			if d.Value != "" {
				line = d.Key + "=" + d.Value
			}
			if !d.Active {
				line = "#" + line
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return []byte(b.String()), nil
}

// directiveGroup returns a numeric group id for sorting directives in the
// serialized file. Lower id = earlier in file. Order: dns, dhcp, log, other.
func directiveGroup(key string) int {
	switch key {
	case "domain", "domain-needed", "bogus-priv", "no-resolv", "no-hosts",
		"listen-address", "bind-interfaces", "except-interface", "interface",
		"server", "address", "local", "expand-hosts", "no-poll", "resolv-file",
		"strict-order", "all-servers", "clear-on-reload":
		return 0
	case "dhcp-range", "dhcp-option", "dhcp-lease-max", "dhcp-authoritative",
		"dhcp-no-override", "dhcp-hostsfile", "dhcp-leasefile", "no-dhcp-interface":
		return 1
	case "log-queries", "log-dhcp", "log-facility", "log-async":
		return 2
	}
	return 3
}

// writeConfigWithTest writes new content to a .conf file and validates the
// result via `dnsmasq --test`. If the test fails the previous content is
// restored from the .bak backup created just before writing.
// aliasLinePrefixes returns true if the given trimmed line is a managed
// DNS alias directive (address= or cname=).
func isAliasDirective(line string) bool {
	return strings.HasPrefix(line, "address=") || strings.HasPrefix(line, "cname=")
}

// parseAliasLine parses a single "address=" or "cname=" line into a
// DnsAliasEntry. Returns ok=false if the line is malformed or unsupported
// (e.g. address=/#/IP wildcard is out of scope).
func parseAliasLine(line, file string, hasBak bool) (DnsAliasEntry, bool) {
	entry := DnsAliasEntry{File: file}
	if hasBak {
		entry.File = file + "|has_bak"
	}
	if strings.HasPrefix(line, "address=") {
		val := strings.TrimPrefix(line, "address=")
		// Expected form: /domain/IP. Domain is between two slashes.
		if !strings.HasPrefix(val, "/") {
			return DnsAliasEntry{}, false
		}
		rest := strings.TrimPrefix(val, "/")
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return DnsAliasEntry{}, false
		}
		entry.Type = "A"
		entry.Domain = rest[:slash]
		entry.Target = strings.TrimSpace(rest[slash+1:])
		if entry.Domain == "" || entry.Target == "" {
			return DnsAliasEntry{}, false
		}
		// Wildcards (#, *.domain) are out of scope for the UI.
		if entry.Domain == "#" || strings.HasPrefix(entry.Domain, "*") {
			return DnsAliasEntry{}, false
		}
		return entry, true
	}
	if strings.HasPrefix(line, "cname=") {
		val := strings.TrimPrefix(line, "cname=")
		parts := strings.Split(val, ",")
		if len(parts) < 2 {
			return DnsAliasEntry{}, false
		}
		entry.Type = "CNAME"
		entry.Domain = strings.TrimSpace(parts[0])
		entry.Target = strings.TrimSpace(parts[1])
		if entry.Domain == "" || entry.Target == "" {
			return DnsAliasEntry{}, false
		}
		// Skip tagged cnames (cname=alias,target,tag:...) — keep alias/target only.
		return entry, true
	}
	return DnsAliasEntry{}, false
}

func aliasToLine(a DnsAliasEntry) string {
	if a.Type == "CNAME" {
		return fmt.Sprintf("cname=%s,%s", a.Domain, a.Target)
	}
	return fmt.Sprintf("address=/%s/%s", a.Domain, a.Target)
}

// readAllAliases scans all .conf files in ConfigDir and returns every
// address= and cname= directive as a structured DnsAliasEntry.
func readAllAliases() []DnsAliasEntry {
	aliases := []DnsAliasEntry{}
	files, err := os.ReadDir(*ConfigDir)
	if err != nil {
		return aliases
	}
	for _, f := range files {
		if filepath.Ext(f.Name()) != ".conf" {
			continue
		}
		fullPath := filepath.Join(*ConfigDir, f.Name())
		hasBak := false
		if _, err := os.Stat(fullPath + ".bak"); err == nil {
			hasBak = true
		}
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		for _, raw := range strings.Split(string(content), "\n") {
			line := strings.TrimSpace(raw)
			if !isAliasDirective(line) {
				continue
			}
			if entry, ok := parseAliasLine(line, fullPath, hasBak); ok {
				aliases = append(aliases, entry)
			}
		}
	}
	return aliases
}

// findAliasesByDomain returns aliases whose Domain matches (case-insensitive)
// the given domain, excluding one with the provided file+type combination.
// Used for duplicate detection.
func findAliasesByDomain(domain string, excludeType, excludeFile string) []DnsAliasEntry {
	result := []DnsAliasEntry{}
	domainLower := strings.ToLower(domain)
	for _, a := range readAllAliases() {
		if strings.ToLower(a.Domain) != domainLower {
			continue
		}
		if a.Type == excludeType && cleanAliasFile(a.File) == excludeFile {
			continue
		}
		result = append(result, a)
	}
	return result
}

// cleanAliasFile strips the "|has_bak" marker appended by readAllAliases.
func cleanAliasFile(f string) string {
	if i := strings.Index(f, "|"); i >= 0 {
		return f[:i]
	}
	return f
}

// appendAliasLine appends a single alias directive to the file, preserving
// existing content. Does NOT validate; caller must do that.
func appendAliasLine(filePath string, entry DnsAliasEntry) error {
	content, _ := os.ReadFile(filePath)
	line := aliasToLine(entry)
	out := strings.TrimRight(string(content), "\n")
	if out != "" {
		out += "\n"
	}
	out += line + "\n"
	return os.WriteFile(filePath, []byte(out), 0644)
}

// removeAliasLine removes the first alias directive matching the given
// type+domain from the file. Returns true if a line was removed.
func removeAliasLine(filePath, aliasType, domain string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(content), "\n")
	newLines := []string{}
	removed := false
	domainLower := strings.ToLower(domain)
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if !removed && isAliasDirective(clean) {
			if entry, ok := parseAliasLine(clean, "", false); ok && entry.Type == aliasType && strings.ToLower(entry.Domain) == domainLower {
				removed = true
				continue
			}
		}
		if clean != "" {
			newLines = append(newLines, line)
		}
	}
	if !removed {
		return false, nil
	}
	return true, os.WriteFile(filePath, []byte(strings.Join(newLines, "\n")+"\n"), 0644)
}

func aliasesToCSV(aliases []DnsAliasEntry) []byte {
	buf := new(bytes.Buffer)
	w := csv.NewWriter(buf)
	w.Write([]string{"type", "domain", "target"})
	for _, a := range aliases {
		w.Write([]string{a.Type, a.Domain, a.Target})
	}
	w.Flush()
	return buf.Bytes()
}

func parseCSVAliases(r io.Reader, targetFile string) ([]DnsAliasEntry, error) {
	reader := csv.NewReader(r)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	aliases := []DnsAliasEntry{}
	for i, row := range records {
		if i == 0 {
			// Skip header if present.
			if len(row) >= 1 && strings.EqualFold(row[0], "type") {
				continue
			}
		}
		if len(row) < 3 {
			continue
		}
		t := strings.ToUpper(strings.TrimSpace(row[0]))
		domain := strings.TrimSpace(row[1])
		target := strings.TrimSpace(row[2])
		if t != "A" && t != "CNAME" {
			continue
		}
		if !aliasDomainRegex.MatchString(domain) {
			continue
		}
		if t == "A" {
			if net.ParseIP(target) == nil {
				continue
			}
		} else {
			if !aliasDomainRegex.MatchString(target) {
				continue
			}
		}
		aliases = append(aliases, DnsAliasEntry{Type: t, Domain: domain, Target: target, File: targetFile})
	}
	return aliases, nil
}

// ensureAliasesFile creates the default aliases file if it does not exist,
// with a small header comment. Path is returned unchanged when it already
// exists. Used as a fallback when req.File is empty.
func ensureAliasesFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if !isSafePath(path) {
		return os.ErrPermission
	}
	header := "# DNS aliases managed by Intermasq\n# Format: address=/domain/IP  or  cname=alias,target\n"
	return os.WriteFile(path, []byte(header), 0644)
}

// unifiedDiff produces a minimal unified-style line diff between a and b.
// It is intentionally simple (LCS-based) — sufficient for short config
// files and avoids pulling in external dependencies.
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

func writeConfigWithTest(path string, content []byte) error {
	if !isSafePath(path) {
		return os.ErrPermission
	}
	createLocalBackup(path)
	if err := os.WriteFile(path, content, 0644); err != nil {
		return err
	}
	testCmd := exec.Command("/usr/bin/dnsmasq", "--test")
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		_ = rollbackFile(path)
		return fmt.Errorf("dnsmasq_test_failed: %s", testOut)
	}
	return nil
}

func readFileRaw(path string) ([]byte, error) {
	if !isSafePath(path) {
		return nil, os.ErrPermission
	}
	return os.ReadFile(path)
}

func writeFileRaw(path string, content []byte) error {
	if !isSafePath(path) {
		return os.ErrPermission
	}
	createLocalBackup(path)
	if err := os.WriteFile(path, content, 0644); err != nil {
		return err
	}
	testCmd := exec.Command("/usr/bin/dnsmasq", "--test")
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		_ = rollbackFile(path)
		return fmt.Errorf("dnsmasq_test_failed: %s", testOut)
	}
	return nil
}

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

	testCmd := exec.Command("/usr/bin/dnsmasq", "--test")
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		for _, name := range restoredFiles {
			fullPath := filepath.Join(*ConfigDir, name)
			bakPath := fullPath + ".restore.bak"
			if bakContent, err := os.ReadFile(bakPath); err == nil {
				os.WriteFile(fullPath, bakContent, 0644)
			}
		}
		return fmt.Errorf("dnsmasq_test_failed: %s", testOut)
	}

	return nil
}

func getNewDevices() []NewDeviceInfo {
	arp := getArpTable()
	leases := parseLeases()
	hosts := readAllHosts()

	knownMacs := make(map[string]bool)
	for _, l := range leases {
		knownMacs[strings.ToLower(l.Mac)] = true
	}
	for _, h := range hosts {
		knownMacs[strings.ToLower(h.Mac)] = true
	}

	var devices []NewDeviceInfo
	for mac := range arp {
		macLower := strings.ToLower(mac)
		if !knownMacs[macLower] {
			devices = append(devices, NewDeviceInfo{
				Mac:    macLower,
				Vendor: lookupOUI(macLower),
			})
		}
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Mac < devices[j].Mac
	})
	return devices
}
