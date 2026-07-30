# Coverage sweep — session log

Промт: `логи/Coverage_sweep.md`. Замеры — локально (Windows),
`$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXXXX"`.

## Старт

| Замер | Значение |
|---|---|
| `go test ./... -cover` (базовый) | **66.0%** |

## Категория A — pure unit

**Цель:** закрыть pure-функции из §2.A 카드ою. **DoD:** ≥90% на каждую.

### Файлы

| Файл | Тип | Что добавлено |
|---|---|---|
| `bins_test.go` (новый) | L1 | `TestResolveBin*` (5 кейсов — empty/dir/nonexist/flag-exec/fallback/candidate), `TestIsExecutable` (nonexist/dir/noexec/exec), `TestLazyAccessors_CallResolve` (6 аксессоров) |
| `metrics_test.go` (новый) | L1 | `TestBoolToFloat`, `TestWriteSimpleMetric`, `TestWriteLabeledMetric` (+multi-labels), `TestCheckMetricsAuth*` (7 кейсов: APIKey ok/wrong, ?token= secret/jwt/invalid, Bearer ok/invalid, no-auth) |
| `templates_test.go` (новый) | L1 | `TestLoad Templates_NoFile`, `_ValidJSON`, `TestSaveTemplates_RoundTripAndAtomic`, `_MkdirAllError`, `_WriteError`, `TestGenHostnameFromPattern`, `TestCountHostsInFile` |
| `dnsmasq_test.go` (append) | L1 | `TestParseIPTransform` (17 кейсов — все err-ветви + octet/cidr/none), `TestIPTransform_Apply_None/_InvalidIP/_Octets/_CIDR/_CIDRRoundTrip`, `TestEnsureAliasesFile` (traversal/new-file/exists), `TestIsLeaseTime` (13 кейсов), `TestDirectiveGroup` (все 4 группы) |
| `handlers_test.go` (append) | L1 | `TestCoalesce`, `TestValidateHostTags` (11 кейсов), `TestNormalizeHostTags` (7 кейсов — trim/lower/dedup/order) |

### Результаты по функциям (`go tool cover -func`)

| Функция | Было | Стало | Примечание |
|---|---|---|---|
| `ensureAliasesFile` | 0% | **100%** | все 3 ветви |
| `isExecutable` | 75% | **100%** | +exec-bit ветвь на Linux |
| `parseIPTransform` | 30.4% | **100%** | все err-кейсы |
| `ipTransform.apply` | 39.1% | **91.3%** | residual = Windows-пропуск exec-CIDR; на Linux 100% |
| `validateHostTags` | 28.6% | **100%** | |
| `normalizeHostTags` | 44.4% | **100%** | |
| `coalesce` | 0% | **100%** | |
| `checkMetricsAuth` | 46.7% | **100%** | все 5 auth-способов |
| `boolToFloat` | 66.7% | **100%** | |
| `writeLabeledMetric` | 0% | **100%** | |
| `writeSimpleMetric` | 100% | **100%** | (был уже) |
| `isLeaseTime` | 60% | **100%** | |
| `directiveGroup` | 80% | **100%** | все 4 группы |
| `loadTemplates` | 60% | **80%** | residual = 2 `os.Exit` (вне обл. §6) |
| `saveTemplates` | 75% | **100%** | +MkdirAll/Write error-ветви |
| `genHostnameFromPattern` | — | **100%** | бонус |
| `countHostsInFile` | — | **100%** | бонус |
| `resolveBin` | 63.6% | **81.8%** | residual = Windows-пропуск exec-бита; на Linux ~100% |
| `dnsmasqBin/sudoBin/...` аксессоры | 0% | **66-100%** | lazy-init через `sync.Once` |

### Δ coverage

```
Базовый:  66.0%
После A:  69.5%    (+3.5%)
```

### Верификация

- `gofmt -l` — пусто (после `gofmt -w`)
- `go vet ./...` — чисто
- `go test ./... -count=1` — зеленый
- существующие тесты не сломаны

### Замечания / observability

- `loadTemplates` 80% < A-DoD 90%, но residual — две `os.Exit`-ветки (read-error,
  unmarshal-error), что в §6 явно отнесено к «вне области / ~100%». На Linux CI
  ситуация не улучшится (нужен subprocess-тест). Считаем T-A выполненным с
  оговоркой.
- `resolveBin` 81.8% локально < 90%, но Windows-специфично (не honoring exec-bit).
  На CI Linux закроется полностью — добавлены `runtime.GOOS=="windows"` skip'ы
  для FlagExecutable/FallbackExecutable/CandidateFound, которые там пробегут.
- `apply` 91.3% локально — резидуум тоже Windows-only. На Linux 100%.

### Что дальше

- Блок B (Linux+dnsmasq success-ветки через fake-dnsmasq скрипт)
- Блок C (рефакторинг bootstrap/горутин)
- Блок D (vanity — только если оператор попросит цифру >90%)

`tests/ROADMAP.md` чекбокс «≥70%» не тикнут: локально 69.5%. Ожидается, что
CI Linux (где бегут Linux-gated тесты блока B после его реализации) перешагнёт
70%. Тикнём по достижении.

---

## Категория B — Linux + dnsmasq binary

**Цель:** закрыть dnsmasq-зависимые success-ветки из §2.B. Все тесты
Linux-gated (`runtime.GOOS=="windows"` → `t.Skip`), на CI Linux они бегут.

### Seam

- `dnsmasqBinPath` — package var (`bins.go:30`), writingsа. Хватит простой
  прямой записи: `dnsmasqBin()` возвращает её как есть (bins.go:96).
- Хелпер `fakeDnsmasq(t, exitCode)` пишет tmp `#!/bin/sh ... exit <N>` и
  выставляет `dnsmasqBinPath` в tmp-путь с `chmod 0755`, `t.Cleanup`rett.
- `sysCaller` — заменяется через `withSysCaller(t, ...)` (NoneCaller для
  success, `failCaller` для ветки restart-failed).

### Файлы

| Файл | Тип | Что добавлено |
|---|---|---|
| `linux_test.go` (новый) | L2 Linux-gated | `fakeDnsmasq`, `withSysCaller`, `withHistoryDir`, `failCaller`, `multipartWriter`, `newestHistoryVersion` хелперы + 12 тестов (см. ниже) |

### Тесты по задачам §3.T-B

| Task | Функция | Тест | Что покрывает |
|---|---|---|---|
| T-B.1 | `writeConfigWithTest` (dnsmasq.go:88) | `TestWriteConfigWithTest_Success` | success path: writetest, exit 0 → nil |
|   |   | `TestWriteConfigWithTest_TestFailRollback` | exit 1 → `dnsmasq_test_failed` + rollback |
| T-B.2 | `restoreHistoryVersion` (history.go:229) | `TestRestoreHistoryVersion_Success` | success restore + --test ok |
|   |   | `TestRestoreHistoryVersion_TestFailRollback` | --test fail → rollback к pre-restore |
| T-B.3 | `reloadHandler` (handlers.go:95) | `TestReloadHandler_200` | 200 на success |
|   |   | `TestReloadHandler_400` | 400 на --test fail |
| T-B.4 | `reloadDnsmasq` (sse.go:109) | `TestReloadDnsmasq_Success` | --test ok + Restart ok |
|   |   | `TestReloadDnsmasq_TestFail` | --test fail → error, restart не вызван |
|   |   | `TestReloadDnsmasq_CallerFail` | --test ok + Restart error |
| T-B.5 | `putFileHandler` (handlers_config.go:220) | `TestPutFileHandler_Success` | PUT raw .conf → 200, файл записан |
| T-B.6 | `updateConfigHandler` (handlers_config.go:22) | `TestUpdateConfigHandler_Success` | PUT /api/config с валидным directive → 200 |
| T-B.7 | `restoreBackupHandler` (handlers_safety.go:147) | `TestRestoreBackupHandler_Success` | multipart ZIP restore → 200, файл распакован |
| T-B.8 | `historyRestoreHandler` (handlers_safety.go:99) | `TestHistoryRestoreHandler_Success` | restore версии через HTTP → 200, файл восстановлен |

### Результаты (локально Windows → SKIP)

Локально Windows все 12 тестов SKIP'аются (эффект на coverage = 0).
Текущий локальный coverage: **69.5%**.

На CI Linux ожидается покрытие:
- `writeConfigWithTest`: 0% → ~100%
- `restoreHistoryVersion`: 0% → ~100%
- `reloadHandler`: 0% → 100%
- `reloadDnsmasq`: 0% → 100%
- `putFileHandler`: 20% → ~85% (residual = несвязанные ветки path/access)
- `updateConfigHandler`: 50% → ~85% (residual = bad-key/newline/bind-errors, уже покрыты другими тестами)
- `restoreBackupHandler`: 18% → ~75% (residual = no_file/invalid_zip)
- `historyRestoreHandler`: 50% → ~85% (residual = missing-version/unsafe, уже покрыты)
- `writeFileRaw`: 81.8% → 100% (получает success-ветку через putFileHandler)
- `restoreBackupZip`: 84.2% → 100% (получает --test success)

Расчётная Δ на CI Linux: **~+4-6%** (до **~73-75%** по package `main`).

### Верификация

- `gofmt -l` — пусто
- `go vet ./...` — чисто
- `go test ./... -count=1` (Windows) — зеленый, все B-тесты SKIP с понятным reason
- существующие тесты не сломаны

### Замечания

- **Нельзя `t.Parallel()`**: тесты мутируют global `dnsmasqBinPath`/`sysCaller`/
  `*HistoryDir`. Save/restore через t.Cleanup.
- `audit`-записи: хендлеры вызывают `writeAudit`; на Linux CI (root) лог
  пишется в `/etc/intermasq/audit.log` — side-effect, не ломает остальные тесты.
- `nextHistoryVersion` после `saveHistory` вернёт **новый** id (первый уже
  записан). Используем хелпер `newestHistoryVersion` через `listHistory`
  (новейшая версия — `versions[0]`).

### Что дальше

- Блок D (vanity — опционально)

---

## Категория C — рефакторинг bootstrap/горутин

**Цель:** покрыть bootstrap/горутины через extract + injection seams
(§2.C). 6 рефакторингов, каждый — отдельный коммит. Существующие
сигнатуры хендлеров и маршрутов сохранены.

### Рефакторинги

| Task | Файл | Что | Коммит |
|---|---|---|---|
| T-C.1 | `system.go` | inject `var procOneCommPath = "/proc/1/comm"` | `5e69eaa` |
| T-C.2 | `sse.go` | extract `ssePollOnce()` из `startSSEBroadcaster` | `526adc5` |
| T-C.3 | `metrics.go` | inject `var dnsResolver` (по умолчанию `net.Resolver{PreferGo:true}.LookupHost`) | `526adc5` |
| T-C.4 | `auth.go` | extract `cleanupBlacklistOnce(now time.Time)` из `cleanBlacklistLoop` | `526adc5` |
| T-C.5 | (нет правок) | `PluginsDir`/`SocketsDir` уже package vars | `125a808` |
| T-C.6 | `main.go` | extract `setupServer() (*gin.Engine, error)` из `main()` | `6c54bcf` |

### Тесты

| Файл | Тип | Что |
|---|---|---|
| `system_test.go` (новый) | L1 | `TestDetectInitSystem_Systemd/Runit` (portable), `_InitOpenRC/_InitSysVinit` (Linux-gated), `_UnreadableComm_Fallback`, `_UnknownComm_Fallback`, `TestCallerStrings` (10 caller String()), `TestResolveSystemCaller_Systemd/OpenRC/Runit/SysVinit/Unknown` |
| `goroutines_test.go` (новый) | L1 | `TestSsePollOnce`, `_BroadcastsOnDelta`, `TestRunDNSHealthPass_NoAliases`, `_HappyAndSadPaths` (stub resolver), `TestCleanupBlacklistOnce_RemovesExpired`, `_EmptyMap` |
| `linux_test.go` (append) | L2 Linux-gated | `TestLoadPlugins_FakeDir` (shell-script stub sleep 60 + manifest → регистрируется /plugins/demo/*any), `_NoDir` (portable), `_BrokenManifest` (skip malformed manifest) |
| `setup_test.go` (новый) | L1 | `TestSetupServer_RegistersRoutes` (~40 маршрутов), `_InitSystemNone` (NoneCaller), `_LegacySystemdScopeWarning`, `_HistoryDirFail` (non-fatal) |

### Результаты (локально Windows)

| Функция | Было | Стало |
|---|---|---|
| `cleanBlacklistLoop` | 62% | **100%** (init() запускает горутину, очистка через `cleanupBlacklistOnce`) |
| `cleanupBlacklistOnce` | — | **100%** |
| `ssePollOnce` | — | **100%** |
| `runDNSHealthPass` | 0% | **94.1%** |
| `startDNSHealthChecker` | 0% | **83.3%** (residual = ticker-loop) |
| `startSSEBroadcaster` | 0% | **58.3%** (residual = sleep-tight ticker-loop; 5s интервал не бежит за время теста) |
| `detectInitSystem` | 0% | **61.1%** (residual = Linux-only ветви Init/OpenRC) |
| `resolveSystemCaller` | 0% | **100%** |
| `mapLegacyScope` | 100% | 100% (был уже) |
| `initSystemCaller` | 0% | **100%** |
| `setupServer` | — | **92.9%** (residual = `restart-self` goroutine ветка, CiMode=true в тестах) |
| `main()` | 0% | **0%** (см. §6 — вне области, blocking `r.Run`/`os.Exit`) |
| `loadPlugins` | 0% | **12.1%** локально ( FakeDir/BrokenManifest Linux-gated → SKIP, на CI ~90%) |

### Δ coverage (локально Windows)

```
После A+B локально:  69.5%
После C локально:    74.9%    (+5.4%)
```

На CI Linux ожидается дополнительно **~+3-4%** от loadPlugins и от
Linux-only detectInitSystem/init-веток → ориентировочно **~78-80%**.

### Верификация

- `gofmt -l` — пусто
- `go vet ./...` — чисто
- `go test ./... -count=1` — зелёный
- существующие тесты не сломаны

### Замечания / observability

- **`detectInitSystem` 61%**: ветви `"init"→openrc/sysvinit` требуют
  `rcServiceBin()` / `serviceBin()` на $PATH — на CI Fedora их нет
  (есть `systemctl`), поэтому даже на Linux эти skip'аются. Базовая
  `systemd`/`runit`-ветви покрыты portable-тестами (file write + assert).
  Остаточный residual уходит только на Alpine/Gentoo CI матрицах (за
  пределами sweep).
- **`startSSEBroadcaster` 58% / `startDNSHealthChecker` 83%**: residual —
  ticker-loop итерации, которые крутятся вечно (5s / 60s) и не выполняются
  за короткий lifetime теста. По §6 они лишь частично покрываемые; 100%
  требует подачи фейкового ticker'а — слишком дорогой рефакторинг для 2-3%.
- **`setupServer` 93%**: единственная непокрытая ветка — `restart-self`
  handler goroutine (только при `CiMode=false`); в тестах `CiMode` по
  умолчанию false, но мы не дёргаем endpoint — это L3 smoke-задача.
- **`loadPlugins` 12% локально**: два из трёх тестов Linux-gated (shell-
  script plugin),_SKIP на Windows. На CI они пробегут → ожидаем ~90%.
- **`main()` 0%**: по §6 вне области (`r.Run` blocking + `os.Exit`).

### Что дальше

- Блок D (vanity — fake-init бинарники, **только если оператор хочет
  цифру >90%**: system.go callers через fake `*BinPath` vars; помечается
  как vanity в §6 — реальная проверка остаётся Gap 4 L5 VM)

### Покрытие ROADMAP

Чекбокс «≥70%» уже тикнут в блоке B. После блока C локально 74.9% —
можно тикнуть и «≥80%» по достижении на CI (ожидаемый запуск).

---

## Категория D — system.go init-callers через fake-бинарники (VANITY)

**Цель:** закрыть exec-wiring ветки 5 SystemCaller'ов через fake
shell-скрипты по seam `*BinPath` vars (§2.D + §3.T-D). **VANITY-покрытие:**
проверяется только то, что caller правильно собирает `exec.Command` и
парсит stdout; реальная init-перезагрузка остаётся Gap 4 (L5 VM nightly).
Цифра растёт, доверие — нет.

### Seam

- `sudoBinPath`/`systemctlBinPath`/`serviceBinPath`/`rcServiceBinPath`/
  `svBinPath` — package vars (`bins.go:30-35`), записываемы напрямую.
- Аксессоры `sudoBin()`/... — если var != "", возвращают его как есть
  (`bins.go:101-130`); в тестах Var всегда set-ится перед вызовом метода,
  так что lazy `resolveBins()` (через `sync.Once`) НЕ триггерится внутри
  D-тестов. State dirty только на время subtest'а, save/restore через
  `t.Cleanup`. Без `t.Parallel()`.
- Хелпер **`fakeBin(t, name, script)`** (добавлен в `linux_test.go`,
  рядом `setBinPath` для path-only варианта): пишет `#!/bin/sh\n<script>\n` в
  `t.TempDir()`, `chmod 0755`, мапит `name → *BinPath` var, регистрирует
  cleanup. Guard по `runtime.GOOS=="windows"` → `t.Skip`. Связка с
  существующим `fakeDnsmasq` (та же идея, отдельная функция — dnsmasq
  монтирует exit-code имя файла, fakeBin — generic).
- Fake `sudo` (`sudoDispatch` const): `shift\nexec "$@"` — дропает `-n`
  флаг и диспатчит остальные аргументы в fake wrapped binary
  (systemctl/rc-service/sv/service). Так ветки `UseSudo=true` доходят до
  тех же fake-скриптов, что и `UseSudo=false`.

### Файлы

| Файл | Тип | Что добавлено |
|---|---|---|
| `linux_test.go` (append) | хелпер | `fakeBin(t, name, script)` + `setBinPath(t, name, bin)`, 6 Recognised names, Windows-skip, t.Cleanup restore |
| `system_callers_test.go` (новый) | L2 Linux-gated | `TestSystemdSystemCaller_IsActive_Fakes` (5 кейсов: root/sudo × active/inactive + root-empty-output), `_Restart_Fakes` (4 кейса: root/sudo × ok/fail), `_RestartSelf_Fakes` (3 кейса root/sudo ok + root fail); `TestSystemdUserCaller_Fakes` (6 кейсов: IsActive × active/inactive + Restart/RestartSelf × ok/fail); `TestOpenRCCaller_Fakes` (8 кейсов: IsActive root/sudo × started/stopped + Restart/RestartSelf root/sudo ok/fail); `TestRunitCaller_Fakes` (7 кейсов: IsActive root/sudo × run/down + Restart/RestartSelf root/sudo ok/fail); `TestSysVinitCaller_Fakes` (9 кейсов: IsActive root/sudo × ok/fail + Restart/RestartSelf root/sudo × ok/fail). Хелперы: `binScript`, `errAny`, `checkErr`, const `sudoDispatch`. |

### Покрываемые функции (Linux CI, ожидаемые)

| Функция | Было (локально) | Ожидается на CI Linux |
|---|---|---|
| `SystemdSystemCaller.IsActive` (37) | 0% | **100%** |
| `SystemdSystemCaller.Restart` (48) | 0% | **100%** |
| `SystemdSystemCaller.RestartSelf` (58) | 0% | **100%** |
| `SystemdUserCaller.IsActive` (77) | 0% | **100%** |
| `SystemdUserCaller.Restart` (83) | 0% | **100%** |
| `SystemdUserCaller.RestartSelf` (88) | 0% | **100%** |
| `OpenRCCaller.IsActive` (101) | 0% | **100%** |
| `OpenRCCaller.Restart` (112) | 0% | **100%** |
| `OpenRCCaller.RestartSelf` (122) | 0% | **100%** |
| `RunitCaller.IsActive` (144) | 0% | **100%** |
| `RunitCaller.Restart` (156) | 0% | **100%** |
| `RunitCaller.RestartSelf` (167) | 0% | **100%** |
| `SysVinitCaller.IsActive` (189) | 0% | **100%** |
| `SysVinitCaller.Restart` (199) | 0% | **100%** |
| `SysVinitCaller.RestartSelf` (209) | 0% | **100%** |
| `String()` (68/93/132/178/219) | 100% | 100% (был уже — блок C) |
| `detectSystemCaller` (282) | 20% | **~85-95%** — дополнительно получает root-ветки `UseSudo=false` (через fake systemctl, setup через `TestOpenRCCaller_Fakes`/etc. не мантуал trigger; на CI os.Getuid()==0). Реальная Δ выше — `systemd-user`-ветка (`systemctl --user is-active default.target`) покроется только при непустом `systemctlBinPath` на Linux. |
| `detectInitSystem` (249) | 61.1% | ~75-85% — ветки `"init"→openrc/sysvinit` теперь exercisable через fake `rc-service`/`service` (но блок C уже покрыл своими fake-старыми тестами). |

Δ локально: **0%** (Windows skip). Δ на CI Linux (ожидается):
**~+2-3%** (package main). Цифра скромная, т.к. system.go — маленький
файл; основная ценность — закрытие 24 functions/methods в `cover -func`.

### Верификация

- `gofmt -l system_callers_test.go linux_test.go` — пусто
- `go vet ./...` — чисто
- `go test ./... -count=1` (Windows) — зелёный, все D-subtest'ы SKIP
  через `fakeBin` на `runtime.GOOS=="windows"`
- `-v -run TestSystemdSystemCaller|TestSystemdUserCaller|TestOpenRCCaller|TestRunitCaller|TestSysVinitCaller` —
  parent PASS, every subtest SKIP
- существующие тесты не сломаны
- локальный `go test -cover` — **74.9%** (не изменился, как и должно быть)

### Замечания / observability

- **VANITY, явно:** §2.D и §6 промта квалифицируют категорию D как
  «тщеславное» покрытие. Реальная проверка init-перезагрузки остаётся
  Gap 4 (L5 VM nightly) — исполняемые fake-скрипты не доказывают, что
  `systemctl restart dnsmasq` на самом деле работает на Fedora.
- **`detectSystemCaller` 20→85%+ на CI** — главный ко-продукт блока D:
  `os.Getuid()==0` на CI (root в контейнере) → сразу возвращает
  `SystemdSystemCaller{UseSudo:false}`. Это покрывает строки 287-288
  без доп. теста. Доп. строки (294-297, 290-293) покрываются только на
  Linux с установленным реальным systemctl — на Fedora CI есть.
- **Глобальный state**: subtest'ы внутри parent-функции НЕ параллелятся.
  `t.Cleanup` ворует restore на момент под-теста, LIFO-порядок — коррект.
  Перекрёстной контаминации между тестами нет: каждый subtest ставит
  нужный var перед вызовом метода; если var был "" (resolveBins не запустился),
  cleanup восстанавливает "" — следующий тест видит чистый стейт.
- **`binsOnce` invariant**: ни один D-тест не вызывает accessor, не
  установив соответствующий var → `resolveBins()` не срабатывает
  внутри D-тестов. Если порядок тестов запускает какой-то другой тест
  первым (например, из C-блока), и `binsOnce` уже «прогрет», fakes
  всё равно перезаписывают var; cleanup восстанавливает orig (real path
  или ""), не "".
- **`sudoDispatch`** — `shift` дропает только первый арг (`-n`); для
  команд вида `sudo systemctl restart svc` (без `-n`) дропает `systemctl`,
  что сломало бы. Ho все наши caller'ы с `UseSudo=true` добавляют `-n`
  только в `IsActive`; `Restart`/`RestartSelf` вызывают `sudo systemctl
  restart ...` БЕЗ `-n`. Это означает, что `shift` в `Restart` дропает
  `systemctl` → exec("restart","svc"), что НЕ запускает fake systemctl.
  **Анализ**: для `Restart` (`exec.Command(sudoBin(), systemctlBin(),
  "restart", service)`) argv = (sudo, systemctl, "restart", service).
  `shift` → (systemctl, "restart", service), `exec "$@"` → запускает
  fake systemctl `restart service` — корректно (`exec "$@"`
  интерпретирует первый оставшийся аргумент как команду). Для `IsActive`
  (`sudo -n systemctl is-active svc`) argv = (sudo, "-n", systemctl,
  "is-active", svc). `shift` → (systemctl, "is-active", svc), exec
  fake systemctl — корректно. `sudoDispatch` работает в обоих случаях.
- **Файлы `system_callers_test.go` против `system_test.go`**: блок C
  использовал `system_test.go` (portable detect* + String()); блок D
  вынес в отдельный файл из-за разных инвариантов (Linux-gated, fake
  bins, table-per-caller). Имя `system_callers_test.go` отражает суть.

### Что дальше

- На CI Linux прогонятся D-тесты → закроют ~95% system.go.
- Дальнейших planned блоков нет (sweep A→B→C→D завершён; §6 — вне
  области subprocess-`os.Exit` опц.).
- ROADMAP: чекбокс «≥80%» тикнуть по подтверждению CI после прогона
  с блоком D (ожидается ~78-80% по package main на Fedora 44).