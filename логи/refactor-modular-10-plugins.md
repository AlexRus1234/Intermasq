# Stage 10: `internal/plugins`

Перенесены загрузчик и reverse-proxy плагинов из `main.go` в
`internal/plugins`. Поведение контракта сохранено дословно: те же
`PluginsDir`/`SocketsDir` по умолчанию (`/etc/intermasq/plugins`,
`/run/intermasq/sockets`), тот же `os.MkdirAll(SocketsDir, 0770)` с
игнорированием ошибки, те же env-переменные запускаемого бинаря
(`INTERMASQ_KEY` из `INTERMASQ_SECRET` + `PLUGIN_SOCKET=<id>.sock`),
тот же `httputil.ReverseProxy` с `Dial("unix", sockPath)` и маршрут
`/plugins/<id>/*any`, тот же молчаливый `continue` на отсутствующем
`manifest.json`/битом JSON/пустом PluginsDir.

## Exported API (`internal/plugins`)

- `PluginManifest` (struct: `ID`/`Name`/`Bin` с прежними json-тегами).
- `PluginsDir` / `SocketsDir` — экспортированные `var` со значениями по
  умолчанию (как и были в main; НЕ флаги).
- `Load(r *gin.Engine)` — бывшая `loadPlugins`; вызывается из
  `setupServer`.
- `Loaded() []PluginManifest` — снимок `loadedPlugins` для хендлера
  `/api/plugins` (возвращает срез-алиас пакета, без копирования — как
  раньше).
- `SetDirsForTest(t, pluginsDir, socketsDir)` — cross-package шов:
  снапшотит `PluginsDir`/`SocketsDir`/`loadedPlugins`, выставляет новые
  значения, обнуляет `loadedPlugins` и восстанавливает всё в `t.Cleanup`.
  Используется `withSandboxFlags` из package main.

`loadedPlugins` остался неэкспортированным (доступ к нему — через
`Loaded()` снаружи и напрямую из white-box тестов пакета).

## Швы

- `SetDirsForTest` помечена `// Exported for cross-package tests during
  modularization.` по тому же шаблону, что `initd.SetCurrentForTest`,
  `bins.SetPathForTest`, `auth.SetSecretForTest`.
- `withSandboxFlags` (setup_test.go) больше не хранит в своём `orig`-struct
  поля `PluginsDir`/`SocketsDir`/`loadedPlugins` и не восстанавливает их
  вручную — всё делает `plugins.SetDirsForTest(t, ...)`. Поведение теста
  прежнее (пути → temp, `loadedPlugins` обнулён, восстановление на cleanup).

## Изменения в `main.go`

- Удалены: `PluginManifest`, `loadPlugins`, `loadedPlugins`,
  `PluginsDir`/`SocketsDir` (переехали).
- `setupServer`: `loadPlugins(r)` → `plugins.Load(r)`.
- Хендлер `/api/plugins`: `c.JSON(200, loadedPlugins)` →
  `c.JSON(200, plugins.Loaded())`.
- Почищены импорты, бывшие нужны только `loadPlugins`: `context`,
  `encoding/json`, `net`, `net/http/httputil`, `os/exec`, `path/filepath`.
  Добавлен `intermask/internal/plugins`.

## Тесты

В `internal/plugins/plugins_test.go` (white-box, `package plugins`)
переехали из `linux_test.go`:

- `TestLoadPlugins_FakeDir` — manifest + sh-фейк бинаря, проверка маршрута,
  `loadedPlugins`, создания `SocketsDir`.
- `TestLoadPlugins_NoDir` — early-return при отсутствии `PluginsDir`.
- `TestLoadPlugins_BrokenManifest` — пропуск битого `manifest.json`.

Тесты обращаются к пакетным переменным напрямую (in-package),
переименованы только вызовы `loadPlugins` → `Load`. Из `linux_test.go`
секция `===== T-C.5 loadPlugins =====` удалена целиком; остальные тесты
файла (T-B.* хендлеры, wiring-гарды A13/A14) не затронуты.

## Прочее

- В `tests/fixtures/plugins/hello/main.go` поправлен устаревший ссылочный
  комментарий (`main.go::loadPlugins()` → `internal/plugins.Load()`);
  поведение мок-плагина и его контракт (`PLUGIN_SOCKET`) без изменений.
- CI-шаг «Build & install mock plugin» правок не требовал: дефолт
  `/etc/intermasq/plugins` не изменился, шаг не зависит от путей Go-пакетов.

## Проверки

- `gofmt -l .` — зелёный.
- `go vet ./...` — зелёный.
- `go test . -count=1` (с `INTERMASQ_SECRET`) — зелёный; в т.ч.
  `TestSetupServer_RegistersRoutes`, `TestSetupServer_InitSystemNone`,
  `TestSetupServer_LegacySystemdScopeWarning`, `TestSetupServer_HistoryDirFail`,
  `TestWithSandboxFlags_RestoresSysCaller` проходят с новым швом.
- `go test ./internal/plugins/ -count=1` — зелёный. SKIP-лист совпадает с
  baseline: `TestLoadPlugins_FakeDir` и `TestLoadPlugins_BrokenManifest`
  SKIP на Windows (sh-фейки не исполняются под `os/exec`), в CI под Linux
  выполняются; `TestLoadPlugins_NoDir` PASS везде.

## CI

Forgejo-воркфлоу `.forgejo/workflows/build.yml` запускается только по
`workflow_dispatch` (ручной запуск); автозапуска на push нет. Локальные
проверки зелёные; ручной прогон воркфлоу — по необходимости (пользователем).
