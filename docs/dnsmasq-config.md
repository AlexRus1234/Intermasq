# Настройка dnsmasq (полная конфигурация)

Начиная с v3.0 в Intermasq добавлена полноценная веб-панель для редактирования
**всех** директив dnsmasq — а не только `dhcp-host=`. Теперь Intermasq перестал
быть «просмотрщиком хостов» и стал полноценной панелью управления dnsmasq.

---

## Что было раньше

До этого изменения парсился только `dhcp-host=mac,hostname,ip`. Все остальные
директивы (`server`, `domain`, `dhcp-range`, `no-resolv`, `log-queries` и т.д.)
были невидимы для Intermasq — их можно было править только вручную через SSH и
текстовый редактор. IP-диапазоны для шаблонов и кнопки 🎲 нужно было вводить
руками в виде CIDR.

## Что появилось

- **Вкладка «Настройки dnsmasq»** в UI — визуальный редактор всех директив.
- **`GET /api/config`** — чтение всех директив (кроме `dhcp-host`) из всех
  `.conf`-файлов в `-conf-dir`, сгруппированных по файлам.
- **`PUT /api/config`** — обновление директив с автоматической проверкой
  `dnsmasq --test` перед сохранением и откатом при ошибке.
- **`GET /api/templates/ranges`** — список CIDR, вычисленных из всех
  `dhcp-range=` в конфигах. Используется в dropdown-селекторах.
- **`POST /api/config/file`** — создание нового `.conf`-файла через UI.
- **Автоопределение IP-диапазонов** — кнопка 🎲 и форма шаблонов теперь
  предлагают выбрать диапазон из списка существующих `dhcp-range`, а не
  вводить CIDR вручную.

---

## Как устроен парсер

### Чтение (`readConfigSnapshot`)

Функция обходит все `.conf`-файлы в `-conf-dir` и для каждой строки:

1. Пропускает пустые строки.
2. Пропускает `dhcp-host=` (он обрабатывается отдельно на вкладке «Статика»).
3. Распознаёт закомментированные директивы: `#no-resolv` →
   `Directive{Key:"no-resolv", Active:false}`.
4. Распознаёт активные директивы: `server=8.8.8.8` →
   `Directive{Key:"server", Value:"8.8.8.8", Active:true}`.
5. Булевы директивы (без значения): `no-resolv` →
   `Directive{Key:"no-resolv", Value:"", Active:true}`.
6. Обычные комментарии (не директивы, например `# Моя сеть`) игнорируются
   в UI, но сохраняются при записи (см. ниже).

Дополнительно для всех `dhcp-range=` строится структурированное представление
`DhcpRange` с полями `start`, `end`, `mask`, `lease_time`, `tag` и
вычисляемым `cidr`.

### Разбор `dhcp-range`

Поддерживаются все стандартные формы dnsmasq:

| Формат ввода                        | Пример                                  |
|-------------------------------------|-----------------------------------------|
| `start,end,netmask,lease`           | `192.168.1.50,192.168.1.150,255.255.255.0,12h` |
| `start,end,lease`                   | `192.168.1.50,192.168.1.150,12h`        |
| `prefix/len,lease` (CIDR)           | `192.168.0.0/24,1h`                     |
| `set:tag,start,end,netmask,lease`   | `set:corp,192.168.1.10,192.168.1.100,255.255.255.0,2h` |

Поле `cidr` вычисляется из `start` и `mask` (или берётся напрямую из CIDR-формы)
и используется в dropdown-селекторах шаблонов и кнопки 🎲.

### Запись (`serializeConfigFile`)

При сохранении файла происходит следующее:

1. Читается старый файл.
2. Из него извлекаются **все `dhcp-host=` строки** в исходном порядке —
   они будут сохранены как есть (вкладка «Статика» ими управляет отдельно).
3. Извлекаются **комментарии-заголовки** (строки `#...`, не являющиеся
   закомментированными директивами) — сохраняются в начале файла.
4. Новые директивы сортируются по группам для читаемости:
   - **DNS / Сеть** (`server`, `domain`, `no-resolv`, `listen-address`, ...)
   - **DHCP** (`dhcp-range`, `dhcp-option`, ...)
   - **Логирование** (`log-queries`, `log-facility`, ...)
   - **Прочее** (всё остальное, включая пользовательские директивы)
5. Внутри группы директивы сортируются по ключу.
6. Для неактивных директив (`Active:false`) строка получает префикс `#`.
7. Файл склеивается: header → dhcp-host блок → секция
   `# === Managed by Intermasq ===` с новыми директивами.

### Проверка перед сохранением (`writeConfigWithTest`)

1. Создаётся `.bak`-копия (через существующий `createLocalBackup`).
2. Файл перезаписывается новым содержимым.
3. Запускается `dnsmasq --test`.
4. Если тест **провалился** — файл автоматически восстанавливается из `.bak`,
   возвращается ошибка `dnsmasq_test_failed` с деталями stderr.
5. Если тест **прошёл** — сохранение завершено, но **перезапуск dnsmasq не
   происходит автоматически**. Пользователь должен нажать кнопку
   «Применить» (как и раньше).

---

## API

### `GET /api/config`

**Ответ:** `ConfigSnapshot`

```json
{
  "files": [
    {
      "path": "/etc/dnsmasq.d/00-test.conf",
      "name": "00-test.conf",
      "has_bak": true,
      "directives": [
        {"key": "server", "value": "8.8.8.8", "active": true, "file": "...", "line_no": 2},
        {"key": "domain", "value": "lan", "active": true, "file": "...", "line_no": 3},
        {"key": "no-resolv", "value": "", "active": true, "file": "...", "line_no": 5},
        {"key": "domain-needed", "value": "", "active": false, "file": "...", "line_no": 4}
      ]
    }
  ],
  "dhcp_ranges": [
    {
      "raw": "192.168.1.50,192.168.1.150,255.255.255.0,12h",
      "start": "192.168.1.50",
      "end": "192.168.1.150",
      "mask": "255.255.255.0",
      "lease_time": "12h",
      "tag": "",
      "cidr": "192.168.1.0/24",
      "file": "/etc/dnsmasq.d/00-test.conf",
      "line_no": 6
    }
  ]
}
```

### `PUT /api/config`

**Тело:** `ConfigUpdateReq`

```json
{
  "file": "/etc/dnsmasq.d/00-test.conf",
  "directives": [
    {"key": "server", "value": "1.1.1.1", "active": true},
    {"key": "domain", "value": "home.local", "active": true},
    {"key": "no-resolv", "value": "", "active": true},
    {"key": "domain-needed", "value": "", "active": false}
  ]
}
```

**Ответ:** обновлённый `ConfigSnapshot` (как у `GET`).

**Ошибки:**
- `400 invalid_directive_key` — ключ не соответствует `^[a-z][a-z0-9-]*$`.
- `400 invalid_directive_value` — значение содержит перевод строки.
- `400 dnsmasq_test_failed` + `detail` — `dnsmasq --test` упал, файл откачён.
- `403 access_denied` — путь вне `-conf-dir` (`isSafePath`).
- `500 write_error` — ошибка записи на диск.

### `GET /api/templates/ranges`

**Ответ:**

```json
{"ranges": ["192.168.1.0/24", "10.0.0.0/24"]}
```

CIDR дедуплицируются (два `dhcp-range` в одной /24 дают одну запись).

### `POST /api/config/file`

**Тело:**

```json
{"name": "guests.conf"}
```

**Ответ:** обновлённый `ConfigSnapshot` (с новым файлом).

**Ошибки:**
- `400 invalid_filename` — нет `.conf`, содержит `/` или `\`.
- `409 file_exists` — файл уже существует.

---

## UI: вкладка «Настройки dnsmasq»

### Структура

1. **Табы файлов** (как на вкладке «Статика») — по одному на каждый `.conf`.
   Если у файла есть `.bak` — рядом с именем значок ⏪.
2. Кнопка **+ Новый файл** — создаёт новый `.conf` (запрашивает имя).
3. Кнопки справа:
   - **⏪ Откатить файл** — восстанавливает из `.bak` (через `POST /api/rollback`).
   - **💾 Сохранить конфигурацию** — собирает все директивы файла и отправляет
     `PUT /api/config` с подтверждением.

### Карточки директив

Директивы группируются по категориям (DNS/DHCP/Log/Прочее) в отдельные
карточки. Тип элемента управления зависит от директивы (реестр в
`frontend/src/components/config/directives.js`):

| Тип        | UI                              | Примеры                          |
|------------|---------------------------------|----------------------------------|
| `bool`     | Чекбокс-свитч + имя             | `no-resolv`, `domain-needed`     |
| `string`   | Имя + текстовый инпут + свитч   | `domain`, `resolv-file`          |
| `list`     | Имя + инпут + свитч (многократно)| `server`, `listen-address`       |
| `dhcprange`| Спец. блок: tag/start/end/mask/lease | `dhcp-range`                |

У каждой директивы есть:
- **Свитч «Вкл/Выкл»** — добавляет/убирает `#` перед строкой.
- **🗑** — удалить директиву из файла (строка не сохранится).

### Реестр директив (`directives.js`)

Файл `frontend/src/components/config/directives.js` содержит `DIRECTIVE_SCHEMA` —
словарь «ключ → {type, group}». Директивы вне реестра попадают в группу
«Прочее» и редактируются как `string`. Чтобы добавить новую директиву в реестр:

```js
export const DIRECTIVE_SCHEMA = {
  // ...
  'cache-size':       { type: 'string', group: 'dns' },
  'dns-forward-max': { type: 'string', group: 'dns' },
}
```

### Добавление директивы

1. В нужной группе нажать **+ Добавить директиву**.
2. Появится строка с dropdown известных ключей из этой группы + текстовое
   поле для своего ключа.
3. Выбрать (или вписать) ключ → нажать **＋**.
4. Директива добавится в локальный список (с типом из schema или `string`).
5. Сохранить.

### Добавление `dhcp-range`

1. В блоке **DHCP** нажать **+ Добавить диапазон**.
2. Появится пустая строка с полями: tag / start / end / mask / lease.
3. Заполнить, сохранить.

---

## Интеграция с шаблонами и кнопкой 🎲

### Кнопка 🎲 (авто-IP в форме хоста)

Раньше: при первом нажатии появлялся текстовый инпут для CIDR.

Теперь:
1. При нажатии 🎲 загружается список CIDR из `GET /api/templates/ranges`.
2. Показывается **dropdown** с диапазонами. Если выбран — IP подбирается
   из него.
3. Есть опция «manual CIDR» — fallback на ручной ввод текстом.
4. Если диапазонов в конфиге нет — сразу показывается текстовый инпут
   (как раньше).
5. Приоритет: dropdown → шаблон (`tpl.ip_range`) → первый диапазон.

### Форма создания шаблона

В `TemplatesModal` поле `IP Range` теперь:
- Если есть `dhcp-range` в конфиге — **dropdown** с CIDR + опция «manual CIDR».
- Если нет — обычный текстовый инпут (как раньше).

---

## Безопасность

- **Path Traversal:** `isSafePath` проверяет, что `file` находится внутри
  `-conf-dir`. Работает и для `PUT /api/config`, и для `POST /api/config/file`.
- **Mutex:** `updateConfigHandler` берёт `mu.Lock()`, чтобы параллельный
  `addHostHandler` не затёр директивы.
- **Валидация ключей:** regex `^[a-z][a-z0-9-]*$` — заглавные и спецсимволы
  запрещены.
- **Защита от инъекций:** значение директивы не может содержать `\n`
  (иначе можно было бы внедрить лишние строки).
- **Имя файла:** должно заканчиваться на `.conf`, не содержать `/` и `\`.
- **Backup:** `.bak` создаётся перед каждой записью, откат доступен через
  `POST /api/rollback`.

---

## Audit

При сохранении конфигурации пишется запись в audit log:

```json
{
  "action": "config_update",
  "file": "/etc/dnsmasq.d/00-test.conf",
  "mac": "5 directives",
  "user": "admin"
}
```

При создании файла:

```json
{
  "action": "config_create_file",
  "file": "/etc/dnsmasq.d/guests.conf",
  "user": "admin"
}
```

На вкладке «История» эти действия отображаются с локализованными метками
(`audit.action_config_update`, `audit.action_config_create_file`) и синим
бейджем.

---

## Файлы изменены / добавлены

### Бэкенд (Go)

| Файл             | Что изменилось                                                |
|------------------|---------------------------------------------------------------|
| `models.go`      | Новые типы: `Directive`, `ConfigFile`, `ConfigSnapshot`, `DhcpRange`, `ConfigUpdateReq`, `CreateConfigFileReq` |
| `dnsmasq.go`     | `readConfigSnapshot`, `parseDhcpRange`, `dhcpRangeToCIDR`, `detectDhcpRangesCIDR`, `serializeConfigFile`, `directiveGroup`, `writeConfigWithTest`, `splitDirective`, `isLeaseTime` |
| `handlers.go`    | `getConfigHandler`, `updateConfigHandler`, `getDhcpRangesHandler`, `createConfigFileHandler` |
| `main.go`        | Регистрация роутов `/api/config`, `/api/templates/ranges`, `/api/config/file` |
| `dnsmasq_test.go`| 11 unit-тестов парсера и сериализатора                         |

### Фронтенд (Vue 3)

| Файл                                    | Что изменилось                                          |
|-----------------------------------------|---------------------------------------------------------|
| `src/components/config/directives.js`   | **Новый.** Реестр `DIRECTIVE_SCHEMA`, группы, `schemaFor()` |
| `src/components/config/DnsmasqConfig.vue`| **Новый.** Главный компонент вкладки настроек          |
| `src/components/config/DhcpRangeRow.vue`| **Новый.** Редактор одной строки `dhcp-range`           |
| `src/App.vue`                           | Кнопка таба, скрытие поиска, импорт `DnsmasqConfig`     |
| `src/store.js`                          | `configSnapshot`, `dhcpRanges`, 4 новых action          |
| `src/components/static/HostForm.vue`    | Dropdown из `dhcpRanges` для кнопки 🎲                  |
| `src/components/static/TemplatesModal.vue`| Dropdown из `dhcpRanges` для `ip_range`               |
| `src/components/audit/AuditTab.vue`     | Локализация `action_*`, цвет для `config_*`             |
| `src/locales/ru.json`                   | Секции `config.*`, новые `api.*`, `audit.action_*`      |
| `src/locales/en.json`                   | То же на английском                                     |

---

## Известные ограничения

- `dhcp-host=` **не редактируется** через вкладку «Настройки dnsmasq» —
  только через «Статику». При сохранении конфига `dhcp-host=`-строки
  сохраняются как есть.
- **Удаление .conf-файла** через UI не реализовано (только создание).
  Удаляйте вручную через SSH.
- **Diff перед сохранением** не показывается — только confirm-диалог.
- **Перетаскивание директив** (drag-and-drop reorder) не реализовано —
  порядок определяется группировкой и сортировкой.
- **IPv6 `dhcp-range`** парсится, но `cidr` для IPv6 не вычисляется
  (только IPv4). `findFreeIP` для IPv6 не работает.
