# Intermasq — состояние проекта и следующие шаги

## Что такое Intermasq

Web-панель для управления dnsmasq (DHCP/DNS). Backend на Go (gin),
frontend на Vue 3 + Bootstrap 5. Релиз v1.0 pre-release. CI на
Forgejo Actions, контейнер Fedora 44.

Репозиторий: `B:\Repo\Intermasq\Intermasq`, ветка `main`.

## Текущее состояние тестового покрытия

| Слой | Coverage | Детали |
|---|---|---|
| L1+L2 Go (unit + httptest) | **65.6%\*** (измерено) | один package `main` → L1/L2 совместно не делятся. Парсеры/handler'ы 80-100%; ~50/52 handlers покрыты (skip `eventsHandler` SSE, `reloadHandler`). 241 тест, все с `-race`. Разрыв — в init-system/bootstrap/goroutine-коде (см. сноску). |
| L3 smoke.sh | ~75-80% API | 136 проверок, 29 suite-файлов в `tests/suites/`. Все Gap 1 endpoints закрыты. Плагин-прокси покрыт (`82-plugins.sh`). Иная метрика — доля эндпоинтов, не строки. |
| Perf/stress (opt-in) | реализован | `tests/perf.sh`: read-load, reload-storm, CRUD+RSS, SSE-endurance. Не coverage-слой — informational, soft thresholds. Opt-in через CI input `run_perf_tests`. |
| L4 Playwright UI | 25 specs | батч 1+2 + фазы А,Б,В закрыты (+rollback-ui/history-modal/discovery-tab/backup-restore-ui к фазе Б). Хелпер разбит: `lib/api.ts` → barrel + `api-auth.ts` + `api-hosts.ts`. A5 воспроизведён (`test.fail`, root cause pinned). Остаток — батч 4 + SSE + A5-фикс. См. `логи/gap2-batch3-phaseV.md` и предыдущие |
| L5 Real VM | 0% | Не начат. |

> **\*** `65.6%` — измерено `go test -cover ./...` (package `main`). Раньше в
> доках стояли оценки «~85-90%», но то был подсчёт «handler'ов с тестом», а не
> statement-coverage. ~34% непокрытых строк сосредоточены в `system.go`
> (init-system exec — это **Gap 4**), `bins.go` (резолв linux-бинарных),
> `main.go` (`main`/`loadPlugins` — bootstrap), `sse.go` (`startSSEBroadcaster`/
> `reloadDnsmasq`), `metrics.go` (`startDNSHealthChecker`/`runDNSHealthPass`).
> **~99% в текущем окружении недостижимо** — нужен Gap 4 (real VM) + рефактор
> bootstrap (правка исходников). Реалистичный потолок без них — ~80-85%.

Тестовая инфраструктура:
- `tests/smoke.sh` — entrypoint, source-ит suites в лексальном порядке
- `tests/perf.sh` — отдельный perf-orchestrator (opt-in), НЕ в smoke.sh
- `tests/lib/state.sh, common.sh, http.sh, auth.sh` — shared helpers
- `tests/suites/` — 29 файлов по компонентам (00-preflight → 90-logout)
- `tests/fixtures/` — `arp-sample.txt`, `gen-hosts.sh`, `plugins/hello/` (mock)
- `tests/known-bugs.txt` — список ID известных багов (KNOW-fail маркеры)
- `tests/bugreport/bugs.md` — детальные описания багов
- `tests/ROADMAP.md` — дорожная карта покрытия с оценками
- `.forgejo/workflows/build.yml` — CI pipeline (вручную `workflow_dispatch`,
  с opt-in `run_perf_tests`)

## Известные баги (12 открытых, НЕ правим — собираем)

| ID | Severity | Component | Описание |
|---|---|---|---|
| A1 | CRITICAL | frontend HostTable.vue | Дублирование строк при сортировке (Vue key collision: `:key="h.mac"` не уникален) |
| A2 | CRITICAL | backend aliases.go | Дубликаты DNS-alias можно добавлять (`findAliasesByDomain` исключает self для add-flow) |
| A3 | HIGH | backend main.go | Zero/broadcast MAC (`00:00:..`, `ff:ff:..`) принимаются |
| A4 | HIGH | backend main.go | MAC с `-` разделителем сохраняется verbatim, dnsmasq падает |
| A5 | HIGH | frontend BulkEditModal.vue | Модалка не рендерится при открытии: `preview` computed звёт `store_hosts.find(...)` вместо `store_hosts.hosts.find(...)` → TypeError. Воспроизведён Playwright (`bulk-ops.spec` → `test.fail`). Фикс = 1 строка. |
| A6 | MEDIUM | backend handlers_hosts.go | Bulk JSON response не имеет `count` поля |
| A7 | MEDIUM | frontend TemplatesModal.vue | UI layout не соответствует чек-листу (не баг, cosmetic) |
| A8 | MEDIUM | backend metrics.go | `/metrics` 401 имеет пустое body |
| A10 | LOW | backend arp_leases.go | Discovered devices не показывают IP (feature gap) |
| A11 | LOW | security | Path traversal (большинство закрыто, defence in depth) |
| A12 | HIGH | backend main.go | `aliasDomainRegex` отвергает `_` в домене (ломает DMARC/DKIM) |
| A13 | HIGH | backend dnsmasq.go | `writeFileRaw` гоняет `dnsmasq --test` без `--conf-file=<path>` |

**A1, A5, A7, A10** — frontend/feature, не представлены в `known-bugs.txt`
(smoke.sh их в принципе не может поймать). A1 под Playwright guard
(`hosts-sort.spec`). **A5 воспроизведён** в `bulk-ops.spec` (`test.fail`,
root cause pinned — см. баг-таблицу); true-фикс = 1 строка в проде, после
снять `.fail`. A7 (cosmetic) ждёт 3-й батч E2E.
**A2, A3, A4, A6, A8, A11, A12, A13** — в `known-bugs.txt`, имеют regression
test в smoke.sh с тегом `check ... Axx`.

Пока тег есть в `known-bugs.txt`, failure показывается как KNOWN-fail
(жёлтый, pipeline зелёный). При фиксе бага: удалить ID из
`known-bugs.txt`, обновить ожидание в smoke.sh.

Полные описания и фиксы: `tests/bugreport/bugs.md`.

## Что осталось по тестам

Gap 1 (smoke endpoints), Gap 3 (L2 edge cases), Gap 5 (perf), Gap 6 (plugins),
Gap 2 (1-я итерация Playwright) — **закрыты** (см. `tests/ROADMAP.md` → «Уже
закрыто»). Остались задачи, требующие новой инфраструктуры.

### Gap 2 — Playwright (UI тесты) — 3 батча ЗАКРЫТЫ (25 specs), остаток = батч 4 + SSE/A5

**Закрыто (батч 1+2 + фазы А,Б,В, 25 specs):** Playwright против `intermasq-ci`
в CI (Fedora 44, opt-in `run_e2e_tests`), отдельный `tests/e2e/` со своим
`package.json`/lockfile (продуктовый `frontend/package.json` не тронут).
Батч 1: auth/theme/i18n/hosts-sort (A1 guard)/host-crud. Батч 2:
host-add-ui/host-tags/search-filter/bulk-ops (bulk-move + bulk-edit)/
config-files + seed-хелпер. Фаза А: host-edit-ui/bulk-delete/templates-modal
(A7 smoke)/users-tab. Фаза Б: dns-aliases-add/bulk-import-text/csv-import/
reload-ui. Фаза В: rollback-ui/history-modal/discovery-tab/backup-restore-ui.
Хелпер разбит на `api-auth.ts` + `api-hosts.ts` + barrel `api.ts`.
Логи: `gap2-playwright-bootstrap.md`, `gap2-batch2-ui-coverage.md`,
`gap2-batch3-phaseA.md`, `gap2-batch3-phaseB.md`, `gap2-batch3-phaseV.md`.

**Бонус батча 2:** A5 пойман точным репродюсером — `bulk-ops.spec` (bulk-edit)
помечен `test.fail()` (CI зелёный). Root cause: `BulkEditModal.vue:67`
`store_hosts.find(...)` → `store_hosts.hosts.find(...)`.

**Осталось:**
- Батч 4 (6 нишевых + 2 жёстких): audit/plugins-iframe/i18n-api-error/
  config-template-fill/setup-screen + sse-live (мутация arp-файла) +
  config-directive/raw (блок A13).
- true A5-фикс (1 строка) → снять `test.fail` с bulk-edit-теста.
- (опционально) mutation-pass для эмпирической уверенности.

### Gap 4 — Real VM (init-system), +5%

**Что нужно:** persistent test VM (Proxmox API или virsh).

**Скрипт:**
1. Snap VM к чистому состоянию
2. Установить intermasq как systemd unit
3. Прогнать smoke.sh с `-init-system=systemd`
4. Проверить что dnsmasq реально рестартует
5. Повторить с systemd-user, openrc, runit
6. Отчёт

**Где добавить:** nightly cron job, отдельный workflow или внешний
скрипт. Это также единственный способ покрыть rootless-режим (обычный
юзер + sudo на `systemctl restart dnsmasq`/`intermasq`, см. `system.go`).

### Fuzzing, +2-3%

**Где:** Go built-in fuzzing (`go test -fuzz`).

**Цели:**
- `FuzzParseDhcpHostLine` — рандомные dhcp-host= строки
- `FuzzParseArpContent` — рандомный /proc/net/arp контент
- `FuzzParseLeases` — рандомный dnsmasq.leases
- `FuzzParseAliasLine` — рандомные address=/cname= строки

## Архитектура кода

```
main.go              flags, роуты, init, plugin loader
handlers.go          root handlers (status, login, arp, leases, discovery, SSE)
handlers_hosts.go    static host CRUD, bulk, CSV, templates
handlers_aliases.go  DNS alias CRUD, bulk, CSV
handlers_config.go   visual config editor, raw file editor
handlers_safety.go   rollback, history, ZIP backup/restore
handlers_users.go    user management, logout
dnsmasq.go           parsers, serializers, file I/O, IP transform
aliases.go           alias parsers, serializers
config_snapshot.go   config reader, directive splitter
history.go           versioned history (save/list/diff/restore)
backup.go            ZIP backup/restore, config file delete
arp_leases.go        ARP table, leases, new-device discovery
auth.go              JWT, bcrypt, rate limit, blacklist
metrics.go           Prometheus metrics, DNS health checker
sse.go               SSE broadcaster, dnsmasq status check
system.go            init-system callers (systemd/openrc/runit/sysv/none)
templates.go         host template store (load/save/apply)
audit.go             audit log (JSONL)
oui.go               OUI vendor lookup

frontend/src/
  App.vue            main layout, tab switcher
  store.js           reactive global state, API calls
  toast.js           toast composable (success/error/warning/info)
  components/
    ToastContainer.vue
    AuthScreen.vue
    static/          HostForm, HostTable, StaticView, BulkEditModal, TemplatesModal
    dns/             AliasForm, DnsAliasesView
    config/          DnsmasqConfig, DhcpOptionRow, ForwardingRow
    safety/          SafetyTab, AuditTab
    DiscoveredTab.vue, UsersTab.vue

tests/
  smoke.sh           L3 functional orchestrator
  perf.sh            perf/stress orchestrator (opt-in)
  lib/               shared bash helpers (state/common/http/auth)
  suites/            29 NN-*.sh functional suites
  fixtures/          arp-sample.txt, gen-hosts.sh, plugins/hello/ (mock)
  known-bugs.txt     expected-fail bug IDs
  bugreport/bugs.md  detailed bug descriptions
  ROADMAP.md         coverage roadmap
```

## Конвенции

- Коммиты прямо в `main` (один разработчик)
- Сообщения коммитов: `prefix: description` (smoke.sh:, Fix:, docs:, tests:, handlers_test.go:)
- gofmt обязателен (CI проверяет)
- Go тесты: `go test -race -count=1` (CI)
- smoke.sh: `KNOWN-fail` для багов из known-bugs.txt = pipeline зелёный
- perf.sh: opt-in (CI input `run_perf_tests`), hard-fail только на функциональные
  поломки; throughput/RSS — warnings
- Логи сессий: `логи/<name>.md` (формат: контекст → что сделано → результат)
- Frontend: Vue 3 `<script setup>`, Bootstrap 5 classes, vue-i18n (ru/en)
- Секреты: `INTERMASQ_SECRET` env var (32+ bytes), CI имеет хардкод
- Исходники продукта не правятся ради тестов — тестовая инфраструктура
  живёт в `tests/` + workflow; fixture-плагин изолирован своим `go.mod`
