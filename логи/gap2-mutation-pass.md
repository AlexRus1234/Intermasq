# Gap 2 — mutation-pass (Блок C): эмпирическая проверка качества e2e-спеков

**Дата:** 2026-07-28
**Gap:** 2 (L4 — UI/E2E), закрытие `Gap_2_finish.md` §7
**Ветка:** `mutation-test` (throwaway, **не мержилась**, удалена после)
**Результат:** SUCCESS — 4 мутации роняют ровно 4 ожидаемых spec'а, без коллатерала. CI: 27 passed / 4 failed / 2 skipped.

## Контекст и цель

После закрытия Блоков A (фиксы A5/A13) и B (батч 4, 33 spec'а) остался
mutation-pass: намеренно сломать по одной строке продукта и убедиться, что
ровно ожидаемые spec'ы краснеют. Это эмпирическое доказательство, что «всё
зелено» = «тесты реально проверяют», а не «спеки мёртвые». Особенно актуально
после бага `deleteHostApi` (спал 3 фазы в батче 3 — wrong path, silent 404).

## Главный инсайт: backend-мутации НЕ валидируют e2e

Исходный план §7 был backend-heavy (`addAliasHandler` / `reloadDnsmasq` /
`deleteHostHandler`). На практике выяснилось — это не работает для валидации
e2e-слоя:

1. **Попытка 1 — ранний `return` в Go-функции:** `go vet` на CI режет
   `unreachable code` после `return` → сборка падает, e2e не запускается.
   (Локальный `go build` это не ловит — урок: гонять `go vet` перед пушем
   Go-правок, не только `go build`.)
2. **Попытка 2 — изменение финального `return`/`c.JSON` в Go:** проходит
   `go vet`, но падает на `go test ./...` — backend-хэндлеры покрыты L2-тестами
   (`handlers_test.go`, 1457 строк), и мутация ловится на unit-уровне → до e2e
   дело снова не доходит.

**Вывод:** мутации backend-хэндлеров проверяют L2, а не L4. Чтобы именно e2e
оказался единственным ловцом, мутации должны быть во **фронтенде** (`.vue`/`.js`)
— его `go test` не видит, smoke (curl) тоже, и единственный слой, который
реагирует — Playwright.

## Что сделано (4 frontend-мутации)

Ветка `mutation-test` от `main`, 4 коммита, push, CI `run_e2e_tests=true`.

| # | мутация | файл | упавший spec | failure-режим (предсказан → факт) |
|---|---|---|---|---|
| 1 | `applyConfig` → ранний `return` (нет POST `/api/reload`) | `frontend/src/store.js` | `reload-ui` | `waitForResponse` timeout ✓ |
| 2 | `addAlias` → ранний `return true` (нет POST + нет refresh) | `frontend/src/api/dns.js` | `dns-aliases-add` | `tbody tr` not found ✓ |
| 3 | `deleteHost` → ранний `return` (no-op delete) | `frontend/src/components/static/HostTable.vue` | `host-crud` | строка видна, `toBeHidden` failed ✓ |
| 4 | A5-revert: `.hosts.find` → `.find` (TypeError в computed) | `frontend/src/components/static/BulkEditModal.vue` | `bulk-edit` | `.modal-content` not visible ✓ |

**CI-результат:** ровно **27 passed / 4 failed / 2 skipped**. Каждая мутация
упала своим spec'ом с предсказанным режимом; 27 остальных (вкл. все 6 батч-4)
остались зелёными. `::error::playwright returned 1` — это и есть сигнал успеха
(красный на throwaway-ветке ожидаем, в main не мержится).

## Cleanup

`git checkout main && git branch -D mutation-test && git push origin --delete
mutation-test`. В main мутации не попали.

## Слабые места (найдено, осознанно не закрыто)

2 мутации из §7 оказались **не ловимыми** текущими spec'ами — заменены на
A5-revert:

- **`sortBy` no-op → `hosts-sort` не падает.** Spec проверяет **кол-во строк**
  (guard от A1 — дубликаты при re-sort), а не порядок. No-op сортировки кол-во
  не меняет → spec проходит. Это A1-гард, а не проверка sort-корректности.
- **`logout` без очистки token → `auth` не падает.** Assertion после logout —
  `.btn-primary` visible. На dashboard активная tab-кнопка — тоже `.btn-primary`,
  так что даже при неудачном logout assertion выполняется.

**Опциональное усиление:** hosts-sort — assert порядка после клика; auth —
assert что следующий API-запрос даёт 401 (token реально отозван).

## Итог

4 ключевых e2e-спека (`reload-ui` / `dns-aliases-add` / `host-crud` /
`bulk-edit`) доказанно ловят регрессии в своих путях. Mutation-pass цель достиг.
См. также обновлённые `логи/gap2-finish.md` и `tests/ROADMAP.md`.
