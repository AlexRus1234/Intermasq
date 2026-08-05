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
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"intermask/internal/stats"
)

// metricsCounters / counters live in internal/stats (var stats.Counters) so
// that packages which bump the counters do not import the metrics handler.

// dnsHealthEntry is the latest result of resolving a managed domain.
type dnsHealthEntry struct {
	Up      bool
	Latency time.Duration
	LastChk time.Time
}

var (
	dnsHealthMu sync.RWMutex
	dnsHealth   = make(map[string]dnsHealthEntry)
)

// metricsHandler exposes operational metrics in Prometheus exposition format.
// Authentication is identical to the rest of the API: either an
// Authorization: Bearer <jwt> header, X-API-Key: <secret> header, or a
// ?token=<jwt-or-secret> query parameter (the last one is convenient for
// Prometheus scrape configs which cannot easily send custom headers).
func metricsHandler(c *gin.Context) {
	if !checkMetricsAuth(c) {
		c.AbortWithStatusJSON(401, gin.H{"error": "auth_required"})
		return
	}

	hosts := readAllHosts()
	leases := parseLeases()
	arp := getArpTable()
	active := checkDnsmasqStatus()

	var b strings.Builder
	writeSimpleMetric(&b, "intermasq_hosts_total", "Total number of managed dhcp-host entries.", float64(len(hosts)))
	writeSimpleMetric(&b, "intermasq_leases_active", "Current number of active DHCP leases.", float64(len(leases)))
	writeSimpleMetric(&b, "intermasq_arp_online_total", "Number of devices currently flagged online by ARP.", float64(len(arp)))
	writeSimpleMetric(&b, "intermasq_dnsmasq_active", "1 if dnsmasq unit is active, 0 otherwise.", boolToFloat(active))
	writeSimpleMetric(&b, "intermasq_reloads_total", "Total number of successful dnsmasq reloads triggered via the panel.", float64(stats.Counters.Reloads.Load()))
	writeSimpleMetric(&b, "intermasq_dnsmasq_test_failures_total", "Number of times dnsmasq --test rejected a change.", float64(stats.Counters.TestFailures.Load()))
	writeSimpleMetric(&b, "intermasq_uptime_seconds", "Seconds since the panel process started.", time.Since(stats.Counters.StartedAt).Seconds())

	dnsHealthMu.RLock()
	for domain, h := range dnsHealth {
		writeLabeledMetric(&b, "intermasq_domain_up", "1 if the managed domain currently resolves, 0 otherwise.",
			map[string]string{"domain": domain}, boolToFloat(h.Up))
		writeLabeledMetric(&b, "intermasq_domain_resolve_seconds", "Last DNS resolve latency for the managed domain.",
			map[string]string{"domain": domain}, h.Latency.Seconds())
	}
	dnsHealthMu.RUnlock()

	c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	c.String(200, b.String())
}

// checkMetricsAuth replicates the relevant slice of authMiddleware but also
// accepts the ?token= query value equal to SecretKey (API key), so a
// Prometheus scrape_url can be `http://host/metrics?token=<secret>`.
func checkMetricsAuth(c *gin.Context) bool {
	if apiKey := c.GetHeader("X-API-Key"); apiKey != "" && apiKey == string(SecretKey) {
		return true
	}
	if tok := c.Query("token"); tok != "" {
		if tok == string(SecretKey) {
			return true
		}
		if token, _ := jwt.Parse(tok, func(t *jwt.Token) (interface{}, error) { return SecretKey, nil }); token != nil && token.Valid {
			return true
		}
	}
	if authHeader := c.GetHeader("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, _ := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) { return SecretKey, nil })
		if token != nil && token.Valid {
			return true
		}
	}
	return false
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func writeSimpleMetric(b *strings.Builder, name, help string, value float64) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s gauge\n", name)
	fmt.Fprintf(b, "%s %g\n", name, value)
}

func writeLabeledMetric(b *strings.Builder, name, help string, labels map[string]string, value float64) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s gauge\n", name)
	var parts []string
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=%q", k, v))
	}
	fmt.Fprintf(b, "%s{%s} %g\n", name, strings.Join(parts, ","), value)
}

// startDNSHealthChecker launches a background goroutine that periodically
// resolves every managed domain (A/CNAME aliases) and records the result so
// it can be scraped via /metrics as intermasq_domain_up{domain=...}.
func startDNSHealthChecker() {
	go func() {
		// First pass quickly so /metrics has data right after startup.
		runDNSHealthPass()
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			runDNSHealthPass()
		}
	}()
}

// dnsResolver is the lookup function used by runDNSHealthPass. Default is
// net.Resolver{PreferGo: true}.LookupHost; tests can swap this for a stub
// to avoid real network I/O and to drive the up/down/error branches.
var dnsResolver = func(ctx context.Context, domain string) ([]string, error) {
	resolver := net.Resolver{PreferGo: true}
	return resolver.LookupHost(ctx, domain)
}

func runDNSHealthPass() {
	aliases := readAllAliases()
	if len(aliases) == 0 {
		return
	}
	for _, a := range aliases {
		if a.Type != "A" && a.Type != "CNAME" {
			continue
		}
		domain := a.Domain
		if domain == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		start := time.Now()
		_, err := dnsResolver(ctx, domain)
		cancel()
		latency := time.Since(start)

		dnsHealthMu.Lock()
		dnsHealth[domain] = dnsHealthEntry{
			Up:      err == nil,
			Latency: latency,
			LastChk: time.Now(),
		}
		dnsHealthMu.Unlock()
	}
}
