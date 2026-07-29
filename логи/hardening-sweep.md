# Сессия: Hardening sweep — закрытие остаточных gap'ов

**Дата:** 2026-07-29.
**Промт:** `логи/Hardening_sweep.md`.
**Цель:** закрыть 4 остаточные задачи харденинга (в порядке ROI):
T1 — fuzzing парсеров, T2 — A11 path-traversal hardening, T3 — усилить
2 слабых Playwright spec'а, T4 — разблокировать 2 infra-spec'а.

> **Статус сессии:** выполняется поэтапно. Закрыто: **T2 (A11)**, **T1
> (fuzzing парсеров)**. T3/T4 — предстоят (см. «Что НЕ сделано»).

## Контекст на старте

- L1+L2 Go: 65.6% coverage (`go test -cover ./...`), 241+ тестов, `-race` чист.
- L3 smoke.sh: 0 Fail / 0 Known-fail; в `tests/known-bugs.txt` остался
  только **A11** (path traversal, wontfix/hardening).
- L4 Playwright: 33 spec'а (31 pass + 2 skip).
- `tests/bugreport/bugs.md`: A11 = PARTIAL.

Локальная среда — Windows: проверки идут **только в CI** (Fedora 44,
Forgejo Actions); локально допускается лишь `gofmt -l` (синтаксис/формат).

---

## Что сделано — по-задачно

### T2 — A11 path-traversal hardening (LOW → FIXED)

- **Коммит:** `fb1ce14` (`Fix A11: add isSafePath defense-in-depth to file handlers`).
- **Файлы:** `handlers_config.go` (`getFileHandler:197`, `putFileHandler:221`).
- **Фикс:** после `filepath.Join(*ConfigDir, name)` в обоих хендлерах добавлен
  вызов `isSafePath(path)` → тот же `403 access_denied`, что и существующий
  substring-фильтр (`/`/`\`) + `.conf`-extension-чек. Поведение эндпоинтов
  **не изменилось** → все 9 векторов в `tests/suites/81-path-traversal.sh`
  остаются с теми же статусами.
- **Слоистость:** теперь triple-check — substring-фильтр → `isSafePath`
  после Join → `isSafePath` внутри `readFileRaw`/`writeFileRaw`
  (`dnsmasq.go:60,70`). Оба хендлера повторяют единый chokepoint-паттерн
  остальных ~22 call site'ов.
- **Regression:** `TestGetFileHandlerRejectsUnsafePath` (`dnsmasq_test.go`),
  `TestPutFileHandlerRejectsUnsafePath` (`handlers_test.go`) — table-driven
  по 3 векторам с `.conf`-расширением (`../etc/evil.conf`, `..\evil.conf`,
  `../../etc/dnsmasq.conf`), assert `403` + `access_denied`. PUT-кейс
  безопасен на Windows: substring срабатывает до `writeFileRaw` → `dnsmasq
  --test` не запускается.
- **Документы:** A11 → FIXED в `tests/bugreport/bugs.md` (таблица, блок
  «Итого», детальная секция с rationale); A11 удалён из
  `tests/known-bugs.txt` (файл пуст, оставлен заголовочный комментарий);
  `tests/ROADMAP.md` — добавлена строка Hardening sweep, поправлена P0✓
  (больше не «оставлен только A11»), отмечен чекмет «known-bugs.txt пуст».

####Nuance (зафиксирован в комментах и bugs.md)

Substring-фильтр на `/`/`\` — **строгое надмножество**: для любого
traversal-входа, достижимого через URL, он срабатывает первым, а `Join`
без сепараторов не может вылезти за `ConfigDir`. Поэтому
`isSafePath`-после-Join для текущего хендлера — **недостижим для reject'а**
и является чистым defense-in-depth на случай будущего ослабления
substring-фильтра или нового call site'а. Изолированно проверить новую
строку через хендлер нельзя (надмножество всегда стреляет первым), поэтому
regression-тесты лочат **сам контракт 403** — это явно разрешено промтом
(«если возможно придумать Join-escape — иначе ограничиться substring
cases»). `isSafePath` сам по себе покрыт отдельным `TestIsSafePath`
(`dnsmasq_test.go`).

---

### T1 — Fuzzing парсеров (P2 → ГОТОВО)

- **Коммит:** `981e88f` (`T1: fuzz the four text parsers (parseLeases refactor + FuzzXxx)`).
- **T1.1 (единственная продуктовая правка):** в `arp_leases.go` тело
  `parseLeases` вынесено в чистую `parseLeasesContent(content string)
  []LeaseEntry` (без I/O — `bufio.NewScanner(strings.NewReader(content))`);
  `parseLeases()` читает файл через `os.ReadFile` и делегирует. Поведение
  идентично → `TestGetNewDevices*` и leases-тесты в `handlers_test.go`
  остались зелёными.
- **T1.2:** новый `fuzz_test.go` с 4 native `FuzzXxx`:
  - `FuzzParseDhcpHostLine(raw, file)` — MAC остаётся macRegex-валидным,
    `File` пропагируется, `formatDhcpHostLine` round-trip'ит (re-parse →
    тот же MAC);
  - `FuzzParseArpContent(content)` — ключи возвращённой map непустые и
    lowercase;
  - `FuzzParseAliasLine(line, file, hasBak)` — `aliasToLine` round-trip'ит
    (Type/Domain/Target совпадают), `File` с учётом `|has_bak`;
  - `FuzzParseLeasesContent(content)` — каждый lease имеет непустые `Ip`/`Mac`.
  Oracle = «не паникует + структурные инварианты на успех» (промт §1.5);
  эталонный вывод не строится (парсеры принимают мусор как легитимный ввод).
- **T1.3 (seed corpus):** реализовано через `f.Add(...)` (см. решение ниже),
  без файлов в `testdata/`. В дефолтном `go test` seed'ы работают как
  subtest'ы — ~40 бесплатных edge-case'ов.
- **T1.4 (opt-in `-fuzz` шаг в CI):** сознательно отложен — опционален по
  промту. Без него реальный `-fuzz` в пайплайне не гоняется, только seed'ы
  в unit-режиме.

#### Решение: `f.Add` вместо `testdata/corpus/`

Промт предписывал `testdata/corpus/<FuzzName>/`, но **Go's fuzz engine
автозагружает `testdata/fuzz/<Name>/`, а не `corpus/`** — файлы в
`corpus/` были бы инертны. При этом `f.Add`-seed'ы и так исполняются как
subtest'ы в дефолтном `go test`, и компилируются (type-checked), что даёт
**нулевой риск** malformed-корпуса, который мог бы повернуть дефолтный CI
в красный. С учётом «проверка только в CI» это ответственный выбор.
Эдж-кейсы (NUL, unicode, длинные строки, repeat-patterns) включены в
`f.Add`; все seed-инварианты предварительно проверены вручную
(reject-path'и, двойные MAC, TXT с quoted value, CNAME с лишним полем —
round-trip держится).

---

## Приёмка (DoD) — по промту

- [x] **T2:** `getFileHandler` и `putFileHandler` вызывают `isSafePath`
      после `filepath.Join`. Smoke `81-path-traversal.sh` — те же 9
      статусов. A11 → FIXED в `bugs.md`, удалён из `known-bugs.txt`.
- [x] **T1:** 4 `FuzzXxx` после рефакторинга `parseLeases` →
      `parseLeasesContent`; seed corpus через `f.Add` (обоснование ниже).
      Opt-in `-fuzz` CI-шаг отложен (опционален).
- [ ] **T3:** `hosts-sort.spec.ts` (assert порядка), `auth.spec.ts`
      (assert 401 + `token===null`). — *предстоит*
- [ ] **T4:** `setup-screen.spec.ts` / `sse-live.spec.ts` — реальные тесты
      + 2-я инстанция :18084 + writable ARP_FILE в CI yml. — *предстоит*
- [x] `tests/ROADMAP.md`: A11 отмечен закрытым.
- [x] Этот файл (контекст → по-задачно → верификация → результат).

`go vet ./...` / `go test ./... -race` / smoke — подтверждаются в CI
(см. ниже); локально по решению оператора гоняется только `gofmt -l`.

---

## Подтверждение CI

После пуша T2 (`8181140..fb1ce14`) и T1 (`1206476..981e88f`) дефолтные
прогоны CI (Forgejo Actions, Fedora 44) — **зелёные** (подтверждено
оператором).

- **T2 — go vet / go test -race:** чисто/зелёный; regression-тесты
  (`TestGetFileHandlerRejectsUnsafePath`, `TestPutFileHandlerRejectsUnsafePath`)
  прошли.
- **T2 — smoke.sh:** 0 Fail / 0 Known-fail (A11 снят с known-bugs; 9 векторов
  `81-path-traversal.sh` сохранили статусы — поведение хендлеров не менялось).
- **T1 — go vet / go test -race:** чисто/зелёный; рефакторинг `parseLeases`
  поведение сохранил (`TestGetNewDevices*`, leases-тесты — зелёные); 4
  `FuzzXxx` прогнали seed corpus как unit-subtest'ы без падений инвариантов.
- **Локально (Windows):** `gofmt -l arp_leases.go fuzz_test.go` — пусто
  (формат корректен). `go vet`/`go test`/smoke локально не запускались по
  решению оператора — только CI.

---

## Файлы изменены/добавлены

### Изменённые (Go)
- `handlers_config.go` — `isSafePath(path)` после `filepath.Join` в
  `getFileHandler` и `putFileHandler` (+ поясняющие комменты).
- `arp_leases.go` — вынесена чистая `parseLeasesContent`; `parseLeases`
  делегирует (T1.1).

### Добавленные (tests)
- `fuzz_test.go` — 4 `FuzzXxx` (T1.2) + `f.Add` seed corpus (T1.3).

### Изменённые (tests)
- `dnsmasq_test.go` — `TestGetFileHandlerRejectsUnsafePath`.
- `handlers_test.go` — `TestPutFileHandlerRejectsUnsafePath`.
- `tests/known-bugs.txt` — A11 удалён (файл пуст).
- `tests/bugreport/bugs.md` — A11 FIXED (таблица, «Итого», детальная секция).
- `tests/ROADMAP.md` — Hardening sweep строка + правка P0✓ + чекмет known-bugs.

---

## Что НЕ сделано (сознательный descoping / предстоящее)

- **T1.4 (opt-in `-fuzz` CI-шаг)** — отложен: опционален по промту; без
  него реальный fuzz-режим в пайплайне не гоняется (только seed'ы в
  unit-режиме). Добавить можно по образцу `run_e2e_tests` в
  `.forgejo/workflows/build.yml`.
- **T3 (усиление 2 Playwright spec'ов)** — предстоящая задача сессии;
  локально на Windows не запускается, только CI L4.
- **T4 (2 infra-spec'а)** — предстоит; требует правок
  `.forgejo/workflows/build.yml` (2-я инстанция :18084 + writable ARP_FILE).
- **Go coverage → 70%/90%** — вне этой сессии: T1 дал ~+2-3% (точная
  дельта — по CI-отчёту coverage); остаток — edge-case тесты на `bins.go`
  (исходник менять не нужно, только `bins_test.go`), `reloadDnsmasq`,
  `startDNSHealthChecker`.
- **Gap 4 (L5 Real VM nightly)**, **A10** (feature gap) — отдельные задачи.
- Любой рефакторинг продуктовых исходников сверх описанного — запрещён
  промтом (§1.3).

---

## Решения, зафиксированные до/в ходе старта

| Развилка | Выбор | Обоснование |
|---|---|---|
| Локальная верификация | **только `gofmt -l`** | по решению оператора: все проверки (`go vet`/`go test`/smoke) — в CI. |
| A11: менять статус эндпоинтов | **нет** | промт §1.3: «A11 НЕ меняет публичное поведение»; тот же `403 access_denied`. |
| Тестировать новую строку изолированно | **нельзя** | substring-фильтр — надмножество; лочим контракт 403 (разрешено промтом). |
| Порядок исполнения | **T2 первым** | рекомендация промта §5: самый дешёвый (~15 мин), снимает wontfix. |
| Push | **сразу после T2** | по решению оператора: коммит+пуш, ожидание CI. |
| T1: seed corpus — `testdata/corpus/` vs `f.Add` | **`f.Add`** | Go автозагружает `testdata/fuzz/`, не `corpus/`; `f.Add` compile-checked и работает в дефолтном `go test` — нулевой риск для CI. |
| T1: opt-in `-fuzz` CI-шаг (T1.4) | **отложить** | опционален по промту; отложен до явного запроса. |
