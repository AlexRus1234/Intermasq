# Сессия: predrel — DNS aliases (A / CNAME)

## Контекст

Реализована отдельная вкладка «DNS» для управления DNS-записями dnsmasq
двух типов:

- **A**    — `address=/domain/IP`   (имя → IPv4)
- **CNAME**— `cname=alias,target`   (алиас → другое имя)

Раньше Intermasq работал только с `dhcp-host=` и общими директивами через
вкладку «Настройки dnsmasq». Теперь `address=` и `cname=` вынесены в
отдельный CRUD, как и было запрошено в плане развития. HomeLab-кейс:
named-хосты вместо IP в браузере/`dig`.

Документация: `docs/dns-aliases.md`.

---

## Коммит

| Хэш     | Описание                                                              |
|---------|----------------------------------------------------------------------|
| 2d46842 | Add DNS aliases tab: address=/domain/IP and cname=alias,target CRUD |

---

## Принятые решения (входные параметры)

Выбрано в диалоге с пользователем до реализации:

- **Сосуществование с Config:** `address=` и `cname=` **исключены** из
  вкладки «Настройки dnsmasq» (как и `dhcp-host=`). Управляются только
  новой вкладкой «DNS». При сериализации файла через Config-вкладку
  alias-строки сохраняются как есть (не теряются).
- **Массовые операции:** CRUD + bulk import (Raw-текст **и** CSV).
  Bulk-delete чекбоксами **не** реализован (вне скоупа).
- **Формат `address=`:** только базовый `/domain/IP`. Wildcard-формы
  `address=/#/IP` и `address=/*.domain/IP` сознательно **отклоняются**
  парсером (вышли бы за пределы HomeLab-кейса и усложнили валидацию).
- **Файл по умолчанию:** при добавлении без явного выбора файла создаётся
  `/etc/dnsmasq.d/10-dns-aliases.conf` с header-комментарием.
- **Удаление:** через `POST /api/aliases/delete` с JSON-телом
  (`type`, `domain`, `file`). `DELETE /api/aliases/:domain` не подходит —
  gin плохо переваривает точки в path-параметрах.

---

## 1. Бэкенд — модели (`models.go`)

Новые типы:

- `DnsAliasEntry` — одна запись: `Type` (`"A"`|`"CNAME"`), `Domain`,
  `Target`, `File`.
- `BulkAliasReq` — тело `POST /api/aliases/bulk`: `Aliases[]` + `File`.
- `DeleteAliasReq` — тело `POST /api/aliases/delete`: `Type`, `Domain`,
  `File`.

---

## 2. Бэкенд — парсер/CRUD (`dnsmasq.go`)

### Чтение

- `isAliasDirective(line) bool` — `address=` или `cname=`.
- `parseAliasLine(line, file, hasBak) (DnsAliasEntry, bool)` — разбор
  одной строки:
  - `address=/nas.lan/192.168.1.10` → `Type:"A"`,
    `Domain:"nas.lan"`, `Target:"192.168.1.10"`.
  - `cname=wiki,nas.lan` → `Type:"CNAME"`, `Domain:"wiki"`,
    `Target:"nas.lan"`.
  - Tagged cname `cname=wiki,nas.lan,tag:lan` парсится, тег
    игнорируется (alias/target берутся).
  - Wildcard `address=/#/...` и `address=/*.x/...` → `ok=false`.
  - Malformed (нет закрывающего `/`, пустой target, cname без target) →
    `ok=false`.
- `readAllAliases() []DnsAliasEntry` — обходит все `.conf` в `-conf-dir`,
  собирает `address=`/`cname=`. Как и `readAllHosts`, добавляет суффикс
  `|has_bak` к `File` при наличии `.bak`.
- `cleanAliasFile(f) string` — убирает маркер `|has_bak`.

### Запись

- `aliasToLine(a DnsAliasEntry) string` — обратная сериализация:
  `address=/domain/target` или `cname=domain,target`.
- `appendAliasLine(filePath, entry) error` — дописывает строку в конец
  файла, сохраняя существующий контент.
- `removeAliasLine(filePath, type, domain) (bool, error)` — удаляет
  **первое** совпадение по `type+domain` (case-insensitive). Возвращает
  `false`, если строка не найдена (без ошибки).
- `findAliasesByDomain(domain, excludeType, excludeFile) []DnsAliasEntry`
  — проверка дублей по домену (case-insensitive), с исключением
  редактируемой записи.

### CSV

- `aliasesToCSV(aliases) []byte` — заголовок `type,domain,target` + rows.
- `parseCSVAliases(r, targetFile) ([]DnsAliasEntry, error)` —
  CSV-парсер с валидацией: `A` требует валидный IP в target, `CNAME` —
  валидный домен. Header-строка пропускается автоматически.

### Дефолтный файл

- `ensureAliasesFile(path) error` — создаёт
  `10-dns-aliases.conf` с header-комментарием, если файла нет. Вызывается
  из хендлеров через `resolveAliasesTargetFile`.

### Интеграция с Config-вкладкой

- `readConfigSnapshot` — добавлен skip `address=`/`cname=` (как и
  `dhcp-host=`). В Config-вкладке они не видны.
- `serializeConfigFile` — добавлен блок `aliasLines`: при пересохранении
  файла через Config alias-строки **сохраняются** в исходном виде, а не
  затираются. Порядок в файле: header comments → dhcp-host → aliases →
  managed directives.

---

## 3. Бэкенд — хендлеры (`handlers.go`)

6 новых хендлеров в конце файла:

| Метод  | Путь                  | Хендлер                    | Что делает                              |
|--------|-----------------------|----------------------------|-----------------------------------------|
| GET    | `/api/aliases`        | `getAliasesHandler`        | Список всех alias-записей               |
| POST   | `/api/aliases`        | `addAliasHandler`          | Добавление одной записи                 |
| POST   | `/api/aliases/bulk`   | `bulkAddAliasesHandler`    | Массовое добавление (JSON)              |
| POST   | `/api/aliases/delete` | `deleteAliasHandler`       | Удаление по `type+domain+file`          |
| GET    | `/api/aliases/csv`    | `exportAliasesCSVHandler`  | Экспорт в CSV                           |
| POST   | `/api/aliases/csv`    | `importAliasesCSVHandler`  | Импорт из CSV (FormData)                |

Вспомогательные функции:

- `resolveAliasesTargetFile(reqFile) (string, bool)` — если `reqFile`
  пустой, создаёт и возвращает дефолтный `10-dns-aliases.conf`. Проверяет
  `isSafePath`.
- `validateAliasEntry(a) bool` — `Type ∈ {A,CNAME}`, `Domain` матчит
  `aliasDomainRegex`, для A — target это IP, для CNAME — домен.

### Защитные меры (как у host-хендлеров)

- **Мьютекс** `mu.Lock()` на запись.
- **`createLocalBackup`** перед любым изменением → `.bak` файл.
- **Проверка дублей** — 409 с `conflicts` в ответе.
  - При bulk: сначала intra-batch (дубли внутри массива), потом
    cross-config (по всем `.conf`).
- **Audit-лог** — `alias_add`, `alias_delete`, `alias_bulk_add` с
  указанием пользователя, типа, домена, target и файла.
- **Path Traversal** — `isSafePath` на каждый путь.
- **Pre-flight `dnsmasq --test`** — **не** вызывается при add/delete
  alias (как и в host-хендлерах). Проверка происходит при ручном нажатии
  «Применить» (кнопка `/api/reload`). Это сознательное решение: alias —
  простая строка, валидируется regex'ом; вызов `--test` на каждое
  добавление замедлил бы UX.

---

## 4. Бэкенд — роутинг (`main.go`)

Добавлены 6 роутов в `auth`-группу (после `/api/config/file`):

```go
auth.GET("/aliases", getAliasesHandler)
auth.POST("/aliases", addAliasHandler)
auth.POST("/aliases/bulk", bulkAddAliasesHandler)
auth.POST("/aliases/delete", deleteAliasHandler)
auth.GET("/aliases/csv", exportAliasesCSVHandler)
auth.POST("/aliases/csv", importAliasesCSVHandler)
```

Новая константа:

```go
DefaultAliasesFileName = "10-dns-aliases.conf"
```

Новый regex:

```go
aliasDomainRegex = ^[a-zA-Z0-9]([a-zA-Z0-9-.]*[a-zA-Z0-9])?$
```

Принимает `nas`, `nas.lan`, `a-b.c.d` (но не `.nas`, `nas.`, `..`).

---

## 5. Бэкенд — тесты (`dnsmasq_test.go`)

Добавлено 11 новых кейсов:

| Тест                                  | Что проверяет                                         |
|---------------------------------------|-------------------------------------------------------|
| `TestParseAliasLineA`                 | `address=/nas.lan/192.168.1.10` → корректный entry    |
| `TestParseAliasLineCNAME`             | `cname=wiki,nas.lan` → корректный entry               |
| `TestParseAliasLineCNAMEWithTag`      | `cname=wiki,nas.lan,tag:lan` — тег игнорируется       |
| `TestParseAliasLineRejectsWildcard`   | `#` и `*.evil` отклоняются                            |
| `TestParseAliasLineRejectsMalformed`  | Нет `/`, пустой target, cname без target              |
| `TestAliasToLineRoundTrip`            | `parse(aliasToLine(x)) == x` для A и CNAME            |
| `TestReadAllAliases`                  | Чтение из `.conf` игнорирует `server=` и т.п.         |
| `TestReadAllAliasesHasBakMarker`      | Маркер `|has_bak` + `cleanAliasFile`                  |
| `TestRemoveAliasLine`                 | Удаляет только нужную запись, остальные сохраняются   |
| `TestRemoveAliasLineNotFound`         | `removed=false` для отсутствующего домена             |
| `TestSerializeConfigFilePreservesAliases` | Alias-строки не теряются при пересохранении через Config |
| `TestReadConfigSnapshotFiltersAliases`   | Alias-строки не видны во вкладке «Настройки dnsmasq»  |

Все 40+ тестов проходят: `go test ./...` → `ok`.

---

## 6. Фронтенд — store (`store.js`)

- Новое поле `aliases: []` в `store` (reactive).
- `loadData()` — добавлен 9-й `Promise.all`-запрос
  `api.get('/aliases')`.
- 6 новых actions (в конце объекта `actions`):
  - `loadAliases()`
  - `addAlias(alias)` — с `translateApiError` и `alert`.
  - `bulkAddAliases(aliases, file)` — возвращает `{status, count}`.
  - `deleteAlias(type, domain, file)`.
  - `downloadAliasesCSV()` — blob → download.
  - `importAliasesCSV(file, targetFile)` — FormData.

---

## 7. Фронтенд — компоненты (`components/dns/`)

### `DnsAliasesView.vue` (координатор)

Клон паттерна `StaticView.vue`:

- `AliasForm` сверху.
- Nav-tabs по файлам (как у Static) + «Все файлы».
- Кнопка «Откат» (rollback) при наличии `.bak`.
- `AliasTable` снизу.

### `AliasForm.vue`

Многофункциональная форма, 3 режима (через `<select>`):

1. **single** — тип (A/CNAME) + Domain + Target + File. Под формой —
   live-превью итоговой строки:
   - `address=/nas.lan/192.168.1.10`
   - `cname=wiki,nas.lan`
2. **text** — textarea с bulk-парсингом. Поддерживаемые форматы на
   строку:
   - `A nas.lan 192.168.1.10`
   - `CNAME wiki nas.lan`
   - `address=/nas.lan/192.168.1.10`
   - `cname=wiki,nas.lan`
   Счётчик «Распознано: N записей» обновляется live.
3. **csv** — file input + кнопка «Импортировать».

При редактировании (edit-режим): карточка получает жёлтую рамку, старая
запись удаляется перед добавлением новой (как `HostForm.vue`).

### `AliasTable.vue`

- Колонки: **Type** (badge: A=синий, CNAME=голубой), **Domain**, **Target**
  (A — синий, CNAME — голубой), **File** (только в «Все файлы»), **Actions**.
- Сортировка по клику на заголовок (`domain`, `target`). Для `target` —
  natural sort по октетам IP (как в `HostTable`).
- Поиск через глобальный `store.searchQuery` (по type/domain/target).
- Кнопки ✏️ (edit) и ✕ (delete с confirm).

---

## 8. Фронтенд — App.vue

- Импорт `DnsAliasesView`.
- В `btn-group` добавлена кнопка `🌐 DNS` между «Статика» и «Аренды».
- `<DnsAliasesView v-if="store.tab === 'dns'" />` в области рендера.
- Кнопка `📥 CSV` стала контекстно-зависимой: на вкладке DNS вызывает
  `actions.downloadAliasesCSV()`, иначе — `actions.downloadCSV()`. Это
  сделано через локальную функцию `downloadCSV()` в `<script setup>`.

---

## 9. Локализация (`locales/ru.json`, `locales/en.json`)

Добавлены ключи (RU / EN зеркально):

- `app.tabDns` — «DNS» / «DNS».
- `dns.*` — целый раздел: `editing`, `newAlias`, `addTo`, `importList`,
  `cancel`, `filePlaceholder`, `save`, `add`, `bulkPlaceholder`,
  `parsed`, `aliases`, `destFile`, `importBtn`, `csvMode`, `allFiles`,
  `rollback`, `rollbackTooltip`, `type`, `domain`, `target`, `fileCol`,
  `actions`, `editTooltip`, `deleteTooltip`, `searchEmpty`, `empty`,
  `domainPlaceholder`, `targetIpPlaceholder`, `targetDomainPlaceholder`.
- `confirm.deleteAlias` — «Удалить DNS-запись {domain}?».
- `alert.invalidData`, `alert.aliasAddError`, `alert.aliasDeleteError`.
- `api.alias_duplicate`, `api.alias_duplicate_bulk`,
  `api.no_valid_entries`, `api.alias_not_found`.
- `audit.action_alias_add`, `audit.action_alias_delete`,
  `audit.action_alias_bulk_add`.

---

## 10. Файлы изменены / добавлены

### Бэкенд

| Файл              | Статус    | Что                                              |
|-------------------|-----------|--------------------------------------------------|
| `models.go`       | изменён   | 3 новых типа                                     |
| `dnsmasq.go`      | изменён   | 11 новых функций + правки `readConfigSnapshot`/`serializeConfigFile` |
| `handlers.go`     | изменён   | 6 новых хендлеров + 2 вспомогательные функции    |
| `main.go`         | изменён   | 6 новых роутов + `aliasDomainRegex` + `DefaultAliasesFileName` |
| `dnsmasq_test.go` | изменён   | 11 новых тестов                                  |

### Фронтенд

| Файл                                       | Статус    | Что                          |
|--------------------------------------------|-----------|------------------------------|
| `src/components/dns/DnsAliasesView.vue`    | **новый** | Координатор вкладки          |
| `src/components/dns/AliasForm.vue`         | **новый** | Форма (single/bulk/csv)      |
| `src/components/dns/AliasTable.vue`        | **новый** | Таблица записей              |
| `src/App.vue`                              | изменён   | Кнопка таба + контекстный CSV |
| `src/store.js`                             | изменён   | Поле `aliases` + 6 actions   |
| `src/locales/ru.json`                      | изменён   | Секция `dns.*` + ключи       |
| `src/locales/en.json`                      | изменён   | Секция `dns.*` + ключи       |

### Документация

| Файл                       | Статус    | Что                            |
|----------------------------|-----------|--------------------------------|
| `docs/dns-aliases.md`      | **новый** | Пользовательская документация  |
| `логи/predrel-dns-aliases.md` | **новый** | Этот лог-файл               |

### Бинарник

| Файл              | Статус    | Что                                            |
|-------------------|-----------|------------------------------------------------|
| `intermasq-linux` | пересобран | Linux/amd64, embed обновлённого фронтенда     |

> `intermasq` и `intermasq.exe` (старые бинарники) **намеренно не
> трогались** — пользователь компилирует их сам.

---

## 11. Проверка сборки

```text
$ go vet ./...              OK
$ go test ./...             ok  intermask  2.123s
$ cd frontend && npm run build  ✓ built in 2.06s
$ GOOS=linux GOARCH=amd64 go build -o intermasq-linux .
→ 43 903 638 bytes, embed frontend/dist/
```

---

## 12. Известные ограничения

- **Bulk-delete чекбоксами не реализован** — по договорённости (только
  single + bulk import). При необходимости добавляется по образцу
  `HostTable.vue`.
- **Wildcard `address=/#/IP` и `address=/*.domain/IP`** сознательно
  отклоняются парсером. Для captive portal нужно расширять
  `parseAliasLine` и форму.
- **IPv6 `address=`** не поддерживается в UI: `validateAliasEntry`
  принимает только IPv4 для типа A (`net.ParseIP` пропустил бы и IPv6,
  но regex валидации domain это не учитывает). При желании — добавить
  отдельный тип `AAAA` или ослабить валидацию target.
- **Tagged cname** (`cname=alias,target,tag:lan`) парсится, но тег
  теряется при round-trip через `aliasToLine`. Если теги нужны —
  добавить поле `Tag` в `DnsAliasEntry`.
- **Pre-flight `dnsmasq --test`** не вызывается на каждом add/delete
  (только на «Применить»). Синтаксис alias прост, regex валидирует;
  вызов test на каждое действие замедлил бы UX.
- **Drag-and-drop reorder** alias-строк не реализован — порядок в файле
  определяется порядком добавления.

---

## 13. Пример использования

1. Открыть вкладку «DNS».
2. Выбрать тип **A**, домен `nas.lan`, target `192.168.1.10`, нажать
   «Добавить». Файл `10-dns-aliases.conf` создастся автоматически.
3. Проверить через `dig @192.168.1.1 nas.lan` → `A 192.168.1.10`.
4. Для алиаса: тип **CNAME**, домен `wiki`, target `nas.lan`.
5. Нажать «Применить» в шапке (триггерит `dnsmasq --test` + restart).
6. `dig @192.168.1.1 wiki` → `CNAME nas.lan` → `A 192.168.1.10`.
