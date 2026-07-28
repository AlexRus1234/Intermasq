# Сессия: Bugfix sweep — закрытие известных багов

**Дата:** 2026-07-28.
**Промт:** `логи/Bugfix_sweep.md`.
**Цель:** закрыть открытые баги с regression-тестами, убрать их из
`tests/known-bugs.txt`, довести smoke до 0 Fail / 0 Known-fail.

## Контекст на старте

- L1+L2 Go: 65.6% coverage.
- L3 smoke.sh: ~138 проверок, 7 known-fail (A2, A3, A4, A6, A8, A11, A12).
- `tests/known-bugs.txt`: A2, A3, A4, A6, A8, A11, A12 (A5 + A13 уже FIXED
  в Блоке A, см. `логи/gap2-blockA-a5a13-fixes.md`).

Решения зафиксированы до старта (см. конец файла): A4 → нормализация;
A1 → минимальный `:key`-фикс; A2 → сигнатуру `findAliasesByDomain` не
трогать; A11 → skip; пуш — в конце.

---

## Что сделано — по-бажно

Порядок исполнения: A12 → A8 → A6 → (A3 + A4) → A2 → A1. Каждый шаг =
отдельный коммит после зелёного `go vet` + `go test`.

### A12 — aliasDomainRegex отвергал `_` (CRITICAL→FIXED)

- **Коммит:** `5faa36d`.
- **Файл:** `main.go:80`.
- **Фикс:** регекс ослаблен до
  `^[a-zA-Z0-9_]([a-zA-Z0-9-._]*[a-zA-Z0-9_])?$`. Теперь принимает
  `_dmarc.local`, `_sip._tcp`, `default._domainkey`, `_acme-challenge`.
- **Regression:** `TestAliasDomainRegexUnderscore` (`dnsmasq_test.go`) —
  accept/reject lists; smoke `A12: Add TXT with underscore domain → 200`
  (`30-aliases-happy.sh`).
- **Заметка:** сознательно НЕ добавлял per-type валидацию (A/CNAME строже,
  TXT/PTR свободнее) — вне объёма фикса. Регекс по-прежнему принимает
  `double..dot` (как и старый) — это не регрессия.

### A8 — `/metrics` 401 без body (MEDIUM→FIXED)

- **Коммит:** `a76894d`.
- **Файл:** `metrics.go:62`.
- **Фикс:** `AbortWithStatus(401)` → `AbortWithStatusJSON(401, gin.H{"error":"auth_required"})`.
- **Regression:** augmented `TestMetricsHandler_NoAuth_401`
  (`handlers_test.go`) — assert non-empty body + содержит `auth_required`;
  smoke `A8: 401 has body` (`80-metrics.sh`, body >2 байт).

### A6 — bulk JSON без `count` (MEDIUM→FIXED)

- **Коммит:** `3ceb279`.
- **Файл:** `handlers_hosts.go:269` (`bulkAddHostsHandler`).
- **Фикс:** `c.JSON(200, gin.H{"status":"ok", "count": len(req.Hosts)})`.
- **Regression:** augmented `TestBulkAddHostsHandler_Success` — парсит JSON
  и assert `.count == N`; smoke `Bulk JSON response has count field`
  (`24-hosts-bulk.sh`).

### A3 + A4 — MAC validation (HIGH→FIXED, связанные)

- **Коммит:** `dc0c346`.
- **Файлы:** `dnsmasq.go` (`validateHostFields`, `parseCSVHosts`),
  `handlers_hosts.go` (`addHostHandler`, `bulkAddHostsHandler`),
  `tests/suites/21-hosts-bugs.sh`.
- **A3 фикc:** в `validateHostFields` добавлен чёрный список
  zero/broadcast MAC через `strings.EqualFold` (после нормализации —
  ловит и `00-00-...`).
- **A4 фикс:** новый хелпер `normalizeMAC(s) string` (`-`→`:`), вызывается
  - на входе `addHostHandler` (`req.Mac = normalizeMAC(req.Mac)` до
    validate),
  - pre-pass в `bulkAddHostsHandler` (нормализует все `req.Hosts[i].Mac`
    до in-batch cross-check),
  - в `parseCSVHosts` (после TrimSpace, до append в HostEntry),
  - дефенсивно внутри `validateHostFields`.
  Записанный dhcp-host всегда в canonical colon-форме.
- **Regression:** `TestNormalizeMAC`, `TestAddHostHandlerRejectsZeroBroadcastMAC`,
  `TestAddHostHandlerDashMACNormalized`, `TestParseCSVHostsNormalizesDashMAC`
  + table-cases в `TestValidateHostFieldsAllCombinations`.
- **Smoke:** `21-hosts-bugs.sh` A4-чек переписан — раньше ждал 400 (reject
  OR normalize), теперь ждёт 200 + `grep` что в файле `aa:bb:cc:dd:ee:07`
  (colon) и НЕ `aa-bb-...` (dash). A3-чеки ждали 400 → зелёные.
- **Заметка:** `bulkLeaseToStaticHandler` НЕ тронут — leases всегда
  colon-форма (вывод dnsmasq). bulkMove/bulkEdit оперируют уже хранящимися
  MAC; если в файле есть legacy dash-MAC, он переедет как есть — это
  намеренно (фикс про новые записи, не про миграцию данных).

### A2 — дубликаты DNS-alias (CRITICAL→FIXED)

- **Коммит:** `235946e`.
- **Файл:** `handlers_aliases.go:76` (`addAliasHandler`).
- **Фикс:** `findAliasesByDomain(req.Domain, req.Type, req.File)` →
  `findAliasesByDomain(req.Domain, "", "")`. Сигнатуру функции НЕ менял
  (по решению) — `excludeType/excludeFile` формально мёртвы, но это
  cosmetic tech debt, меньше blast radius.
- **Knock-on:** smoke `Delete again → 404` (`32-aliases-delete.sh`, tagged
  A2) теперь честно возвращает 404 (дубля нет → second delete мазит).
- **Regression:** `TestAddAliasHandlerDuplicateRejected` (предзаписать A,
  второй POST → 409 + ровно 1 строка), `TestDeleteAliasHandlerSecondDeleteNotFound`
  (200 → 404). Проверил также что `bulkAddAliasesHandler:139` и
  `importAliasesCSVHandler:287` уже корректно зовут без exclude.

### A1 — дубли строк таблицы при сортировке (CRITICAL→FIXED)

- **Коммит:** `caf1144`.
- **Файл:** `frontend/src/components/static/HostTable.vue:27`.
- **Фикс:** `:key="h.mac"` → `:key="h.mac + '|' + (h.file||'')"`. Ключ
  уникален: `h.file` различается между `.conf`-файлами, суффикс `|has_bak`
  различает bak/non-bak варианты одного MAC.
- **Regression:** существующий Playwright `hosts-sort.spec.ts` (guard —
  count строк стабилен при сортировке) остаётся зелёным. Усиление (assert
  порядка) сознательно не сделано — отдельная опциональная задача (см.
  `tests/ROADMAP.md` «Что осталось»).
- **Опциональный `has_bak`-рефакторинг** (отдельное поле `HasBak bool` в
  `HostEntry` вместо суффикса в File) сознательно НЕ сделан — вне объёма
  фикса (по решению).
- **Сборка:** `cd frontend && npm run build` — OK, 121 модуль, 381 КБ JS.

### A11 — skip (опциональный hardening)

Не сделан. Большинство path-traversal векторов закрыто через `isSafePath`,
остаток — Go `net/http` path cleaning (defence in depth). На known-fail не
влияет. Оставлен в `tests/known-bugs.txt` с пометкой.

---

## Приёмка (DoD)

- [x] A1, A2, A3, A4, A6, A8, A12 пофикшены (A11 — опциональный hardening,
      оставлен wontfix).
- [x] `tests/known-bugs.txt`: остался только A11 (с комментарием-пометкой).
- [x] `tests/bugreport/bugs.md`: 7 статусов FIXED + коммиты, сводка и
      «Итого» обновлены.
- [x] smoke.sh: **0 Fail / 0 Known-fail** — подтверждено CI (см. ниже).
- [x] `go vet ./...` чисто; `go test ./... -race -count=1` зелёный.
- [x] `tests/ROADMAP.md` P0 отмечен закрытым.
- [x] Этот файл.

---

## Подтверждение CI

После пуша (`fbe9d4e..13b2b41 main -> main`) CI (Forgejo Actions, Fedora 44
контейнер) прогнал полный smoke.sh и go test. **Результат: всё зелёное.**

- **smoke.sh:** 0 Fail, 0 Known-fail (раньше было 7 known-fail). 7 багов
  сняты с known-bugs → соответствующие check'и стали честно зелёными:
  - A2 → `30-aliases-happy.sh` / `31-aliases-bugs.sh` (409 + count=1) и
    knock-on `32-aliases-delete.sh` (Delete again → 404).
  - A3 → `21-hosts-bugs.sh` (zero/broadcast MAC → 400).
  - A4 → `21-hosts-bugs.sh` (dash-MAC → 200 + colon-form в файле).
  - A6 → `24-hosts-bulk.sh` (`count` поле = 2).
  - A8 → `80-metrics.sh` (401 body >2 байт).
  - A12 → `30-aliases-happy.sh` (TXT `_dmarc.local` → 200).
- **go test:** зелёный, новые regression-тесты прошли (включая
  `-race`-прогон с конкурентной записью users.json).
- **gofmt / go vet:** чисто.

Локальная верификация (Windows) совпала с CI:
```
go vet ./...                    # чисто
go test ./... -race -count=1    # ok intermask 80.680s
cd frontend; npm run build      # 121 модуль, built in 3.06s
```

Цель сессии (DoD) достигнута полностью.

---

## Файлы изменены/добавлены

### Изменённые (Go)
- `main.go` — A12 regex.
- `metrics.go` — A8 body.
- `handlers_hosts.go` — A6 count, A4 normalizeMAC pre-pass.
- `dnsmasq.go` — A3 blacklist + A4 normalizeMAC helper + parseCSVHosts.
- `handlers_aliases.go` — A2 убран exclude.

### Изменённые (frontend)
- `frontend/src/components/static/HostTable.vue` — A1 `:key`.

### Изменённые (tests)
- `dnsmasq_test.go` — regression-тесты A2, A3, A4, A12.
- `handlers_test.go` — regression-тесты A6, A8 (+ `encoding/json` import).
- `tests/suites/21-hosts-bugs.sh` — A4 чек переписан под нормализацию.
- `tests/known-bugs.txt` — сняты A2, A3, A4, A6, A8, A12.
- `tests/bugreport/bugs.md` — 7 статусов FIXED, сводка, Итого, приоритеты.
- `tests/ROADMAP.md` — P0 отмечен закрытым.

---

## Что НЕ сделано (сознательный descoping)

- **A11 hardening** — опциональный (см. выше).
- **`has_bak`-рефакторинг** (A1) — вне объёма фикса.
- **Per-type alias regex** (A12) — A/CNAME строже, TXT/PTR свободнее;
  текущий универсальный регекс достаточно строг для UI.
- **bulkMove/bulkEdit нормализация legacy dash-MAC** — миграция данных,
  не предотвранение новых.
- **Усиление `hosts-sort.spec.ts`** (assert порядка) — отдельная опциональная
  задача из ROADMAP.
- **A7** (не баг), **A10** (feature gap, отдельный PR) — вне сессии.

---

## Решения, зафиксированные до старта

| Развилка | Выбор | Обоснование |
|---|---|---|
| A4: normalize vs reject | **normalize** | UX-friendly (частый paste из Windows `getmac`); smoke-чек переписан под 200 + colon-form verify. |
| A1: `:key` only vs `has_bak` refactor | **`:key` only** | «строго объём фикса»; `:key` с `h.file` уже уникален из-за `|has_bak` суффикса. |
| A2: убрать exclude-параметры vs оставить мёртвыми | **оставить** | меньше blast radius, не трогаю контракт функции и docs/testing-v1.md. |
| A11: делать vs skip | **skip** | промт явно говорит «можно пропустить»; known-fail нет. |
| Push | **в конце** | после всех фиксов и зелёной верификации. |
