# Сессия: predrel — аудит, валидация дубликатов, CSV импорт/экспорт

## Контекст

Ветка `predrel`. Добавлены три новые функции: аудит-лог изменений, жёсткая валидация дубликатов IP/MAC, импорт/экспорт через CSV.

## Изменения

### Новый файл: `audit.go`

- Структура `AuditEntry` — timestamp, user, action, mac, hostname, ip, file
- `writeAudit(entry)` — дописывает JSON-lines в файл (по умолчанию `/etc/intermasq/audit.log`), создаёт директорию при необходимости, логирует ошибки в консоль
- `auditHandler(c)` — читает лог, возвращает массив `[]AuditEntry`
- Флаг `-audit-log` (default `/etc/intermasq/audit.log`) — настраиваемый путь к файлу аудита

### `auth.go` — проброс пользователя в контекст

- `authMiddleware`: после успешной проверки JWT извлекает `sub` из claims, кладёт в `c.Set("user", username)`
- Для API-key авторизации — `c.Set("user", "api-key")`
- Хелпер `getUser(c *gin.Context) string` в `handlers.go`

### `handlers.go` — аудит во всех мутирующих операциях

- `addHostHandler` — `writeAudit` с action=`add`, записывает mac/hostname/ip/file
- `bulkAddHostsHandler` — `writeAudit` с action=`bulk_add`, mac = количество хостов
- `deleteHostHandler` — `writeAudit` с action=`delete`, извлекает удалённые hostname/ip из строки перед удалением
- `rollbackHandler` — `writeAudit` с action=`rollback`
- `reloadHandler` — `writeAudit` с action=`reload`

### `dnsmasq.go` — валидация дубликатов

- `findHostsByIP(ip, excludeMac string) []HostEntry` — обходит все `.conf` в `ConfigDir`, ищет `dhcp-host=` с указанным IP, исключая запись с MAC = `excludeMac`
- `findHostsByMac(mac string) []HostEntry` — обходит все `.conf`, ищет `dhcp-host=` с указанным MAC (case-insensitive)

### `handlers.go` — валидация дубликатов

- `addHostHandler` — перед записью:
  - `findHostsByIP(req.Ip, req.Mac)` → если есть конфликты → `409 ip_duplicate`
  - `findHostsByMac(req.Mac)` → если есть конфликты → `409 mac_duplicate`
- `bulkAddHostsHandler` — для каждого хоста в пачке:
  - Внутренние дубликаты IP (разные MAC, один IP) → `409 ip_duplicate_bulk`
  - `findHostsByIP` → `409 ip_duplicate`
  - `findHostsByMac` → `409 mac_duplicate`
- `importCSVHandler` — аналогично bulk: внутренние дубликаты + `findHostsByIP` + `findHostsByMac`
- Логирование в консоль: `[VALIDATION] IP/MAC duplicate detected` или `is unique, proceeding`

### `dnsmasq.go` — CSV helpers

- `readAllHosts() []HostEntry` — обобщает логику чтения хостов из всех `.conf` (без хака `|has_bak`)
- `hostsToCSV(hosts) []byte` — сериализует в CSV: `mac,ip,hostname` (без колонки `file`)
- `parseCSVHosts(reader, targetFile) ([]HostEntry, error)` — парсит CSV через `encoding/csv`, валидирует MAC/IP/hostname, назначает `targetFile`

### `handlers.go` — CSV эндпоинты

- `GET /api/hosts/csv` (`exportCSVHandler`) — экспорт всех хостов в CSV, `Content-Type: text/csv`, `Content-Disposition: attachment`
- `POST /api/hosts/csv` (`importCSVHandler`) — multipart/form-data с полями `file` (CSV) и `target_file` (путь назначения), парсит → валидирует дубликаты → пишет в файл → аудит

### `main.go` — новые маршруты и флаги

- Флаг `-audit-log` (default `/etc/intermasq/audit.log`)
- Маршруты:
  - `GET /api/audit` — список записей аудита
  - `GET /api/hosts/csv` — экспорт CSV
  - `POST /api/hosts/csv` — импорт CSV

### Фронтенд

#### `store.js`
- `store.auditLog: []` — новое поле в reactive store
- `loadData()` — теперь грузит аудит в `Promise.all` вместе с остальными данными
- `loadAudit()` — отдельный метод для загрузки аудита
- `downloadCSV()` — скачивание CSV через blob
- `importCSV(file, targetFile)` — FormData POST, alert об успехе/ошибке

#### `App.vue`
- Новая вкладка «История» (`tabAudit`) в `btn-group`
- Кнопка «📥 CSV» в панели инструментов (рядом с Backup)
- Импорт `AuditTab`, рендер `<AuditTab v-if="store.tab === 'audit'" />`

#### `HostForm.vue`
- Селектор режимов импорта: Одиночный (`single`) / Список (`text`) / CSV (`csv`)
- CSV-режим: `<input type="file" accept=".csv">` + поле файла назначения + кнопка импорта
- Удалён старый чекбокс `isImportMode`, заменён на `importMode` ref
- Исправлена сломанная HTML-структура (отсутствующий `</div>`)

#### `components/audit/AuditTab.vue` (новый)
- Таблица: время, пользователь, действие, MAC, hostname, IP, файл
- Цветные бейджи для действий (add=зелёный, delete=красный, rollback=жёлтый, reload=синий)
- Обратная сортировка (свежие сверху)
- `formatTime()` — локализованное время

#### Локали (`ru.json`, `en.json`)
- `app.tabAudit`, `app.csvExport`, `app.csvExportTooltip`
- `hosts.csvMode`
- `alert.csvExportError`, `alert.csvImportError`, `alert.csvImportSuccess`, `alert.ipDuplicate`, `alert.ipDuplicateBulk`
- `api.ip_duplicate`, `api.ip_duplicate_bulk`, `api.mac_duplicate`, `api.no_file`, `api.invalid_csv`, `api.csv_empty`
- `audit.*` — заголовки таблицы

### Тесты

- Все 13 существующих тестов проходят без изменений
- Go-бэкенд компилируется (`GOOS=linux GOARCH=amd64`)
- Фронтенд собирается (`vite build`)
