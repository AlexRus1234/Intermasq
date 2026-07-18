# Сессия: predrel — Шаблоны при создании конфига (config/file templates)

## Контекст

Раньше `POST /api/config/file` создавал новый `.conf`-файл с единственной
строкой `# === Managed by Intermasq ===\n`. Дальше админ должен был либо знать
синтаксис dnsmasq наизусть, либо копировать директивы из существующих конфигов,
либо редактировать файл внешним редактором по SSH. Механика создания файла была,
но начального содержимого не было — «с нуля в панели создать типовой конфиг»
было нельзя.

Добавлены именованные шаблоны: `empty`, `basic-dhcp`, `forwarder`, `pxe`,
`aliases`. При создании файла админ выбирает шаблон в dropdown-селекте и видит
его preview до подтверждения. Шаблон кладёт в файл безопасный скелет: активны
только boolean-директивы (`domain-needed`, `bogus-priv`, …), директивы со
значениями закомментированы с примером — админ раскомментирует и подставит свои
значения, после чего сработает привычный `dnsmasq --test` через `PUT /api/config`.

Старые файлы документации (`docs/*.md`) не тронуты — для новой фичи создан
отдельный файл `docs/config-templates.md`. Этот файл — журнал сессии.

---

## Коммит

| Хэш | Описание |
|-----|----------|
| (в этом PR) | Add config file templates (basic-dhcp, forwarder, pxe, aliases) |

---

## Принятые решения

Выбрано в диалоге с пользователем до реализации:

- **Не визуальные формы для каждой директивы.** Это обесценило бы существующие
  редакторы `dhcp-option` и `ForwardingRow` и создало бы источник регрессий
  (dnsmasq развивается, поддерживать 100+ форм нереально). Limits: только
  template selector + уже существующие визуальные редакторы.
- **Шаблоны консервативны.** Активны только безопасные boolean-директивы, всё
  со значениями закомментировано. Так файл проходит `dnsmasq --test` сам по
  себе, но не делает никаких предположений о топологии сети админа.
- **`empty` — всегда в списке и значение по умолчанию.** Поле `template` в
  запросе опционально (`omitempty`), пустая строка нормализуется в `"empty"`.
  Обратная совместимость со старыми клиентами сохранена.
- **`dnsmasq --test` при создании НЕ запускается.** Это новая точка в API:
  файл создаётся как скелет. Если бы мы запускали test на каждом шаблоне, набор
  шаблонов был бы ограничен тем, что проходит валидацию в отрыве от остальной
  конфигурации. Контракт синтаксической валидности удерживается тестом
  `TestConfigTemplatesValidForDnsmasqSyntax`.
- **Каталог templates — публичная ручка.** `GET /api/config/templates`
  возвращает ID + preview для каждого шаблона, чтобы UI мог показать админу
  что именно попадёт в файл до подтверждения. Лучше, чем захардкоженный список
  на фронте.
- **Имена шаблонов lowercase с дефисами.** Нормализация через `ToLower+TrimSpace`
  — `Basic-DHCP` и `"  forwarder  "` дают тот же результат. Защита от копипаста.
- **Поле `Template` в `AuditEntry`.** Аудит-лог должен показывать, через какой
  шаблон был создан файл. `omitempty` — не ломает существующие парсеры.

---

## 1. Бэкенд — каталог шаблонов (`config_templates.go`, новый файл)

Пакетная `map[string]string` с 5 записями:

| ID | Что кладёт в файл |
|----|---|
| `empty` | Только заголовок `# === Managed by Intermasq ===` (обратная совместимость) |
| `basic-dhcp` | `domain-needed`, `bogus-priv`, `expand-hosts`, `domain=lan` + закомментированные `dhcp-range`, `dhcp-option` |
| `forwarder` | `domain-needed`, `bogus-priv`, `no-resolv`, `strict-order` + закомментированные `server=`, `address=` |
| `pxe` | Все строки закомментированы (`dhcp-match`, `dhcp-boot`, `pxe-service`) — админ дополняет базовым DHCP |
| `aliases` | Заголовок + закомментированные примеры `address=`, `cname=` |

Каждый шаблон обязан начинаться с маркера `# === Managed by Intermasq ===` —
контракт проверяется тестом `TestConfigTemplatesAllStartWithManagedHeader`.

Хелпер `knownConfigTemplateIDs() []string` возвращает отсортированный список
ключей — нужен для подсказки в ошибке `unknown_template` и для стабильного
порядка в `GET /api/config/templates`.

---

## 2. Бэкенд — модели (`models.go`)

```go
type CreateConfigFileReq struct {
    Name     string `json:"name"`
    Template string `json:"template,omitempty"`
}
```

`Template` — опциональный. Пустая строка эквивалентна `"empty"` (нормализация
в handler).

---

## 3. Бэкенд — аудит (`audit.go`)

```go
type AuditEntry struct {
    // ... существующие поля ...
    Version  string `json:"version,omitempty"`
    Template string `json:"template,omitempty"`  // новое
}
```

`omitempty` сохраняет обратную совместимость со старыми парсерами audit-лога.

---

## 4. Бэкенд — API (`handlers.go`, `main.go`)

### Изменённый эндпоинт

`POST /api/config/file` теперь принимает `{name, template}`:

```
POST /api/config/file
{"name":"dhcp.conf","template":"basic-dhcp"}

→ 200 ConfigSnapshot  (как и раньше)
→ 400 invalid_filename            (name пустой / содержит / или \ / не .conf)
→ 400 unknown_template            (+ "available": ["aliases","basic-dhcp",…])
→ 403 access_denied               (путь вне ConfigDir)
→ 409 file_exists                 (такой .conf уже есть)
→ 500 write_error
```

Нормализация template ID:
1. `strings.ToLower(strings.TrimSpace(req.Template))` — `"  Basic-DHCP  "` → `"basic-dhcp"`.
2. Пустая строка → `"empty"`.
3. Lookup в `configTemplates`. Если нет — 400 с `available` списком.

Поведение `409 file_exists` **не изменилось**: даже при выборе шаблона нельзя
перезаписать существующий файл. Содержимое файла не трогается. Покрыто тестом
`TestCreateConfigFileHandlerExistingFileStill409`.

### Новый эндпоинт

```
GET /api/config/templates
Authorization: Bearer <token>

→ 200 {
  "templates": [
    {"id":"aliases","preview":"# === Managed by Intermasq ===\n…"},
    {"id":"basic-dhcp","preview":"…"},
    {"id":"empty","preview":"…"},
    {"id":"forwarder","preview":"…"},
    {"id":"pxe","preview":"…"}
  ]
}
```

Каталог нужен, чтобы UI не захардкоживал список шаблонов и мог показывать
preview до создания файла.

### Роутинг (`main.go`)

```go
auth.GET("/config", getConfigHandler)
auth.PUT("/config", updateConfigHandler)
auth.POST("/config/file", createConfigFileHandler)
auth.GET("/config/templates", listConfigTemplatesHandler)  // новое
```

Оба эндпоинта под `authMiddleware`.

---

## 5. Бэкенд — тесты (`dnsmasq_test.go`)

11 новых тестов (все проходят, `go test ./...` зелёный, 1 skip без бинарника dnsmasq):

| Тест | Что проверяет |
|------|---------------|
| `TestCreateConfigFileHandlerEachTemplate` | Table-driven по всем 5 шаблонам: контент в файле точно совпадает с `configTemplates[id]` |
| `TestCreateConfigFileHandlerEmptyTemplateDefault` | Отсутствие `template` в теле → используется `empty` (обратная совместимость) |
| `TestCreateConfigFileHandlerUnknownTemplate` | `template:"nonexistent"` → 400 + список available + файл не создаётся |
| `TestCreateConfigFileHandlerTemplateCaseInsensitive` | `"Basic-DHCP"` ≡ `"basic-dhcp"` |
| `TestCreateConfigFileHandlerTemplateWhitespace` | `"  forwarder  "` триммится |
| `TestCreateConfigFileHandlerExistingFileStill409` | Регресс: при выборе шаблона нельзя перезаписать существующий файл |
| `TestListConfigTemplatesHandler` | Каталог отдаёт все ID с непустым preview и маркером |
| `TestKnownConfigTemplateIDsSorted` | Контракт стабильного порядка (`sort.StringsAreSorted`) |
| `TestKnownConfigTemplateIDsContainsEmpty` | Инвариант: `empty` всегда есть в `configTemplates` |
| `TestConfigTemplatesAllStartWithManagedHeader` | Каждый шаблон начинается с `# === Managed by Intermasq ===` |
| `TestConfigTemplatesValidForDnsmasqSyntax` | Каждый шаблон проходит `dnsmasq --test` (skipped без бинарника, как existing-тесты) |
| `TestCreateConfigFileHandlerTemplateAuditWritten` | Audit-лог содержит `template:"forwarder"` при создании через шаблон |

Импорты в `dnsmasq_test.go` расширены: добавлены `fmt`, `os/exec`, `sort`.

---

## 6. Фронтенд — state (`store.js`)

В `store` добавлено поле `configTemplates: []`.

Обновлённый action:

```js
async createConfigFile(name, template = 'empty') {
    const res = await api.post('/config/file', { name, template })
    // …
}
```

Новый action:

```js
async loadConfigTemplates() {
    try {
        const res = await api.get('/config/templates')
        store.configTemplates = res.data.templates || []
    } catch (e) {
        // Fallback — без каталога UI всё равно работает, только без пресетов.
        store.configTemplates = [{ id: 'empty', preview: '# === Managed by Intermasq ===\n' }]
    }
}
```

Fallback гарантирует, что при недоступности бэкенда форма создания файла
продолжает работать (с одним вариантом `empty`).

---

## 7. Фронтенд — компонент `DnsmasqConfig.vue`

В форму создания файла добавлены:

- **`<select>` шаблонов** рядом с полем имени. Опции: `∅ Empty file` + все
  не-`empty` шаблоны из `store.configTemplates`. По умолчанию выбран `empty`.
- **`<pre>` preview** под формой. Показывает содержимое выбранного шаблона
  до создания файла (моноширинный шрифт, серый фон, max-height 200px со
  скроллом).
- **Ленивая загрузка каталога** при первом открытии формы через флаг
  `templatesLoaded`. Не падает, если бэкенд ещё не ответил — fallback в
  `loadConfigTemplates`.
- После успешного создания `newFileTemplate` сбрасывается в `'empty'`.

Изменены только template и `<script setup>` секции — остальной компонент не
тронут.

```html
<select v-model="newFileTemplate" …>
  <option value="empty">∅ {{ $t('config.templateEmpty') }}</option>
  <option v-for="tpl in nonEmptyTemplates" :key="tpl.id" :value="tpl.id">
    {{ tpl.id }}
  </option>
</select>

<pre v-if="selectedTemplatePreview" …>{{ selectedTemplatePreview }}</pre>
```

Computed-свойства:

```js
const nonEmptyTemplates = computed(() =>
    (store.configTemplates || []).filter(t => t.id !== 'empty')
)
const selectedTemplatePreview = computed(() =>
    (store.configTemplates || []).find(t => t.id === newFileTemplate.value)?.preview || ''
)
```

---

## 8. Фронтенд — локали

`ru.json`:
```json
"template": "Шаблон содержимого",
"templateEmpty": "Пустой файл",
```

`en.json`:
```json
"template": "Content template",
"templateEmpty": "Empty file",
```

Существующие ключи не тронуты.

---

## 9. Документация

Создан новый файл **`docs/config-templates.md`** с описанием фичи для
конечного пользователя: список шаблонов, API, поведение при ошибках, пример
UI-флоу.

`README.md` / `README.en.md` намеренно не тронуты — пользователь обновит их
сам, как и для других фич из `логи/predrel-*.md`.

---

## Проверки

- `go build ./...` — OK.
- `go vet ./...` — OK.
- `gofmt -l` на новых/изменённых файлах — пусто (pre-existing проблемы в
  `bins.go`/`main.go`/`models.go` не правились, они не относятся к этой фиче).
- `go test ./... -count=1` — все тесты проходят, 1 skip
  (`TestConfigTemplatesValidForDnsmasqSyntax`, как и existing-тесты без
  бинарника dnsmasq).
- `npm run build` (vite) — OK, 115 модулей, ~373 КБ JS.

---

## Риски и нюансы

- **Обратная совместимость:** `POST /api/config/file` без поля `template`
  продолжает работать как раньше — нормализуется в `"empty"`, в файл ложится
  та же строка `# === Managed by Intermasq ===`. Старые клиенты не ломаются.
- **Содержимое шаблонов не активирует ничего опасного.** Активны только
  boolean-директивы (`domain-needed`, `bogus-priv`, …) и `domain=lan` /
  `no-resolv` / `strict-order`. Директивы со значениями закомментированы —
  админ явно решает что включить.
- **`dnsmasq --test` при create не запускается** — это сознательно (см.
  «Принятые решения»). Контракт валидности удерживается тестом на самих
  шаблонах, а последующий `PUT /api/config` всё равно запускает `--test`.
- **Имена шаблонов — не user input.** Список зафиксирован в коде
  (`config_templates.go`). End-user не может подсунуть свой template ID —
  unknown ID даёт 400 без побочных эффектов.
- **Path traversal:** name-параметр проверяется тем же кодом, что и раньше
  (regex `.conf` + отсутствие `/` и `\`), путь проверяется через `isSafePath`.
  Никаких новых векторов атаки.
- **Acl:** Оба эндпоинта (`POST /api/config/file`, `GET /api/config/templates`)
  под `authMiddleware`. Неавторизованный доступ — 401, как и остальные
  `/api/*` ручки.
- **Размер binary:** `config_templates.go` ≈ 1.5 КБ исходника, ~2 КБ в
  скомпилированном виде. Копейки против ~10 МБ бинарника.
