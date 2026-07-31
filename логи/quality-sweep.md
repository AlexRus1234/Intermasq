# Quality sweep — session-лог по этапам

Сборник результатов по этапам из `логи/Quality_sweep.md`. Порядок исполнения:
4 → 1 → 2 → 3 → ВМ. Каждый этап — отдельный раздел ниже.

---

## Этап 4 — Go mutation-testing (ВЫПОЛНЕН 2026-07-31)

**Подход:** ручные point-мутации (без `go-mutesting`) на throwaway-ветке
`mutation-go` (локально, не в main, не пушится). База: `main` @ `fb5b4fb`.
Локально Go 1.26.3 (Windows); мутации M1–M11 — чистая кросс-платформенная
логика (парсеры/валидация/HTTP/rate-limit), дельта CI/Windows
(`linux_test.go` для `system.go`) в матрицу не входит → результаты
идентичны Linux для данных сайтов.

**Pre-flight:** `$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXXXX"`
(обязательно — иначе `main.go:107 init()` делает `os.Exit(1)` и все
мутанты выглядят «killed»). `go vet ./...` чист, база `go test ./...`
зелёная (12.5s). Процедура на мутацию: 1 правка → `go vet` → `go test` →
запись killed/survived → `git checkout -- <файл>`.

### Матрица «мутация | expected spec | killed/survived»

| # | Сайт (file:line) | Мутация | Предсказание | Факт | Какой тест убил |
|---|------------------|---------|--------------|------|-----------------|
| M1 | `dnsmasq.go:132` | `macRegex.MatchString(p)` → `!...` | KILLED | **KILLED** | `TestParseDhcpHostLine_TrailingNewline` + 7 коллатерал (parser ядро: getNewDevices/bulkMove/bulkEdit/IPDup + fuzz) |
| M2 | `dnsmasq.go:142-144` | убрать `if entry.Mac=="" {return false}` | SURVIVED | **KILLED** ⚠ | `FuzzParseDhcpHostLine` seed-корпус (`dhcp-host=` → ok=true → oracle `macRegex` падает на пустом MAC, `fuzz_test.go:65`). Предсказание ошиблось: fuzz-seeds гоняются как unit-тесты под `go test`. R1 не нужен. |
| M3 | `dnsmasq.go:383` | `ReplaceAll(mac,"-",":")` → `ReplaceAll(mac,":","-")` | KILLED | **KILLED** | `TestNormalizeMAC` + 10 коллатерал (central helper) |
| M4 | `dnsmasq.go:398` | убрать zero-MAC `EqualFold` | KILLED | **KILLED** | `TestValidateHostFieldsAllCombinations` + `TestAddHostHandlerRejectsZeroBroadcastMAC` (чисто 2) |
| M5 | `dnsmasq.go:399` | убрать broadcast-MAC `EqualFold` | KILLED | **KILLED** | те же 2 теста (чисто) |
| M6 | `aliases.go:75` | `Domain=="" \|\| Target==""` → `Domain==""` | KILLED | **KILLED** | `TestParseAliasLineRejectsMalformed` (чисто 1) |
| M7 | `aliases.go:78` | убрать wildcard `#`/`*` reject (return → no-op) | KILLED | **KILLED** | `TestParseAliasLineRejectsWildcard` (чисто 1) |
| M-eq | `aliases.go:69` | `slash < 0` → `slash <= 0` | EQUIVALENT | **SURVIVED (эквивалентный)** | `address=//IP` (slash==0) reject'ится на line 69 вместо line 75, но `ok=false` в обоих случаях → наблюдаемое поведение идентично, различить нельзя. Не слабость теста. |
| M8 | `handlers_hosts.go:61` | убрать `isSafePath(req.File)` guard | SURVIVED | **SURVIVED → R2** | gap подтверждён. До правки — 0 падений. |
| M9 | `handlers_hosts.go:85-90` | убрать MAC-duplicate 409 block | SURVIVED | **SURVIVED → R3** | gap подтверждён. `TestConcurrentAddHost_SameMAC` толерантен 200/409, поэтому не ловил. До правки — 0 падений. |
| M10 | `handlers_aliases.go:76` | `findAliasesByDomain(req.Domain,"","")` → `(req.Domain, req.Type, req.File)` (self-exclude) | KILLED | **KILLED** | `TestAddAliasHandlerDuplicateRejected` (чисто 1) |
| M11 | `auth.go:128` | `len(recent) > maxAttempts` → `> maxAttempts+1000` | KILLED | **KILLED** | `TestRateLimitOverLimit` + `TestRateLimitDifferentIPs` (чисто 2) |

### Сводка метрик

- **Всего мутаций:** 12 (M1–M11 + M-eq).
- **KILLED:** 9 (M1, M2, M3, M4, M5, M6, M7, M10, M11).
- **SURVIVED:** 3, из них:
  - **1 эквивалентный** (M-eq) — нетестируем по определению.
  - **2 реальных gap'а** (M8, M9) → закрыты regression-тестами R2/R3.
- **Mutation score:** 9/11 не-эквивалентных = **81.8%** (до добавления R2/R3).
  После R2/R3 формально 11/11 не-эквивалентных покрыты (M8/M9 теперь killed
  regression-тестами). M-eq остаётся эквивалентным.
- **Предсказание «SURVIVED» точность:** 1/3 (M8, M9 — верно; M2 — ошиблось,
  fuzz-seed покрыл). Это полезная находка: fuzz-target'ы с seed-корпусом
  дают детерминированное regression-покрытие даже без `-fuzz` (релевантно
  для этапа 1).

### Artefact: survived-мутации + добавленные regression-тесты

Закоммичены в `main` (commit `f8fd404`, запушен в `origin/main`):

| ID | Тест | Файл | Убивает | Что проверяет |
|----|------|------|---------|---------------|
| R2 | `TestAddHostHandlerRejectsUnsafeFile` | `dnsmasq_test.go` | M8 | `isSafePath` guard в `addHostHandler`: unsafe path (`/etc/passwd` + `..`-traversal) → 400 `invalid_data` ДО валидации полей. 2 кейса (table-driven). |
| R3 | `TestAddHostHandlerMACDuplicateRejected` | `dnsmasq_test.go` | M9 | MAC-duplicate 409 `mac_duplicate` + существующая запись не перезаписывается. IP опущен, чтобы IP-dup-ветка не маскировала MAC-чек. |

Оба теста **верифицированы** на ветке `mutation-go`: на немутированной базе
PASS, при повторном применении целевой мутации — FAIL (точно падает
целевой тест, без коллатерала).

### Финальная верификация

- `mutation-go` ветка: `go vet ./...` чист + `go test "./..." -count=1`
  зелёный (10.5s) с R2/R3 в комплекте.
- `main` (`f8fd404`, после пуша): `go vet` чист + `go test` зелёный.
  Дефолтный CI (Forgejo, fedora:44) прогонит пуш — это подтверждение на
  Linux, что regression-тесты зелёные и там (расхождений не ожидается:
  тесты без build-tag, ОС-нейтральны).

### Замечания / knock-on

- **Ветка `mutation-go` оставлена локально** как audit-артефакт (по решению
  оператора; не пушится). Удалить: `git branch -D mutation-go`.
- **Продуктовый код не тронут** — все 12 мутаций откачены; в `main`
  изменён только `dnsmasq_test.go` (+57 строк, R2/R3).
- **Находка для этапа 1 (fuzz):** fuzz-seed-корпус уже сейчас ловит мутации
  (M2) как unit-тесты — это усиливает аргумент за реальный `-fuzz` прогон
  (этап 1) для поиска crash'ей, которые statement-coverage не видит.
- M-eq (`slash<0` vs `slash<=0`) — пример «equivalent mutant»: инструмент
  mutation-testing пометил бы его survived и это был бы false-positive;
  ручной подход позволил классифицировать его корректно.

---
