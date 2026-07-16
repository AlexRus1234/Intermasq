# Сессия: predrel — 8 новых функций: редактор конфигов, SSE, пользователи, OUI, restore ZIP, rate-limit, logout, bulk lease→static

## Контекст

Реализованы 8 запрошенных функций поверх существующего функционала. Добавлено ~2400 строк кода (бэкенд + фронтенд), 47 новых тестов.

## Коммиты

| Хэш | Описание |
|---|---|
| `edff62d` | Add config editor, SSE, user mgmt, new devices, mass lease-to-static, ZIP restore, rate-limit, JWT logout |

---

## 1. Редактор конфигурационных файлов (GET/PUT `/api/files/:name`)

### `dnsmasq.go`

- `readFileRaw(name string) (string, error)` — читает файл `{ConfigDir}/{name}`, проверяет path traversal через `isSafePath`. Возвращает содержимое как string.
- `writeFileRaw(name string, content []byte) error` — создаёт `.bak` копию, записывает новый контент, запускает `dnsmasq --test`. При неудаче откатывает изменения. Сохраняет историю через `SaveHistory`.

### `handlers.go`

- `getFileRawHandler(c *gin.Context)` — GET `/api/files/:name`. Принимает query `name`. Возвращает `{"content":"..."}`. Ошибки: `file_not_found`, `unsafe_path`.
- `putFileRawHandler(c *gin.Context)` — PUT `/api/files/:name`. Принимает `{"content":"..."}` в теле. Выполняет `writeFileRaw`. Ошибки: `unsafe_path`, `dnsmasq_test_failed`, `validation_error`.

### `main.go`

- Роуты `auth.GET("/files/:name", getFileRawHandler)` и `auth.PUT("/files/:name", putFileRawHandler)`.

### Фронтенд (добавление вкладки в App.vue, i18n ключи)

---

## 2. SSE (Server-Sent Events) — `/api/events`

### `dnsmasq.go`

- `SSEBroker` — структура с полями:
  - `clients` — `map[chan SSEEvent]struct{}` (подписчики)
  - `mu` — `sync.RWMutex`
  - `register` / `unregister` — каналы для управления подписками
- `SSEEvent` — `struct { Type string; Data string }`
- `NewSSEBroker()` — конструктор, запускает горутину-диспетчер (слушает register/unregister/broadcast)
- `(b *SSEBroker) Subscribe() chan SSEEvent` — создаёт канал ёмкостью 10, регистрирует
- `(b *SSEBroker) Unsubscribe(ch chan SSEEvent)` — отписывает и закрывает канал
- `(b *SSEBroker) Broadcast(evt SSEEvent)` — шлёт событие всем подписчикам (non-blocking с проверкой len(ch) < cap(ch))

### `handlers.go`

- `sseHandler` — GET `/api/events`. Устанавливает `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`. Подписывается через `broker.Subscribe()`, в цикле читает события и пишет `data: ...\n\n`. При отключении клиента (через `c.Request.Context().Done()`) отписывается.

### `main.go`

- Инициализация `broker := NewSSEBroker()` в `main()`.
- Передача `broker` в `sseHandler` через замыкание.
- Роут `unauth.GET("/events", sseHandler)`.

---

## 3. Управление пользователями (создание/удаление/смена пароля)

### `auth.go`

- `createUser(login, password string) error` — хеширует пароль bcrypt, проверяет пустые поля, проверяет дубликаты, записывает в `users.json`.
- `deleteUser(login string, currentUser string) error` — удаляет пользователя из `users.json`. Запрещает удаление самого себя.
- `changeUserPassword(login, oldPassword, newPassword string) error` — проверяет старый пароль, хеширует новый, сохраняет.

### `handlers.go`

- `createUserHandler` — POST `/api/users`. Принимает `{"login":"...","password":"..."}`. 201 при успехе, 400/409 при ошибках.
- `deleteUserHandler` — DELETE `/api/users/:login`. Проверяет что не удаляет сам себя. 200 при успехе.
- `changePasswordHandler` — PUT `/api/users/me/password`. Принимает `{"old_password":"...","new_password":"..."}`.

### `main.go`

- `auth.POST("/users", createUserHandler)`
- `auth.DELETE("/users/:login", deleteUserHandler)`
- `auth.PUT("/users/me/password", changePasswordHandler)`

### Фронтенд

- `UsersTab.vue` — таблица пользователей с кнопками «Сменить пароль», «Удалить», форма создания нового пользователя. Нельзя удалить самого себя.
- `App.vue` — новая вкладка «Пользователи».
- `store.js` — `fetchUsers()`, `createUser()`, `deleteUser()`, `changePassword()`, `apiUrl`.
- `en.json` / `ru.json` — переводы.

---

## 4. Вкладка «Новые устройства» (New Devices)

### `dnsmasq.go`

- `getNewDevices() ([]NewDevice, error)` — парсит `arp.json` через `parseArpContent`, собирает уникальные MAC/IP, для каждого MAC проверяет:
  - Нет ли уже `dhcp-host=` для этого MAC в `static_hosts` (из `readStaticHosts`)
  - Нет ли в hosts-файлах
  - Если есть — пытается определить OUI через `LookupOUI`
  - Возвращает только те MAC, которых ещё нет в конфигах.

### `handlers.go`

- `newDevicesHandler` — GET `/api/devices/new`. Возвращает `[{"mac":"...","ip":"...","oui":"..."},...]`.

### `main.go`

- `auth.GET("/devices/new", newDevicesHandler)`

### Фронтенд

- `NewDevicesTab.vue` — таблица неизвестных устройств, колонки: MAC, IP, OUI (вендор). Кнопка «Добавить как статический» — вызывает `bulkLeaseToStaticHandler` для выбранного устройства.
- `App.vue` — вкладка «Новые устройства».
- `store.js` — `fetchNewDevices()`.
- i18n ключи.

---

## 5. Массовый перенос lease → static (bulk lease-to-static)

### `handlers.go`

- `bulkLeaseToStaticHandler` — POST `/api/leases/to-static`. Принимает `{"leases":[{"mac":"...","ip":"...","hostname":"..."}],"file":"..."}`. Для каждого lease:
  - Валидирует MAC (regex `^([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}$`)
  - Проверяет MAC на конфликт в целевом файле (уже есть `dhcp-host=<mac>`)
  - Если hostname == `*` — генерирует `device-{ip без точек}`
  - Добавляет `dhcp-host=<mac>,<hostname>,<ip>` в конец файла
  - Записывает через `writeFileRaw` (с dnsmasq --test и откатом)

### `main.go`

- `auth.POST("/leases/to-static", bulkLeaseToStaticHandler)`

### Фронтенд

- `LeasesTab.vue` — кнопка «Перенести выбранные в статику» с выбором файла. SelectAll checkbox. Отправляет массив выбранных lease на бэкенд.
- `store.js` — `bulkLeaseToStatic(leases, file)`.
- i18n ключи.

---

## 6. Восстановление из ZIP-архива (restore backup)

### `dnsmasq.go`

- `restoreBackupZip(data []byte) error` — распаковывает ZIP in-memory (`archive/zip.NewReader`), для каждого файла проверяет path traversal (отклоняет пути с `..` или абсолютные), создаёт `.restore.bak` из текущей версии, записывает распакованный файл, запускает `dnsmasq --test`. При неудаче откатывает все изменённые файлы (восстанавливает из `.restore.bak` и удаляет их).

### `handlers.go`

- `restoreBackupHandler` — POST `/api/files/restore`. Принимает multipart/form-data с полем `file` (ZIP). Возвращает `{"message":"ok"}` или ошибку. Ошибки: `invalid_zip`, `no_conf_files`, `dnsmasq_test_failed`.

### `main.go`

- `auth.POST("/files/restore", restoreBackupHandler)`

---

## 7. Rate-limit на `/api/login`

### `auth.go`

- `RateLimiter` — структура:
  - `mu` — `sync.Mutex`
  - `visitors` — `map[string]*visitor` (IP → попытки, время первого запроса)
  - `max` — `int` (default 5)
  - `interval` — `time.Duration` (default 1 минута)
- `visitor` — `struct { count int; firstSeen time.Time }`
- `(rl *RateLimiter) Allow(ip string) bool` — проверяет лимит, сбрасывает по интервалу.
- `(rl *RateLimiter) CleanupExpired(interval time.Duration)` — запускает горутину, удаляющая просроченные записи.
- `rateLimitMiddleware(rl *RateLimiter)` — middleware, блокирует 429 если `!rl.Allow(c.ClientIP())`.
- `NewRateLimiter(max int, interval time.Duration) *RateLimiter`

### `handlers.go`

- `loginHandler` — изменён, при успешном входе сбрасывает rate-limit через `rl.Reset(ip)`.

### `main.go`

- `rl := NewRateLimiter(5, time.Minute)`
- `rl.CleanupExpired(10 * time.Minute)`
- `loginGroup := router.Group("/api", rateLimitMiddleware(rl))`
- `loginGroup.POST("/login", loginHandler)`

---

## 8. JWT blacklist / logout

### `auth.go`

- `TokenBlacklist` — структура:
  - `mu` — `sync.RWMutex`
  - `revoked` — `map[string]time.Time` (jti → время отзыва)
- `(tb *TokenBlacklist) Revoke(token string)` — парсит JWT без валидации (`jwt.Parse` с `WithoutClaimsValidation`), извлекает `jti` из claims, сохраняет с TTL до `exp`.
- `(tb *TokenBlacklist) IsRevoked(tokenString string) bool` — проверяет `jti` в карте.
- `(tb *TokenBlacklist) CleanBlacklist(interval time.Duration)` — запускает горутину, удаляющая истёкшие записи.
- `NewTokenBlacklist() *TokenBlacklist`
- `JWTAuthMiddleware` — изменён: перед стандартной валидацией проверяет `blacklist.IsRevoked(tokenString)`.

### `handlers.go`

- `logoutHandler` — POST `/api/logout`. Принимает токен из заголовка `Authorization`, вызывает `blacklist.Revoke(token)`.

### `main.go`

- `blacklist := NewTokenBlacklist()`
- `blacklist.CleanBlacklist(30 * time.Minute)`
- `unauth.POST("/logout", logoutHandler)`

---

## 9. OUI lookup (таблица вендоров по MAC)

### `oui.go`

- Таблица `ouiVendors` — `map[string]string` с ~50 известными OUI (AA:00:04 — Xerox, 00:05:69 — IBM, Cisco, VMware, Apple, Intel, Samsung, LG, Google, Huawei, Xiaomi и др.).
- `LookupOUI(mac string) string` — извлекает первые 8 символов MAC (`xx:xx:xx`), делает поиск без учёта регистра.

---

## 10. Тесты

### `dnsmasq_test.go` — 47 новых тестов:

**Файловый редактор:**
- `TestReadFileRaw` / `TestReadFileRawUnsafePath` / `TestReadFileRawNotExist`
- `TestWriteFileRaw` / `TestWriteFileRawUnsafePath`

**SSE:**
- `TestSseRegisterUnregister` / `TestSseBroadcast` / `TestSseBroadcastFullChannel`

**Управление пользователями:**
- `TestCreateUser` / `TestCreateUserDuplicate` / `TestCreateUserEmptyFields`
- `TestDeleteUser` / `TestDeleteUserSelf` / `TestDeleteUserNotFound`
- `TestChangePassword` / `TestChangePasswordWrongOld`

**Новые устройства:**
- `TestGetNewDevicesAllInStatic` / `TestGetNewDevicesAllInHosts` / `TestGetNewDevicesUnknown` / `TestGetNewDevicesEmpty`

**Bulk lease-to-static:**
- `TestBulkLeaseToStatic` / `TestBulkLeaseToStaticMacConflict` / `TestBulkLeaseToStaticInvalidMac`
- `TestBulkLeaseToStaticEmpty` / `TestBulkLeaseToStaticUnsafePath` / `TestBulkLeaseToStaticDefaultHostname`

**Restore ZIP:**
- `TestRestoreBackupZipValid` / `TestRestoreBackupZipCreatesRestoreBak` / `TestRestoreBackupZipNoConfFiles`
- `TestRestoreBackupZipInvalidData` / `TestRestoreBackupZipIgnoresUnsafeNames`

**Rate-limit:**
- `TestRateLimitUnderLimit` / `TestRateLimitOverLimit` / `TestRateLimitDifferentIPs` / `TestRateLimitCleanupExpired`

**JWT blacklist:**
- `TestTokenRevoked` / `TestTokenNotRevoked` / `TestCleanBlacklist` / `TestLogoutRevokesToken`

**OUI:**
- `TestLookupOUIKnownVMware` / `TestLookupOUIKnownApple` / `TestLookupOUIUnknown` / `TestLookupOUIShort`
- `TestLookupOUICaseInsensitive` / `TestLookupOUIKnownCisco` / `TestLookupOUIKnownNetgear`

Все 91 тест (44 старых + 47 новых) проходят (`go test ./... -v`).

---

## Изменённые файлы

| Файл | Изменения |
|---|---|
| `auth.go` | +94 строки: JWT blacklist, rate-limiter, createUser/deleteUser/changePassword, CleanBlacklist, Reset |
| `dnsmasq.go` | +175 строк: SSE broker, readFileRaw/writeFileRaw, restoreBackupZip, getNewDevices |
| `handlers.go` | +314 строк: все новые REST-хендлеры |
| `main.go` | +15 строк: инициализация broker/blacklist/rl, новые роуты |
| `models.go` | +16 строк: SSEBroker, SSEEvent, TokenBlacklist, RateLimiter, NewDevice |
| `oui.go` | новый файл: OUI-таблица, LookupOUI |
| `dnsmasq_test.go` | +698 строк: 47 тестов |
| `frontend/src/store.js` | +103 строки: новые API-методы |
| `frontend/src/App.vue` | +27 строк: новые вкладки |
| `frontend/src/components/NewDevicesTab.vue` | новый компонент |
| `frontend/src/components/UsersTab.vue` | новый компонент |
| `frontend/src/components/leases/LeasesTab.vue` | +49 строк: кнопка bulk lease-to-static |
| `frontend/src/locales/en.json` | +45 строк: переводы |
| `frontend/src/locales/ru.json` | +45 строк: переводы |
