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

package netstate

import (
	"bufio"
	"flag"
	"os"
	"sort"
	"strings"

	"intermask/internal/dnsmasq"
	"intermask/internal/models"
	"intermask/internal/oui"
)

var (
	LeasesPath = flag.String("leases", "/var/lib/misc/dnsmasq.leases", "Path to dnsmasq.leases")
	ArpPath    = flag.String("arp-file", "/proc/net/arp", "Path to ARP table file")
)

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

func parseLeasesContent(content string) []models.LeaseEntry {
	leases := []models.LeaseEntry{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 {
			l := models.LeaseEntry{Ip: fields[2], Mac: fields[1]}
			if len(fields) > 3 {
				l.Hostname = fields[3]
			}
			leases = append(leases, l)
		}
	}
	return leases
}

func parseLeases() []models.LeaseEntry {
	data, err := os.ReadFile(*LeasesPath)
	if err != nil {
		return []models.LeaseEntry{}
	}
	return parseLeasesContent(string(data))
}

func getNewDevices() []models.NewDeviceInfo {
	arp := getArpTable()
	leases := parseLeases()
	hosts := dnsmasq.ReadAllHosts()

	knownMacs := make(map[string]bool)
	for _, l := range leases {
		knownMacs[strings.ToLower(l.Mac)] = true
	}
	for _, h := range hosts {
		knownMacs[strings.ToLower(h.Mac)] = true
	}

	var devices []models.NewDeviceInfo
	for mac := range arp {
		macLower := strings.ToLower(mac)
		if !knownMacs[macLower] {
			devices = append(devices, models.NewDeviceInfo{Mac: macLower, Vendor: oui.LookupOUI(macLower)})
		}
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Mac < devices[j].Mac })
	return devices
}

func GetArpTable() map[string]bool                          { return getArpTable() }
func ParseArpContent(content string) map[string]bool        { return parseArpContent(content) }
func ParseLeases() []models.LeaseEntry                      { return parseLeases() }
func GetNewDevices() []models.NewDeviceInfo                 { return getNewDevices() }
