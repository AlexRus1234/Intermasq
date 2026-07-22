# Сессия: smoke.sh refactor + Gap 1 coverage

**Дата:** 22 июля 2026
**Ветка:** `main`
**Коммитов:** 3 (ef9048b, 94a0295, cf7816b)

## Контекст

После CI-автоматизации (см. `ci-automation-session.md`) smoke.sh вырос до
626 строк и становился нечитаемым. Каждый новый endpoint удлинял файл
линейно, git-diff на правку одного сценария портил контекст остальных.

Цель сессии: (1) разбить smoke.sh на дерево фокусных файлов,
(2) заполнить Gap 1 из ROADMAP — 13 endpoints без integration-тестов.

## Что было сделано

### 1. Рефакторинг smoke.sh → lib/ + suites/ (ef9048b)

Старый 626-строчный smoke.sh превратился в 64-строчный orchestrator +
23 фокусных файла. Структура:

```
tests/
├── smoke.sh              # 64 строки: source lib, гоняет suites, summary
├── lib/
│   ├── state.sh          # counters, KNOWN_BUGS, init_state, print_summary
│   ├── common.sh         # colors, section, check, skip, fatal
│   ├── http.sh           # GET/POST/DELETE/PGET/PPOST/KGET, body, jval
│   └── auth.sh           # have_jwt, require_jwt
└── suites/
    ├── 00-preflight.sh … 90-logout.sh   # 19 файлов на старой логике
```

Поведение и вывод byte-for-byte идентичны оригиналу. Все 24 файла
проходят `bash -n`. Executable bit smoke.sh сохранён (100755).

Принцип: каждая suite — тело без функции, использует shared state из
`lib/`. Orchestrator в цикле `source`-ит `suites/[0-9]*.sh` в
лексальном порядке. Добавление нового теста = новый файл без правки
entrypoint.

### 2. Gap 1 — 10 новых suites (94a0295)

13 endpoints из ROADMAP Gap 1 разнесены по 10 фокусным файлам.
Каждый покрывает happy path + 3-4 edge case (empty/invalid/unsafe).

| Suite | Endpoint(s) | Покрытие |
|---|---|---|
| `26-hosts-bulk-move.sh` | `POST /api/hosts/bulk-move` | happy, empty hosts, unsafe target, same_file |
| `27-hosts-bulk-edit.sh` | `POST /api/hosts/bulk-edit` | prefix transform, empty hosts, partial prefix, unknown host |
| `33-aliases-bulk.sh` | `POST /api/aliases/bulk` | happy, no_valid_entries, in-batch dup, unsafe file |
| `34-aliases-csv.sh` | `GET/POST /api/aliases/csv` | export, import round-trip, no file |
| `41-config-put.sh` | `PUT /api/config` | valid directives, bad key, uppercase key, newline value, unsafe file |
| `42-templates-hosts.sh` | `GET/POST/DELETE /api/templates`, `/ranges` | list, create, dup-create, missing fields, delete, delete-missing, ranges |
| `51-history-diff-restore.sh` | `GET /api/history/diff`, `POST /api/history/restore` | diff happy, missing params, unknown version, restore happy, missing/invalid version |
| `52-backup-restore.sh` | `POST /api/backup/restore` | round-trip, no file, non-ZIP |
| `82-plugins.sh` | `GET /api/plugins` | shape check |
| `83-discovery.sh` | `/api/leases`, `/api/arp`, `/api/new-devices`, `/api/hosts/next-ip` | shape, ARP count from fixture |

### 3. Fix 2 test-design bugs (cf7816b)

Первый прогон после Gap 1 дал 9 unexpected fails. Разбор показал: 0
новых багов приложения, все 9 — баги самих тестов.

**Bug 1: `27-hosts-bulk-edit.sh` выбрал хост без hostname.**
bulkEditHandler валидирует `validHostname(newHostname)` даже если
hostname не трансформируется. ee:04 был создан в `20-hosts-happy.sh`
без hostname → `existing.Hostname = ""` → `validHostname("")` = false
→ 400 invalid_hostname ещё до применения IP-transform.
Фикс: переключились на ee:11 (hostname="csv2", IP=10.0.0.21 → 10.0.1.21).

**Bug 2: `91-plugins.sh` и `92-discovery.sh` шли после `90-logout.sh`.**
Logout блэклистит JWT — все 6 последующих проверок получали 401.
Фикс: переименовали 91 → 82 и 92 → 83 (перед logout). Git отследил
как rename, история сохранена.

## Результат

Pipeline зелёный после фиксов:

```
Pass:        126 / 136
Fail:          0 / 136
Known-fail:   10 / 136  (bugs: A11 A12 A13 A2 A3 A4 A6 A8)
Skipped:      0 / 136
```

Все 10 KNOWN-fails точно соответствуют `tests/bugreport/bugs.md`.
Ни одного нового бага приложения не выявлено.

### Coverage до/после

| Слой | До | После |
|---|---|---|
| L1 Go unit | ~85% | ~85% (без изменений) |
| L2 Go handler (httptest) | ~30% | ~30% (без изменений) |
| L3 smoke.sh | ~60% API | **~75-80% API** |
| L4 Playwright | 0% | 0% |
| L5 Real VM | 0% | 0% |
| **Суммарно** | **~60-65%** | **~75-80%** |

Прирост +15% соответствует оценке Gap 1 из ROADMAP.

## Что НЕ сделано (отложено)

- **Gap 2 (Playwright, +20%)** — UI-баги A1/A5/A7 ничем другим не
  покрыть. Нужно поднять Chromium в Fedora-контейнере, ~2 дня.
- **Gap 3 (Go edge cases, +5%)** — race, unicode hostname, IPv6,
  пустые .conf. 1 день, не требует новой инфраструктуры.
- **L2 expansion (+5-7%)** — httptest для ~44 непокрытых handlers.
  Только 6/50 сейчас протестированы на уровне handler-middleware.
- **Баги приложения** — сознательно не правятся в этой сессии
  (см. `tests/bugreport/bugs.md`, 11 открытых).
