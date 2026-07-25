# Gap 2 — Playwright E2E, батч 3 фаза Б (UI coverage, +4 specs)

**Дата:** 2026-07-25
**Gap:** 2 (L4 — UI/E2E), продолжение `gap2-batch3-phaseA.md`
**Коммиты:** `26cb811` (4 specs)
**Результат:** CI `run_e2e_tests=true` — **20/20 passed** (с первого прогона, без фиксов)

## Контекст

Фаза А закрыла низкорисковые UI-потоки (16 specs). Фаза Б — средний риск:
DNS-вкладка, импорт (text/csv), reload. По плану
(`C:\Users\alexr\AppData\Local\Temp\opencode\l4-batch3-plan.md`).

## Что сделано (+4 → 20)

| spec | что проверяет | нюансы |
|---|---|---|
| `dns-aliases-add` | DNS-таб → A-запись → реактивный `<code>`directivePreview → строка в AliasTable | `addAlias` успех без alert; превью `address=/domain/target` — стабильный сигнал |
| `bulk-import-text` | text-режим → 3 строки → клиентский parsed=3 → Import → 3 строки | `saveBulkHosts` success → toast (не alert) |
| `csv-import` | csv-режим → upload in-memory CSV → Import → alert с count=3 → 3 строки | `setInputFiles({buffer})`; count через `expect.poll` на сообщении alert (A6-смежное — CSV-путь count корректен) |
| `reload-ui` | 🔄 Apply → `waitForResponse(/api/reload → 200)` | успех детектю по HTTP 200, alert просто accept (текст i18n); при `-init-system=none` reload = no-op |

**Ключевое решение по селекторам:** на DNS/static-табе в тулбаре (`App.vue`)
живёт ещё search-инпут (`.form-control`). Поэтому ВСЕ input-селекторы форм
скауплены к карточке `.card.p-3.shadow-sm` — иначе search перебивал бы
`nth(0)`. `importMode`-селект — `.first()` (в single-mode их два:
importMode + template).

## Результат

CI (Forgejo, `fedora:44`): **20/20 passed**, с первого прогона. Никаких
правок-фиксов не понадобилось — разведка (`api/dns.js`, `reloadDnsmasq`,
скаупинг к карточке) сработала.

## Что дальше

- **Фаза В (4):** rollback-ui (нужен `.bak`), history-modal (нужны версии +
  diff + restore-confirm), discovery-tab (arp-fixture), backup-restore-ui
  (самый сложный: dynamic file input → `page.on('filechooser')` + valid zip
  через API `/backup`).
- Потом батч 4 (6 нишевых + 2 жёстких).

## Замечание про confidence тестов

Фаза Б прошла зелёно с первого раза — это хорошо, но виден паттерн: из 20
спеков новые баги ловит только A5-репродюсер (известный). Это НЕ значит, что
тесты мёртвые — 3 спека ранее падали на реальных вещах (btn-warning
ambiguous, alert-race, и сам A5) и были починены, т.е. харнес реально
выполняет код и asserts реальные. НО для эмпирической уверенности имеет
смысл разовый **mutation-check**: намеренно сломать 3-4 строки продукта на
throwaway-ветке и прогнать CI — соответствующие спеки должны покраснеть.

## Локальная проверка

- `npx playwright test --list` — **20 tests in 17 files**.
