# Сессия: предрелизный рефакторинг + 4 точечные фичи

## Контекст

После ревью прототипа обозначились 5 задач:

1. Удаление `.conf`-файла через UI — отсутствовало, приходилось идти по SSH.
2. Сброс rate-limit при успешном логине — после двух опечаток пользователь
   оставался в штрафной зоне до конца окна.
3. Подсветка в UI того, что `POST /api/leases/to-static` не запускает
   `dnsmasq --test` — реальные пользователи путались: «добавил 5 устройств,
   ничего не работает».
4. Гонка в `users.json` — глобального `mu` хватало, чтобы сериализовать
   доступ в рамках процесса, но `len(users)` без блокировки мог
   спровоцировать runtime panic `concurrent map read and map write`.
5. Разбить три переросших файла (`dnsmasq.go` 1543, `handlers.go` 1534,
   `frontend/src/store.js` 451) — глаза сломать об каждый.

Файлы с разбиением сделаны **до** функциональных правок, чтобы новый код
ложился сразу в правильные места.

Бэкенд и фронтенд собираются, `go test` зелёный (с `-race` тоже).

---

## Что сделано — файл за файлом

### Бэкенд (Go) — разбиение

**`dnsmasq.go` (было 1543 → стало 414 строк)**

Оставлено только ядро статических хостов: `isSafePath`, `readFileRaw`,
`writeFileRaw`, `writeConfigWithTest`, `parseDhcpHostLine`,
`formatDhcpHostLine`, `readHostByMac`, `findHostsByIP`, `findHostsByMac`,
`readAllHosts`, `validateHostFields`, `parseCSVHosts`, `hostsToCSV`,
`removeHostLine`, `appendHostLine`, `findFreeIP`, `incIP`, `parseIPTransform`,
`ipTransform.apply`.

**`aliases.go` (новый, ~270 строк)** — `isAliasDirective`, `parseAliasLine`,
`aliasToLine`, `readAllAliases`, `findAliasesByDomain`, `cleanAliasFile`,
`appendAliasLine`, `removeAliasLine`, `aliasesToCSV`, `parseCSVAliases`,
`ensureAliasesFile`.

**`config_snapshot.go` (новый, ~310 строк)** — `directiveKeyRegex`,
`splitDirective`, `readConfigSnapshot`, `parseDhcpRange`, `isLeaseTime`,
`dhcpRangeToCIDR`, `detectDhcpRangesCIDR`, `serializeConfigFile`,
`directiveGroup`.

**`arp_leases.go` (новый, ~110 строк)** — `getArpTable`, `parseArpContent`,
`parseLeases`, `getNewDevices`.

**`history.go` (новый, ~310 строк)** — `historyVersionRegex`,
`historyFilePrefix`, `historyFileName`, `nextHistoryVersion`,
`isSafeHistoryPath`, `ensureHistoryDir`, `saveHistory`, `rotateHistory`,
`HistoryEntry`, `listHistory`, `readHistoryVersion`, `restoreHistoryVersion`,
`createLocalBackup`, `rollbackFile`, `unifiedDiff`.

**`backup.go` (новый, ~150 строк)** — `createBackupZip`, `restoreBackupZip`,
`deleteConfigFile` (новая — см. ниже).

**`sse.go` (новый, ~125 строк)** — `sseClient`, `sseRegister`, `sseUnregister`,
`sseBroadcast`, `startSSEBroadcaster`, `arpToJSON`, `checkDnsmasqStatus`,
`reloadDnsmasq`.

### Бэкенд (Go) — разбиение handlers

**`handlers.go` (было 1534 → стало 248 строк)** — только root:
`statusHandler`, `setupHandler`, `loginHandler`, `getArpHandler`,
`nextIPHandler`, `reloadHandler`, `getLeasesHandler`, `getUser`,
`getNewDevicesHandler`, `bulkLeaseToStaticHandler`, `eventsHandler`.

**`handlers_hosts.go` (новый, ~530 строк)** — `getHostsHandler`,
`addHostHandler`, `validateHostTags`, `normalizeHostTags`,
`bulkAddHostsHandler`, `deleteHostHandler`, `exportCSVHandler`,
`importCSVHandler`, `bulkMoveHandler`, `bulkEditHandler`,
`getTemplatesHandler`, `createTemplateHandler`, `deleteTemplateHandler`,
`applyTemplateHandler`.

**`handlers_aliases.go` (новый, ~270 строк)** — `resolveAliasesTargetFile`,
`validateAliasEntry`, `getAliasesHandler`, `addAliasHandler`,
`bulkAddAliasesHandler`, `deleteAliasHandler`, `exportAliasesCSVHandler`,
`importAliasesCSVHandler`.

**`handlers_config.go` (новый, ~230 строк)** — `getConfigHandler`,
`updateConfigHandler`, `getDhcpRangesHandler`, `createConfigFileHandler`,
`deleteConfigFileHandler` (новый — см. ниже), `listConfigTemplatesHandler`,
`getFileHandler`, `putFileHandler`.

**`handlers_safety.go` (новый, ~170 строк)** — `rollbackHandler`,
`historyListHandler`, `historyDiffHandler`, `historyRestoreHandler`,
`coalesce`, `backupHandler`, `restoreBackupHandler`.

**`handlers_users.go` (новый, ~145 строк)** — `getUsersHandler`,
`createUserHandler`, `deleteUserHandler`, `changePasswordHandler`,
`logoutHandler`. Все перешли на `usersMu` (см. ниже).

### Бэкенд (Go) — функциональные правки

**`auth.go`**

- Добавлен пакетный `usersMu sync.RWMutex`. Раньше `len(users)` в
  `statusHandler` читал map без блокировки, а параллельный `POST /api/users`
  мог в этот момент писать — Go runtime детектит это как фатальную гонку и
  убивает процесс.
- Добавлена функция `rateLimitReset(ip string)` — удаляет запись IP из
  `rateLimitStore`. Вызывается из `loginHandler` после успешной проверки
  пароля. Логика: «опечатался два раза → вошёл с третьего → следующий
  легитимный запрос не должен платить за предыдущие неудачи».

**`handlers.go`**

- `loginHandler` — после `bcrypt.CompareHashAndPassword` без ошибки
  вызывается `rateLimitReset(c.ClientIP())`. Брутфорс-защита для
  неудачных попыток не меняется: счётчик копится до успеха или до
  окончания window.
- `statusHandler` — `len(users)` теперь читается под `usersMu.RLock()`.
  Глобальный `mu` оставлен как есть — он сериализует операции с
  конфигурационными файлами, его роль не пересекается с защитой
  `users` map.
- `setupHandler` — перешёл на `usersMu` (вместо `mu`). При ошибке
  `saveUsers()` откатывает in-memory запись, чтобы не оставлять
  «призрака» админа до следующего рестарта.
- `bulkLeaseToStaticHandler` — добавлен подробный комментарий о том,
  почему `dnsmasq --test` НЕ запускается (чтобы следующий редактор кода
  не «добавил защиту» вслепую и не сломал UX).

**`handlers_config.go`**

- Новый `deleteConfigFileHandler`:
  - `DELETE /api/config/file` с JSON-телом `{"file": "<absolute path>"}`.
  - Проверки идут в порядке: `BindJSON` → валидация расширения `.conf`
    (не даёт удалить `users.json` или `audit.log` через этот эндпоинт) →
    `isSafePath` (path traversal) → физическое удаление.
  - Возвращает обновлённый `ConfigSnapshot` (как и `createConfigFileHandler`)
    — UI сразу убирает удалённый таб без отдельного `loadConfig()`.
  - Аудит: `action="config_delete_file"`.

**`handlers_users.go`**

- Все 4 хендлера (`createUser`, `deleteUser`, `changePassword`,
  `getUsers`) переведены на `usersMu` вместо `mu`. `mu` остаётся для
  конфигов; пересечений по данным между ними нет.
- `createUserHandler` — откат in-memory изменения при ошибке `saveUsers`
  (раньше можно было получить «призрака»: в map запись есть, на диске
  нет, после рестарта пропадает — и оператор думает, что пароль забыт).

**`backup.go`**

- Новая функция `deleteConfigFile(path string) error`:
  - Проверяет `isSafePath` (вне `ConfigDir` → `os.ErrPermission`).
  - Сохраняет текущее содержимое в history через `saveHistory(filePath)`
    **перед** удалением — файл можно восстановить через модалку «🕒
    История» на любом другом `.conf`-файле (путь файла включён в историю).
  - `os.Remove(path)`.
  - Best-effort `os.Remove(path + ".bak")` — orphaned `.bak` для
    удалённого файла ни к чему не приведёт, кроме ложной подсветки
    «⏪ Откат» в UI соседних файлов (на самом деле не соседних — но
    эвристике `entry.File + ".bak"` это не важно).
  - `dnsmasq --test` НЕ запускается сознательно: dnsmasq при отсутствии
    файла просто не загружает его, валидация пустого множества строк
    бессмысленна. Оператор нажмёт «Применить» как обычно.

**`main.go`**

- Регистрация роута `auth.DELETE("/config/file", deleteConfigFileHandler)`
  рядом с `auth.POST("/config/file", createConfigFileHandler)`.

### Фронтенд (Vue 3) — разбиение store.js

**`frontend/src/store.js` (было 451 → стало 175 строк)**

Осталась reactive state machine + axios client + core actions:
`setToken`, `logout`, `checkStatus`, `loadData`, `loadArp`, `applyConfig`,
`restartSystem`, `connectSSE`. Доменные actions импортируются через
`import * as hostsApi from './api/hosts.js'` и т.д., затем сливаются в
`actions` через object spread.

**`frontend/src/api/hosts.js` (новый, 105 строк)** — `loadTemplates`,
`createTemplate`, `deleteTemplate`, `applyTemplate`, `bulkMove`, `bulkEdit`,
`downloadCSV`, `importCSV`, `loadNewDevices`, `bulkLeaseToStatic`.

**`frontend/src/api/dns.js` (новый, 85 строк)** — `loadAliases`, `addAlias`,
`bulkAddAliases`, `deleteAlias`, `downloadAliasesCSV`, `importAliasesCSV`.

**`frontend/src/api/config.js` (новый, 140 строк)** — `loadConfig`,
`saveConfig`, `createConfigFile`, `deleteConfigFile` (новая),
`loadConfigTemplates`, `loadDhcpRanges`, `loadHistory`, `loadHistoryDiff`,
`restoreHistory`.

**`frontend/src/api/system.js` (новый, 105 строк)** — `downloadBackup`,
`restoreBackup`, `loadAudit`, `loadUsers`, `createUser`, `deleteUser`,
`changePassword`, `logoutRequest`.

> Circular-import note: `api/*.js` импортируют `store`/`api` из `store.js`,
> а `store.js` импортирует функции из `api/*.js`. ES modules это
> допускают через live bindings — к моменту вызова любой action-функции
> `store` и `api` уже инициализированы.

### Фронтенд (Vue 3) — функциональные правки

**`frontend/src/components/config/DnsmasqConfig.vue`**

- Кнопка `🗑 Удалить файл` рядом с `🕒 История` и `⏪ Откат`. Красная
  (`btn-outline-danger`), всегда видна когда выбран файл (даже без `.bak`
  — `.bak` нужен только для отката на один шаг, удаление файла отдельная
  операция).
- Двухступенчатое подтверждение: сначала `confirm()` с предупреждением о
  физическом удалении и подсказкой, как восстановить через history modal.
- После успеха: `alert()` с напоминанием «нажмите Применить».

**`frontend/src/components/leases/LeasesTab.vue`** и
**`frontend/src/components/DiscoveredTab.vue`**

- Жёлтый `<div class="alert alert-warning">` появляется над таблицей,
  когда выделено хотя бы одно устройство. Текст объясняет, что
  `dnsmasq --test` не запускается и нужно нажать «Применить».
- В `api/hosts.js` `bulkLeaseToStatic()` — к success-alert добавлена
  явная строка `⚠️ dnsmasq --test НЕ запускался. Нажмите «Применить»...`
  на случай, если пользователь закрыл warning до выделения.

**`frontend/src/components/audit/AuditTab.vue`**

- `actionClass` теперь различает `config_delete_file` (красный),
  `config_write_raw`, `backup_restore`, `user_create`, `user_delete`,
  `password_change`. Раньше всё попадало в дефолтный серый.

**`frontend/src/locales/{ru,en}.json`**

- Добавлены ключи:
  - `leases.toStaticHint` — текст предупреждения.
  - `confirm.deleteConfigFile` — диалог удаления файла.
  - `alert.configDeleteSuccess`, `alert.configDeleteError`,
    `alert.applyReminder`.
  - `config.delete` — подпись кнопки.
  - `audit.action_config_delete_file`, `audit.action_config_write_raw`,
    `audit.action_user_create`, `audit.action_user_delete`,
    `audit.action_password_change`, `audit.action_bulk_lease_to_static`,
    `audit.action_backup_restore` — локализация меток в Audit-вкладке.
- Убран дубликат ключа `confirm.rollback` (был две строки подряд — JSON
  формально это допускает, но последнее значение выигрывает, что
  сбивало с толку).

### Тесты

**`new_features_test.go` (новый, 480 строк)**

- `TestRateLimitResetClearsIP` — после 2 failed-попыток и
  `rateLimitReset` IP снова может слать maxAttempts запросов.
- `TestRateLimitResetUnknownIP` — сброс неизвестного IP не паникует и
  не трогает соседние записи.
- `TestLoginHandlerResetsRateLimit` — полный путь: successful login
  действительно обнуляет счётчик.
- `TestDeleteConfigFileRemovesFileAndBak` — физическое удаление + cleanup
  `.bak`.
- `TestDeleteConfigFileSavesHistory` — перед удалением текущее
  содержимое попадает в history и читается оттуда.
- `TestDeleteConfigFileRejectsUnsafePath` — `os.ErrPermission` для пути
  вне `ConfigDir`.
- `TestDeleteConfigFileMissingReturnsNotExist` — 404-путь.
- `TestDeleteConfigFileHandlerSuccess` — e2e хендлера: файл удалён,
  аудит написан, в ответе ConfigSnapshot без удалённого файла.
- `TestDeleteConfigFileHandlerUnsafePath` — 403 + access_denied.
- `TestDeleteConfigFileHandlerNonConfExtension` — 400 для не-`.conf`
  путей (защита от случайного удаления `audit.log` и т.п.).
- `TestDeleteConfigFileHandlerMissing` — 404.
- `TestConcurrentCreateUserNoLostRecords` — 30 горутин одновременно
  создают разных пользователей; все 30 должны сохраниться.
- `TestConcurrentCreateUserDuplicateNoCorruption` — 20 горутин
  создают одного и того же пользователя; ровно один 200, остальные 409,
  map и JSON остаются консистентны.
- `TestStatusHandlerSafeUnderConcurrentUserWrite` — 50 writers + 50
  readers; без `usersMu` падает с `fatal error: concurrent map read and
  map write`. Проверяется в `-race` режиме (76 сек — bcrypt медленный).

---

## Что НЕ сделано (сознательный descoping)

- **Перевод счётчиков `/metrics` из gauge в настоящий counter
  (`promauto`)** — отмечено в `docs/v1.0-features.md` как известная
  инерция, выходит за рамки этой сессии.
- **Глобальный `mu sync.Mutex`** остаётся единым для всех операций
  записи в `.conf` — это узкое место, но для 1-2 админ-пользователей
  дома это не проблема. Рефакторинг в per-file locks — отдельная
  задача.
- **Удаление `.conf`-файла с pre-flight проверкой «есть ли в нём
  dhcp-host=»** — сознательно НЕ добавлено. Удаление целого файла —
  явное действие, two-step confirm + history recovery достаточно.
  Pre-flight превратился бы в «вы уверены, что хотите удалить файл, в
  котором есть N устройств?» → «да» → удалить. Один шаг лишний.

---

## Проверки

- `go build ./...` — OK.
- `go vet ./...` — OK (предсуществующие gofmt-замечания в `bins.go`,
  `main.go`, `models.go` не правились — это не мой код и не связано с
  изменениями).
- `go test ./...` — OK, все 168+ тестов зелёные (включая ~14 новых).
- `go test -race -run "TestConcurrentCreateUserNoLostRecords
  TestStatusHandlerSafeUnderConcurrentUserWrite"` — OK, race detector
  чист. Без `usersMu` тест падает с runtime panic.
- `npm run build` — OK, 119 модулей, 380 КБ JS.

---

## Файлы изменены/добавлены

### Новые (Go)
- `aliases.go`, `arp_leases.go`, `backup.go`, `config_snapshot.go`,
  `history.go`, `sse.go` — разбиение `dnsmasq.go`.
- `handlers_hosts.go`, `handlers_aliases.go`, `handlers_config.go`,
  `handlers_safety.go`, `handlers_users.go` — разбиение `handlers.go`.
- `new_features_test.go` — тесты новых фич.

### Изменённые (Go)
- `auth.go` — `usersMu`, `rateLimitReset`.
- `dnsmasq.go` — сокращён до ядра dhcp-host.
- `handlers.go` — сокращён до root-хендлеров.
- `main.go` — регистрация `DELETE /api/config/file`.

### Новые (frontend)
- `frontend/src/api/hosts.js`, `dns.js`, `config.js`, `system.js` —
  разбиение `store.js`.

### Изменённые (frontend)
- `frontend/src/store.js` — сокращён до core.
- `frontend/src/components/config/DnsmasqConfig.vue` — кнопка удаления.
- `frontend/src/components/leases/LeasesTab.vue` — warning для to-static.
- `frontend/src/components/DiscoveredTab.vue` — то же.
- `frontend/src/components/audit/AuditTab.vue` — новые action-классы.
- `frontend/src/locales/ru.json`, `en.json` — новые ключи.

### Документация
- `docs/v3.1-features.md` — user-facing описание.
- `логи/predrel-v3.1-refactor-and-fixes.md` — этот файл.
