# Новые возможности Intermasq

8 функций, добавленных поверх базового функционала: прямой редактор конфигов,
SSE-события, управление пользователями, вкладка «Новые устройства» с OUI-lookup,
массовый перенос lease → static, восстановление из ZIP, rate-limit на логин,
logout с отзывом JWT.

---

## 1. Редактор конфигурационных файлов

Просмотр и редактирование любых `.conf` файлов в `-conf-dir` через веб.
Подстраховка для строк, которые GUI не покрывает, и для ручного редактирования
без SSH.

### API

```
GET /api/files/:name
Authorization: Bearer <token>

→ 200 {"path":"/etc/dnsmasq.d/hosts.conf","content":"server=1.2.3.4\n"}
→ 403 {"error":"access_denied"}   # name без .conf, содержит / или \
→ 404 {"error":"file_not_found"}
```

```
PUT /api/files/:name
Authorization: Bearer <token>
Content-Type: application/json

{"content":"новое содержимое\n"}

→ 200 {"status":"ok"}
→ 403 {"error":"access_denied"}                       # name без .conf или с разделителем
→ 400 {"error":"dnsmasq_test_failed","detail":"..."}  # невалидный синтаксис
→ 500 {"error":"write_error"}
```

### Механизм работы (`dnsmasq.go`)

- `readFileRaw(path) ([]byte, error)` — чтение, проверка пути через `isSafePath`.
- `writeFileRaw(path, content) error` — `createLocalBackup` (`.bak`) → запись →
  `dnsmasq --test` → при неудаче `rollbackFile`. История версий здесь не
  сохраняется (см. отдельную подсистему history).

Все эндпоинты под `authMiddleware`. `:name` обязано оканчиваться на `.conf` и не
содержать разделителей пути.

---

## 2. SSE-события (Server-Sent Events)

Сервер пушит изменения ARP-таблицы и статуса dnsmasq подключённым клиентам,
вместо опроса раз в 30 сек. Односторонний пуш, работает через reverse proxy без
донастройки.

### API

```
GET /api/events?token=<JWT>
Content-Type: text/event-stream

event: arp
data: {"aa:bb:cc:dd:ee:ff":true}

event: dnsmasq_status
data: {"active":true}
```

Эндпоинт под `authMiddleware`. EventSource не умеет ставить заголовок
`Authorization`, поэтому токен передаётся в query-параметре `?token=` (fallback в
`authMiddleware`, когда нет заголовка).

### Механизм (`dnsmasq.go`)

- `sseClient{ch chan string}`, пакетные `sseClients` map + `sseClientsMu`.
- `sseRegister` / `sseUnregister` / `sseBroadcast(event, data)` (non-blocking).
- `startSSEBroadcaster()` (горутина в `main`) опрашивает ARP и статус dnsmasq
  каждые 5 сек и пушит только при изменении (`arp`, `dnsmasq_status`).
- `eventsHandler` при подключении шлёт текущее состояние ARP, далее слушает
  broadcast до `c.Request.Context().Done()`.

---

## 3. Управление пользователями

Создание/удаление пользователей и смена собственного пароля (вкладка
«Пользователи»). Имена — `username` (не `login`).

### API

```
POST /api/users            {"username":"newuser","password":"secret123"}
GET  /api/users
DELETE /api/users/:name
POST /api/users/password   {"old_password":"old","new_password":"new"}
```

| Метод+путь | Успех | Ошибки |
|---|---|---|
| `POST /api/users` | 200 `{"status":"ok"}` | 400 `missing_fields` / `username_too_long`, 409 `user_exists` |
| `GET /api/users` | 200 `{"users":["admin","bob"]}` | — |
| `DELETE /api/users/:name` | 200 `{"status":"deleted"}` | 400 `cannot_delete_self`, 404 `user_not_found` |
| `POST /api/users/password` | 200 `{"status":"ok"}` | 400 `missing_fields`, 401 `invalid_credentials` |

### Безопасность

- Пароли — bcrypt (`DefaultCost`), хранятся в `users.json`.
- Нельзя удалить самого себя (защита от локапа: список никогда не опустеет).
- Все роуты под `authMiddleware`; действия пишутся в audit-лог (`user_create`,
  `user_delete`, `password_change`).

---

## 4. Вкладка «Новые устройства»

ARP-MAC, которых нет ни в `dhcp-host=` (static), ни в leases. Опционально
определение вендора по первым 3 октетам MAC.

### API

```
GET /api/new-devices
Authorization: Bearer <token>

→ 200 [
  {"mac":"aa:bb:cc:dd:ee:ff","ip":"","vendor":"Apple"},
  {"mac":"11:22:33:44:55:66","ip":"","vendor":""}
]
```

### Логика (`dnsmasq.go`)

- `getNewDevices() []NewDeviceInfo` читает ARP через `getArpTable()` (флаг
  `-arp-file`, по умолчанию `/proc/net/arp`), leases и static-хосты.
- MAC, отсутствующий в обоих списках, помечается как «новый».
- Вендор — `lookupOUI(mac)` по встроенной таблице (`oui.go`). Поле `ip`
  возвращается пустым.

UI: таблица MAC + вендор, кнопка «Добавить» перекидывает в форму static с
предзаполненным MAC.

---

## 5. Массовый перенос lease → static

Расширение «В статику» для нескольких выделенных аренд сразу.

### API

```
POST /api/leases/to-static
Authorization: Bearer <token>
Content-Type: application/json

{
  "leases": [{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.1","hostname":"myhost"}],
  "file": "/etc/dnsmasq.d/hosts.conf"
}

→ 200 {"status":"ok","count":1}
→ 403 {"error":"access_denied"}                          # unsafe path (проверяется первым)
→ 400 {"error":"no_leases"}                              # пустой список
→ 400 {"error":"invalid_mac","mac":"bad"}
→ 409 {"error":"mac_duplicate","conflicts":[...]}
```

### Особенности

- **MAC-конфликт:** если MAC уже есть в `dhcp-host=` — 409, ничего не пишется.
- **Авто-hostname:** при `hostname == "*" | ""` генерируется
  `device-<первые 8 символов MAC без двоеточий>` (например `device-aabbccdd`).
- **Валидация MAC:** `^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$` (допускает `:` и `-`).
- Перед записью — `isSafePath(file)` и `createLocalBackup(file)`.
- **Внимание:** `dnsmasq --test` здесь НЕ запускается (прямая запись в файл).

---

## 6. Восстановление из ZIP-архива

Зеркало `GET /api/backup`: загрузка архива, распаковка в `-conf-dir` с `.bak`
заменяемых файлов и проверкой конфига.

### API

```
POST /api/backup/restore
Authorization: Bearer <token>
Content-Type: multipart/form-data

file: backup.zip

→ 200 {"status":"ok"}
→ 400 {"error":"no_file"}                                   # нет поля file
→ 400 {"error":"invalid_zip"}
→ 400 {"error":"no_valid_conf_files"}                       # в архиве нет .conf
→ 400 {"error":"dnsmasq_test_failed","detail":"..."}
```

### Механизм (`dnsmasq.go`)

`restoreBackupZip(data) error`:

1. `zip.NewReader`, обход всех записей.
2. Берётся `filepath.Base(name)`, пропускаются не-`.conf` и небезопасные имена
   (защита от `../` и абсолютных путей).
3. Для существующего файла пишется `<file>.restore.bak`.
4. Запись содержимого из архива.
5. `dnsmasq --test`; при неудаче все изменённые файлы откатываются из
   `.restore.bak` (сами `.restore.bak` не удаляются).

---

## 7. Rate-limit на `/api/login`

Защита от брутфорса (bcrypt + неограниченные попытки = уязвимость).

### Поведение (`auth.go`)

- Лимит **10 попыток с одного IP за 1 минуту** (зашито в регистрации роута в
  `main.go`: `rateLimitMiddleware(10, time.Minute)`).
- При превышении — `429 {"error":"too_many_attempts"}`.
- Хранилище: пакетный `rateLimitStore map[string][]time.Time` + `sync.Mutex`.
- Очистка **ленивая**: внутри middleware, если с последней очистки прошло >5 мин,
  просроченные метки удаляются. Фонового cleanup-горутины НЕТ.
- **Сброса счётчика при успешном логине нет.**

> За reverse-proxy: `c.ClientIP()` вернёт реальный IP только при корректной
> настройке доверенных прокси (deployment-конфигурация).

---

## 8. Logout / JWT blacklist

Простой in-memory blacklist отозванных токенов.

### API

```
POST /api/logout
Authorization: Bearer <token>

→ 200 {"status":"logged_out"}
```

### Механизм (`auth.go`)

- Пакетные `blacklist map[string]time.Time` (jti → exp) + `blacklistMu sync.RWMutex`.
- `logoutHandler` парсит токен, достаёт `jti` и `exp`, вызывает
  `revokeToken(jti, exp)`.
- `authMiddleware` проверяет `isTokenRevoked(jti)` и при попадании в blacklist
  отдаёт 401.
- Фоновая горутина `cleanBlacklistLoop` каждые 10 минут удаляет записи с истёкшим `exp`.

### Ограничение

Blacklist in-memory: при рестарте процесса очищается, отозванные токены снова
валидны до `exp` (72ч). Это сознательный выбор («простой blacklist» по ТЗ).

---

## 9. OUI Lookup

Встроенная таблица производителей по первым 3 октетам MAC.

- `oui.go` — `ouiTable map[string]string` (сотни записей: VMware, Apple, Cisco,
  Netgear, Dell, HP, Raspberry Pi, QEMU/KVM и др.) + `lookupOUI(mac) string`.
- Без учёта регистра, ключ — `xx:xx:xx` (первые 8 символов). При отсутствии — `""`.

---

## Изменённые файлы

| Файл | Назначение |
|---|---|
| `auth.go` | blacklist, rate-limiter, `authMiddleware` (заголовок + `?token=`) |
| `dnsmasq.go` | SSE broker, `readFileRaw`/`writeFileRaw`, `restoreBackupZip`, `getNewDevices` |
| `handlers.go` | REST-хендлеры всех новых функций |
| `main.go` | инициализация `startSSEBroadcaster`, регистрация роутов |
| `models.go` | `UserPasswordReq`, `NewDeviceInfo`, `BulkLeaseToStaticReq` |
| `oui.go` | OUI-таблица и `lookupOUI` |
| `dnsmasq_test.go` | тесты для всех новых функций |
| `frontend/src/App.vue` | вкладки «Новые устройства», «Пользователи» |
| `frontend/src/components/NewDevicesTab.vue` | компонент новых устройств |
| `frontend/src/components/UsersTab.vue` | управление пользователями |
| `frontend/src/components/leases/LeasesTab.vue` | bulk lease-to-static UI |
| `frontend/src/store.js` | API-методы + `EventSource('/api/events?token=…')` |
| `frontend/src/locales/{en,ru}.json` | переводы |
