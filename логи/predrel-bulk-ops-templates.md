# Сессия: predrel — bulk-операции, шаблоны, auto-IP, фикс порта

## Контекст

Ветка `predrel`. Реализованы пункты 2 и 3 из плана расширения функционала:
групповые операции на хостах и шаблоны конфигураций с авто-подбором IP.
Заодно исправлен критичный баг молчаливого падения при ошибке бинда порта
и изменён порт по умолчанию из-за конфликта с Crowdsec.

## Коммиты

| Хэш | Описание |
|---|---|
| `017f297` | Add bulk-ops (move/edit) + auto next-IP + templates |
| `eb7b7c3` | Fix silent exit on bind failure |
| `f655c88` | Change default port 8080 -> 8081 |

---

## 1. Auto next-IP (`GET /api/hosts/next-ip`)

### `dnsmasq.go`

- `findFreeIP(cidr string) (string, error)` — парсит CIDR (`net.ParseCIDR`),
  проверяет что это IPv4 и маска >= /30, итерирует адреса от `network+1`
  до `broadcast-1`, для каждого вызывает `findHostsByIP(ip, "")` —
  первый без конфликта возвращает. Лимит в 256 итераций — защита от
  сканирования /16 и больше.
- `incIP(ip net.IP)` — инкремент IP побайтово с переносом.
- Валидация: `invalid_cidr`, `ipv6_not_supported`, `range_too_small`
  (маска >= /31), `range_exhausted` (прошли 256 адресов или дошли до broadcast).

### `handlers.go`

- `nextIPHandler` — принимает `?range=CIDR`, возвращает `{"ip":"..."}` или
  `400` с кодом ошибки. Под `authMiddleware`.

### `main.go`

- Роут `auth.GET("/hosts/next-ip", nextIPHandler)` — зарегистрирован
  **до** `/hosts/:mac`, чтобы не попасть под wildcard.

### `HostForm.vue`

- Кнопка `🎲` рядом с полем IP. Если у выбранного шаблона есть `ip_range` —
  использует его. Иначе показывает инпут для ручного ввода CIDR.
- Состояния: `autoIPLoading` (спиннер), `showRangeInput` (инпут CIDR).

---

## 2. Шаблоны конфигураций

### `models.go`

```go
type Template struct {
    ID, Name, IPRange, HostnamePattern, TargetFile string
}
type ApplyTemplateReq struct { Mac, TemplateID string }
```

### `templates.go` (новый файл)

- `templates map[string]Template` — глобальное хранилище в памяти.
- `loadTemplates()` — читает JSON в map при старте (по аналогии с `loadUsers`).
  Если файла нет — `MkdirAll` + пустая map.
- `saveTemplates()` — marshalIndent в `*TemplatesPath` (0600).
- `genHostnameFromPattern(pattern, index)` — заменяет `{NNN}` на
  `fmt.Sprintf("%03d", index)`.
- `countHostsInFile(file)` — считает строки `dhcp-host=` в файле
  (для индекса hostname).

### `handlers.go`

- `getTemplatesHandler` — список всех шаблонов.
- `createTemplateHandler` — валидация (Name/IPRange/HostnamePattern/TargetFile
  обязательны, `isSafePath` для TargetFile), ID генерируется из Name
  (lowercase, пробелы → дефисы). При дублировании ID — `409 template_exists`.
- `deleteTemplateHandler` — удаление по ID, `404` если нет.
- `applyTemplateHandler` — принимает `{mac, template_id}`, находит шаблон,
  вызывает `findFreeIP(tpl.IPRange)`, генерирует hostname
  (`countHostsInFile + 1`), возвращает предзаполненную форму. **Не пишет
  в файл** — только предзаполняет, пользователь подтверждает сохранение
  через обычный `POST /api/hosts`.

### `main.go`

- Флаг `-templates` (default `/etc/intermasq/templates.json`).
- `loadTemplates()` в `main()` после `loadUsers()`.
- Роуты: `GET/POST /api/templates`, `DELETE /api/templates/:id`,
  `POST /api/hosts/apply-template` — все под `authMiddleware`.

### `store.js`

- `templates: []` в state.
- `loadTemplates()`, `createTemplate()`, `deleteTemplate()`, `applyTemplate()`
  — в `actions`. Загрузка в `loadData()` через `Promise.all`.

### `TemplatesModal.vue` (новый компонент)

- Список существующих шаблонов с кнопкой удаления.
- Форма создания: Name, IP Range (CIDR), Hostname Pattern (`device-{NNN}`),
  Target File.
- Кнопка `⚙️` в `HostForm.vue` открывает модалку.

### `HostForm.vue`

- Dropdown шаблонов над полями формы (single-режим).
- При выборе шаблона: `form.file` блокируется, `form.ip`/`form.hostname`
  заполняются через `applyTemplate` (если введён MAC).
- Шаблон можно комбинировать с кнопкой `🎲` — диапазон берётся из шаблона.

---

## 3. Bulk-move (`POST /api/hosts/bulk-move`)

### `models.go`

```go
type BulkMoveReq struct {
    Hosts  []HostEntry  // mac + file (source)
    Target string       // путь назначения
}
```

### `dnsmasq.go`

- `removeHostLine(filePath, mac)` — вырезает строку `dhcp-host=...,mac,...`
  из файла, сохраняет остальные. Не создаёт бэкап (это делает вызывающий).
- `appendHostLine(filePath, mac, hostname, ip)` — дописывает строку в конец
  файла, сохраняя существующий контент. Создаёт файл если его нет.
- `readHostByMac(filePath, mac) *HostEntry` — поиск конкретной записи по MAC.

### `handlers.go`

- `bulkMoveHandler`:
  1. Валидация: `isSafePath` для каждого `h.File` и `Target`,
     `macRegex` для MAC. Если `h.File == Target` → `400 same_file`.
  2. Под мьютексом: для каждого хоста — `readHostByMac` (если не найден —
     в `skipped`), проверка конфликтов IP/MAC в целевом файле через
     `findHostsByIP`/`findHostsByMac` (если есть — в `skipped`),
     `createLocalBackup` обоих файлов, `removeHostLine` + `appendHostLine`.
  3. Audit: `action: "bulk_move"`, mac = `N moved, M skipped`.
  4. Ответ: `{"moved": N, "skipped": [...]}`.

### `main.go`

- Роут `auth.POST("/hosts/bulk-move", bulkMoveHandler)`.

### `BulkMoveModal.vue` (новый компонент)

- Селектор файла назначения (из существующих) + опция своего пути.
- Предпросмотр количества перемещаемых хостов.
- Кнопка `📦 Переместить` в панели выделения `HostTable.vue`.

---

## 4. Bulk-edit (`POST /api/hosts/bulk-edit`)

### `models.go`

```go
type BulkEditReq struct {
    Hosts             []HostEntry
    IPTransform       IPTransformSpec    { OldPrefix, NewPrefix }
    HostnameTransform HostnameTransformSpec { Suffix, StripOld }
}
```

### `dnsmasq.go`

- Тип `ipTransform` с режимами: `ipTransformNone`, `ipTransformOctets`
  (3-октетная строка вида `10.0.0`), `ipTransformCIDR` (CIDR вида `10.0.0.0/24`).
- `parseIPTransform(oldStr, newStr)` — автоопределение формата:
  - Оба с `/` → CIDR-режим, маски должны совпадать (`prefix_mismatch`).
  - Оба без `/` → октетный режим, число октетов должно совпадать.
  - Смешанный формат → `prefix_format_mismatch`.
  - Валидация через regex `^(\d{1,3}\.){0,2}\d{1,3}$`.
- `(t *ipTransform) apply(ip)`:
  - Октетный режим: `strings.HasPrefix` + проверка границы октета,
    затем строковая замена префикса.
  - CIDR-режим: битовая арифметика `(oldIP & ^mask) | newNetwork`.
  - Если IP не подпадает под старый префикс → `prefix_not_matched`.

### `handlers.go`

- `bulkEditHandler` — **строго транзакционный** (всё или ничего):
  1. Парсинг трансформа (`parseIPTransform`).
  2. Для каждого хоста: `readHostByMac` (если нет — `404 host_not_found`),
     применение IP-трансформа (`prefix_not_matched` → `400`),
     валидация нового IP (`net.ParseIP`),
     применение hostname-трансформа (`TrimSuffix` + конкатенация),
     валидация нового hostname (`hostnameRegex`).
  3. Pre-check внутренних дубликатов: если два хоста из выделения
     получают одинаковый новый IP → `409 ip_duplicate_bulk`.
  4. Pre-check внешних дубликатов: `findHostsByIP(newIP, existing.Mac)`.
  5. Только после прохождения всех проверок — `mu.Lock()`,
     `createLocalBackup` для каждого затронутого файла,
     `removeHostLine` + `appendHostLine` для каждого хоста.
  6. Audit: `action: "bulk_edit"`, mac = `N hosts`, ip = `old -> new`.

### `main.go`

- Роут `auth.POST("/hosts/bulk-edit", bulkEditHandler)`.

### `BulkEditModal.vue` (новый компонент)

- Поля: старый/новый IP-префикс, суффикс для добавления/удаления из hostname.
- Предпросмотр первых 5 хостов: `IP →`, `Hostname →`. CIDR-режим в превью
  помечен `(computed)` — реальное вычисление только на бэке.
- Кнопка `✏️ Изменить` в панели выделения `HostTable.vue`.

### `HostTable.vue`

- Панель выделения расширена: `📦 Move` / `✏️ Edit` / `🗑️ Delete`.
- Импорт `BulkMoveModal` и `BulkEditModal`.
- `onMoveDone` / `onEditDone` — закрытие модалок + очистка выделения.

---

## 5. Фикс молчаливого падения (`eb7b7c3`)

### `main.go`

```go
// Было:
r.Run(":" + *Port)

// Стало:
if err := r.Run(":" + *Port); err != nil {
    fmt.Printf("[FATAL] Server failed: %v\n", err)
    os.Exit(1)
}
```

**Причина:** `gin.Engine.Run()` возвращает `error`, но он игнорировался.
При занятом порте процесс выходил с кодом 0, systemd считал это успешным
завершением и крутил рестарт-цикл без явной ошибки в `journalctl`.

После фикса: при ошибке бинда в логе `[FATAL] Server failed: listen tcp :8080:
bind: address already in use`, exit code 1, systemd показывает
`status=1/FAILURE`.

---

## 6. Смена порта по умолчанию (`f655c88`)

### `main.go`

- `flag.String("port", "8080", ...)` → `flag.String("port", "8081", ...)`

### Причина

На тестовом сервере `crowdsec.service` слушает `127.0.0.1:8080`. Пока
intermasq работал — crowdsec не мог занять порт. При остановке intermasq
для замены бинарника crowdsec тут же занял `127.0.0.1:8080`, и новый
бинарник не смог забиндиться на `0.0.0.0:8080` (включает loopback).

### Документация

- `README.md` / `README.en.md`: обновлены примеры запуска (`-port 8081`),
  таблица флагов, добавлено примечание о причине смены порта и способе
  вернуть старый (`-port 8080`).
- Заодно добавлены ранее не задокументированные флаги `-audit-log` и
  `-templates`.

---

## Новые эндпоинты (итог)

| Метод | Путь | Описание | Auth |
|---|---|---|---|
| GET  | `/api/hosts/next-ip?range=CIDR` | Свободный IP в диапазоне | да |
| GET  | `/api/templates` | Список шаблонов | да |
| POST | `/api/templates` | Создать шаблон | да |
| DELETE | `/api/templates/:id` | Удалить шаблон | да |
| POST | `/api/hosts/apply-template` | Сгенерировать host по шаблону | да |
| POST | `/api/hosts/bulk-move` | Перенос хостов между .conf | да |
| POST | `/api/hosts/bulk-edit` | Массовая смена IP/hostname | да |

## Новые файлы

- `templates.go` — бэкенд-логика шаблонов.
- `frontend/src/components/static/TemplatesModal.vue` — CRUD шаблонов.
- `frontend/src/components/static/BulkMoveModal.vue` — модалка переноса.
- `frontend/src/components/static/BulkEditModal.vue` — модалка редактирования.

## Изменённые файлы

- `main.go` — флаги, роуты, фикс `r.Run`, порт 8081.
- `models.go` — новые контракты.
- `dnsmasq.go` — `findFreeIP`, `moveHostLine`/`appendHostLine`/`readHostByMac`,
  `ipTransform`.
- `handlers.go` — 7 новых обработчиков.
- `frontend/src/store.js` — `templates` в state, 6 новых actions.
- `frontend/src/components/static/HostForm.vue` — dropdown шаблонов, кнопка `🎲`.
- `frontend/src/components/static/HostTable.vue` — расширение панели выделения.
- `README.md`, `README.en.md` — порт, новые флаги, примечание.

## Хранение

- `/etc/intermasq/templates.json` — JSON с шаблонами (рядом с `users.json`).
  Создаётся автоматически при первом добавлении шаблона.
- `/etc/intermasq/audit.log` — дописываются записи `bulk_move`, `bulk_edit`.
- `.bak` файлы создаются для каждого затронутого `.conf` перед bulk-операцией.
