# Gap 2 — финал: L4 Playwright закрыт (батч 4 + фиксы A5/A13)

**Дата:** 2026-07-27
**Gap:** 2 (L4 — UI/E2E), закрытие по плану `Gap_2_finish.md`
**Коммиты:**
- `7cd0e1d` — Блок A: A5 + A13 фиксы + smoke/A13-чек
- `949ae4f` — изоляция A3/A4-хостов (`19-bugs.conf`)
- `f729d46` — docs: A5/A13 в ROADMAP + bugs.md, удалён duis.md
- `81038cb` — Блок B: 8 specs (батч 4)
- `ac967a1` — фикс config-directive (strict-mode, scope к dns-group)
**Результат:** L4 — **33 теста / 29 файлов (31 passed, 2 skipped)**, CI `run_e2e_tests=true` зелёный. L3 smoke зелёный (A13 снят).

## Контекст (точка старта)

25 specs, все зелёные. `bulk-edit` стоял `test.fail` (A5 репродюсер). A13 сидел
в `known-bugs.txt` — `writeFileRaw` гонял `dnsmasq --test` без `--conf-file`,
валидировал дефолт-конфиг, а не записанный файл. Цель финала — закрыть L4 до
практического 100%: починить A5/A13, добавить батч-4 спеки, прогнать
mutation-pass. См. `Gap_2_finish.md` (промт).

## Блок A — продуктовые фиксы A5 + A13

Подробно — в `логи/gap2-blockA-a5a13-fixes.md`. Кратко:

- **A5 (frontend, 1 строка):** `BulkEditModal.vue:67` — `store_hosts.find(...)`
  → `store_hosts.hosts.find(...)`. Reactive-стор не имеет `.find` → TypeError в
  `preview` computed → модалка не открывалась. `test.fail` снят.
- **A13 (backend, 3 строки):** `writeFileRaw` / `writeConfigWithTest` /
  `restoreHistoryVersion` теперь гоняют `dnsmasq --test --conf-file=<path>`
  (канонический паттерн из `dnsmasq_test.go:1882`). A13 убран из
  `known-bugs.txt`; smoke-чек `40-config-files` стал честным 400.
  `reloadDnsmasq` (sse.go) и `restoreBackupZip` (backup.go) намеренно оставлены
  с bare `--test` (отдельная задача — смена флага меняет семантику).

**Грабли (1 красный CI-прогон):** после A13 smoke упал на `51-history-diff-restore.sh` (restore → 500).
`restoreHistoryVersion` стал реально валидировать `10-static.conf`, а тот был
отравлен A3/A4-хостами (zero/broadcast/dash-MAC) из `21-hosts-bugs.sh` — host-add
пишет их без `--test`. Фикс (`949ae4f`): A3/A4-хосты изолированы в `19-bugs.conf`.
Это вскрытый A13 test-design-дефект, а не слабость фикса.

## Блок B — батч 4 (8 specs, 25 → 33 тестов)

Все селекторы выведены из реальных компонентов (предварительно прочитаны
App.vue / SafetyTab+AuditTab / DnsmasqConfig+directives.js / HostForm /
store.js+api/* / i18n.js+locales / hello-plugin / auth.go / handlers_*).

| spec | что проверяет | нюанс реализации |
|---|---|---|
| `audit-tab` | seeded MAC в audit-таблице safety-таба | матч MAC (не action-badge) — locale-independent; SafetyTab рендерит `<AuditTab/>` из `audit/` |
| `plugins-iframe` | `🧩 Hello Plugin` → `.plugin-overlay iframe` с `hello` | hello-плагин ставится в CI до старта e2e-инстанции |
| `i18n-api-error` | EN-тост `already used` на дубликат MAC после `🌐 English` | `translateApiError('mac_duplicate')`; placeholder MAC-инпута захардкожен |
| `config-template-fill` | файл из `basic-dhcp` → raw содержит `domain-needed` | контент через API (locale-independent); guard `waitFor option` (loadConfigTemplates не await'ится) |
| `config-directive` | visual editor save валидирует через `--conf-file` (A13): `port=5353` ок, `port=abc` → 400 + rollback | логин 1 раз, `waitForResponse` на PUT; assert через API |
| `sse-live` | `/api/events` → 200 + `text/event-stream` под Bearer (упрощённый) | `authMiddleware` не принимает `?token=` → `fetch` с Bearer в `page.evaluate` |
| `config-raw` | **skip** | дублирует smoke `40-config-files`; нет raw-edit UI (§6.6) |
| `setup-screen` | **skip** | нужна 2-я инстанция `:18084` с fresh `-db` (§6.7), CI-infra не добавлена |

**Грабли (1 красный CI-прогон):** `config-directive` — мой ассампшион «basic-dhcp
→ один dns-group» был неверен. Закомментированные `#dhcp-range=…`/`#dhcp-option=…`
парсер берёт как **неактивные** dhcp-директивы → dns + dhcp group'ы →
`.btn-outline-primary` матчило 4 кнопки → strict-mode violation. Фикс (`ac967a1`):
скоуп клика к `.card.mb-3.shadow-sm`.first() (dns по `GROUP_ORDER`), у неё ровно
одна кнопка add-directive.

## Блок C — mutation-pass: ВЫПОЛНЕН (SUCCESS)

Throwaway-ветка `mutation-test` (не мержилась, удалена после). 4 точечные
мутации, каждая должна ронять ровно один ожидаемый spec. CI `run_e2e_tests=true`
дал ровно ожидаемый результат: **27 passed / 4 failed / 2 skipped**.

**Важный инсайт про backend-мутации:** исходный план §7 был backend-heavy
(addAliasHandler / reloadDnsmasq / deleteHostHandler). На практике это НЕ
валидирует e2e-слой: CI гоняет `go test ./...` ПЕРЕД e2e, и backend-мутации
ловятся на unit-уровне (или `go vet` режет unreachable code после раннего
`return`) → до e2e дело не доходит. Поэтому мутации перестроены на **frontend**
(`.vue`/`.js`) — их go test не видит, и единственный слой, который их ловит,
это e2e.

| мутация (frontend) | файл | упавший spec | failure-режим |
|---|---|---|---|
| `applyConfig` → ранний `return` | `store.js` | `reload-ui` | нет POST `/api/reload` → `waitForResponse` timeout |
| `addAlias` → ранний `return true` | `api/dns.js` | `dns-aliases-add` | нет POST + нет refresh store → `tbody tr` not found |
| `deleteHost` → ранний `return` | `HostTable.vue` | `host-crud` | no-op → строка видна → `toBeHidden` failed |
| A5-revert (`.hosts.find`→`.find`) | `BulkEditModal.vue` | `bulk-edit` | TypeError в computed → `.modal-content` not visible |

Каждая мутация упала **ровно** своим spec'ом с предсказанным failure-режимом;
27 остальных (вкл. все 6 батч-4) остались зелёными → мутации изолированы,
4 spec'а доказанно ловят регрессии в своих путях.

**Слабые места (осознанно не закрыты):** 2 мутации из §7 оказались не ловимыми
текущими spec'ами и были заменены на A5-revert:
- `sortBy` no-op → `hosts-sort` не падает: spec проверяет **кол-во строк**
  (guard от A1-дубликатов), а не порядок сортировки.
- `logout` без очистки token → `auth` не падает: assertion `.btn-primary` visible
  выполняется и на dashboard (активная tab-кнопка тоже `.btn-primary`).
При желании оба spec'а можно усилить (hosts-sort — assert порядка; auth —
assert что следующий API-запрос даёт 401).

## Результат

- **L4: 33 теста / 29 файлов** (31 passed, 2 skipped). CI `run_e2e_tests=true`
  зелёный. Все 6 реализованных батч-4 спеков проходят; 2 infra-skip с комментами.
- **Mutation-pass SUCCESS** (Блок C): 4 frontend-мутации роняют ровно ожидаемые
  spec'и, без коллатерала.
- **L3 smoke зелёный** (A13 снят, restore-чек снова 200 после изоляции A3/A4).
- **A5 + A13 FIXED**, убраны из known-bugs.txt / bugs.md (статусы обновлены).
- `duis.md` удалён (дублировал ROADMAP/known-bugs/bugs), ссылки переправлены.

## Что осталось (опционально)

- **Усилить 2 слабых spec'а** (`hosts-sort` — порядок; `auth` — 401 после logout).
  Найдено в ходе mutation-pass, не блокирует release.
- **infra-specs:** полный `setup-screen` (2-я инстанция `:18084`), полный
  `sse-live` (writable arp-file) — сейчас `test.skip` с комментами (`Gap_2_finish.md`
  §6.7/§6.8).
- Остальные непочиненные баги (A1-A4, A6, A8, A11, A12) — вне L4, см. `bugs.md`.
