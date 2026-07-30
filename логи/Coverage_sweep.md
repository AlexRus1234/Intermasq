# Coverage sweep — промт на поднятие Go statement-coverage (A → B → C → D)

**Назначение:** самодостаточный промт для ИИ-ассистента (или будущего себя),
чтобы поднять `go test -cover ./...` с ~66% до ~95-98% в 4 категориях по
убыванию ROI. По формату — продолжение `логи/Hardening_sweep.md`. Прочитай
ЦЕЛИКОМ перед стартом. **Контекст и карта покрытия уже внутри** — не
перечитывай исходники и не гоняй `cover` без нужды (см. §7 «Экономия токенов»).

---

## 0. Проект и точка старта

**Intermasq** — веб-панель для dnsmasq. Backend Go 1.25 (gin), frontend
Vue 3, embed через `go:embed`. Репо `B:\Repo\Intermasq\Intermasq`, ветка
`main`. CI — Forgejo Actions, контейнер `fedora:44`, runs as root, Go 1.26
tarball, npm/go/rpm через прокси Nora. Один package `main` → L1/L2 делят
одну цифру coverage.

**Coverage на старте (после Hardening sweep 2026-07-29):** **66.0%** локально
(Windows), ~66-68% на CI Linux. Замер:
```powershell
$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXXXX"
go test "./..." -count=1 -coverprofile coverage.out
go tool cover -func coverage.out     # ВАЖНО: через пробел, не -func=file (PowerShell режет «=»)
```
CI Linux даёт чуть выше (там бегают dnsmasq-зависимые тесты, что skipped на Windows).

**Цель сессии:** закрыть 4 категории по порядку ROI: A (pure unit) → B
(Linux+dnsmasq ветки) → C (рефакторинг bootstrap/горутин) → D (fake-init
бинарники). Реалистичный потолок **~95-98%**; 100% требует subprocess-тестов
для `os.Exit`-веток и даёт околонулевую ценность (см. §6).

---

## 1. ЖЁСТКИЕ ограничения

1. **Перед пушем ЛЮБОЙ Go-правки — `go vet ./...` ОБЯЗАТЕЛЬНО** (не только
   `go build`). CI режет unreachable code после ранних `return`; локальный
   `go build` это не ловит (был красный прогон).
2. **Локально — только `gofmt -l` и `go vet`/`go test`** (оператор на Windows).
   Smoke/Playwright/`-fuzz` — только CI. Не запускай серверные/CI-only вещи
   локально и не пытайся их починить «под Windows».
3. **Не ломай существующие тесты.** После каждой категории:
   `$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXXXX"; go test "./..." -count=1`.
4. **Продуктовые правки — рефакторинг разрешён** (в отличие от Hardening
   sweep): оператор явно разрешил «все средства хороши». НО: **не меняй
   публичное поведение** — только extract в тестируемые функции + injection
   seams. Сохрани сигнатуры хендлеров и маршруты.
5. **Linux-gated тесты** (dnsmasq/fake-binary) —_guard `if dnsmasqBin()==""`
   или `runtime.GOOS=="windows"` → `t.Skip`. На CI Linux они бегут.
6. **Fake-бинарники** — НЕ тащи фреймворк. пиши tmp shell-скрипты через
   `os.WriteFile` + `os.Chmod(...,0755)` на Linux. На Windows skip.
7. **Коммиты — после `go vet` + `go test` зелёного.** Пуш — по просьбе
   оператора. CI подтверждает.
8. **Синхронизируй доки в конце:** `tests/ROADMAP.md` (метрика coverage ≥70%
   тикни по достижении), session-лог `логи/coverage-sweep.md`.

---

## 2. Карта непокрытого (из `cover -func`, базис для приоритизации)

Формат: `файл:строка  функция  тек.покрытие  →  категория`.

### Категория A — pure unit (дёшево, ~+5-8%, высокая ценность)
```
aliases.go:298      ensureAliasesFile        0.0%    → тест temp-файла
handlers_safety.go:128  coalesce             0.0%    → table-test
metrics.go:131      writeLabeledMetric       0.0%    → пишем в httptest
metrics.go:60       boolToFloat             66.7%    → edge (true/false/...)
metrics.go:96       checkMetricsAuth        46.7%    → Bearer/X-API-Key/нет
dnsmasq.go:203      parseIPTransform        30.4%    → table (octet N, +N, -N, set)
dnsmasq.go:240      apply                   39.1%    → apply к IP, все трансформы
handlers_hosts.go:136  validateHostTags     28.6%    → set:/tag:/invalid/empty
handlers_hosts.go:151  normalizeHostTags    44.4%    → trim/lower/dedup
config_snapshot.go:200  isLeaseTime         60.0%    → lease-time/infinite/мусор
config_snapshot.go:354  directiveGroup      80.0%    → dns/dhcp/alias/unknown
templates.go:29     loadTemplates          60.0%    → нет файла / битый JSON / ок
templates.go:47     saveTemplates          75.0%    → write error path
bins.go:46          resolveBin             63.6%    → flag/LookPath/fallback/нет
bins.go:67          isExecutable           75.0%    → нет/директория/без бита/с битом
bins.go:101..125    sudoBin/systemctlBin/serviceBin/rcServiceBin/svBin  0.0%  → см. D (var-seam)
```
**Куда писать:** pure-функции домена — рядом с существующими тестами того же
домена: `parseIPTransform/apply`, `isLeaseTime`, `directiveGroup`, bins →
**новый `bins_test.go`**; aliases → `dnsmasq_test.go` (там уже
`parseAliasLine`-тесты, строка ~458); `coalesce`/`validateHostTags`/
`normalizeHostTags`/metrics → **`handlers_test.go`** (или новый
`metrics_test.go`); templates → новый `templates_test.go` или `dnsmasq_test.go`.

### Категория B — Linux + dnsmasq binary (CI уже гоняет, ~+3-5%)
0% на Windows, на CI частично покрыто; нужно добить success-ветки:
```
dnsmasq.go:89       writeConfigWithTest       0.0%   → success (dnsmasq --test ok) + rollback
history.go:229      restoreHistoryVersion     0.0%   → success restore + dnsmasq --test
handlers.go:95      reloadHandler             0.0%   → 200/400 (reloadDnsmasq ok/fail)
sse.go:109          reloadDnsmasq             0.0%   → ok/fail (fake dnsmasq через dnsmasqBinPath)
handlers_config.go:221  putFileHandler       20.0%   → success write path
handlers_config.go:22   updateConfigHandler  50.0%   → success serialize+test
handlers_safety.go:147  restoreBackupHandler 18.2%   → success unzip+restore
handlers_safety.go:100  historyRestoreHandler 50.0%  → success restore
```
**Seam для dnsmasq-путей:** переменная `dnsmasqBinPath` (`bins.go`) —
записываемая. `dnsmasqBin()` возвращает её как есть, если не пустая
(`bins.go:96`: `if dnsmasqBinPath=="" {resolveBins()}; return dnsmasqBinPath`).
В Linux-тесте: положи fake `dnsmasq` скрипт (echo/plausible), `dnsmasqBinPath =
tmp/dnsmasq`, `chmod 0755` → success-ветки покрыты без реального dnsmasq.
**guard:** `if runtime.GOOS=="windows" {t.Skip()}` или `if dnsmasqBin()=="" ...`
(но раз ты set var — guard по GOOS).

### Категория C — рефакторинг bootstrap/горутин (~+8-12%, средняя ценность)
**Разрешён рефакторинг (extract + injection), без смены поведения.**

```
main.go:183  main()               0%   → extract setupServer() (*gin.Engine, error)
main.go:119  loadPlugins          0%   → override PluginsDir/SocketsDir + fake plugin
main.go:107  init()               20%  → os.Exit-ветка только subprocess-тестом
system.go:244  detectInitSystem   0%   → inject путь /proc/1/comm (package var)
sse.go:73    startSSEBroadcaster  0%   → extract pollOnce(), тест одной итерации
metrics.go:144 startDNSHealthChecker 0% → extract runDNSHealthPass уже есть, inject resolver
metrics.go:156 runDNSHealthPass   0%   → stub resolver (test server) или inject func
auth.go:51   cleanBlacklistLoop   62%  → extract cleanupOnce(), тест
```
**Конкретные реfactоринги:**
- **`main()`**: вынести всё между `flag.Parse()` и `r.Run()` в
  `func setupServer() (*gin.Engine, error)`. `main()` остаётся = `setupServer()`
  + `r.Run()` + `os.Exit`. Тест: `setupServer()` → assert routes registered,
  `loadPlugins`/`startSSEBroadcaster` вызваны (через флаги/stub). Строки
  `r.Run(...)` и `os.Exit(1)` остаются непокрытыми (E).
- **`loadPlugins`**: `PluginsDir`/`SocketsDir` — package vars (`main.go:69-70`),
  перезаписываемы. Тест на Linux: tmp PluginsDir с `manifest.json` + маленький
  бинарник (можно собрать fixture `tests/fixtures/plugins/hello/` или trivial
  `sleep`-скрипт), вызвать `loadPlugins(gin.New())` → assert маршрут
  `/plugins/<id>/*` зарегистрирован, `loadedPlugins` пополнен. Покроет ~90%.
- **`detectInitSystem`**: заменить хардкод `"/proc/1/comm"` на package var
  `var procOneCommPath = "/proc/1/comm"`. Тест: write tmp file с `systemd\n` /
  `runit\n` / `init\n` (+ наличие rc-service) / `пусто` → assert возвращаемое.
- **Горутины**: `startSSEBroadcaster` (sse.go:73) — вынести тело одной итерации
  в `func ssePollOnce() (arpJSON string, status bool)`; тест вызывает её с
  mock-ами. `runDNSHealthPass` — inject `resolver` (параметр-функция
  `func(domain string) ([]string, error)`), в тесте stub.
- **`init()` os.Exit**: отдельный subprocess-тест (см. E) — ОПЦИОНАЛЬНО.

### Категория D — system.go init-callers через fake-бинарники (~+10-15%, VANITY)
**Внимание: это «тщеславное» покрытие — проверяет exec-wiring против моков, не
реальный systemd. Цифра растёт, уверенность — нет. Реальная проверка = Gap 4
(L5 VM), вне этой сессии. Делать ПОСЛЕДНЕЙ.**
```
system.go:37/48/58   SystemdSystemCaller {IsActive,Restart,RestartSelf}     0%
system.go:77/83/88   SystemdUserCaller                                       0%
system.go:101/112/122 OpenRCCaller                                           0%
system.go:144/156/167 RunitCaller                                            0%
system.go:189/199/209 SysVinitCaller                                         0%
system.go:277  detectSystemCaller    0%   (os.Getuid + пробы через bin vars)
system.go:342  initSystemCaller      0%   (тонкая обёртка)
```
**Seam (БЕЗ правки system.go):** все callers зовут `sudoBin()`/`systemctlBin()`/
`serviceBin()`/`rcServiceBin()`/`svBin()` — а те читают package vars
(`bins.go:30-35`). **В тесте**: пишем fake shell-скрипт под каждый
(`systemctl` печатает `active`; `service` exit 0; и т.д.), ставим соотв.
`*BinPath` var в tmp-путь, `chmod 0755`, вызываем метод → покрыто. guard по
`runtime.GOOS=="windows"`. Покроет ~95% system.go на Linux CI.
- `String()`-методы каждого caller (system.go:68/93/132/178/219) — pure,
  покрываются тривиально без fakes.
- `detectSystemCaller`/`initSystemCaller` — после fake bins + после C-рефака
  `procOneCommPath` становятся тестируемыми.

---

## 3. Задачи (исполняемый список, по порядку A → D)

Для каждой: файл(ы) → корень → фикс/тест → knock-on. **После каждой задачи —
`go vet` + `go test` + коммит.** Пуш по просьбе оператора.

### T-A. Pure unit (Категория A)
По списку §2.A. Каждый — table-driven, инварианты из doc-комментариев функций.
Особое:
- **`bins_test.go` (НОВЫЙ):** `TestResolveBin` (flag-executable / flag-not→PATH /
  LookPath / fallback / ""), `TestIsExecutable` (нет/директория/exec-бит/без).
  Внимание: `resolveBins()` под `sync.Once` — НЕ тестируй его повторно; тестируй
  `resolveBin` напрямую (он pure). Для ленивых accessor'ов (sudoBin...) —
  соотнеси с T-D (var-seam).
- **`parseIPTransform`/`apply`** — заложены в `IPTransformSpec` (dnsmasq.go ~190):
  префикс `10.0`/`10`/полный; сдвиги. Проверь round-trip: apply(parse(s))==s.
- **`validateHostTags`/`normalizeHostTags`** — `dhcpTagRegex` (main.go:85):
  `set:foo`/`tag:bar` ok; `id:..`/пусто/дубли → reject/normalize.
- Knock-on: все pure — риск нулевой.

### T-B. Linux+dnsmasq ветки (Категория B)
По списку §2.B. Через `dnsmasqBinPath` seam + fake-dnsmasq скрипт. guard GOOS.
Особое:
- fake `dnsmasq`: exit 0 на `--test` (success), exit 1 на `--test` с
  определённым conf (simulate fail) → проверь rollback (`rollbackFile`).
- `reloadDnsmasq` (sse.go:109) — посмотри сигнатуру (она вызывает sysCaller +
  dnsmasq); может потребоваться `sysCaller = &NoneCaller{}` + fake dnsmasq.

### T-C. Рефакторинг bootstrap/горутин (Категория C)
По списку §2.C. **Каждый рефакторинг — отдельный коммит** (blast radius).
Порядок внутри: `detectInitSystem` (inject path) → горутины (extract) →
`loadPlugins` (fake dir) → `setupServer()` (extract из main) → (опц.)
subprocess-`init`. После каждого — `go vet` + `go test`.

### T-D. Fake-init бинарники (Категория D, VANITY)
По списку §2.D. Новый `system_test.go`. Один хелпер `fakeBin(t, name, script)`,
который пишет скрипт и выставляет соотв. `*BinPath` var; `t.Cleanup` сбрасывает
в "". Table по 5 callers × {IsActive,Restart,RestartSelf} × {sudo, no-sudo}.
**Делать только если оператор явно хочет цифру >90%.** Обязательно пометить в
логе: «vanity-покрытие, реальная init-проверка = Gap 4».

---

## 4. Верификация (после каждой задачи + финальная)

**Локально (Windows):**
```powershell
gofmt -l <изменённые .go>                    # пусто
go vet ./...                                  # чисто (ОБЯЗАТЕЛЬНО)
$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXXXX"
go test "./..." -count=1                      # зелёный (Linux-gated skip'аются)
go test "./..." -count=1 -coverprofile coverage.out
go tool cover -func coverage.out              #gap закрылся
```
**CI (Fedora 44):** Linux-gated тесты (B, C-fake, D) бегут; дефолтный прогон
зелёный, smoke 0 Fail/0 Known-fail, opt-in L4 не трогается.

---

## 5. Приёмка (DoD)

- [ ] **A:** pure-функции из §2.A покрыты (≥90% каждая); +~5-8% к coverage.
- [ ] **B:** dnsmasq-зависимые success-ветки (§2.B) покрыты на CI Linux.
- [ ] **C:** `main()` → `setupServer()`; `detectInitSystem` inject; горутины
      extract; `loadPlugins` тестируем через fake dir. ~+8-12%.
- [ ] **D (опц.):** system.go callers через fake bins; помечено как vanity.
- [ ] `go vet`/`go test` зелёные; существующие тесты не сломаны.
- [ ] `tests/ROADMAP.md`: метрика coverage ≥70% тикнута (и ≥80% при достижении).
- [ ] Session-лог `логи/coverage-sweep.md` (по-категорийно: фикс → verify → delta %).

---

## 6. Что ВНЕ области / ~100%

- **`r.Run(":port")` в `main`** — блокирующая строка; только run-in-goroutine+kill
  (хрупко). Обычно exclude.
- **`os.Exit`-ветки** (`init` fatal, `main` Run-fail) — только subprocess-тестом
  (`os/exec` реинвок бинарника с env-switch). Ок. +1-2%, много усилий.
- **`docs/docs.go init`** — swagger embed; тривиально, можно exclude или
  no-op тест.
- **Gap 4 (L5 Real VM nightly)** — реальная init/dnsmasq проверка; отдельная
  задача, даёт уверенность, не statement-%.
- **Enterprise** (mutation testing Go, compat-matrix dnsmasq, cross-distro,
  browser-matrix) — post-v1.0.

**Вердикт:** реалистичный потолок **~95-98%**. 100% — ценой subprocess-тестов
для ~2% кода с околонулевой ценностью. Гнать к ~85-90% (A+B+C) — оптимально.

---

## 7. Экономия токенов (для новой сессии)

- Этот файл **уже содержит** карту `cover -func` (§2) и все seams. НЕ гоняй
  `cover` и НЕ читай `system.go`/`main.go`/`bins.go` целиком до того, как
  начнёшь соответствующую категорию.
- Читай исходник **только** той функции, которую сейчас тестируешь (Read по
  `file:line` с узким offset/limit). Остальное — инварианты из doc-комментариев.
- После каждой категории — ОДИН замер `cover` (команды §4), сохрани дельту в
  лог, дальше работай по карте.
- PowerShell-quirks: `go test "./..."` (кавычки обязательно); `go tool cover
  -func coverage.out` (через пробел); `>` в PowerShell пишет UTF-16 → не
  перенаправляй `cover` через `>` в файл для чтения, гоняй в stdout.
- Не используй `Select-Object`/`Get-Content` для больших go-выводов — bash-
  инструмент сам поймает stdout.
- Session-лог пиши incrementally, не перечитывая логи прошлых sweep'ов
  (bugfix/hardening) — они не нужны для coverage-задач.
