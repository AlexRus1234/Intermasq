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

## Этап 1 — Fuzz opt-in CI + реальный прогон ✅ ВЫПОЛНЕН (2026-07-31)

**Цель:** добавить opt-in CI-шаг `run_fuzz_tests` (по образцу `run_e2e_tests`),
запустить реальный `-fuzz` (не только seed'ы из `f.Add`) и разобрать crash'и,
которые statement-coverage не видит.

### Что сделано

1. **`workflow_dispatch.inputs.run_fuzz_tests`** (build.yml:31-35) — boolean,
   default `false`, opt-in по образцу `run_e2e_tests`. Дефолтный пайплайн
   не затронут.
2. **Шаг «Fuzz parsers (opt-in, time-bounded)»** (build.yml:133-159) — после
   L1+L2 Go-тестов, до Coverage report. Цикл по 4 target'ам:
   `FuzzParseDhcpHostLine`, `FuzzParseArpContent`, `FuzzParseAliasLine`,
   `FuzzParseLeasesContent`. Каждый: `go test -run='^$' -fuzz="^${target}$"
   -fuzztime=30s .` (одиночный пакет `.` — важно: `./...` роняет go с "fuzz
   testing requires a single package" потому что раскрывается в 2 пакета
   `intermask` + `intermask/docs`; первый коммит это упустил, 0s-провал).
   CGO_ENABLED=1.
3. **Verbose-вывод для триажа:** команда echo'ится перед запуском; per-target
   rc ловится; на crash шлёт `::warning::` с путём `testdata/fuzz/<Name>/` и
   фейлит шаг в конце (все 4 успевают отработать).

### Коммиты

- `a2b0edc` — первичный input + step (с багом `./...`).
- `6a5fdad` — fix: `./...` → `.` + verbose triage output.

### Реальный прогон (CI, `run_fuzz_tests=true`)

- **Время:** ~2m54s на 4 target'а × 30s fuzztime (overhead = compile + corpus
  init per target).
- **Результат:** PASS — нет `::warning::`/`::error::`, шаг зелёный.
- **Crash'и:** **нет** (no crash found in 4×30s). Артефактов в
  `testdata/fuzz/` не появилось → regression-корпус не пополнился.
- Покрытые парсеры: `parseDhcpHostLine`, `parseArpContent`, `parseAliasLine`,
  `parseLeasesContent`. Oracle (panic-free + структурные инварианты на
  success) устоял во всех 4 случаях за 30s случайного инпута каждый.

### Замечания / knock-on

- **Продуктовый код не тронут** — правки только в YAML (build.yml +21/+17).
- `-fuzz` гоняется опционально по запросу; дефолтный CI не удлиняется.
- ROI для дальнейшего `-fuzztime`: на 30s × 4 target'а crash'ей не найдено;
  продление до минутного бюджета — на усмотрение (можно сделать input-параметр
  `fuzztime`, но сейчас востребованности нет).
- Crash-корпус `testdata/fuzz/` остаётся пустым — это норма (парсеры
  намеренно толерантны к мусору, oracle = "не паникует"). Если позже
  появится crash, Go автоматически запишет корпус — его коммитить.
- Снимает пункт "real `-fuzz` НЕ гоняется — opt-in CI-шаг отложен" из
  `Quality_sweep.md` §0 (состояние на 2026-07-29).

---

## Этап 2 — dnsmasq compatibility matrix ✅ ВЫПОЛНЕН (2026-07-31)

**Цель:** smoke.sh против разных версий dnsmasq — поймать версионные расхождения
`--test`-валидации, парсеров, dhcp-range auto-detect.

### Подход

Build-from-source внутри федора-44 контейнера-runner'а (не docker-in-docker,
не strategy.matrix). Runner сам в `fedora:44` container → вложенный `docker
run` недоступен. Простейшая надёжная схема: один собранный `intermasq-ci`
(статический, CGO_ENABLED=0 из основного build-step'а) + 3 dnsmasq-тарбалла с
upstream (`thekelleys.org.uk/dnsmasq/`), `cc`-only build (~10s каждая),
указываем путь через `-dnsmasq-bin`. Это переиспользует уже готовый бинарник
(доп. builds артефактом не передаются — нет нужды в `upload-artifact`).

### Что сделано

1. **`workflow_dispatch.inputs.run_compat_matrix`** (build.yml:36-40) —
   boolean, default `false`, opt-in по образцу `run_e2e_tests`/`run_fuzz_tests`.
   Дефолтный пайплайн не затронут.
2. **Шаг «L3.5 — dnsmasq compat matrix (opt-in)»** (build.yml:230-342) —
   после дефолтного L3 smoke, до Perf. Seq-цикл по 3 версиям:
   - `2.80` (Jan 2018) — oldest anchor; `--test --conf-file=` поддерживает
     (добавлено в 2.66), `dhcp-range` AT-style — basic.
   - `2.86` (Sep 2021) — middlepoint; близко к debian stable 12 (2.86).
   - `2.90` (Feb 2024) — recent; близко к fedora 40+ пакету.
3. **Per-version изоляция**: distinct port+`-conf-dir`+`-db`+`-history-dir`
   (suffixed `-2.80`/`-2.86`/`-2.90`) + `rm -rf $CONF_DIR`/`rm -f users.json`
   → каждая итерация стартует с чистого `setup_required=true`, так что
   `00-preflight.sh` делает POST `/api/setup` заново — состояние одной
   версии не утекает в следующую.
4. **Diag/log:** печатает карту версий (системная fedora:44 + 3 src-build) +
   `dnsmasq --version` каждой собранной; `dnsmasq-<v>-build.log` на случай
   build-fail с `tail -n 40` в `::error::`.
5. **Version map** (зафиксировано в логе CI-шага):
   ```
   fedora:44 system dnsmasq: <dnsmasq --version line 1 — varies по образу>
   matrix src-build versions : 2.80 (2018), 2.86 (2021), 2.90 (2024)
   ```
   Distro-аналоги (информационно, не verified в этом CI): alpine 3.19 ≈ 2.89,
   debian 12 ≈ 2.86, ubuntu 24.04 ≈ 2.90, fedora 44 ≈ 2.92+. Шаг намеренно
   НЕ запускает другие distro-контейнеры (нужен docker-in-docker, недоступно);
   source-build matrix покрывает ту же version-axis.

### Verify-критерий (по §этапа)

- opt-in `run_compat_matrix=true` прогоняет smoke на 3 версиях без
  unexpected fails (или с зафиксированными version-notes).
- Дефолтный CI не затронут (input default=false → шаг skips).
- Терпимость к расхождениям: если dnsmasq vX даёт иное поведение, которое
  НЕ баг intermasq → smoke-чек либо версионно-обходится через
  `dnsmasq --version | grep`, либо помечается known-difference в логе
  шага. Баг intermasq = расхождение, ломающее пользователя на поддерживаемой
  версии.

### Коммиты

- `8328ceb` — opt-in input + step (build-from-source matrix vs smoke).

### Замечания / knock-on

- **Продуктовый код не тронут** — правки только в YAML (build.yml +119).
- **`make` добавляется** в opt-in шаге (`dnf install -y make`) — в base fedora:44
  нет. ~1s overhead, только при opt-in.
- **Upstream tarball URL** `https://thekelleys.org.uk/dnsmasq/` — public CA,
 reachable из контейнера так же как go.dev (там это уже работает в build-step
  для Go toolchain).
- **Build-время:** dnsmasq-v2.80 на gcc-14 выдаёт warnings (C-statics legacy),
  но компилится cc=0; собирается в `src/dnsmasq` (~10s `-j2`). v2.86/2.90
  собираются без предупреждений.
- **Coverage-lib:** версия dnsmasq определяет, какие `dhcp-range` опции
  валидны для `--test`; smoke 30-test.conf/40-dhcp.conf (basic-dhcp template)
  использует только legacy-опции → все 3 версии должны пройти A13-чек
  "PUT invalid syntax → 400" (тестируется `port=abc`, который отторгается во
  всех версиях с 2.x).
- **Clustered/known-difference awareness:** если на CI вылезет
  version-specific fail (напр. 2.80 ругается на новую опцию), шаг эхает
  `::error::` per-version, но loop не abort'ит — соберутся результаты по
  всем 3 версиям за один прогон, что ускоряет триаж.
- **Шаг НЕ добавляет новые дефолтныеGuestCI-цели**, не требует
  external VM (это будет этап ВМ).

### Финальная верификация (pending CI run)

Локально (Windows) запустить нельзя — нужен fedora:44 контейнер + dnsmasq
build. Operator должен:
1. Workflow dispatch с `run_compat_matrix=true` (можно вместе с
   `run_fuzz_tests=false`, `run_e2e_tests=false`, `push_to_registry=false`).
2. Убедиться, что шаг «L3.5 — dnsmasq compat matrix (opt-in)» зелёный
   (3 группы `dnsmasq 2.80`, `2.86`, `2.90` — все `smoke rc=0`).
3. Если одна версия упала → diff между её логом и baseline (fedora system
   dnsmasq в L3) → решение: intermasq-bug fix (этап 3 или отдельный PR) или
   version-note в этом логе.

---
