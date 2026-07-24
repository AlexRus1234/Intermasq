# Intermasq — состояние проекта и следующие шаги

## Что такое Intermasq

Web-панель для управления dnsmasq (DHCP/DNS). Backend на Go (gin),
frontend на Vue 3 + Bootstrap 5. Релиз v1.0 pre-release. CI на
Forgejo Actions, контейнер Fedora 44.

Репозиторий: `B:\Repo\Intermasq\Intermasq`, ветка `main`.

## Текущее состояние тестового покрытия (~87-90%)

| Слой | Coverage | Детали |
|---|---|---|
| L1 Go unit | ~85% | 241 тест: `dnsmasq_test.go` (155), `new_features_test.go` (14), `handlers_test.go` (86). Все проходят с `-race`. |
| L2 Go handler (httptest) | ~85-90% | ~50 из 52 handlers покрыты. Skip: `eventsHandler` (SSE stream), `reloadHandler` (нужен dnsmasq binary). |
| L3 smoke.sh | ~75-80% API | 136 проверок, 29 suite-файлов в `tests/suites/`. Архитектура: `tests/smoke.sh` (orchestrator) + `tests/lib/` + `tests/suites/NN-*.sh`. Все Gap 1 endpoints закрыты. |
| L4 Playwright UI | 0% | Не начат. |
| L5 Real VM | 0% | Не начат. |

Тестовая инфраструктура:
- `tests/smoke.sh` — entrypoint, source-ит suites в лексальном порядке
- `tests/lib/state.sh, common.sh, http.sh, auth.sh` — shared helpers
- `tests/suites/` — 29 файлов по компонентам (00-preflight → 90-logout)
- `tests/known-bugs.txt` — список ID известных багов (KNOW-fail маркеры)
- `tests/bugreport/bugs.md` — детальные описания 11 багов (A1-A13)
- `tests/ROADMAP.md` — дорожная карта покрытия с оценками
- `.forgejo/workflows/build.yml` — CI pipeline (вручную `workflow_dispatch`)

## Известные баги (11 открытых, НЕ правим — собираем)

| ID | Severity | Component | Описание |
|---|---|---|---|
| A1 | CRITICAL | frontend HostTable.vue | Дублирование строк при сортировке (Vue key collision: `:key="h.mac"` не уникален) |
| A2 | CRITICAL | backend aliases.go | Дубликаты DNS-alias можно добавлять (`findAliasesByDomain` исключает self для add-flow) |
| A3 | HIGH | backend main.go | Zero/broadcast MAC (`00:00:..`, `ff:ff:..`) принимаются |
| A4 | HIGH | backend main.go | MAC с `-` разделителем сохраняется verbatim, dnsmasq падает |
| A5 | HIGH | frontend BulkEditModal.vue | Модалка не реагирует / no_hosts при пустом выборе |
| A6 | MEDIUM | backend handlers_hosts.go | Bulk JSON response не имеет `count` поля |
| A7 | MEDIUM | frontend TemplatesModal.vue | UI layout не соответствует чек-листу (не баг, cosmetic) |
| A8 | MEDIUM | backend metrics.go | `/metrics` 401 имеет пустое body |
| A10 | LOW | backend arp_leases.go | Discovered devices не показывают IP (feature gap) |
| A11 | LOW | security | Path traversal (большинство закрыто, defence in depth) |
| A12 | HIGH | backend main.go | `aliasDomainRegex` отвергает `_` в домене (ломает DMARC/DKIM) |
| A13 | HIGH | backend dnsmasq.go | `writeFileRaw` гоняет `dnsmasq --test` без `--conf-file=<path>` |

Каждый баг имеет regression test в smoke.sh с тегом `check ... Axx`.
Пока тег есть в `known-bugs.txt`, failure показывается как KNOWN-fail
(жёлтый, pipeline зелёный). При фиксе бага: удалить ID из
`known-bugs.txt`, обновить ожидание в smoke.sh.

Полные описания и фиксы: `tests/bugreport/bugs.md`.

## Что осталось по тестам

Все «лёгкие» приросты на Go и bash исчерпаны. Оставшиеся задачи требуют
новой инфраструктуры.

### Gap 2 — Playwright (UI тесты), +20%

**Что нужно:** Chromium в Fedora CI-контейнере, Playwright spec-тесты.

**Где добавить:**
- CI: шаг установки Chromium + `npx playwright install` в `.forgejo/workflows/build.yml`
- Спеки: `frontend/tests/` или `tests/e2e/`

**Сценарии для покрытия (20-30 specs):**
- A1 regression: кликнуть на сортировку колонки 3 раза, проверить что
  count строк не изменился
- A5 regression: открыть bulk-edit модалку, снять чекбоксы, убедиться
  что модалка закрылась
- Auth flow: login → dashboard → logout → redirect
- i18n: переключение RU/EN, проверка ключевых строк
- Dark/light тема: переключение, сохранение в localStorage
- SSE updates: подключиться к /api/events, получить ARP update
- CRUD: add host → редактировать → удалить, без full page reload
- Tags: add host с set:iot,tag:guest → проверить отображение badge
- Bulk operations: выбрать 3 хоста → bulk-move → bulk-edit
- Config editor: создать файл → edit directive → raw PUT → delete
- Search/filter: ввести текст в search box → проверить фильтрацию

**Что закроет:** regression для A1, A5, A7 + весь browser-side logic.

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
скрипт.

### Gap 5 — Performance/stress, +3%

**Что нужно:**
- `tests/fixtures/gen-hosts.sh` — генератор .conf с N хостами (N=200)
- `hey` или `wrk` для load testing API endpoints
- Тест: 50 параллельных SSE клиентов держатся 60с без обрыва
- Тест: 10 одновременных reload не ломают dnsmasq
- Тест: memory RSS стабилен после 1000 add/delete циклов

### Gap 6 — Plugin system, +2%

**Что нужно:** mock plugin в `tests/fixtures/plugins/hello/`:
- `manifest.json`: `{"id":"hello","name":"Hello Plugin","bin":"hello.sh"}`
- `hello.sh`: открывает unix socket, отвечает "hello" на любой запрос
- smoke.sh: проверяет что `/api/plugins` показывает плагин, что
  `/plugins/hello/` проксируется

### Fuzzing, +2-3%

**Где:** Go built-in fuzzing (`go test -fuzz`).

**Цели:**
- `FuzzParseDhcpHostLine` — рандомные dhcp-host= строки
- `FuzzParseArpContent` — рандомный /proc/net/arp контент
- `FuzzParseLeases` — рандомный dnsmasq.leases
- `FuzzParseAliasLine` — рандомные address=/cname= строки

## Архитектура кода

```
main.go              flags, роуты, init
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
```

## Конвенции

- Коммиты прямо в `main` (один разработчик)
- Сообщения коммитов: `prefix: description` (smoke.sh:, Fix:, docs:, handlers_test.go:)
- gofmt обязателен (CI проверяет)
- Go тесты: `go test -race -count=1` (CI)
- smoke.sh: `KNOWN-fail` для багов из known-bugs.txt = pipeline зелёный
- Логи сессий: `логи/<name>.md` (формат: контекст → что сделано → результат)
- Frontend: Vue 3 `<script setup>`, Bootstrap 5 classes, vue-i18n (ru/en)
- Секреты: `INTERMASQ_SECRET` env var (32+ bytes), CI имеет хардкод
