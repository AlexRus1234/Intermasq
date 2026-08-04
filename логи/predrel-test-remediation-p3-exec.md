# predrel-test-remediation — P3: polish тестовой инфраструктуры до v1.0

**Дата:** 2026-08-04
**Скоуп:** `predrel-test-remediation.md` (родительский план) → фаза P3
(`predrel-test-remediation-p3.md`, промт, 9 задач).
**Результат:** `go vet`/`go test ./...`/`go test -race` зелёные на Windows
(107.997с, 0 data races); `bash -n` чист на всех 9 тронутых/новых suite'ах;
`vite build` чист (121 модулей); `playwright --list` парсит 34 теста в 29
файлах (включая переписанные audit-tab/templates-modal). Linux-gated кейсы
(`TestWriteConfigWithTest_StrictFakeRejectsInvalid`, новые smoke-suites против
запущенного binary, Playwright против intermasq-ci:18083) гоняются оператором
на CI Fedora 44.

## Контекст

P3 = бывший P2 triage — polish, **не блокирующий рефакторинг**, но снижающий
читаемость/поддерживаемость и дающий хрупкие тесты. Делаается до v1.0 release,
после P1 (`7a84a2e`) и P2 (`8a489f3` + `04ad007` + follow-ups `a8998e0`/
`9f2a447`). Продуктовый код P3 не трогает (единственное исключение —
additive `data-testid` атрибуты в `TemplatesModal.vue`, не меняющие
поведение).

## Что сделано

### Трек A — Go

**P3.1 — `TestIsSafePath` doc-комментарий.** Расхождение с промтом: прямой
unit-тест `isSafePath` (`dnsmasq_test.go:89`) **уже существовал** и покрывал
A11-слой напрямую, **включая prefix-collision case** `/etc/dnsmasq.d_evil/...`
(→ false), который и ловит целевую мутацию. Также comments «substring filter
fires first» (Variant A из промта) **уже были** в handler-level тестах
(`dnsmasq_test.go:1642`, `handlers_test.go:505`). Реальный пробел —
самодостаточный doc-комментарий к `TestIsSafePath` (acceptance требовал «с
комментарием»). Добавлен: объясняет DiD-отношение substring-фильтра и
isSafePath, кросс-ссылки на handler-level тесты, явно отмечает `_evil` case
как discriminating. **Мутация проверена:** убрать `+string(os.PathSeparator)` в
`isSafePath` (`dnsmasq.go:54`) → `TestIsSafePath` FAIL на
`/etc/dnsmasq.d_evil/host.conf` (стал true, want false). Откатано, PASS.

**P3.2 — `fakeDnsmasqStrict` + `TestWriteConfigWithTest_StrictFakeRejectsInvalid`.**
Расхождение: промт использовал несуществующий хелпер `withDnsmasqBin(t, path)`
(тот же грабль, что в P1) — заменён на внутренний wiring зеркально `fakeDnsmasq`
(`linux_test.go:51`): прямое присваивание `dnsmasqBinPath` + `t.Cleanup` +
`t.Skip` на Windows. Скрипт парсит `--conf-file=<path>` из argv, читает файл,
`grep -q '# INVALID'` → `exit 1` (иначе `exit 0`), с коротким сообщением на
stdout чтобы `CombinedOutput` поймал body (зеркало реального `dnsmasq --test`
failure-shape). Тест: valid content (`# valid\ndomain=lan`) → accept + file
updated; invalid (`# INVALID\n...`) → `dnsmasq_test_failed` + rollback к valid.
Покрывает пробел: существующий `TestWriteConfigWithTest_TestFailRollback`
(`fakeDnsmasq(1)`) тестит только rollback-plumbing (fake игнорирует контент), а
здесь wiring **+** content-валидация проверяются совместно. Linux-gated
(skip на Windows). **Мутация (структурно):** убрать вызов `dnsmasq --test` в
`writeConfigWithTest` (`dnsmasq.go:97`) → маркер `# INVALID` не детектится,
err=nil → тест FAIL на «expected dnsmasq_test_failed».

### Трек B — smoke

**P3.3 — `80-metrics.sh` A8 инверсия.** Описание чека говорило «(currently
empty)», а PASS-ветка требовала body non-empty (>2 bytes) — инверсия: держатель
кода, «починив» сравнение по комментарию, сломал бы зелёный прогон. A8 уже FIXED
(`metrics.go:62` `AbortWithStatusJSON(401, gin.H{"error":"auth_required"})`) и
нет в `known-bugs.txt`. Чек переписан как honest regression: body >2 bytes **AND**
`grep -q 'auth_required'`; тег `A8` и `|| true` сняты. Комментарий объясняет
историю (was empty → fixed).

**P3.4 — `11-auth-ratelimit.sh` RL_BLOCKED-aware.** Обе ветки `if/else` были
идентичны (`check ... 429 "$S" || true`, без bug_id) — на медленном CI, где
rate-limiter не успевает сработать за 12 попыток (протухшее окно / successful
login сбросил счётчик), `$S`=401 → hard FAIL. Разведены: `if RL_BLOCKED` →
ассерт 429; `else` → ассерт 401 (минимально-корректное поведение для bad
password) + информационная пометка «env-dependent». `|| true` снят — обе ветки
теперь дают корректный ассерт.

**P3.5 — `grep -c || echo 0` → `|| true`.** `grep -c` в no-match печатает `0`
и exit 1; `|| echo 0` допечатывает ещё `0` → переменная = `"0\n0"`, искажая
сообщения failure. Найдено ровно 2 вхождения (`rg 'grep -c.*\|\| echo 0' tests/`
→ `20-hosts-happy.sh:29`, `31-aliases-bugs.sh:8`), оба заменены на `|| true`
(не допечатывает, `LINES` остаётся чистый `0`). **Верифицировано изолированным
bash-тестом:** fixed-паттерн даёт `L=[0]`, старый баг — `L=[0\n0]`.

### Трек C — Playwright

**P3.6 — `audit-tab.spec.ts`.** Расхождение: промт предлагал матчить MAC +
action badge, но badge **i18n-переведён** (`audit.action_add`=«Добавление»/
«Add», locales ru.json/en.json:233), так что raw-матч «add» нерабочий. Реальная
проблема: `seedHosts` трактует 409 как success (api-hosts.ts:30) → на local
re-run хост уже существует, новый audit-entry не пишется, старая строка с тем же
MAC матчится вакуумно (writeAudit no-op прошёл бы). Решение: (1) clean-slate —
`deleteHostApi(MAC, file)` в `beforeAll` перед seed (404 на свежем CI
игнорируется), так что seed всегда создаёт хост свежим и пишет «add» THIS run;
(2) per-run-unique hostname `audit-${process.pid}-${Date.now()}` — hostname-cell
рендерится verbatim, НЕ локаль-зависим (`AuditTab.vue:32`), поэтому матч MAC +
unique-hostname эксклюзивно пинит THIS run's entry. `addHostHandler` пишет
`Hostname` в audit (`handlers_hosts.go:125`). hostname валиден по
`hostnameRegex` (`main.go:79`, alnum start/end).

**P3.7 — `templates-modal.spec.ts` + `TemplatesModal.vue`.** Позиционные
`.nth(0/1/3)` хрупки: при непустом `store.dhcpRanges` `<input>` для ip_range
заменяется на `<select>`, индексы плывут. Расхождение: промт предлагал
placeholder-матчи, но placeholders name/target_file тоже **i18n-переведены**
(`templates.namePlaceholder`/`filePlaceholder`) → locale-dependent. Использован
вариант «preferred» из промта: добавлены additive `data-testid` на 4 input'а в
`TemplatesModal.vue` (`tpl-name`/`tpl-ip-range`/`tpl-hostname-pattern`/
`tpl-target-file`), spec переведён на них. Поведение компонента не изменилось
(атрибуты additive). Spec не зависит от порядка/локали.

### Трек D — endpoint coverage (P3.8)

5 новых suite'ов (порядковый учёт того, что **после `90-logout` JWT
невалиден** — restart-self/reload/events размещены до 90, не после как в промте):
- **`28-hosts-apply-template.sh`** (NEW): cleanup-delete → create template
  «Apply E2E»→`apply-e2e` → `POST /api/hosts/apply-template` → 200 + non-empty
  `.ip` → cleanup-delete. Endpoint не пишет файл (только считает).
- **`44-leases-to-static.sh`** (NEW): `POST /api/leases/to-static` с 2
  synthetic leases в свежий файл → 200 + count==2. Handler не гоняет
  `dnsmasq --test` (handlers.go:123).
- **`84-restart-self.sh`** (NEW): `POST /api/restart-self` → 200. Safe в
  ci-mode (горутина RestartSelf gated на `!*CiMode`, main.go:264).
- **`85-reload.sh`** (NEW): `POST /api/reload` → 200 OR 400 (нет dnsmasq /
  NoneCaller). `check` pass-ветка передаёт `exp=got="$S"`.
- **`86-events-sse.sh`** (NEW): `GET /api/events` (SSE), `timeout 10 curl -sN |
  head -n 5`, grep `^event:` — eventsHandler шлёт `event:arp` сразу на коннекте
  (handlers.go:239).

`GET /api/aliases` уже покрыт в P2.1 (`30-aliases-happy.sh:27`) — пропущен.

### Трек E — docs (P3.9)

`system_callers_test.go` VANITY-комментарий («цифра coverage растёт, доверие —
нет») **уже на месте** (`system_callers_test.go:19-25`). Добавлена заметка в
`tests/ROADMAP.md` в секции Gap 4 (blockquote): при рефакторинге `system.go`
полагаться ТОЛЬКО на L5 real-VM, не на `system_callers_test.go` (statement-% без
функциональной уверенности в реальных systemctl/rc-service/sv семантиках).

## Расхождения с промт-планом (исправлено по ходу)

Верификация до старта нашла неточности в 6 из 9 задач:

1. **P3.1:** прямой тест `isSafePath` **уже существовал** с prefix-collision
   case; comments (Variant A) тоже уже были. Реальный пробел — только
   doc-комментарий. Сделано минимально.
2. **P3.2:** хелпер `withDnsmasqBin` не существует — внутренний wiring зеркально
   `fakeDnsmasq`.
3. **P3.6:** action badge i18n-переведён → raw-матч «add» нерабочий; нужен
   non-locale дискриминатор (hostname) + clean-slate для local re-run robustness.
4. **P3.7:** placeholders name/target_file i18n-переведены → placeholder-матчи
   нерабочие под RU; использован `data-testid` (preferred-вариант промта),
   правка products-кода additive.
5. **P3.8:** `GET /api/aliases` уже покрыт (P2.1) — пропущен; нумерация
   suite'ов изменена (84/85/86 вместо 91/92/93), т.к. после `90-logout` JWT
   невалиден; `reload` ослаблен до 200|400 (промт указывал 200|400 тоже).
6. **P3.9:** VANITY-комментарий в `system_callers_test.go` уже на месте —
   только заметка в ROADMAP.

P3.3, P3.4, P3.5 — допущения подтверждены дословно.

## Верификация

Локально (Windows):
- `go vet ./...` — чисто.
- `go test ./... -count=1` (`INTERMASQ_SECRET` задан) — **ok** 16.066с.
- `go test ./... -race -count=1` (`CGO_ENABLED=1`) — **ok** 107.997с, без data
  races. (Первый прогон упал на `go:embed frontend/dist/*` из-за гонки с
  параллельно запущенным `vite build`, перетеревшим asset-хеши; повтор после
  завершения vite — зелёный. Продуктовый код не виноват.)
- Мутации проверены:
  - P3.1: `+string(os.PathSeparator)` → удалить → `TestIsSafePath` FAIL на
    `/etc/dnsmasq.d_evil/host.conf`. Откатано.
  - P3.2: убрать `dnsmasq --test` → `# INVALID` не детектится → тест FAIL
    (Linux-gated, структурно).
  - P3.5: изолированный bash-тест подтвердил `|| true` даёт чистый `0`, `||
    echo 0` даёт `0\n0`.
- `bash -n` на 9 тронутых/новых suite'ах — exit 0.
- `vite build` (frontend) — ok, 121 модулей, 6.80с (TemplatesModal.vue
  компилируется, `frontend/dist/` gitignored — артефактов нет).
- `npx playwright test --list` — 34 теста в 29 файлах (audit-tab + templates-
  modal парсятся).

CI (Fedora 44, оператор прогоняет отдельно):
- `BASE=http://localhost:18081 ./tests/smoke.sh` — новые suite'ы (28/44/84/85/86)
  PASS; 80-metrics A8 честный non-empty+auth_required; 11-ratelimit не флапает на
  медленном CI; 20/31 failure-сообщения с корректным `0`.
- `cd tests/e2e && npx playwright test` — audit-tab ловит writeAudit-регрессию
  (unique-hostname); templates-modal не зависит от порядка/локали (data-testid).
- `go test -run 'TestWriteConfigWithTest_StrictFakeRejectsInvalid'` — strict-fake
  ловит контент-валидацию.

## Изменённые файлы

```
 dnsmasq_test.go                                   | 18 ++++++++ (
   doc-комментарий к TestIsSafePath)
 frontend/src/components/static/TemplatesModal.vue |  8 ++-- (
   4× data-testid, additive)
 linux_test.go                                     | 87 ++++++++++++++++++++++++ (
   fakeDnsmasqStrict + TestWriteConfigWithTest_StrictFakeRejectsInvalid)
 tests/ROADMAP.md                                  |  9 ++++ (
   vanity-заметка в Gap 4)
 tests/e2e/specs/audit-tab.spec.ts                 | 45 +++++++++------- (
   unique-hostname + clean-slate)
 tests/e2e/specs/templates-modal.spec.ts           | 22 +++++---- (
   data-testid вместо .nth())
 tests/suites/11-auth-ratelimit.sh                 | 13 ++---- (
   RL_BLOCKED-aware)
 tests/suites/20-hosts-happy.sh                    |  2 +- (
   || echo 0 → || true)
 tests/suites/31-aliases-bugs.sh                   |  2 +- (
   || echo 0 → || true)
 tests/suites/80-metrics.sh                        | 13 ++---- (
   A8 honest regression)
 tests/suites/28-hosts-apply-template.sh           | (NEW, ~30 строк)
 tests/suites/44-leases-to-static.sh               | (NEW, ~16 строк)
 tests/suites/84-restart-self.sh                   | (NEW, ~12 строк)
 tests/suites/85-reload.sh                         | (NEW, ~18 строк)
 tests/suites/86-events-sse.sh                     | (NEW, ~17 строк)
 10 modified + 5 new files; ~250 insertions
```

## Что осталось (вне этой сессии)

- **Продуктовый security-аудит** (вне тестовых промтов): JWT alg-confusion в
  `auth.go:214`, plugin trust boundary в `main.go:131-193`, X-Forwarded-For в
  `rateLimitMiddleware`, `hash, _ :=` в `handlers.go:47`.
- **A15** — KNOWN-CONDITIONAL на dnsmasq 2.80 по решению оператора.
- **v1.0 release** — CHANGELOG.md / README (последние unchecked чекбоксы в
  `tests/ROADMAP.md:199-200`).
