# Stage 11: `internal/webapi` (HTTP-хендлеры) + тонкий `main`

Финальный большой этап модуляризации. Все HTTP-хендлеры и регистрация
маршрутов `/api` вынесены из `main` в новый пакет `internal/webapi`;
`main.go` превратился в тонкий bootstrap-composer. Этапы 0–10 были
завершены ранее; локальные проверки этапа 11 — зелёные.

## Что сделано

### Новый пакет `internal/webapi` (`package webapi`, white-box)

Перенесены через `git mv` (история сохранена, rename detection 93–96 %):

| файл                         | роль                                                       |
| ---------------------------- | --------------------------------------------------------- |
| `handlers.go`                | status/setup/login/reload/arp/next-ip/leases/events/...    |
| `handlers_hosts.go`          | dhcp-host= CRUD, bulk, CSV, templates                      |
| `handlers_aliases.go`        | DNS alias CRUD, bulk, CSV                                  |
| `handlers_config.go`         | визуальный config-editor, raw file editor, file delete     |
| `handlers_safety.go`         | rollback, history, ZIP backup/restore                      |
| `handlers_users.go`          | user management, logout                                    |
| `register.go` (новый)        | `func Register(r *gin.Engine, ciMode bool)`                |
| `helpers_test.go` (новый)    | общие тестовые хелперы (см. ниже)                          |
| `handlers_test.go`           | handler-level L2 + Gap3 тесты                              |
| `dnsmasq_test.go`            | остатки: ARP/new-devices/lease-to-static/users/files/...   |
| `linux_test.go`              | Linux-gated fake-dnsmasq тесты (только Test*)              |
| `new_features_test.go`       | rate-limit reset, delete-config, concurrent user safety    |

### `func Register(r *gin.Engine, ciMode bool)`

Прямой перенос блока регистрации из `setupServer` (бывшие строки
142–209). Поведение дословно прежнее: те же маршруты, методы, коды,
порядок middleware, `auth.RateLimitMiddleware(10, time.Minute)` на
`/login`, группа с `auth.Middleware`. Inline-хендлеры сохранены:

- `/api/plugins` → `plugins.Loaded()`;
- `/api/restart-self` → `initd.Current().RestartSelf()`, гейт `ciMode`
  (флаг `CiMode` остался в `main`, передаётся параметром).

`/metrics`, swagger, static-FS — остались в `setupServer` (вне `/api`).

### Квалификаторы доменов

Вся работа с доменами — через квалификаторы: `dnsmasq.`, `auth.`,
`audit.`, `templates.`, `netstate.`, `control.`, `validate.`,
`models.`, `initd.`, `plugins.`. В частности:

- типы-DTO → `models.*` напрямую (`models.HostEntry`, `models.AuthReq`,
  `models.Template`, `models.DnsAliasEntry`, …);
- аудит → `audit.WriteAudit(audit.AuditEntry{...})` и `audit.Handler`;
- helper `getUser` — в `webapi` (handlers.go).

### Удалён файл type-алиасов (этап 1)

`models.go` (type-алиасы `HostEntry = models.HostEntry` и т.д., var-алиасы
`ArpPath/LeasesPath/AuditLogPath/TemplatesPath`, func-обёртки
`writeAudit/auditHandler/resetTemplates/setTemplate/hasTemplate/parseArpContent/...`)
удалён: после переноса хендлеров в webapi ни один алиас не используется
ни в `main`, ни в `webapi` (везде прямые квалификаторы). Проверено
компилятором. `auth_compat_test.go` (`var DBPath` + `setUsers`) тоже
удалён — `DBPath`/`setUsers` переехали в `webapi/helpers_test.go`, а
`setup_test.go` использует `auth.DBPath` напрямую.

### Тесты: white-box, `package webapi`

- `handlers_test.go`, `new_features_test.go`, `dnsmasq_test.go`,
  `linux_test.go` — `package webapi`, обращаются к хендлерам и
  production-хелперам (`validateAliasEntry`, `resolveAliasesTargetFile`,
  `coalesce`) напрямую.
- общие хелперы консолидированы в `webapi/helpers_test.go`:
  `newTestDir`, `newJSONContext`, `jsonPath`, `multipartWriter`,
  `DBPath`/`setUsers`, template-обёртки `resetTemplates`/`setTemplate`/
  `hasTemplate`, fake-bin harness (`fakeDnsmasq`,
  `fakeDnsmasqArgvInspect`, `readArgvLog`, `fakeDnsmasqStrict`,
  `fakeBin`, `setBinPath`, `itoa`), history-хелперы (`withHistoryDir`,
  `newestHistoryVersion`, `firstVersion`). Дубликатов хелперов в `main`
  не осталось.
- `linux_test.go` переписан: из него убраны определения хелперов
  (переехали в `helpers_test.go`), остались только `Test*`-функции.
- path-флаги в тестах квалифицированы: `*netstate.ArpPath`,
  `*netstate.LeasesPath`, `*audit.AuditLogPath`,
  `*templatepkg.TemplatesPath`, `*auth.DBPath`; вызовы
  `parseArpContent`→`netstate.ParseArpContent`,
  `getNewDevices`→`netstate.GetNewDevices`.

### `setup_test.go` остался в `main`

`TestSetupServer_*` + `withSandboxFlags` — в `package main` (т.к.
`setupServer` остался там). `withSandboxFlags` адаптирован к прямым
квалификаторам: `*auth.DBPath`, `*templatepkg.TemplatesPath`,
`*netstate.ArpPath`, `*netstate.LeasesPath`, `*dnsmasq.HistoryDir`,
`*dnsmasq.ConfigDir`, плюс прежние `*InitSystem`/`*SystemdScope` и
Fn-швы `startSSEBroadcasterFn`/`startDNSHealthCheckerFn`. Плагиновые
директории — через `plugins.SetDirsForTest` (как с этапа 10).

## Итоговая структура `main.go` (152 строки, цель была ≤ ~150)

```
package main

import: embed, flag, fmt, io/fs, net/http,
        gin, swaggo/{files,gin-swagger}, _ docs,
        authpkg(=auth), bins, control, dnsmasq, initd, metrics,
        plugins, templatepkg(=templates), webapi

//go:embed frontend/dist/*           staticFiles
var: Port, InitSystem, SystemdScope, CiMode   (flag.String/Bool)
init()                             // проверка authpkg.SecretKey != ""
var: startSSEBroadcasterFn, startDNSHealthCheckerFn   (Fn-швы горутин)
main()                             // flag.Parse → setupServer → r.Run + os.Exit
setupServer() (*gin.Engine, error) // композиция:
                                   //   bins.Resolve()
                                   //   authpkg.LoadUsers()
                                   //   templatepkg.Load()
                                   //   dnsmasq.EnsureHistoryDir()
                                   //   legacy-scope → initd.Init(...)
                                   //   gin.Default()
                                   //   plugins.Load(r)
                                   //   startSSEBroadcasterFn()/startDNSHealthCheckerFn()
                                   //   r.GET("/metrics", metrics.Handler)
                                   //   webapi.Register(r, *CiMode)
                                   //   swagger + static NoRoute
```

Доменной логики в `main` нет: `DefaultAliasesFileName` и прочие
пережитки убраны — остались только флаги, embed, init-проверка секретa,
композиция bootstrap и `Run()`.

## Корневые go-файлы бэкенда

Только `main.go` + `setup_test.go` (как и планировалось). `docs/docs.go`
— swagger-генерация, не бэкенд.

## Локальные проверки (зелёные)

```
gofmt -l .                                   # чисто
go vet ./...                                 # чисто
$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXX"
go test . -count=1                           # ok intermask 2.888s
go test ./internal/webapi/ -count=1          # ok intermask/internal/webapi 6.144s
go test ./... -count=1                       # все пакеты ok
```

### SKIP-лист (main + webapi), сверка с baseline

12 skip'ов — все Linux-gated fake-dnsmasq тесты из
`webapi/linux_test.go` (skip на runtime.GOOS=="windows", т.к. shebang
не выполняется через `os/exec` на Windows):

```
TestReloadHandler_200/400
TestPutFileHandler_Success / _DnsmasqTestFail_400 / _PassesConfFileToTest
TestUpdateConfigHandler_Success / _DnsmasqTestFail_400 / _PassesConfFileToTest
TestRestoreBackupHandler_Success / _DnsmasqTestFail_400 / _PassesConfFileToTest
TestHistoryRestoreHandler_Success
```

Раньше все 12 жили в `linux_test.go` package main; теперь — в
`internal/webapi`. Суммарно (main + webapi) = 12, т.е. перенос
консервирован: новых skip'ов нет, ни один тест не потерян. (Skip'ы
`TestLoadPlugins_*` из этапа 10 живут в `internal/plugins` — в сумму
main+webapi не входят.)

## Живые швы `...ForTest` (зачистит этап 12)

Cross-package test-seams, экспортированные из доменных пакетов и
используемые теперь в т.ч. из `internal/webapi`:

- `bins.SetPathForTest(t, name, path)` — подмена кэшированных путей
  бинарников (`dnsmasq`/`sudo`/`systemctl`/…). Используется
  `webapi/helpers_test.go` (`setBinPath`/`fakeDnsmasq*`).
- `initd.SetCurrentForTest(t, caller)` — подмена `sysCaller`.
  Используется `webapi/linux_test.go`, `webapi/new_features_test.go`.
- `auth.SetSecretForTest(t, secret)` — подмена `SecretKey`.
  Используется `webapi/handlers_test.go`, `webapi/dnsmasq_test.go`.
- `plugins.SetDirsForTest(t, pluginsDir, socketsDir)` — подмена
  плагиновых директорий + сброс `loadedPlugins`. Используется
  `setup_test.go` (`withSandboxFlags`).

Fn-швы в `main` (не `ForTest`-имя, но та же природа — indirection для
тестов):

- `startSSEBroadcasterFn = control.StartBroadcaster`
- `startDNSHealthCheckerFn = metrics.StartDNSHealthChecker`

Эти швы подменяются в `withSandboxFlags` (setup_test.go) на no-op, чтобы
нейтрализовать долгоживущие горутины при тестировании bootstrap'а. Их
существование и состав — то, что этап 12 должен пересмотреть/зачистить.

## Комментарий-инкремент

В `internal/dnsmasq/helpers_test.go` обновлён комментарий о
дублировании fake-dnsmasq хелперов: раньше он ссылался на «package
main's linux_test.go … remain in main until stage 11» — теперь указывает
на `internal/webapi/helpers_test.go` (куда хендлер-тесты и их хелперы
переехали). Дубликат fake-dnsmasq между `internal/dnsmasq` и
`internal/webapi` остаётся намеренным (невозможно разделить без
загрязнения production-API одного из пакетов).

## Коммит

`refactor(modular): stage 11 — extract webapi, slim main`
17 files changed, 551 insertions(+), 560 deletions(-).
