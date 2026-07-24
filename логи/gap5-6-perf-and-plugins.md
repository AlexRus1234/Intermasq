# Сессия: закрытие Gap 6 (плагины) + Gap 5 (perf/stress)

**Дата:** 24 июля 2026
**Ветка:** `main`
**Коммитов:** 3 (`733c879`, `76bc163`, `309cb2a`)

## Контекст

В `логи/duis.md` зафиксированы два оставшихся «лёгких» пробела тестового покрытия:

- **Gap 6 — система плагинов (+2%)**: ни один тест не проверял, что intermasq
  реально поднимает плагин и проксирует запросы на его unix-сокет.
- **Gap 5 — perf/stress (+3%)**: нагрузочное/SSE/RSS-тестирование отсутствовало.

Жёсткие ограничения от автора (сформулированы до старта работ):

1. **Go-исходники не трогать.** `main.go`, handlers и т.д. — без правок.
2. **Никаких новых «дыр»** — мок-плагин обязан работать строго по существующему
   контракту (unix-сокет через env `PLUGIN_SOCKET`), без TCP-слушателя.
3. Workflow (`build.yml`) — трогать можно.

Ключевое наблюдение по коду: `PluginsDir`/`SocketsDir` захардкожены в
`main.go:69-70`, но CI-контейнер `fedora:44` гоняется под **root**, поэтому
плагин можно просто положить в `/etc/intermasq/plugins/hello/` до старта
бинарника — `loadPlugins()` подхватит сам, без новых флагов. Rootless-режим
(обычный юзер + sudo на `systemctl restart dnsmasq`/`intermasq`, `system.go`)
к плагинам ортогонален — это прерогатива Gap 4 (real VM).

## Что было сделано

### Gap 6 — мок-плагин (без правок исходников)

| Файл | Назначение |
|---|---|
| `tests/fixtures/plugins/hello/main.go` | Мок-плагин: читает `PLUGIN_SOCKET`, биндит unix-сокет, отвечает на `/` и `/health`. Свой `go.mod` → изолирован от `./...` основного модуля (проверено: `go list ./...` его не видит, `gofmt -l .` чист). |
| `tests/fixtures/plugins/hello/go.mod` | `module intermasq/test-plugin-hello`, go 1.25. |
| `tests/fixtures/plugins/hello/manifest.json` | `{"id":"hello","name":"Hello Plugin","bin":"hello"}`. |
| `.forgejo/workflows/build.yml` | Шаг **«Build & install mock plugin»** перед smoke: собирает бинарь и кладёт в `/etc/intermasq/plugins/hello/`. |
| `tests/suites/82-plugins.sh` | Расширен с 1 до 3 проверок: `GET /api/plugins → 200`, presence `hello` в списке, проксирование `/plugins/hello/health → 200`. |

### Gap 5 — perf/stress (opt-in, отдельный orchestrator)

| Файл | Назначение |
|---|---|
| `tests/fixtures/gen-hosts.sh` | Детерминированный генератор `dhcp-host=MAC,IP,hostname` на N хостов. MAC/IP из счётчика; IP в 1..254 (без `.0`/`.255`), MAC никогда не all-zero. |
| `tests/perf.sh` | Отдельный orchestrator (НЕ в `smoke.sh`). 4 сценария: read-throughput, reload-storm, CRUD-churn+RSS, SSE-endurance. Hard-fail только на функциональные поломки; throughput/RSS — warnings. RSS через `/proc/<pid>/status` (без зависимости от `procps-ng`). |
| `.forgejo/workflows/build.yml` | Новый input `run_perf_tests` (default false) + шаг «L4 — perf/stress» со своей инстанцией сервера на `:18082` (отдельный conf-dir, чтобы не конфликтовать со smoke). |

Дизайн-решения (согласованы с автором через вопросы):
- Perf **отдельно** от smoke и за opt-in флагом — timing-пороги флапают на shared-раннерах, не должны красить основной pipeline.
- Нагрузка — pure `curl` + `xargs -P` (ноль новых зависимостей, в стиле проекта).
- Мок-плагин — Go-бинарник (Go уже стоит в CI), а не bash+sociat.

## Баги, всплывшие в CI, и их фиксы

Три итерации, каждая подсвечена реальным прогоном CI.

| # | Симптом в CI | Причина | Фикс (`perf.sh`) |
|---|---|---|---|
| 1 | `http.sh:15: $2: unbound variable` | `PPOST "/api/status"` вызван с 1 аргументом, а `PPOST` dereference-ит `$2` (JSON-body); под `set -u` → фатал до получения JWT | `/api/status` — GET → `PGET` (как в `00-preflight.sh:14`) |
| 2 | `xargs: unmatched single quote` | `xargs ... sh -c '...$JWT...$BASE...'` — вложенные кавычки | Убран `sh -c`: `xargs` дёргает `curl` напрямую, переменные разворачивает внешний shell |
| 3 | CRUD `200/200 failures`, все `409 MAC duplicate` | CRUD-цикл генерил MAC'и `aa:bb:cc:…` тем же счётчиком 1..N, что и `seed.conf` из scenario 1; `findHostsByMac` сканит **все** `.conf` в `ConfigDir` → коллизия | CRUD переведён на префикс `aa:bb:dd`. Заодно xargs переведён на null-delimited ввод (`tr '\n' '\0' \| xargs -0`) |

После фикса #3 подтверждено CI-прогоном: все 4 сценария зелёные.

## Результат

```
=== read load — GET /api/hosts x200 (concurrency 20) ===
  200 reqs in 0.179s → 1118.3 req/s | 2xx=200 non-2xx=0 (0.00%)
  ✓ all reads 2xx / throughput ≥ 25 req/s

=== reload storm — POST /api/reload x10 (concurrent) ===
  status codes observed: 10 200
  ✓ no 5xx / all agreed / server still up

=== CRUD churn — 200 add→delete cycles ===
  CRUD failures: 0 / 200
  RSS after: delta -1 MB (leak-hard limit 200 MB)
  ✓ all CRUD cycles succeeded / RSS within budget

=== SSE endurance — 20 clients x 15s ===
  alive=20/20 dropped=0 (0%)
  ✓ all 20 SSE clients survived / server still up

=== PERF SUMMARY ===
perf: no hard failures (warnings are informational).
```

Покрытие (по duis.md): **Gap 6 закрыт (+2%), Gap 5 закрыт (+3%)**.
Существенный бонус Gap 6: расширенный `82-plugins.sh` regression-сетит связку
`loadPlugins()` + ReverseProxy, что ранее не покрывалось вовсе.

Асимптотические цели duis.md (SSE 50 клиентов/60с, CRUD 1000 циклов)
поднимаются env-переменными (`SSE_CLIENTS`, `SSE_SECONDS`, `CRUD_CYCLES` и др.)
— значения по умолчанию CI-friendly.
