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

// arp_leases.go — read-only access to the ARP table and the dnsmasq leases
// file, plus the "discovery" feature that surfaces devices present in ARP
// but missing from both static hosts and active leases.

package main

import (
	"bufio"
	"os"
	"sort"
	"strings"
)

// getArpTable reads /proc/net/arp (path overridable via -arp-file) and
// returns the set of MACs whose flags indicate a reachable neighbour.
func getArpTable() map[string]bool {
	content, err := os.ReadFile(*ArpPath)
	if err != nil {
		return make(map[string]bool)
	}
	return parseArpContent(string(content))
}

// parseArpContent extracts the set of MAC addresses flagged as reachable
// (0x2) from the textual representation of /proc/net/arp. The first line
// (header) is always skipped. Zero MACs are filtered out.
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

// parseLeases reads dnsmasq.leases (path overridable via -leases). The file
// format is whitespace-separated: timestamp MAC IP [hostname] [client-id].
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

// getNewDevices returns MACs that appear in the ARP table (i.e. recently
// talked to the LAN) but are neither configured as static dhcp-host= entries
// nor have an active DHCP lease. Each entry is enriched with a vendor name
// via the embedded OUI table. Sorted by MAC for stable UI output.
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
