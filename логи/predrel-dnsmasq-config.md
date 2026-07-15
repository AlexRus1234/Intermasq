# Сессия: predrel — полная конфигурация dnsmasq через GUI

## Контекст

Реализован пункт 4 плана развития: полноценная веб-панель для редактирования
**всех** директив dnsmasq (раньше парсился только `dhcp-host=`). Intermasq
перестал быть «просмотрщиком хостов» и стал полноценной панелью управления
dnsmasq. Побочный эффект: IP-диапазоны для шаблонов и кнопки 🎲 теперь
автоопределяются из `dhcp-range`.

Документация: `docs/dnsmasq-config.md`.

---

## Коммит

| Хэш | Описание |
|---|---|
| (этот) | Add full dnsmasq config editor + dhcp-range auto-detect |

---

## Принятые решения (входные параметры)

- **Хранение директив:** in-place в существующих `.conf` (PUT переписывает
  тот файл, где директива найдена; новые пишутся в выбранный UI файл).
- **Комментарии:** `#dir` = «выкл», `dir` = «вкл». PUT либо раскомментирует,
  либо добавляет `#`-префикс.
- **Списковые директивы:** блок add/remove. `dhcp-range` получает
  специализированный парсер с полями start/end/mask/lease/tag.
- **ip_range для шаблонов/🎲:** dropdown со всеми `dhcp-range` (в виде CIDR,
  вычисляемого из start/end/mask).
- **Создание новых .conf:** добавлено сразу (через `POST /api/config/file`).
- **Сортировка при сохранении:** по группам (DNS < DHCP < Log < Other),
  внутри — алфавит.

---

## 1. Бэкенд — модели (`models.go`)

Новые типы:

- `Directive` — одна директива: `Key`, `Value`, `Active`, `File`, `LineNo`.
- `ConfigFile` — файл с директивами: `Path`, `Name`, `Directives`, `HasBak`.
- `ConfigSnapshot` — ответ `GET /api/config`: `Files[]` + `DhcpRanges[]`.
- `DhcpRange` — структурированный `dhcp-range=`: `Raw`, `Start`, `End`,
  `Mask`, `LeaseTime`, `Tag`, `CIDR` (вычисляемое), `File`, `LineNo`.
- `ConfigUpdateReq` — тело `PUT /api/config`: `File` + `Directives[]`.
- `CreateConfigFileReq` — тело `POST /api/config/file`: `Name`.

---

## 2. Бэкенд — парсер/сериализатор (`dnsmasq.go`)

### Чтение

- `readConfigSnapshot() ConfigSnapshot` — обходит все `.conf` в `-conf-dir`,
  для каждой строки: пропускает `dhcp-host=` и пустые; определяет
  `key=value` или булеву; помечает `Active`. Группирует по файлам.
  Дополнительно извлекает все `dhcp-range` в структурированном виде.
- `splitDirective(line) (key, value, ok)` — разбирает `key=value`.
  Важно: `-` НЕ разделитель (иначе ломает `domain-needed` и т.п.).
- `parseDhcpRange(raw, file, lineNo) DhcpRange` — поддерживает формы:
  - `start,end,netmask,lease`
  - `start,end,lease` (netmask inferred)
  - `prefix/len,lease` (CIDR)
  - `set:tag,start,end,netmask,lease` (tagged)
- `dhcpRangeToCIDR(r DhcpRange) string` — вычисляет CIDR из start+mask.
  Только IPv4 (IPv6 возвращает `""`).
- `detectDhcpRangesCIDR() []string` — быстрая выборка всех CIDR (с дедупом)
  для `/api/templates/ranges`.
- `isLeaseTime(s string) bool` — проверяет, похоже ли строка на lease time
  (`12h`, `30m`, `1d`, `infinite`, `3600s`). Используется чтобы отличить
  netmask от lease time в третьем аргументе `dhcp-range`.
- `directiveKeyRegex = ^[a-z][a-z0-9-]*$` — валидация имени директивы.

### Запись

- `serializeConfigFile(path, directives) ([]byte, error)` — пересобирает
  файл:
  1. Читает старый файл.
  2. Извлекает все `dhcp-host=` строки (сохраняются как есть).
  3. Извлекает header-комментарии (`#...`, не являющиеся закомментированными
     директивами) — сохраняются в начале.
  4. Новые директивы сортируются через `directiveGroup()`:
     - 0=DNS (`server`, `domain`, `no-resolv`, `listen-address`, ...)
     - 1=DHCP (`dhcp-range`, `dhcp-option`, ...)
     - 2=Log (`log-queries`, `log-facility`, ...)
     - 3=Other (всё остальное)
  5. Внутри группы — алфавит по ключу.
  6. Для `Active=false` — префикс `#`.
  7. Склеивает: header → dhcp-host блок → `# === Managed by Intermasq ===`
     + новые директивы.
- `writeConfigWithTest(path, content) error` — атомарная запись с проверкой:
  1. `createLocalBackup(path)` (уже существующая функция).
  2. `os.WriteFile(path, content)`.
  3. `exec.Command("dnsmasq", "--test")`.
  4. Если ошибка → `rollbackFile(path)` (восстанавливает из `.bak`) +
    возвращается `dnsmasq_test_failed: <stderr>`.
  5. Если OK — оставляем. Перезагрузка dnsmasq — отдельной кнопкой (как раньше).

### Регрессии исправлены

- `handlers.go:440,447` — `fmt.Printf` с `%s` для `len()` → исправлено на
  `%d`. Это был старый баг (виден через `go vet`), мешавший сборке тестов.

---

## 3. Бэкенд — тесты (`dnsmasq_test.go`)

11 unit-тестов парсера и сериализатора:

| Тест | Что проверяет |
|---|---|
| `TestParseDhcpRangeClassic` | `start,end,mask,lease` + вычисление CIDR |
| `TestParseDhcpRangeCIDRForm` | `prefix/len,lease` |
| `TestParseDhcpRangeTagged` | `set:tag,...` |
| `TestParseDhcpRangeNoMask` | `start,end,lease` (без mask → CIDR пустой) |
| `TestDhcpRangeToCIDRIPv6Rejected` | IPv6 → пустой CIDR |
| `TestSerializeConfigFilePreservesDhcpHosts` | `dhcp-host=` не теряется, header-комментарий сохраняется, старая директива заменяется |
| `TestSerializeConfigFileInactiveDirective` | `Active:false` → `#`-префикс |
| `TestReadConfigSnapshotFiltersDhcpHost` | `dhcp-host=` исключён, активные/неактивные директивы распознаны |
| `TestReadConfigSnapshotDhcpRanges` | Структурированный `dhcp_ranges` с CIDR и tag |
| `TestDetectDhcpRangesCIDRDedup` | Дедуп CIDR (два range в одной /24 → одна запись) |
| `TestSerializeConfigFileGroupOrder` | Порядок групп: dns < dhcp < log |

Все тесты проходят: `go test ./... → ok intermask 2.196s`.

---

## 4. Бэкенд — хендлеры (`handlers.go`)

### `getConfigHandler` — `GET /api/config`

Возвращает `ConfigSnapshot` (через `readConfigSnapshot()`).

### `updateConfigHandler` — `PUT /api/config`

1. `BindJSON` → `ConfigUpdateReq`.
2. `isSafePath(req.File)` — path traversal защита.
3. Валидация ключей: `^[a-z][a-z0-9-]*$`.
4. Валидация значений: не содержат `\n`.
5. `mu.Lock()` (параллельная запись с `addHostHandler` безопасна).
6. `serializeConfigFile(req.File, req.Directives)`.
7. `writeConfigWithTest(req.File, content)` — пишет + `dnsmasq --test` +
   авто-rollback при провале.
8. `writeAudit({Action: "config_update", ...})`.
9. Возвращает обновлённый `ConfigSnapshot`.

Ошибки: `invalid_directive_key`, `invalid_directive_value`,
`dnsmasq_test_failed` (с `detail`), `access_denied`, `serialize_error`,
`write_error`.

### `getDhcpRangesHandler` — `GET /api/templates/ranges`

Возвращает `{"ranges": ["192.168.1.0/24", ...]}` через
`detectDhcpRangesCIDR()`. Дедуплицировано.

### `createConfigFileHandler` — `POST /api/config/file`

1. `BindJSON` → `CreateConfigFileReq`.
2. Валидация имени: `.conf` расширение, без `/` и `\`.
3. `isSafePath(fullPath)`.
4. `mu.Lock()`.
5. Проверка что файл не существует (`409 file_exists` если занят).
6. `os.WriteFile(fullPath, "# === Managed by Intermasq ===\n")`.
7. `writeAudit({Action: "config_create_file", ...})`.
8. Возвращает обновлённый `ConfigSnapshot`.

---

## 5. Бэкенд — роутинг (`main.go`)

В `auth`-группу добавлены:

```go
auth.GET("/templates/ranges", getDhcpRangesHandler)
auth.GET("/config", getConfigHandler)
auth.PUT("/config", updateConfigHandler)
auth.POST("/config/file", createConfigFileHandler)
```

---

## 6. Фронтенд — store (`store.js`)

В `store` добавлены:

- `configSnapshot: null` — результат `GET /api/config`.
- `dhcpRanges: []` — список CIDR из `GET /api/templates/ranges`.

В `loadData()` добавлены два запроса в `Promise.all`:

```js
api.get('/config').catch(() => ({ data: null })),
api.get('/templates/ranges').catch(() => ({ data: { ranges: [] } }))
```

Новые actions:

- `loadConfig()` — `GET /api/config` + обновление `dhcpRanges`.
- `saveConfig(file, directives)` — `PUT /api/config`, при ошибке показывает
  alert с `translateApiError` + `detail` (stderr от `dnsmasq --test`).
- `createConfigFile(name)` — `POST /api/config/file`.
- `loadDhcpRanges()` — `GET /api/templates/ranges` (для ленивой загрузки
  при первом нажатии 🎲).

---

## 7. Фронтенд — реестр директив (`components/config/directives.js`)

**Новый файл.** Экспортирует:

- `DIRECTIVE_SCHEMA` — словарь `ключ → {type, group}`. Типы: `bool`,
  `string`, `list`, `dhcprange`. Группы: `dns`, `dhcp`, `log`, `other`.
- `GROUP_ORDER = ['dns', 'dhcp', 'log', 'other']` — порядок групп в UI.
- `GROUP_LABELS` — i18n-ключи для заголовков групп.
- `schemaFor(key)` — возвращает schema или `{type:'string', group:'other'}`
  для неизвестных.
- `defaultDirective(key)` — создаёт директиву с дефолтным значением.

Чтобы добавить новую директиву в реестр — дописать в `DIRECTIVE_SCHEMA`:

```js
'cache-size': { type: 'string', group: 'dns' },
```

---

## 8. Фронтенд — UI (`components/config/`)

### `DnsmasqConfig.vue` (новый, главный компонент вкладки)

Структура:
- **Табы файлов** — по одному на каждый `.conf`. Значок ⏪ если есть `.bak`.
- **+ Новый файл** — зелёная ссылка, открывает инпут для имени.
- **Кнопки:** ⏪ Откатить файл, 💾 Сохранить конфигурацию.
- **Карточки групп** — DNS/DHCP/Log/Other, внутри — директивы.
- **Строка добавления директивы** — dropdown известных ключей + инпут для
  своего.

У каждой директивы:
- Контрол по типу (чекбокс / инпут / список / спец. блок dhcp-range).
- Свитч Вкл/Выкл (добавляет/убирает `#`).
- 🗑 — удалить.

Состояние:
- `localDirectives` — локальная копия с `_uid` для key в v-for.
- `groupedDirectives` — computed, группирует по `schemaFor(d.key).group`.
- `selectFile(path)` — переключение таба, обновляет `localDirectives` из
  `currentFile.directives`.

### `DhcpRangeRow.vue` (новый, подкомпонент)

Отдельный компонент для одной строки `dhcp-range`. Поля: tag / start / end /
mask / lease. При вводе (`@input`) собирает строку обратно в
`directive.value` через `emit()`. `watch` на `directive.value` — обновляет
поля при внешнем изменении (например, после `loadConfig()`).

Важно: вынесен в отдельный компонент, потому что `reactive(parseRange(value))`
в шаблоне ломал `v-model` (создавал новый объект на каждом рендере).

---

## 9. Фронтенд — интеграция в `App.vue`

- Добавлена кнопка таба **⚙️ Настройки dnsmasq** между «Аренды» и «История».
- При `store.tab === 'config'` глобальный поиск скрывается (через `v-if`).
- Импортирован `DnsmasqConfig`, рендерится через `v-if`.

---

## 10. Фронтенд — интеграция dhcp-range в 🎲 и шаблоны

### `HostForm.vue` (кнопка 🎲)

Раньше: при первом 🎲 появлялся текстовый инпут для CIDR.

Теперь:
1. При 🎲 вызывается `actions.loadDhcpRanges()` (лениво, если ещё пусто).
2. Показывается **dropdown** со всеми CIDR из конфига + опция «manual CIDR».
3. Если выбран пункт из списка — `ipRange` = выбранный CIDR.
4. Если «manual CIDR» или списков нет — показывается текстовый инпут
   (`manualRange`).
5. `autoIP()` берёт `ipRange || manualRange`, если пусто — пробует шаблон,
  затем первый диапазон из списка.

### `TemplatesModal.vue` (форма шаблона)

Поле `IP Range`:
- Если `store.dhcpRanges.length > 0` — **dropdown** с CIDR + опция «manual CIDR».
- Если выбран пункт не из списка (или manual) — дополнительный текстовый
  инпут для ручного ввода.
- Если `dhcpRanges` пусто — обычный текстовый инпут (как раньше).

---

## 11. Фронтенд — i18n

### `locales/ru.json` + `en.json`

Добавлены секции:

- `app.tabConfig` — заголовок таба.
- `config.*` — все строки вкладки настроек (группы, кнопки, плейсхолдеры
  dhcp-range, алерты).
- `api.dnsmasq_test_failed`, `api.invalid_directive_key`,
  `api.invalid_directive_value`, `api.invalid_filename`, `api.file_exists`,
  `api.serialize_error` — новые коды ошибок.
- `alert.configSaveSuccess`, `alert.configSaveError`,
  `alert.configCreateError` — алерты.
- `confirm.deleteTemplate` — для TemplatesModal (раньше отсутствовал).
- `audit.action_*` — локализация действий в audit log (`config_update`,
  `config_create_file`, и заодно существующие `add`, `delete`, и т.д.).

### `AuditTab.vue`

- `actionLabel(action)` — возвращает `t('audit.action_' + action)` если
  есть, иначе сырой `action`.
- `actionClass` — добавлен `bg-primary` для `config_update` и
  `config_create_file`.

---

## 12. Безопасность

- **Path Traversal:** `isSafePath` проверяет все пути в `updateConfigHandler`
  и `createConfigFileHandler`.
- **Mutex:** `updateConfigHandler` и `createConfigFileHandler` берут `mu.Lock()`
  — параллельный `addHostHandler` не затрёт директивы.
- **Валидация ключей:** regex `^[a-z][a-z0-9-]*$` — заглавные и спецсимволы
  запрещены.
- **Валидация значений:** `\n` запрещён (иначе можно внедрить лишние строки).
- **Имя файла:** должно заканчиваться на `.conf`, не содержать `/` и `\`.
- **Backup:** `.bak` создаётся перед каждой записью, откат через
  `POST /api/rollback` (уже существующий).
- **dnsmasq --test:** при провале — авто-rollback, файл остаётся прежним.

---

## 13. Сборка

- **Тесты:** `go test ./... → ok intermask 2.196s` (все 11 новых + старые).
- **Frontend:** `npm run build` → `dist/` собран, 103 модуля, ~314KB JS /
  ~232KB CSS (gzip: 104KB / 31KB).
- **Linux-бинарник:** `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build
  -ldflags="-s -w" -o intermasq .` → `intermasq` (33MB, фронтенд встроен
  через `embed.FS`).

---

## 14. Файлы изменены / добавлены

### Бэкенд

| Файл             | Статус      | Что                                              |
|------------------|-------------|--------------------------------------------------|
| `models.go`      | изменён     | 6 новых типов                                    |
| `dnsmasq.go`     | изменён     | 9 новых функций + импорты `sort`, `regexp`       |
| `handlers.go`    | изменён     | 4 новых хендлера + импорт `regexp` + фикс `%s`→`%d` |
| `main.go`        | изменён     | 4 новых роута                                    |
| `dnsmasq_test.go`| изменён     | 11 новых тестов + импорт `os`, `path/filepath`   |

### Фронтенд

| Файл                                      | Статус      | Что                           |
|-------------------------------------------|-------------|-------------------------------|
| `src/components/config/directives.js`     | **новый**   | Реестр schema                 |
| `src/components/config/DnsmasqConfig.vue` | **новый**   | Главный компонент вкладки     |
| `src/components/config/DhcpRangeRow.vue`  | **новый**   | Подкомпонент строки dhcp-range|
| `src/App.vue`                             | изменён     | Кнопка таба, скрытие поиска   |
| `src/store.js`                            | изменён     | 2 поля + 4 action             |
| `src/components/static/HostForm.vue`      | изменён     | Dropdown для 🎲               |
| `src/components/static/TemplatesModal.vue`| изменён     | Dropdown для ip_range         |
| `src/components/audit/AuditTab.vue`       | изменён     | Локализация action_*          |
| `src/locales/ru.json`                     | изменён     | Новые секции                  |
| `src/locales/en.json`                     | изменён     | Новые секции                  |

### Документация

| Файл                       | Статус    | Что                             |
|----------------------------|-----------|---------------------------------|
| `docs/dnsmasq-config.md`   | **новый** | Пользовательская документация   |
| `логи/predrel-dnsmasq-config.md` | **новый** | Этот лог-файл              |

---

## 15. Известные ограничения

- `dhcp-host=` не редактируется через вкладку «Настройки dnsmasq» — только
  через «Статику». При сохранении конфига `dhcp-host=`-строки сохраняются
  как есть.
- Удаление .conf-файла через UI не реализовано (только создание).
- Diff перед сохранением не показывается — только confirm-диалог.
- Перетаскивание директив (drag-and-drop reorder) не реализовано — порядок
  определяется группировкой и алфавитной сортировкой.
- IPv6 `dhcp-range` парсится, но `cidr` для IPv6 не вычисляется (только
  IPv4). `findFreeIP` для IPv6 не работает.
