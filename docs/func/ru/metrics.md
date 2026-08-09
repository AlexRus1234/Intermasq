# Метрики для Prometheus

Эндпоинт `/metrics` отдаёт operational-метрики в exposition-формате. Живёт вне
группы `/api`, чтобы URL соответствовал конвенции Prometheus.

## Аутентификация

```http
GET /metrics
```

Один из вариантов:

| Способ | Заголовок / параметр |
|---|---|
| Bearer JWT | `Authorization: Bearer <JWT>` |
| API key | `X-API-Key: <INTERMASQ_SECRET>` |

## Метрики

| Метрика | Тип | Описание |
|---|---|---|
| `intermasq_hosts_total` | gauge | Кол-во управляемых dhcp-host записей |
| `intermasq_leases_active` | gauge | Текущее кол-во активных DHCP-аренд |
| `intermasq_arp_online_total` | gauge | Устройств онлайн по ARP |
| `intermasq_dnsmasq_active` | gauge | `1` если dnsmasq активен, иначе `0` |
| `intermasq_reloads_total` | gauge | Успешных reload'ов через панель |
| `intermasq_dnsmasq_test_failures_total` | gauge | Сколько раз `dnsmasq --test` отклонил изменение |
| `intermasq_uptime_seconds` | gauge | Аптайм процесса |
| `intermasq_domain_up{domain=…}` | gauge | Резолвится ли домен (health-check каждые 60с) |
| `intermasq_domain_resolve_seconds{domain=…}` | gauge | Latency последнего резолва |

> `*_total` сейчас gauge, а не полноценный Prometheus-counter. Это упрощение
> без внешних зависимостей; переход на `promauto` — в планах рефакторинга.

## DNS health-check

Фоновая горутина `metrics.StartDNSHealthChecker`:

1. При старте — первый быстрый проход, чтобы `/metrics` имел данные сразу.
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

Проще — через API key в заголовке (через `metric_relabel_configs` или
`Authorization` в `static_configs`, в зависимости от версии Prometheus).
Альтернатива в виде `?token=` теперь не поддерживается — используйте заголовки.

## Примеры алертов

```promql
# dnsmasq упал
intermasq_dnsmasq_active == 0

# управляемый домен перестал резолвиться
intermasq_domain_up{domain="wiki.lan"} == 0

# рост отвергнутых dnsmasq --test — кто-то льёт битый конфиг
rate(intermasq_dnsmasq_test_failures_total[5m]) > 0
```
