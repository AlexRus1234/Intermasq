# Prometheus метрики

Endpoint `/metrics` враћа операционе метрики в exposition-формату. Аутентификација
је можни чез `Authorization: Bearer <JWT>` или `X-API-Key: <INTERMASQ_SECRET>`.

| Метрика | Тип | Опис |
|---|---|---|
| `intermasq_hosts_total` | gauge | Число управјаних `dhcp-host` записов. |
| `intermasq_leases_active` | gauge | Активне DHCP-аренде. |
| `intermasq_arp_online_total` | gauge | Устројства online по ARP. |
| `intermasq_dnsmasq_active` | gauge | `1`, когда dnsmasq активни, инако `0`. |
| `intermasq_uptime_seconds` | gauge | Време работы процесса. |
| `intermasq_domain_up{domain=...}` | gauge | Резолвује ли се домен. |

DNS health-checker при запуску дєлаје почетну проверку, потом кажде 60 секунд
резолвује A/CNAME домени с timeout 3 секунды и кешује резултат в памяти.

```yaml
scrape_configs:
  - job_name: intermasq
    scrape_interval: 30s
    metrics_path: /metrics
    bearer_token: '<JWT>'
    static_configs:
      - targets: ['172.20.0.1:8081']
```
