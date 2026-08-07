# Stage 9: `internal/control`

Перенесены SSE-broadcaster и хелперы статуса/перезапуска dnsmasq из
корневого `sse.go` в `internal/control`. Корневой `sse.go` удалён;
поведение интервала поллинга (5 c), формата SSE-фреймов, порядка
`dnsmasq --test` → restart и инкрементов счётчиков сохранено. `control`
идёт после `metrics`, т.к. `ReloadDnsmasq` трогает `stats.Counters`
(зависимость control → stats, обратной нет).

## Exported API (`internal/control`)

- `Client` (struct с полем `Ch chan string`) — тип SSE-подписчика.
- `Register` / `Unregister` — приём/отписка клиента (для `eventsHandler`).
- `Broadcast(event, data string)` — рассылка фрейма всем клиентам.
- `StartBroadcaster()` — фоновый поллер (запускается из main).
- `ArpToJSON(arp map[string]bool) string`.
- `CheckDnsmasqStatus() bool` (для `statusHandler`).
- `ReloadDnsmasq() error` (для `reloadHandler`).
- `pollOnce` оставлен неэкспортированным (доступен white-box тестам пакета).

## Решение по `eventsHandler`

Хендлер остаётся в `package main` до этапа 11 (webapi). Выбран
минимально-инвазивный вариант: тип клиента экспортирован как `control.Client`
с экспортированным каналом `Ch`, `eventsHandler` создаёт
`&control.Client{Ch: make(chan string, 10)}` и читает фреймы из `client.Ch`.
Альтернатива (функция подписки в пакете) всё равно требовала бы открытого
канала, поэтому выбран более простой путь.

## Швы

- `startSSEBroadcasterFn` в `main.go` сохранён (его глушит `setup_test.go`
  для защиты от data-race под `-race`), теперь указывает на
  `control.StartBroadcaster`.

## Тесты

В `internal/control` (white-box, `package control`) переехали:

- `TestSseRegisterUnregister`, `TestSseBroadcast`, `TestSseBroadcastFullChannel`,
  `TestArpToJSON` — из `dnsmasq_test.go`.
- `TestSsePollOnce`, `TestSsePollOnce_BroadcastsOnDelta` — из
  `goroutines_test.go` (файл удалён целиком, т.к. ничего не осталось).
- `TestReloadDnsmasq_Success` / `_TestFail` / `_CallerFail` + `failCaller` —
  из `linux_test.go`.

Минимальные sh-фейки (`fakeDnsmasq`, `failCaller`) продублированы
в пакете `control` (не тянут чужие хелпер-файлы из main). `TestSsePollOnce`
больше не зовёт `newTestDir` — `pollOnce` не читает `ConfigDir`, достаточно
перенаправления `*netstate.ArpPath`. Handler-тесты `TestReloadHandler_*` и
`TestEventsHandlerStreamsSSE` остались в main до этапа 11.

## Проверки

- `gofmt -l .` — зелёный
- `go vet ./...` — зелёный
- `go test . -count=1` (с `INTERMASQ_SECRET`) — зелёный
- `go test ./internal/control/ -count=1` — зелёный (6 тестов PASS,
  3 `TestReloadDnsmasq_*` SKIP на Windows — совпадает с baseline:
  shell-фейки dnsmasq не исполняются под `os/exec` на Windows,
  в CI под Linux они выполняются)
