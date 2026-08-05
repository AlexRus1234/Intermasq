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

// sse.go — Server-Sent Events broker for live updates of ARP table and
// dnsmasq service status. Clients connect via /api/events (auth in the
// Authorization header — see docs/portability-and-validation.md for why
// ?token= was removed). The broadcaster goroutine polls the underlying
// files every 5s and pushes an event only when the value actually changes,
// so a typical client receives a few messages per minute, not per second.

package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"intermask/internal/bins"
	"intermask/internal/stats"
)

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

// sseBroadcast sends an event to every connected client. Non-blocking per
// client: a slow consumer with a full channel is silently skipped — better
// to drop a status update than to block the broadcaster.
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

// startSSEBroadcaster spawns a goroutine that polls ARP and dnsmasq status
// every 5s and pushes a delta event when either changes. Designed to be
// called once from main(); calling more than once will duplicate events.
func startSSEBroadcaster() {
	go func() {
		lastArp := ""
		lastStatus := false
		for {
			time.Sleep(5 * time.Second)
			arpJSON, status := ssePollOnce()
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

// ssePollOnce performs a single polling iteration: reads the current ARP
// table and dnsmasq status. Extracted from startSSEBroadcaster so the
// per-iteration logic is unit-testable without sleeping or spawning a
// goroutine. Returns the JSON-marshalled ARP map and the dnsmasq-active flag.
func ssePollOnce() (arpJSON string, status bool) {
	arp := getArpTable()
	arpJSON = arpToJSON(arp)
	status = checkDnsmasqStatus()
	return arpJSON, status
}

func arpToJSON(arp map[string]bool) string {
	b, _ := json.Marshal(arp)
	return string(b)
}

// checkDnsmasqStatus asks the active init-system caller whether the dnsmasq
// service is currently running. Returns false on any caller error.
func checkDnsmasqStatus() bool {
	return sysCaller.IsActive("dnsmasq")
}

// reloadDnsmasq runs `dnsmasq --test` first and, on success, asks the
// init-system caller to restart the service. A failed test prevents the
// restart so the running dnsmasq keeps working off the previous (valid)
// config. Reload and test-failure counters are bumped for /metrics.
func reloadDnsmasq() error {
	testCmd := exec.Command(bins.Dnsmasq(), "--test")
	if testOut, testErr := testCmd.CombinedOutput(); testErr != nil {
		stats.Counters.TestFailures.Add(1)
		return fmt.Errorf("%s", testOut)
	}
	if err := sysCaller.Restart("dnsmasq"); err != nil {
		return err
	}
	stats.Counters.Reloads.Add(1)
	return nil
}
