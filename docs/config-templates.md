<!--
Intermasq - Web panel for dnsmasq
Copyright (C) 2026 AlexRus1234

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
-->

# Шаблоны при создании конфига (config file templates)

Начиная с v3.0 в Intermasq добавлена возможность выбрать предзаполненный
шаблон при создании нового `.conf`-файла. Раньше `POST /api/config/file`
создавал файл с единственной строкой-маркером; теперь админ может выбрать
скелет типовой конфигурации (DHCP, forwarder, PXE, aliases) и сразу получить
отправную точку для редактирования.

---

## Что было раньше

- `POST /api/config/file {name}` — создавал файл с заголовком
  `# === Managed by Intermasq ===\n`.
- Дальше нужно было либо знать синтаксис dnsmasq, либо копировать директивы
  из существующих конфигов, либо редактировать файл внешним редактором по SSH.

## Что появилось

- Поле `template` в теле `POST /api/config/file`.
- Набор встроенных шаблонов с консервативным содержимым: активны только
  безопасные boolean-директивы, всё со значениями закомментировано с примером.
- Каталог шаблонов через `GET /api/config/templates` (ID + preview) — UI
  показывает содержимое до создания файла.
- UI: в форме создания файла добавлены `<select>` выбора шаблона и `<pre>`
  с preview.

---

## Список шаблонов

| ID | Назначение | Активные директивы | Закомментированные примеры |
|----|------------|--------------------|----------------------------|
| `empty` | Пустой файл (по умолчанию, обратная совместимость) | — | — |
| `basic-dhcp` | Базовый DHCP/DNS сервер | `domain-needed`, `bogus-priv`, `expand-hosts`, `domain=lan` | `dhcp-range`, `dhcp-option=option:router`, `dhcp-option=option:dns-server` |
| `forwarder` | DNS forwarder без локальных зон | `domain-needed`, `bogus-priv`, `no-resolv`, `strict-order` | `server=1.1.1.1`, `server=8.8.8.8`, `address=/nas.lan/192.168.1.10` |
| `pxe` | Сетевая загрузка PXE (дополни `basic-dhcp`) | — | `dhcp-match=set:efi-x86_64,…`, `dhcp-boot=…`, `pxe-service=…` |
| `aliases` | DNS aliases (альтернатива `10-dns-aliases.conf`) | — | `address=/nas.lan/192.168.1.10`, `cname=wiki,nas.lan` |

Каждый шаблон:
- Начинается с маркера `# === Managed by Intermasq ===`.
- Синтаксически проходит `dnsmasq --test` (проверяется тестом
  `TestConfigTemplatesValidForDnsmasqSyntax`).
- Не делает предположений о топологии сети админа — все директивы со
  значениями закомментированы.

---

## API

### `POST /api/config/file`

Создаёт новый `.conf`-файл в `-conf-dir` с содержимым выбранного шаблона.

**Тело:**

```json
{
  "name": "dhcp.conf",
  "template": "basic-dhcp"
}
```

| Поле | Тип | Описание |
|------|-----|----------|
| `name` | string (обязательный) | Имя файла. Должно оканчиваться на `.conf`, не содержать `/` и `\`. |
| `template` | string (опциональный) | ID шаблона. Пустая строка или отсутствие поля → `"empty"` (обратная совместимость). Нормализация: lowercase + trim. |

**Ответы:**

| Код | Тело | Когда |
|-----|------|-------|
| 200 | `ConfigSnapshot` (как от `GET /api/config`) | Файл создан |
| 400 | `{"error":"invalid_data"}` | Невалидный JSON |
| 400 | `{"error":"invalid_filename"}` | Пустое имя, содержит `/`/`\`, не `.conf` |
| 400 | `{"error":"unknown_template","template":"…","available":[…]}` | Неизвестный ID шаблона + список доступных |
| 403 | `{"error":"access_denied"}` | Путь вне `-conf-dir` |
| 409 | `{"error":"file_exists"}` | Файл уже существует (перезапись не допускается) |
| 500 | `{"error":"write_error"}` | Ошибка записи на диск |

При успехе в audit-лог пишется запись:

```json
{
  "timestamp": "2026-07-18T…",
  "user": "admin",
  "action": "config_create_file",
  "file": "/etc/dnsmasq.d/dhcp.conf",
  "template": "basic-dhcp"
}
```

**Замечание:** `dnsmasq --test` при создании файла НЕ запускается — это
сознательно (создаётся скелет, который ещё не подключён к main config).
Синтаксис шаблонов валидируется отдельным тестом. Последующий `PUT /api/config`
запускает `--test` как обычно.

### `GET /api/config/templates`

Отдаёт каталог известных шаблонов. Используется UI для построения
dropdown-селектора и preview.

**Ответ:**

```json
{
  "templates": [
    {
      "id": "aliases",
      "preview": "# === Managed by Intermasq ===\n# DNS aliases: …\n\n#address=/nas.lan/192.168.1.10\n#cname=wiki,nas.lan\n"
    },
    {
      "id": "basic-dhcp",
      "preview": "# === Managed by Intermasq ===\n# Базовый DHCP/DNS сервер. …\n\ndomain-needed\n…"
    },
    …
  ]
}
```

Список отсортирован по ID (стабильный порядок).

---

## UI

В вкладке **«Настройки dnsmasq»** при клике на `+ Новый файл` открывается
форма:

```
┌──────────────────────────────────────────────────────────────┐
│ [filename.conf        ] [∅ Empty file ▾] [＋] [✕]            │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ # === Managed by Intermasq ===                           │ │
│ │ # Базовый DHCP/DNS сервер. Раскомментируй и поправь       │ │
│ │ # значения под свою сеть.                                 │ │
│ │                                                          │ │
│ │ domain-needed                                            │ │
│ │ bogus-priv                                               │ │
│ │ expand-hosts                                             │ │
│ │ domain=lan                                               │ │
│ │ #dhcp-range=192.168.1.50,192.168.1.150,255.255.255.0,12h │ │
│ │ …                                                        │ │
│ └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

- **Filename** — имя файла (auto-add `.conf` если забыли).
- **Template selector** — dropdown со списком доступных шаблонов. По
  умолчанию `∅ Empty file`. Каталог подгружается лениво при первом открытии
  формы через `GET /api/config/templates`; если бэкенд недоступен — fallback
  на `["empty"]`, форма остаётся рабочей.
- **Preview** — содержимое выбранного шаблона до создания файла.
- **＋** — создание файла. На бэкенд уходит `{name, template}`.
- **✕** — отмена.

После успешного создания:

- Файл автоматически выбирается в редакторе директив.
- Форма закрывается, `newFileTemplate` сбрасывается в `empty`.
- Админ видит активные boolean-директивы и может раскомментировать
  нужные через `PUT /api/config` (с привычным `dnsmasq --test`).

---

## Типовой flow

1. Вкладка **«Настройки dnsmasq»** → `+ Новый файл`.
2. Ввести имя `dhcp.lan.conf`.
3. Выбрать шаблон `basic-dhcp` — preview показывает скелет.
4. `＋` → файл создан, открывается в редакторе.
5. Раскомментировать `dhcp-range` и подставить свою подсеть.
6. Раскомментировать `dhcp-option=option:router` и подставить свой gateway.
7. **Сохранить конфигурацию** → `PUT /api/config` запускает `dnsmasq --test`.
   При ошибке (опечатка, невалидный IP) — откат и понятное сообщение.
8. При успехе — изменения применены.

---

## Добавление нового шаблона

Шаблоны захардкожены в `config_templates.go` в `configTemplates` map.
Добавление нового шаблона:

```go
var configTemplates = map[string]string{
    // … существующие …

    "my-tls": `# === Managed by Intermasq ===
# Custom TLS forwarder template.

no-resolv
#server=9.9.9.9
`,
}
```

Контракты, которые нужно соблюсти (проверяются тестами):

1. **ID — lowercase с дефисами** (нормализация в handler).
2. **Файл начинается с** `# === Managed by Intermasq ===`.
3. **Синтаксис проходит `dnsmasq --test`** — иначе `TestConfigTemplatesValidForDnsmasqSyntax`
   упадёт на CI с бинарником dnsmasq.

Тест `TestCreateConfigFileHandlerEachTemplate` table-driven — автоматически
подхватит новый ID. `TestListConfigTemplatesHandler` проверит, что каталог
отдаёт новый шаблон.

---

## Внутренняя реализация (кратко)

- `config_templates.go` — `configTemplates` map + `knownConfigTemplateIDs()`.
- `models.go` — `CreateConfigFileReq{ Name, Template }`.
- `audit.go` — `AuditEntry.Template` (omitempty).
- `handlers.go`:
  - `createConfigFileHandler` — нормализация ID (lowercase + trim), lookup в
    map, при unknown → 400 + available. Audit-запись с `Template` полем.
  - `listConfigTemplatesHandler` — отдаёт каталог.
- `main.go` — роут `auth.GET("/config/templates", listConfigTemplatesHandler)`.
- `frontend/src/store.js` — `createConfigFile(name, template)`,
  `loadConfigTemplates()` с fallback.
- `frontend/src/components/config/DnsmasqConfig.vue` — select + preview,
  ленивая загрузка каталога.
- `frontend/src/locales/{ru,en}.json` — ключи `config.template`,
  `config.templateEmpty`.
- `dnsmasq_test.go` — 11 новых тестов (table-driven по всем шаблонам +
  edge-cases).
