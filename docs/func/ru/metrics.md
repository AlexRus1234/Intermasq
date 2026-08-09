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

# Метрики Prometheus

Эндпоинт `/metrics` возвращает эксплуатационные метрики в формате exposition.
Он расположен вне группы `/api` в соответствии с соглашениями Prometheus.

## Аутентификация

```http
GET /metrics
```

Доступен один из следующих вариантов аутентификации:

| Способ | Заголовок / параметр |
|---|---|
| Bearer JWT | `Authorization: Bearer <JWT>` |
| API key | `X-API-Key: <INTERMASQ_SECRET>` |

## Метрики

| Метрика | Тип | Описание |
|---|---|---|
| `intermasq_hosts_total` | gauge | Количество управляемых записей dhcp-host |
| `intermasq_leases_active` | gauge | Текущее количество активных DHCP-аренд |
| `intermasq_arp_online_total` | gauge | Устройств онлайн по ARP |
| `intermasq_dnsmasq_active` | gauge | `1` если dnsmasq активен, иначе `0` |
| `intermasq_reloads_total` | gauge | Успешных reload'ов через панель |
| `intermasq_dnsmasq_test_failures_total` | gauge | Сколько раз `dnsmasq --test` отклонил изменение |
| `intermasq_uptime_seconds` | gauge | Аптайм процесса |
| `intermasq_domain_up{domain=…}` | gauge | Резолвится ли домен (health-check каждые 60с) |
| `intermasq_domain_resolve_seconds{domain=…}` | gauge | Latency последнего резолва |

> Метрики с суффиксом `*_total` реализованы как `gauge`, а не как Prometheus
> `counter`. Это текущее ограничение реализации.

## DNS health-check

Фоновая горутина `metrics.StartDNSHealthChecker` выполняет следующие действия:

1. При запуске выполняется первичный проход, обеспечивающий наличие данных в `/metrics`.
2. Далее каждые 60 секунд резолвит каждый домен из A/CNAME-записей через
   `net.Resolver{PreferGo: true}` с таймаутом 3 секунды.
3. Результат кешируется in-memory (`map[domain]dnsHealthEntry`).

## Пример `scrape_config`

```yaml
scrape_configs:
  - job_name: intermasq
    scrape_interval: 30s
    metrics_path: /metrics
    bearer_token: '<JWT>'
    static_configs:
      - targets: ['172.20.0.1:8081']
```

Аутентификация с помощью API key выполняется через заголовок. Способ его
передачи зависит от версии Prometheus и используемой конфигурации.
Передача токена в параметре `?token=` не поддерживается.

## Примеры алертов

```promql
# dnsmasq упал
intermasq_dnsmasq_active == 0

# управляемый домен перестал резолвиться
intermasq_domain_up{domain="wiki.lan"} == 0

# рост числа отклонённых проверок dnsmasq --test
rate(intermasq_dnsmasq_test_failures_total[5m]) > 0
```
