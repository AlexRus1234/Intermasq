# Prometheus metriki

Endpoint `/metrics` vrača operacione metriki v exposition-formatu. Autentifikacija
je možna črez `Authorization: Bearer <JWT>` ili `X-API-Key: <INTERMASQ_SECRET>`.

| Metrika | Tip | Opis |
|---|---|---|
| `intermasq_hosts_total` | gauge | Čislo upravljajemyh `dhcp-host` zapisov. |
| `intermasq_leases_active` | gauge | Aktivne DHCP-arendy. |
| `intermasq_arp_online_total` | gauge | Ustroistva online po ARP. |
| `intermasq_dnsmasq_active` | gauge | `1`, kogda dnsmasq aktivny, inače `0`. |
| `intermasq_uptime_seconds` | gauge | Vremja raboty procesa. |
| `intermasq_domain_up{domain=...}` | gauge | Rezolvuje li se domen. |

DNS health-checker pri zapusku děla početny proverku, potom každye 60 sekund
rezolvuje A/CNAME domeny s tajm-autem 3 sekundy i kešuje rezultat v paměti.

```yaml
scrape_configs:
  - job_name: intermasq
    scrape_interval: 30s
    metrics_path: /metrics
    bearer_token: '<JWT>'
    static_configs:
      - targets: ['172.20.0.1:8081']
```
