<!--
Intermasq - Web panel for dnsmasq
Copyright (C) 2026 AlexRus1234

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
-->

# Prometheus Metrics

The `/metrics` endpoint returns operational metrics in exposition format. It
is outside the `/api` group, following Prometheus conventions.

## Authentication

```http
GET /metrics
```

Use one of these authentication methods:

| Method | Header / parameter |
|---|---|
| Bearer JWT | `Authorization: Bearer <JWT>` |
| API key | `X-API-Key: <INTERMASQ_SECRET>` |

## Metrics

| Metric | Type | Description |
|---|---|---|
| `intermasq_hosts_total` | gauge | Number of managed dhcp-host records |
| `intermasq_leases_active` | gauge | Current number of active DHCP leases |
| `intermasq_arp_online_total` | gauge | Devices online according to ARP |
| `intermasq_dnsmasq_active` | gauge | `1` if dnsmasq is active, otherwise `0` |
| `intermasq_reloads_total` | gauge | Successful reloads through the panel |
| `intermasq_dnsmasq_test_failures_total` | gauge | Number of changes rejected by `dnsmasq --test` |
| `intermasq_uptime_seconds` | gauge | Process uptime |
| `intermasq_domain_up{domain=...}` | gauge | Whether a domain resolves (health check every 60s) |
| `intermasq_domain_resolve_seconds{domain=...}` | gauge | Latency of the latest resolution |

> Metrics with the `*_total` suffix are implemented as `gauge`, not Prometheus
> `counter`. This is a current implementation limitation.

## DNS health check

The background goroutine `metrics.StartDNSHealthChecker`:

1. Performs an initial pass at startup so `/metrics` contains data.
2. Resolves every domain from A/CNAME records every 60 seconds using `net.Resolver{PreferGo: true}` with a three-second timeout.
3. Caches results in memory (`map[domain]dnsHealthEntry`).

## `scrape_config` example

```yaml
scrape_configs:
  - job_name: intermasq
    scrape_interval: 30s
    metrics_path: /metrics
    bearer_token: '<JWT>'
    static_configs:
      - targets: ['172.20.0.1:8081']
```

API-key authentication uses a header. How to configure that depends on the
Prometheus version and configuration. Passing the token as a `?token=` query
parameter is not supported.

## Alert examples

```promql
# dnsmasq has stopped
intermasq_dnsmasq_active == 0

# a managed domain no longer resolves
intermasq_domain_up{domain="wiki.lan"} == 0

# dnsmasq --test rejections are increasing
rate(intermasq_dnsmasq_test_failures_total[5m]) > 0
```
