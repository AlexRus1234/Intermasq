# Stage 12: зачистка (опциональный косметический этап)

Финальный этап модуляризации. Кода поведения не меняет — только
документация, комментарии и удаление пустого локального каталога.
Этапы 0–11 завершены ранее; после stage 11 CI стабильно зелёный.

## Что сделано

### 1. Тест-швы `…ForTest` — все оставлены (ещё используются cross-package)

Найдены 4 функции с маркером `Exported for cross-package tests during
modularization`. Для каждой сверен объём cross-package использований
после этапа 11. **Все 4 остаются** — без них тесты других пакетов не
смогут подменять package-level состояние (Go не даёт иного способа
без экспорта мутатора):

| шов | файл | живые cross-package потребители |
| --- | --- | --- |
| `bins.SetPathForTest` | `internal/bins/bins.go:162` | `internal/control`, `internal/initd`, `internal/webapi`, `internal/dnsmasq` (fake-бинари) |
| `initd.SetCurrentForTest` | `internal/initd/system.go:373` | `setup_test.go` (main), `internal/control`, `internal/metrics`, `internal/webapi` |
| `auth.SetSecretForTest` | `internal/auth/auth.go:275` | `internal/metrics`, `internal/webapi` (+ in-package `internal/auth`) |
| `plugins.SetDirsForTest` | `internal/plugins/plugins.go:154` | `setup_test.go` (main, `withSandboxFlags`) |

Комментарии на самих швах не трогались — они объясняют происхождение
функции и остаются исторически корректными; правка ссылок в `internal/*`
в область этого этапа не входила (см. правила ниже).

Fn-швы `main.startSSEBroadcasterFn` / `main.startDNSHealthCheckerFn`
(не `…ForTest`-имя, но та же природа) тоже остаются: `setup_test.go`
через `withSandboxFlags` подменяет их на no-op, чтобы нейтрализовать
долгоживущие горутины при тестировании bootstrap'а.

### 2. Пустой `cmd/intermasq/` — удалён локально, решение задокументировано

`cmd/intermasq/` был пуст и git'ом не отслеживался (git не хранит пустые
каталоги) — локальный артефакт. Каталог удалён. В `README.md` (раздел
«Структура проекта») добавлена одна строка-обоснование: точка входа
остаётся в корне репозитория, т.к. `//go:embed frontend/dist/*` не умеет
подниматься по дереву (`../`); сборка прежняя — `go build -o intermasq .`.

### 3. Раздел «Структура проекта» в `README.md` — переписан

Прежнее дерево описывало плоский `package main` (`auth.go`, `handlers.go`,
`dnsmasq.go`, …). Заменено на актуальную модульную структуру `internal/*`
(15 пакетов) с однострочным назначением каждого. Команды сборки не
менялись.

### 4. Устаревшие ссылки в комментариях — точечно обновлены

После рефакторинга ссылки вида `system.go:61`, `main.go:264`,
`history.go:208-210` указывали не туда. Областью задачи были только
`tests/*.sh`, `tests/suites/*.sh`, `tests/l5/*.md`, `tests/e2e/specs/*.ts`.
Правки — только в комментариях, код скриптов не тронут. Стратегия: путь
обновлён на `internal/…`; номер строки сохранён, если конструкция осталась
на той же позиции (напр. `RestartSelf` всё ещё на `system.go:61`), уточнён
при сдвиге (`handlers.go:123→129`, `:239→244`, `metrics.go:62→59`,
`handlers_hosts.go:121→127`), либо обобщён без номера, если конструкция
размазана по нескольким местам (`history.go` newest-first, init-detection
`os.Getuid`-ветки).

Карта переносов (старый плоский файл → новый пакет):

| старый файл | новый путь |
| --- | --- |
| `system.go` | `internal/initd/system.go` |
| `bins.go` | `internal/bins/bins.go` |
| `metrics.go` | `internal/metrics/metrics.go` |
| `sse.go` | `internal/control/sse.go` |
| `auth.go` | `internal/auth/auth.go` |
| `history.go` | `internal/dnsmasq/history.go` |
| `config_templates.go` | `internal/dnsmasq/config_templates.go` |
| `handlers.go` | `internal/webapi/handlers.go` |
| `handlers_hosts.go` | `internal/webapi/handlers_hosts.go` |
| `handlers_users.go` | `internal/webapi/handlers_users.go` |
| `main.go macRegex/hostnameRegex` | `internal/validate/validate.go` |
| `main.go::loadPlugins` | `internal/plugins.Load` |
| `main.go if !*CiMode` (restart-self gate) | `internal/webapi/register.go` (`if !ciMode`) |

Изменённые файлы (14):

- `tests/suites/44-leases-to-static.sh` — `handlers.go:123` → `internal/webapi/handlers.go:129`
- `tests/suites/80-metrics.sh` — `metrics.go:62` → `internal/metrics/metrics.go:59`
- `tests/suites/84-restart-self.sh` — `main.go:264 if !*CiMode` → `internal/webapi/register.go if !ciMode`
- `tests/suites/86-events-sse.sh` — `handlers.go:239` → `internal/webapi/handlers.go:244`
- `tests/l5/README.md` — `system.go:247` → `internal/initd/system.go`
- `tests/l5/test-flow.md` — `system.go:287/299` (обобщение), `system.go:61` (строка сохранена)
- `tests/l5/vm-setup.md` — `system.go:61` (сохранена), `main.go:264` → `internal/webapi/register.go`
- `tests/e2e/specs/audit-tab.spec.ts` — `handlers_hosts.go:121`→`:127`, `main.go:79`→`internal/validate/validate.go`
- `tests/e2e/specs/history-modal.spec.ts` — `history.go:208-210` → `internal/dnsmasq/history.go`
- `tests/e2e/specs/users-tab.spec.ts` — `handlers_users.go:68` → `internal/webapi/handlers_users.go`
- `tests/e2e/specs/sse-live.spec.ts` — `auth.go`, `sse.go:78` (×2) → `internal/auth/auth.go`, `internal/control/sse.go`
- `tests/e2e/specs/plugins-iframe.spec.ts` — `main.go::loadPlugins` → `internal/plugins.Load`
- `tests/e2e/specs/config-template-fill.spec.ts` — `config_templates.go` → `internal/dnsmasq/config_templates.go`

#### Намеренно НЕ правилось (вне области задачи)

- `tests/l5/vm-check.sh`, `tests/l5/provision.sh` (`.sh` в `l5/` — задача
  перечисляет `tests/l5/*.md`, не `.sh`); в них есть `system.go:61` —
  конструктивно верная ссылка (строка не сместилась), поэтому даже за
  пределами области она не вводит в заблуждение.
- `tests/bugreport/bugs.md`, `tests/ROADMAP.md` — исторические записи
  (фиксация состояния на момент написания); правка переписывала бы
  историю. Ссылки там — на старый плоский layout, что корректно для
  документа того периода.

### 5. Таблица статусов в `рефакторинг/README.md`

Этап 12 отмечен `☑`.

## Локальные проверки (зелёные)

```
gofmt -l .                                   # чисто
go vet ./...                                 # чисто
$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXX"
go test ./... -count=1                       # финальный сводный прогон
```

Сводный прогон всех пакетов (финальная сверка — единственный этап после
0, где `go test ./...` оправдан):

```
ok  	intermask	4.125s
?   	intermask/docs	[no test files]
ok  	intermask/internal/audit	2.303s
ok  	intermask/internal/auth	2.317s
ok  	intermask/internal/bins	1.453s
ok  	intermask/internal/control	0.977s
ok  	intermask/internal/dnsmasq	1.980s
ok  	intermask/internal/initd	1.238s
ok  	intermask/internal/metrics	2.383s
?   	intermask/internal/models	[no test files]
ok  	intermask/internal/netstate	1.101s
ok  	intermask/internal/oui	0.848s
ok  	intermask/internal/plugins	2.816s
?   	intermask/internal/stats	[no test files]
ok  	intermask/internal/templates	1.001s
ok  	intermask/internal/validate	0.849s
ok  	intermask/internal/webapi	6.947s
```

Примечание: первый прогон `go test ./...` упал с `link: invalid symbol
ABI: 2` во всех пакетах — это был мусор в build-кэше после апгрейда
тулчейна (локально go1.26.3), к правкам этапа отношения не имеет (ни
один `.go`-файл не менялся). После `go clean -cache` — все пакеты зелёные.

## Итог

`.go`-исходники на этом этапе не правились вовсе — изменение чисто
документационное (README, комментарии в тестовых скриптах/спеках,
лог). Модуляризация `internal/*` (этапы 0–12) завершена.

## Коммиты

- `refactor(modular): stage 12 — cleanup seams, docs, stale refs` — cleanup
  (14 файлов).
- `docs: mark modular stage 12 complete` — статус-таблица + этот лог.
