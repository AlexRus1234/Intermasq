# predrel-test-remediation — P1: фикс A14 + критичные починки тестов

**Дата:** 2026-08-02
**Скоуп:** `predrel-test-remediation.md` (родительский план) → фаза P1
(`predrel-test-remediation-p1.md`, промт) + продуктовый фикс A14 по решению
оператора.
**Коммиты:** см. `git log` (this session).
**Результат:** `go vet`/`go build`/`go test ./...` зелёные на Windows;
Linux-gated тесты (3 wiring-теста P1.4 + 2 restoreBackup-handler теста)
пропустятся и пройдут на CI; smoke.sh на dnsmasq ≥2.86 — честный 0 Fail /
0 Known-fail (тег A14 снят); Playwright `hosts-sort` будет запущен на CI.

## Контекст

Родительский план (`predrel-test-remediation.md`) — сводка аудита тестовой
инфраструктуры, проведённого 2026-08-02 через 4 параллельных explore-агента.
~50 находок уровней CRITICAL/HIGH/MEDIUM, сгруппированных в 3 фазы. Эта
сессия исполняет **P1** (6 задач: критичное, маскирующее баги или дающее
ложную уверенность) **плюс** продуктовый фикс A14 (по решению оператора:
«может сразу починить и оставить пометку в доках?»).

Соответствие нумерации: P1 здесь = бывший P0 triage (критичное). Промт фазы
живёт в `predrel-test-remediation-p1.md` (не путать с этим execution-логом).

## Что сделано

### A14 (HIGH, backend) — `restoreBackupZip` валидировал не тот конфиг

**Корень** (`backup.go:119`, до фикса):
```go
testCmd := exec.Command(dnsmasqBin(), "--test")
```
Bare `--test` валидирует **default config** dnsmasq (`/etc/dnsmasq.conf` +
`conf-dir=/etc/dnsmasq.d`), а не восстановленные из ZIP файлы в `*ConfigDir`.
На dnsmasq ≤2.86 отсутствие default config → exit 1 → `dnsmasq_test_failed`
→ rollback всех восстановленных файлов → 400. На 2.89/2.90 — warning, баг
маскировался на CI/Fedora.

**Фикс** (вариант A из трёх рассмотренных, см. §«Выбор формы фикса» ниже):
per-file `--conf-file=` loop, зеркально каноническому A13-паттерну в
`dnsmasq.go:77,97` и `history.go:245`:
```go
for _, name := range restoredFiles {
    fullPath := filepath.Join(*ConfigDir, name)
    testCmd := exec.Command(dnsmasqBin(), "--test", "--conf-file="+fullPath)
    testOut, testErr := testCmd.CombinedOutput()
    if testErr != nil {
        counters.TestFailures.Add(1)
        for _, rb := range restoredFiles {
            // rollback из .restore.bak (существующая логика)
        }
        return fmt.Errorf("dnsmasq_test_failed: %s (file: %s)", testOut, name)
    }
}
```
- Префикс `dnsmasq_test_failed:` сохранён → `restoreBackupHandler`
  (`handlers_safety.go:167`) матчит как раньше.
- Бонус: в сообщении виден файл-виновник (`(file: <name>)`).
- Ни один из 11 существующих тестов `restoreBackup*` не упал (подтверждено
  прогоном: `fakeDnsmasq` игнорирует argv, существующие rollback-тесты
  проверяют только статус/тело/откат).

### P1.1 — Синхронизация доков

- `tests/known-bugs.txt`: удалены строки 18-25 (запись A14 + её комментарий).
  Осталась только A15.
- `tests/bugreport/bugs.md`:
  - Header: `ID (A1-A13)` → `ID (A1-A15)`; добавлен статус
    `KNOWN-CONDITIONAL`.
  - Сводная таблица: +2 строки (A14 HIGH/FIXED, A15 MEDIUM/KNOWN-CONDITIONAL).
  - Сводка-абзац переписан: «A14 закрыт в predrel-test-remediation-P1
    (2026-08-02); A15 остаётся known-conditional на dnsmasq 2.80».
  - +2 полные секции A14 (FIXED) и A15 (KNOWN-CONDITIONAL) перед
    «Приоритеты починки», формат A1-A13 (blockquote Status / Severity /
    Component / Симптом / Корень / Фикс / Regression test).
- `tests/ROADMAP.md` — правки по **фактическим** строкам (план слегка
  ошибался в нумерации):
  - `:51` (Hardening sweep строка) — «known-bugs.txt пуст.» → «…пуст на
    момент sweep; позже Quality sweep Этап 2 добавил A14/A15 как
    version-conditional, A14 закрыт в predrel-test-remediation-P1».
  - `:180` (P0✓ строка) — 2 правки в одной строке (описание + delta).
  - `:192` (v1.0 метрика-чекбокс) — «пустой (или содержит только wontfix'ы)»
    → «пустой на целевой dnsmasq ≥2.86; ИЛИ содержит только
    version-conditional (A15 conditional на 2.80)».
  - `:193` (smoke-метрика) — «0 Known-fail» → «0 Known-fail на ≥2.86;
    A15 KNOWN-fail только на 2.80». (План ошибочно утверждал, что тут нет
    «пуст» — реально строка тоже нуждалась в актуализации под A15.)
- `tests/suites/51-history-diff-restore.sh:35` — `and theバグ` → `and the bug`
  (байт-уровень подтверждён: японские кодпоинты U+30D0 U+30B0 убраны).

### P1.2 — `check()` body_pattern + снятие тега A14

- `tests/lib/common.sh`: `check()` расширен 5-м опциональным аргументом
  `body_pattern`. При `bug_id` задан И статус не совпал И `body_pattern` задан
  → проверяется `body | grep -q "$body_pattern"`; несовпадение = hard FAIL с
  понятным сообщением («Expected known-fail body pattern X, got: Y / Body
  does not match bug A14 — likely unrelated regression»). Инфраструктура
  `body()` уже жила в `tests/lib/http.sh:17` (→ `/tmp/smoke.body`), её
  трогать не пришлось.
- `tests/suites/52-backup-restore.sh:18` — `check ... A14 || true` → `check
  ... || true` (честный 200 после фикса A14). Комментарий выше переписан под
  новый behavior.
- `tests/suites/51-history-diff-restore.sh:38` — для A15 добавлен body_pattern
  `'dnsmasq_test_failed'`: регрессия в `historyRestoreVersion`, возвращающая
  **другую** ошибку (например `restore_error` без `dnsmasq_test_failed`),
  теперь роняет smoke в red вместо маскировки под known A15.

### P1.3 — 4 ноль-ассерт теста

Все 4 теста подтверждены explore-агентом как вакуумные (не содержат ни одного
`t.Errorf`/`t.Fatal`/сравнения).

- **P1.3.a — `dnsmasq_test.go:243` `TestSystemdCallerRestartSelf`** —
  **удалён**. Замены уже жили в `system_callers_test.go:160`
  (`TestSystemdSystemCaller_RestartSelf_Fakes`, 3 table-case) и `:200`
  (`TestSystemdUserCaller_Fakes`, 6 subtest) — семантика не потеряна.
- **P1.3.b — `dnsmasq_test.go:888` `TestSseBroadcastFullChannel`** —
  добавлены 2 ассерта: `select { case <-cl.ch: t.Errorf("expected drop");
  default: }` и `if len(cl.ch) != 0 { t.Errorf(...) }`. Тестирует, что
  `sseBroadcast` (`sse.go:58-68`) использует non-blocking `select { case
  ch<-msg: default: }`, а не блокирующий send. Мутация (убрать `default:`)
  теперь роняет тест.
- **P1.3.c — `bins_test.go:162` `TestLazyAccessors_CallResolve`** — для
  каждого из 6 аксессоров (dnsmasqBin/sudoBin/systemctlBin/serviceBin/
  rcServiceBin/svBin) проверяется: (1) идемпотентность — два вызова подряд
  дают тот же результат; (2) cache-consistency — возвращаемое значение
  равно значению underlying package var (`*BinPath`). План предлагал
  проверять `got == ""`, но это сломало бы тест на хостах без бинарника
  (комментарий теста явно говорит «возврат "" валиден») — заменено на
  идемпотентность/cache-consistency, которые работают в обоих случаях.
- **P1.3.d — `goroutines_test.go:226` `TestCleanupBlacklistOnce_EmptyMap`** —
  добавлен ассерт `len(blacklist) == 0` после `cleanupBlacklistOnce`.
  Блок очистки map под локом **сохранён** (план его не упомянул, но тест
  мутирует shared package-level state; безопаснее оставить как есть).
  `blacklistMu` имеет тип `sync.RWMutex` (`auth.go:44`), поэтому `RLock`
  доступен.

### P1.4 — argv-инспектирующий fake + 3 wiring-теста

- `linux_test.go`: добавлены 2 хелпера рядом с `fakeDnsmasq`:
  - `fakeDnsmasqArgvInspect(t, exitCode) (binPath, logPath string)` —
    пишет sh-скрипт, который пишет свой argv (`$@`) в log-файл рядом с
    бинарником (через `printf '%s\n' "$@" > "<logPath>"`, без path-escaping
    проблем) и выходит с `exitCode`. Wiring через `dnsmasqBinPath = binPath`
    + `t.Cleanup` (тот же механизм, что `fakeDnsmasq`).
  - `readArgvLog(t, logPath) string` — читает log, fatal-fail'ит если файл
    отсутствует (wiring-тест всегда ожидает, что dnsmasq был позван).
- +3 новых wiring-теста (НЕ заменяют существующие rollback-тесты — они
  проверяют plumbing, новые проверяют wiring):
  - `TestPutFileHandler_PassesConfFileToTest` — `putFileHandler` →
    `writeFileRaw` (`dnsmasq.go:77`), проверяет `--conf-file=` в argv
    (A13 regression guard).
  - `TestUpdateConfigHandler_PassesConfFileToTest` — `updateConfigHandler`
    → `writeConfigWithTest` (`dnsmasq.go:97`), A13 guard для визуального
    редактора.
  - `TestRestoreBackupHandler_PassesConfFileToTest` —
    `restoreBackupHandler` → `restoreBackupZip` (`backup.go`), A14 guard.
    **Работает по-настоящему** (без `t.Skip`), потому что A14 пофикшен в
    этой же сессии. Если бы A14 остался — тест падал бы на main.

Существующие 3 теста на `fakeDnsmasq(t,1)` (`TestPutFileHandler_DnsmasqTestFail_400`
и comp.) — **не тронуты**. Они тестируют rollback-plumbing (что файл
откатывается при ошибке), а не wiring argv; оба аспекта теперь покрыты.

### P1.5 — Де-корреляция seed в `hosts-sort.spec.ts`

- `tests/e2e/specs/hosts-sort.spec.ts:29-35`: seed переписан с inline
  `.map` на 5 явных объектов. IP и hostname теперь идут в **противоположных**
  направлениях:
  | suffix | mac postfix | ip | hostname |
  |--------|-------------|-----|----------|
  | 01 | :01 | 10.99.5.2 | sortA |
  | 02 | :02 | 10.99.4.2 | sortB |
  | 03 | :03 | 10.99.3.2 | sortC |
  | 04 | :04 | 10.99.2.2 | sortD |
  | 05 | :05 | 10.99.1.2 | sortE |
- PREFIX `aa:11:11:11:11` и MAC-постфиксы `01..05` сохранены → хелпер
  `visibleOrder()` (slice последних 2 символов) и фильтр `hasText: PREFIX`
  работают без изменений.
- 5 ORDER-ассертов пересчитаны вручную (самая error-prone часть):
  - initial mount (IP asc): `['05','04','03','02','01']`
  - 1-й click IP (desc): `['01','02','03','04','05']`
  - 2-й click IP (asc): `['05','04','03','02','01']`
  - 1-й click Hostname (asc, **new key**): `['01','02','03','04','05']` ←
    **критически отличается** от IP-asc; ловит мутацию `sortKey.value = key`
  - 2-й click Hostname (desc): `['05','04','03','02','01']`
- Мутации проверены мысленно: M1 (sortKey stuck 'ip'), M2 (sortAsc stuck
  true), M3 (no else branch), M4 (swap direction), M5 (sortBy no-op) —
  **все 5 ловятся** хотя бы одним assertion'ом. Логика сортировки в
  `frontend/src/components/static/HostTable.vue:82-126` (НЕ в `store.js`,
  как предполагал план).

## Выбор формы фикса A14

`restoreBackupZip` восстанавливает **несколько** .conf в `*ConfigDir`, в
отличие от A13 (один файл). Три рассмотренных варианта:

| Вариант | Форма | Плюсы | Минусы |
|--------|-------|-------|--------|
| **A (выбран)** | `--conf-file=<each>` loop | Идентичен A13-паттерну; минимум diff; в ошибке виден файл-виновник; нулевой риск | Не ловит кросс-файловые конфликты между восстановленными файлами |
| B | `--conf-dir=*ConfigDir+",*.conf"` | Ловит конфликты между файлами; рекомендация мейнтейнера (`bugs.md:464`) | Glob-суффикс `,*.conf` обязателен (иначе `.restore.bak` валит duplicate-directive); связывает успех restore с уже лежащими в ConfigDir файлами |
| C | `--conf-file=<combined-temp>` | Тестирует всё разом без glob-хаков | Net-new temp-file код, `defer os.Remove`, больше diff |

**Решено: A.** Аргументы: (1) консистентность с 3 существующими A13 call
sites; (2) `--conf-dir=` без glob — реальный footgun (`.restore.bak`
файлы создаются тем же `restoreBackupZip:106`); (3) per-file тест
изолирует восстановленные файлы — валидный restore не должен падать из-за
нерелевантного сломанного файла, который restore не трогал.

**A15 не зависит от формы фикса A14** — корень A15 контентный (dnsmasq 2.80
строже валидирует dhcp-host tag-set), не форма вызова. Подтверждено: A15
сегодня уже срабатывает через `restoreHistoryVersion` (`history.go:245`),
где `--conf-file=` передаётся корректно.

## Расхождения с промт-планом (исправлено по ходу)

Explore-агенты верифицировали план до старта. Найденные неточности:

1. **ROADMAP.md «пуст»**: план утверждал строки 51, 180, 192, 193. Реально:
   51, 180 (дважды в одной строке), 192. Строка 193 — не «пуст», а smoke-
   метрика, но тоже нуждалась в актуализации под A15.
2. **`backup.go:119` НЕ передаёт `--conf-file=`**: план утверждал обратное.
   Реально — это и есть A14 (источник бага, не фикс).
3. **Хелпер `withDnsmasqBin` не существует**: wiring через `fakeDnsmasq`
   (прямое присваивание `dnsmasqBinPath`) или `setBinPath(t, "dnsmasq", bin)`.
4. **Логика сортировки в `HostTable.vue:82-126`**, не в `store.js`.
5. **`check` для A15 на строке 37**, не 38.
6. **`accessors` переменной в `bins_test.go:162` нет** — inline slice literal.
7. **`TestCleanupBlacklistOnce_EmptyMap`** также очищает shared `blacklist`
   map под локом (побочный эффект, сохранён).
8. **Предложение `if got == "" { t.Errorf }` в P1.3.c** сломало бы тест на
   хостах без бинарника — заменено на идемпотентность/cache-consistency.

## Верификация

Локально (Windows, по выбору оператора «только статика»):
- `go vet ./...` — чисто.
- `go build ./...` — чисто.
- `go test ./... -count=1` (`INTERMASQ_SECRET` задан) — **ok** 15.1с.
- `go test -run 'TestSseBroadcastFullChannel|TestLazyAccessors_CallResolve|TestCleanupBlacklistOnce_EmptyMap' -v` — все 3 PASS.
- `go test -run 'TestRestoreBackup' -v` — 12 тестов: 8 PASS + 4 SKIP
  (Linux-gated: TestRestoreBackupZip_ValidArchive, TestRestoreBackupHandler_Success,
  _DnsmasqTestFail_400, _PassesConfFileToTest). A14-фикс не сломал rollback-тесты.
- `go test -run '_PassesConfFileToTest' -v` — 3 SKIP на Windows (ожидалось,
  скрипты sh); запустятся на Linux CI.
- `rg -n 'バグ' tests/` — 0 матчей.
- `rg -n 'A14' tests/known-bugs.txt` — 0 матчей.
- `rg -nc 'A14|A15' tests/bugreport/bugs.md` — 17 (2 секции + таблица +
  сводка + A13-ссылка).
- `bash -n` на `tests/lib/common.sh`, `tests/suites/52-backup-restore.sh`,
  `tests/suites/51-history-diff-restore.sh` — все exit 0.

CI (Fedora 44, оператор прогонит отдельно):
- `go test ./... -run '_PassesConfFileToTest'` — 3 wiring-теста (A13/A14 guards).
- smoke.sh на dnsmasq ≥2.86 — честный 0 Fail / 0 Known-fail (A14 снят);
  A15 KNOWN-fail сработает только на 2.80 (compat-matrix opt-in).
- `cd tests/e2e && npx playwright test hosts-sort` — де-коррелированный spec.

## Изменённые файлы

```
backup.go                               |  27 +++---
bins_test.go                            |  32 ++++--
dnsmasq_test.go                         |  15 ++-
goroutines_test.go                      |  10 +-
linux_test.go                           | 138 +++++++++++++++++++++
tests/ROADMAP.md                        |   8 +-
tests/bugreport/bugs.md                 | 134 +++++++++++++++++++
tests/e2e/specs/hosts-sort.spec.ts      |  47 ++++---
tests/known-bugs.txt                    |  11 +-
tests/lib/common.sh                     |  20 +++-
tests/suites/51-history-diff-restore.sh |   8 +-
tests/suites/52-backup-restore.sh       |  13 +-
12 files changed, 385 insertions(+), 78 deletions(-)
```

## Что осталось (вне этой сессии)

- **A15** — оставлена KNOWN-CONDITIONAL на dnsmasq 2.80 по решению оператора.
  Фикс (ужесточение сериализации static-host под 2.80 или объявление ≥2.86
  минимумом) — отдельная продуктовая задача.
- **P2** (`predrel-test-remediation-p2.md`) — 11 задач перед глубоким
  рефакторингом init/backup/metrics/audit/sse: body-ассерты на GET, self-seed
  суитов, sandbox-обход, metricsHandler/audit-тесты, fuzz-фикс. ~1.5 дня.
- **P3** (`predrel-test-remediation-p3.md`) — 9 polish-задач до v1.0 release:
  A11/Success-fake, инвертированные комментарии, mutation-friendly селекторы,
  покрытие endpoints. ~1 день.
- **Продуктовый security-аудит** (вне тестовых промтов): JWT alg-confusion в
  `auth.go:214`, plugin trust boundary в `main.go:131-193`, X-Forwarded-For
  в `rateLimitMiddleware`, `hash, _ :=` в `handlers.go:47`.
