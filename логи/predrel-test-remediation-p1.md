# P1 — критичные починки тестовой инфраструктуры (бывший P0)

**Цель:** устранить находки, которые **маскируют баги или дают ложную
уверенность** в надёжности тестов. Без закрытия P1 рефакторинг небезопасен —
в зоны A11/A13/A14/A15 и ноль-ассерт тестов регрессия въедет молча, с зелёным
CI.

**Трудоёмкость:** ~1 день (6 задач).
**Порядок:** внутри фазы задачи независимы, можно делать в любом порядке.
**Применяемость:** обязательно **до старта любого рефакторинга** продуктового
кода.

---

## Задача P1.1 — Синхронизировать known-bugs.txt ↔ bugreport/bugs.md ↔ ROADMAP.md

**Контекст:** `tests/known-bugs.txt` содержит записи A14 и A15 (найдены
Quality sweep Этап 2, 2026-07-31). Но `tests/bugreport/bugs.md` описывает
только A1–A13, и `tests/ROADMAP.md` в трёх местах утверждает «known-bugs.txt
пуст». Это нарушение явного контракта из `bugs.md:10` («Источник
`tests/known-bugs.txt` синхронизирован с этим файлом»). Также в
`tests/suites/51-history-diff-restore.sh:35` затесался японский иероглиф `バグ`
вместо «баг».

**Файлы:**
- `tests/bugreport/bugs.md` — добавить секции A14 и A15 (по формату A1–A13: Severity, Component, Симптом, Корень, Фикс/Status, Regression test). Данные взять из комментариев в `tests/known-bugs.txt:18-35`. Сводную таблицу `bugs.md:17-30` дополнить двумя строками. Сводку `bugs.md:32-41` обновить («Все записи (A1–A8, A10–A15) теперь FIXED/WONTFIX или known-conditional»).
- `tests/ROADMAP.md` — в трёх местах убрать ложное «пуст»:
  - `:51` (Hardening sweep строка) — «known-bugs.txt пуст» заменить на «known-bugs.txt пуст на момент Hardening sweep; 2026-07-31 (Quality sweep Этап 2) добавлены A14/A15 как version-conditional на dnsmasq 2.80/2.86».
  - `:180` (P0✓ строка Bugfix sweep) — «tests/known-bugs.txt теперь пуст» заменить на «tests/known-bugs.txt чист от A-багов, зафиксированных в Bugfix sweep (A1-A12); позже Quality sweep добавит A14/A15 как known-conditional».
  - `:192` (метрика v1.0) — чекбокс `[x] tests/known-bugs.txt пустой (или содержит только wontfix'ы)` заменить на `[x] tests/known-bugs.txt пустой ИЛИ содержит только version-conditional баги с явной причиной (нынешний статус: A14/A15 conditional на dnsmasq 2.80/2.86; на целевой 2.90 — пуст)`.
  - `:193` — «smoke.sh: 0 Fail, 0 Known-fail, 0 Skipped, ~140+ Pass (139/139 CLEAN PASS)» заменить на «smoke.sh: 0 Fail, 0 Known-fail на dnsmasq ≥2.86; A14/A15 KNOWN-fail срабатывают только на dnsmasq 2.80/2.86 (compat-matrix, opt-in)».
- `tests/suites/51-history-diff-restore.sh:35` — заменить `A15 and theバグ` на `A15 and the bug` (или `A15 и этот баг`, если хотим кириллицу).

**Как верифицировать:**
1. `grep -rn 'バグ' tests/` — пусто.
2. `grep -n 'A14\|A15' tests/bugreport/bugs.md` — находит 2 новые секции + 2 строки в сводной таблице.
3. `grep -n 'пуст' tests/ROADMAP.md` — либо не находит утверждений «known-bugs.txt пуст» без контекста, либо контекст уточняет «на 2.90 / после closed bugs».

**Acceptance criteria:** все три документа (`known-bugs.txt`, `bugs.md`,
`ROADMAP.md`) согласованы по состоянию A14/A15. Японский иероглиф убран. Smoke
при прогоне не меняется.

---

## Задача P1.2 — Сузить A14/A15-теги: проверять тело ошибки, не только статус

**Контекст:** `tests/lib/common.sh:33-42` — тег `<BUG_ID>` в `check` делает
mismatch жёлтым (KNOWN-fail), pipeline остаётся зелёным. Сегодня A14 и A15
ставятся на любой mismatch статуса. Но `restoreBackupZip`
(`handlers_safety.go:166-173`) возвращает **400** для всех ошибок
(`dnsmasq_test_failed`, parse error, IO error — коллапс в один статус). То же
для `historyRestoreHandler` (`handlers_safety.go:105-118`) → 500. **Любая**
регрессия в этих хендлерах маскируется как «A14/A15 всё ещё известна».

**Файлы:**
- `tests/lib/common.sh` — расширить сигнатуру `check` опциональным аргументом
  `body_pattern`. Когда `bug_id` присутствует И статус не совпал, дополнительно
  проверить, что тело ответа содержит `body_pattern`. Если тело НЕ содержит
  паттерн — это **другая** ошибка, не известный баг → hard FAIL с понятным
  сообщением («Bug A14 known to return error containing 'X', but body says
  'Y' — likely unrelated regression»).
- `tests/lib/http.sh` — если ещё не сохраняется последнее тело в `/tmp/smoke.body`,
  убедиться что оно доступно для `common.sh`.
- `tests/suites/52-backup-restore.sh:18` — вызвать `check` с body_pattern
  `'dnsmasq_test_failed'` для A14. A14 — специфичный баг: bare `dnsmasq --test`
  без `--conf-file=` → restoreBackupZip возвращает `dnsmasq_test_failed`. Если
  вместо этого `write_error` / `parse_error` / `access_denied` — это регрессия,
  не A14.
- `tests/suites/51-history-diff-restore.sh:38` — аналогично для A15 с
  body_pattern `'dnsmasq_test_failed'`.

**Реализация `check` (предлагаемая):**
```bash
# was: check <name> <exp_code> <got_code> [bug_id]
# now:  check <name> <exp_code> <got_code> [bug_id [body_pattern]]
check() {
    local name="$1" exp="$2" got="$3" bug_id="${5:-}" body_pat="${6:-}"
    # ... existing logic ...
    # если bug_id задан И статус не совпал:
    if [ -n "$bug_id" ] && [ "$exp" != "$got" ]; then
        if [ -n "$body_pat" ]; then
            local body; body=$(body)   # или cat /tmp/smoke.body
            if ! echo "$body" | grep -q "$body_pat"; then
                printf "  ${RED}✗${RESET} %s\n" "$name"
                printf "      Expected known-fail pattern '%s' in body, got:\n" "$body_pat"
                printf "      %s\n" "$body"
                printf "      This is likely NOT bug %s — investigate.\n" "$bug_id"
                FAIL=$((FAIL + 1))
                return 1
            fi
        fi
        # ... existing KNOWN-fail logic ...
    fi
}
```

**Как верифицировать:**
1. Временно сломать `restoreBackupZip` (например, добавить `return fmt.Errorf("write_error")` в начало) → запустить smoke → A14-чек должен hard FAIL с сообщением «expected pattern 'dnsmasq_test_failed', got 'write_error'».
2. Откатить сломанное → smoke снова зелёный (или жёлтый на A14 если среда 2.80).
3. Аналогично для A15.

**Acceptance criteria:** регрессия в restoreBackupZip/historyRestoreVersion,
возвращающая **другую** ошибку, роняет smoke в red, а не маскируется под
known A14/A15.

---

## Задача P1.3 — Удалить или добавить ассерты в ноль-ассерт тесты

**Контекст:** 4 теста не содержат ни одного `t.Errorf`/`t.Fatal`/сравнения —
они не могут упасть. Создают ложное впечатление покрытия.

**Файлы и точки:**

### P1.3.a — `dnsmasq_test.go:243` `TestSystemdCallerRestartSelf`
Сейчас тело: `caller := &SystemdSystemCaller{UseSudo: false}; _ = caller; callerUser := &SystemdUserCaller{}; _ = callerUser`.

Вариант 1 (рекомендуемый) — **удалить тест целиком**, потому что `RestartSelf`
уже тестируется через fakes в `system_callers_test.go`
(`TestSystemdSystemCaller_RestartSelf_Fakes`, `TestSystemdUserCaller_Fakes`).

Вариант 2 — **добавить ассерты**: `if caller.String() != "systemd (root)" { t.Errorf(...) }` и аналогично для `callerUser`.

### P1.3.b — `dnsmasq_test.go:888` `TestSseBroadcastFullChannel`
Сейчас только регистрирует клиента с zero-capacity channel и зовёт `sseBroadcast`. Нет ни одного ассерта.

**Фикс:** после `sseBroadcast("test", "data")` добавить:
```go
select {
case <-cl.ch:
    t.Errorf("expected broadcast to be dropped on full channel, but message was delivered")
default:
    // OK —分支 full-channel-drop отработала
}
if len(cl.ch) != 0 {
    t.Errorf("expected empty channel after broadcast to full channel, got len=%d", len(cl.ch))
}
```
Тест реально проверяет, что `sseBroadcast` (sse.go:58-68) использует `select { case ch<-msg: default: }` (non-blocking), а не блокирующий send.

### P1.3.c — `bins_test.go:162` `TestLazyAccessors_CallResolve`
Сейчас тело: `for _, fn := range accessors { _ = fn() }`.

**Фикс:** для каждой функции-аксессора:
```go
got := fn()
if got == "" {
    t.Errorf("%s returned empty string", name)
}
// Если dnsmasqBinPath уже заполнен (после resolveBins()), проверить идемпотентность:
if name == "dnsmasqBin" && dnsmasqBinPath != "" && got != dnsmasqBinPath {
    t.Errorf("dnsmasqBin() lazy accessor returned %q, expected cached %q", got, dnsmasqBinPath)
}
```

### P1.3.d — `goroutines_test.go:226` `TestCleanupBlacklistOnce_EmptyMap`
Сейчас только чистит map и зовёт `cleanupBlacklistOnce(time.Now())`, без ассерта.

**Фикс:**
```go
cleanupBlacklistOnce(time.Now())
blacklistMu.RLock()
n := len(blacklist)
blacklistMu.RUnlock()
if n != 0 {
    t.Errorf("cleanupBlacklistOnce on empty map left %d entries", n)
}
```

**Как верифицировать:**
1. После правок — `go test ./... -run 'TestSystemdCallerRestartSelf|TestSseBroadcastFullChannel|TestLazyAccessors|TestCleanupBlacklistOnce' -v` — все PASS.
2. Для P1.3.b: временно изменить `sseBroadcast` на блокирующий send (убрать `select { default: }`) → тест должен зависнуть или упасть. Откатить.
3. Для P1.3.d: временно сделать `cleanupBlacklistOnce` добавляющей записи (мутация) → тест падает. Откатить.

**Acceptance criteria:** все 4 теста содержат осмысленные ассерты и реально
падают при мутации целевого кода.

---

## Задача P1.4 — argv-инспектирующий fakeDnsmasq для пиннинга A13 wiring

**Контекст:** A13-фикс (`dnsmasq.go:77,97` +`backup.go:119`) изменил
`exec.Command(dnsmasqBin(), "--test")` на `... "--test", "--conf-file="+path`.
Но regression-тесты `TestPutFileHandler_DnsmasqTestFail_400` (linux_test.go:616),
`TestUpdateConfigHandler_DnsmasqTestFail_400` (645),
`TestRestoreBackupHandler_DnsmasqTestFail_400` (671) используют `fakeDnsmasq(t,
1)` — это `#!/bin/sh\nexit 1`, который **игнорирует argv**. Тест проходит и
со старым (баговым) кодом, и с новым (фикс). Regression-тест — театр.

**Файлы:**
- `linux_test.go` — рядом с `fakeDnsmasq(t, exitCode)` добавить новый хелпер:

```go
// fakeDnsmasqArgvInspect создаёт sh-скрипт, который пишет свой argv в logPath
// и выходит с exitCode. Используется для тестов, которым важно проверить,
// с какими аргументами позвали dnsmasq (например A13: проверить, что
// --conf-file=<path> присутствует). Возвращает путь к бинарнику и путь к
// лог-файлу argv.
func fakeDnsmasqArgvInspect(t *testing.T, exitCode int) (binPath, logPath string) {
    t.Helper()
    dir := t.TempDir()
    binPath = filepath.Join(dir, "dnsmasq")
    logPath = filepath.Join(dir, "argv.log")
    script := fmt.Sprintf("#!/bin/sh\necho \"$@\" > %q\nexit %d\n", logPath, exitCode)
    if err := os.WriteFile(binPath, []byte(script), 0755); err != nil {
        t.Fatalf("write fake dnsmasq: %v", err)
    }
    // Зарегистрируйте binPath через setBinPath(t, binPath) или
    // withDnsmasqBin(t, binPath) — по той же схеме, как fakeDnsmasq.
    return binPath, logPath
}

// readArgvLog читет argv.log и возвращает содержимое.
func readArgvLog(t *testing.T, logPath string) string {
    t.Helper()
    b, err := os.ReadFile(logPath)
    if err != nil {
        t.Fatalf("read argv log: %v", err)
    }
    return string(b)
}
```

- Добавить **3 новых теста** (НЕ заменять существующие — они проверяют rollback-plumbing):

```go
func TestPutFileHandler_PassesConfFileToTest(t *testing.T) {
    if runtime.GOOS == "windows" { t.Skip("linux-gated") }
    binPath, logPath := fakeDnsmasqArgvInspect(t, 0)
    withDnsmasqBin(t, binPath)
    // ... setup как в TestPutFileHandler_Success ...
    sc := PUT(jwt, "/api/files/30-test.conf", "dhcp-host=aa:bb:cc:dd:ee:01,10.0.0.1")
    if sc != 200 { t.Fatalf("expected 200, got %d", sc) }
    argv := readArgvLog(t, logPath)
    if !strings.Contains(argv, "--conf-file=") {
        t.Errorf("dnsmasq invoked without --conf-file=: argv=%q (A13 regression)", argv)
    }
}
```

Аналогично `TestUpdateConfigHandler_PassesConfFileToTest` и
`TestRestoreBackupHandler_PassesConfFileToTest`.

**Как верифицировать:**
1. На фиксе (текущий main): 3 новых теста PASS.
2. Временная мутация: `dnsmasq.go:77` вернуть к `exec.Command(dnsmasqBin(), "--test")` (без `--conf-file=`) → 3 новых теста FAIL с сообщением «dnsmasq invoked without --conf-file=». Откатить.
3. Существующие A13-тесты (с `fakeDnsmasq(t,1)`) — продолжают PASS (они тестируют rollback-plumbing, не wiring).

**Acceptance criteria:** любая будущая регрессия, убирающая `--conf-file=`
из test-команды, роняет 3 новых теста. Существующие rollback-тесты
продолжают работать.

---

## Задача P1.5 — Де-коррелировать seed в hosts-sort.spec.ts

**Контент:** `tests/e2e/specs/hosts-sort.spec.ts:29-35` сидирует 5 хостов:
`sort1 → 10.99.1.2, "sort1"`, `sort2 → 10.99.2.2, "sort2"`, ..., `sort5 →
10.99.5.2, "sort5"`. IP и hostname растут синхронно, поэтому ascending-by-IP
**идентичен** ascending-by-hostname. Тест ORDER-assertions (строки 54/58/63/68/73)
не отличают «sortBy('hostname') переключил ключ» от «sortBy — no-op, всегда
sort by ip». Мутация `sortKey.value = key` (убрать переключение) — проходит
незамеченной.

**Файлы:**
- `tests/e2e/specs/hosts-sort.spec.ts:29-35` — изменить seed, чтобы IP и
  hostname шли в **противоположных** направлениях:

```typescript
const SORT_HOSTS = [
  { suffix: '01', mac: 'aa:99:99:99:99:01', ip: '10.99.5.2', hostname: 'sortA' },
  { suffix: '02', mac: 'aa:99:99:99:99:02', ip: '10.99.4.2', hostname: 'sortB' },
  { suffix: '03', mac: 'aa:99:99:99:99:03', ip: '10.99.3.2', hostname: 'sortC' },
  { suffix: '04', mac: 'aa:99:99:99:99:04', ip: '10.99.2.2', hostname: 'sortD' },
  { suffix: '05', mac: 'aa:99:99:99:99:05', ip: '10.99.1.2', hostname: 'sortE' },
]
```

Теперь ascending-by-IP = `[01,02,03,04,05]` (по убыванию hostname), а
ascending-by-hostname = `[01,05,04,03,02]` в MAC-порядке — **разные**.

- `tests/e2e/specs/hosts-sort.spec.ts:54,58,63,68,73` — обновить
  ожидаемые массивы в соответствии с новыми seed-данными. Для IP-ascending
  ожидать `['sortA','sortB','sortC','sortD','sortE']` в порядке IP
  `10.99.5.2 → 10.99.4.2 → 10.99.3.2 → 10.99.2.2 → 10.99.1.2` — это значит
  `['sortA','sortB','sortC','sortD','sortE']` идёт от большего IP к меньшему.
  Для hostname-ascending — наоборот, `['sortE','sortD','sortC','sortB','sortA']`
  (по hostname A меньше E). Внимательно пересчитать каждый assert — это легко
  спутать.

**Как верифицировать:**
1. На фиксе: `cd tests/e2e && npx playwright test hosts-sort` PASS.
2. Мутация: во `frontend/src/store.js` (или где `sortKey` logic живёт)
   убрать `sortKey.value = key` → тест должен FAIL на ORDER-assertion (не на
   count — count-assertion по-прежнему пройдёт). Откатить.
3. Мутация: перепутать ascending/descending в `sortedHosts` computed → FAIL.

**Acceptance criteria:** любые изменения в логике сортировки (ключ,
направление, no-op) роняют хотя бы один ORDER-assertion. Count-assertion
(`toHaveCount(5)`) по-прежнему ловит исходный A1 DOM-reuse баг.

---

## Задача P1.6 — Финальная верификация фазы P1

После задач P1.1–P1.5 прогнать полный цикл:

1. **Доки:**
   ```bash
   grep -rn 'バグ' tests/                # пусто
   grep -n 'A14\|A15' tests/bugreport/bugs.md   # 2 секции + 2 строки в таблице
   ```
2. **Go:**
   ```bash
   go vet ./...
   go test ./... -count=1
   go test ./... -race -count=1         # CGO_ENABLED=1
   ```
   Все 4 ранее-ноль-ассерт теста либо удалены, либо содержат ассерты. 3 новых
   A13-wiring теста добавлены и PASS.
3. **Bash smoke** — на dnsmasq 2.90 (где A14/A15 не срабатывают):
   ```bash
   BASE=http://localhost:18081 ./tests/smoke.sh
   ```
   Должен быть 0 Fail, 0 Known-fail.
4. **Bash smoke** — если есть доступ к dnsmasq 2.86/2.80 (compat-matrix),
   проверить, что A14/A15 KNOWN-fail срабатывает **с правильным телом**
   (после P1.2). Дописать в log diff, что body-pattern matчится.
5. **Playwright:**
   ```bash
   cd tests/e2e && npx playwright test hosts-sort
   ```
   PASS. Mutation-pass (deop `sortKey.value = key`) — FAIL.
6. **Коммит:** `predrel-test-remediation-P1: critical test fixes` (или
   эквивалент в стиле repo).

**Acceptance criteria фазы P1:** зелёный `go test`, зелёный smoke (на 2.90),
зелёный Playwright hosts-sort, согласованные дока. Мутации
sortKey/`--conf-file=`/`sseBroadcast`/cleanupBlacklist ловятся новыми или
усиленными тестами.
