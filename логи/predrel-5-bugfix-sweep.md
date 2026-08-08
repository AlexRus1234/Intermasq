# Сессия: predrel — 5 багфиксов перед релизом (auth / config-write / lease-time / alias-delete / plugins)

Поверхностное ревью кодовой базы перед релизом v1.0 выявило 5 багов разной
степени критичности — от необратимой блокировки аккаунта до потери всего
конфига и panic при старте. Все 5 закрыты за один проход, каждый покрыт
регрессионными тестами. Бэкенд-логика, фронтенд, smoke-сьюты и i18n правлены
согласно KISS — ни одной новой подсистемы.

---

## Коммиты

| Хэш | Описание |
|-----|----------|
| `e66b085` | fix: bcrypt oversize passwords, silent config loss, lease-time, PTR/TXT delete (баги 1–4) |
| `a871048` | style(dnsmasq): gofmt struct field alignment in dnsmasq_test.go |
| `f91a349` | test(smoke): expect PTR/TXT alias delete to succeed (200) |
| `f70e0d9` | fix(plugins): kill plugin processes on shutdown and skip duplicate ids (баг 5) |
| `fd7099e` | test(plugins): fix route-count and liveness assertions |

База: `5d6b0b8` → HEAD `fd7099e`. Суммарно: 20 файлов, +712/−30.

---

## 1. Баг 1: bcrypt >72 байт + пустые поля в `setupHandler`

### Симптом

`hash, _ := bcrypt.GenerateFromPassword(...)` в трёх местах (`handlers.go:53`,
`handlers_users.go:43,98`) игнорировал ошибку. В `golang.org/x/crypto v0.48`
пароль длиннее 72 байт возвращает `ErrPasswordTooLong` → `hash=nil` →
`string(hash)=""` сохраняется как хэш → аккаунт необратимо блокируется
(`CompareHashAndPassword` всегда падает). Плюс `setupHandler` не проверял
пустые username/password (в отличие от `createUserHandler`, где проверка есть).

### Фикс

- `handlers.go` — константа `maxPasswordBytes = 72`; в `setupHandler` добавлены
  проверки `req.Username == "" || req.Password == ""`, `len(req.Username) > 64`
  и `len(req.Password) > maxPasswordBytes`; ошибка `GenerateFromPassword`
  обрабатывается → `400 password_too_long`.
- `handlers_users.go` — `createUserHandler` и `changePasswordHandler`: те же
  length-check + обработка ошибки bcrypt.
- Фронтенд: `maxlength="72"` на пароль-инпутах и `maxlength="64"` на username
  (`AuthScreen.vue`, `UsersTab.vue`); переводы `password_too_long` /
  `username_too_long` / `missing_fields` в `ru.json` / `en.json`.

Дизайн-решение (согласовано): пароли >72 байт **отвергаются**, а не
транкейтятся и не pre-хэшируются SHA-256 — честно и совпадает с замыслом bcrypt.

### Тесты

`TestSetupHandler_EmptyFields`, `TestSetupHandler_PasswordTooLong`,
`TestChangePasswordHandler_PasswordTooLong` (handlers_test.go);
`TestCreateUserPasswordTooLong` (dnsmasq_test.go). Критичный ассерт: юзер **не**
сохраняется с пустым хэшем (именно это ломало аккаунт).

---

## 2. Баг 2: `AppendHostLine` / `AppendAliasLine` игнорируют ошибку `ReadFile`

### Симптом

`write.go:144,157` — `content, _ := os.ReadFile(filePath)`. При транзиентной
ошибке чтения `content=nil`, и функция перезаписывала файл одной новой строкой →
потеря всего конфига dnsmasq.

### Фикс

Не-`IsNotExist` ошибка `ReadFile` теперь возвращается наверх; `IsNotExist`
обрабатывается как создание файла (пустой контент). Все вызывальщики
(`bulkMoveHandler`, `bulkEditHandler`, `addAliasHandler`) ошибку уже
пробрасывали — их трогать не понадобилось.

### Тесты

`TestAppendHostLine_PreservesExistingContent`,
`TestAppendAliasLine_PreservesExistingContent` (portable happy-path);
`TestAppendHostLine_ReadErrorPreservesData`,
`TestAppendAliasLine_ReadErrorPreservesData` (POSIX-only регрессионные:
файл `chmod 0o200` — read denied, write ok — воспроизводит потерю данных;
self-skip под root/Windows, restore-attraction через `t.Cleanup`).

---

## 3. Баг 3: `ParseDhcpHostLine` ломает lease-time

### Симптом

`dnsmasq.go:87` — токен `12h` / `infinite` не MAC/IP/`set:`/`tag:`/`id:` →
падал в `default` → затирал hostname. Round-trip через `FormatDhcpHostLine`
терял суффикс; bulk move/edit калечили строку.

### Фикс

- `models.go` — поле `LeaseTime string \`json:"lease_time,omitempty"\`` в
  `HostEntry`.
- `dnsmasq.go` — парсер маршрутирует lease-time в `LeaseTime` через
  **переиспользованный** `IsLeaseTime` (каноническое определение пакета из
  `config_snapshot.go`, с `infinite` и len-guard; отдельный regex не завожу —
  был бы конфликт имён). `FormatDhcpHostLine` эмитит lease-time **последним**
  (после tags), как требует dnsmasq.
- `handlers_hosts.go` — `bulkEditHandler` пересобирал `HostEntry` без
  lease-time; теперь тащит `existing.LeaseTime`.

### Тесты

`TestParseDhcpHostLine_LeaseTime` (table-driven: hours/seconds/infinite/
host-before-ip + round-trip через Format), `TestParseDhcpHostLine_LeaseTimeWithTag`
(суффикс `,set:phone,12h`, порядок сохраняется).

### Вне этого захода

`HostsToCSV` / `ParseCSVHosts` и форма хоста поле `lease_time` не знают →
CSV-экспорт его теряет. Ядро (парсер/форматтер/bulk-edit round-trip) закрыто;
CSV и UI — отдельным заходом.

---

## 4. Баг 4: PTR/TXT нельзя удалить через API

### Симптом

`handlers_aliases.go:208` — `req.Type != "A" && req.Type != "CNAME"` → `400`.
Добавить все 4 типа можно, удалить — только A/CNAME. Низкоуровневый
`RemoveAliasLine` уже generic через `ParseAliasLine`.

### Фикс

Вынесен `validAliasType(t string) bool` (A/CNAME/PTR/TXT) — используется и в
`validateAliasEntry`, и в `deleteAliasHandler`, чтобы правила не разошлись.

### Тесты

`TestDeleteAliasHandler_BadType` переписан (кодировал баг: PTR→400; теперь
невалидный `MX`→400). `TestDeleteAliasHandler_PTR_TXT` — PTR и TXT delete → 200.

### Smoke

`tests/suites/32-aliases-delete.sh` тоже кодировал старое поведение
(`Delete PTR rejected … want 400`). Сьюит переписан: PTR/TXT → 200, добавлена
проверка удаления TXT.

---

## 5. Баг 5: плагины не убиваются при shutdown/restart + panic при дубликате id

### Симптом

- `plugins.go:Load` стартует `cmd`, но никуда его не сохраняет → на shutdown /
  `restart-self` дочерние процессы осиротивают. На openrc/runit/sysvinit
  супервизор убивает только main PID → после рестарта плодятся дубликаты.
- Два манифеста с одним `id` → `r.Any("/plugins/<id>/*any")` дважды → gin
  паникует при старте.

### Фикс

- `internal/plugins/plugins.go` — трекинг `startedCmds []*exec.Cmd` (под
  `startedCmdsMu`); идемпотентный `Stop()` (Kill + Wait, skip уже мёртвых).
  В `Load` — `seen`-map: пустой/дубликат id логируется и skip'ается (первый
  выигрывает), процесс добавляется в трекинг после удачного `Start`.
  `SetDirsForTest` сбрасывает `startedCmds` и зовёт `Stop()` в cleanup.
- `internal/webapi/register.go` — `plugins.Stop()` **до** `RestartSelf()` в
  обработчике `/api/restart-self` (под `!ciMode`).
- `main.go` — обработчик `SIGTERM`/`SIGINT` → `plugins.Stop()` → `os.Exit(0)`.
  Покрывает `systemctl stop` / ручной kill на всех init-системах.

### Тесты

`TestLoadPlugins_DuplicateIDNoPanic` (2 манифеста с одним id → нет паники,
ровно 1 distinct path / 1 процесс), `TestStopKillsStartedProcesses` (процесс
мёртв после Stop — проверка через `signal(0)`, slice очищен),
`TestStopIsIdempotent` (двойной Stop — no-op). Существующий `FakeDir` пофикшен
на утечку `sleep 60` (раньше оставлял процесс висеть).

### Два прохода тестов

Первая итерация CI упала на двух ассертах: `TestLoadPlugins_DuplicateIDNoPanic`
считал записи `r.Routes()` (а `r.Any` регистрирует путь под 9 HTTP-методами →
9 записей) и `TestStopKillsStartedProcesses` опирался на
`cmd.ProcessState.Exited()` (ненадёжно: остаётся nil при ECHILD). Коммит
`fd7099e`: считаю distinct-пути и зондирую OS через `signal(0)` (ground truth).
Сам `Stop()` не менялся — он корректен, проблема была в метрике тестов.

---

## Проверки

| Проверка | Результат |
|---|---|
| `gofmt -l internal/ main.go` | чисто (после `a871048`) |
| `go vet ./...` | чисто |
| `go build .` | OK |
| `go test ./... -count=1` | PASS (все пакеты) |
| `go test -race` (webapi / dnsmasq / plugins / root) | PASS |
| smoke `tests/smoke.sh` | PASS 158/158 (после `f91a349`) |

Process-тесты плагинов skip'аются на Windows (нужен shell-script binary),
отрабатывают на Linux CI.

---

## Изменённые файлы

| Файл | Изменения |
|---|---|
| `internal/webapi/handlers.go` | `maxPasswordBytes`, проверки в `setupHandler`, обработка ошибки bcrypt |
| `internal/webapi/handlers_users.go` | length-check + обработка bcrypt в `createUser`/`changePassword` |
| `internal/webapi/handlers_aliases.go` | `validAliasType`, PTR/TXT в delete |
| `internal/webapi/handlers_hosts.go` | перенос `LeaseTime` в `bulkEditHandler` |
| `internal/webapi/register.go` | `plugins.Stop()` перед `RestartSelf` |
| `internal/dnsmasq/dnsmasq.go` | `IsLeaseTime` в парсере, lease-time в `FormatDhcpHostLine` |
| `internal/dnsmasq/write.go` | обработка ошибки `ReadFile` в `Append*Line` |
| `internal/models/models.go` | поле `HostEntry.LeaseTime` |
| `internal/plugins/plugins.go` | трекинг процессов, `Stop()`, dup-id guard, `SetDirsForTest` |
| `main.go` | `SIGTERM`/`SIGINT` → `plugins.Stop()` |
| `internal/webapi/handlers_test.go` | тесты багов 1, 4 |
| `internal/webapi/dnsmasq_test.go` | `TestCreateUserPasswordTooLong` |
| `internal/dnsmasq/dnsmasq_test.go` | тесты lease-time |
| `internal/dnsmasq/write_test.go` | тесты `Append*Line` |
| `internal/plugins/plugins_test.go` | тесты бага 5 + фиксы ассертов |
| `frontend/src/components/AuthScreen.vue` | `maxlength` |
| `frontend/src/components/UsersTab.vue` | `maxlength` |
| `frontend/src/locales/{ru,en}.json` | переводы ошибок |
| `tests/suites/32-aliases-delete.sh` | PTR/TXT delete → 200 |

---

## Риски и нюансы

- **Выбор «отвергать >72 байт».** Альтернативы — SHA-256 pre-hash (как
  Django/Laravel) или молчаливый truncate. Pre-hash расширяет длину, но менее
  идиоматичен; truncate — security-футгун (коллизии по первым 72 байтам).
  Решение — честный reject + `maxlength` на фронте.
- **lease-time детектится по `IsLeaseTime` во любой позиции токена**, не только
  последней. dnsmasq сам по себе позиционно-гибок, а `IsLeaseTime` —
  каноническое определение пакета. Побочный эффект: hostname, буквально равный
  `12h`/`infinite`, уйдёт в `LeaseTime`. Допустимый edge-case, совпадает с
  семантикой dnsmasq.
- **CSV-экспорт теряет lease-time.** Ядро бага 3 закрыто (round-trip в
  bulk-move/bulk-edit), но `HostsToCSV`/`ParseCSVHosts` и форма хоста поле не
  знают. Осознанно отложено.
- **`Stop()` убивает трекаемый pid плагина**, но не его подпроцессы (если плагин
  форкает детей). Для самого бага (дубликаты плагин-процессов после рестарта)
  этого достаточно — основной процесс плагина умирает. Process-group kill
  (`Setpgid` + `kill -pgid`) сознательно не введён: усложняет кроссплатформу,
  а orphaned-внуки — отдельная история.
- **`SIGTERM`-handler вызывает `os.Exit(0)`.** Это корректный graceful-exit для
  `systemctl stop`/kill. Под `ci-mode` handler тоже ставится — `Stop()` на
  пустом `startedCmds` это no-op, тесты бинарник не дёргают сигналами.
- **Smoke `32-aliases-delete.sh` кодировал баг.** Любой фикс, меняющий
  наблюдаемое поведение, надо сверять со smoke-сьютами — иначе CI ловит
  «неожиданный» PASS/FAIL. PTR-кейс пойман именно там.
