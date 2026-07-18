# Сессия: портируемость путей, безопасный SSE-токен, hostname по RFC

## Контекст

Три независимые проблемы, выявленные ревью кодовой базы:

1. **Хардкоды путей бинарников.** Все вызовы внешних программ ссылались на
   абсолютные пути `/usr/bin/dnsmasq`, `/usr/bin/sudo`, `/usr/bin/systemctl`,
   `/usr/sbin/service`, `/usr/bin/rc-service`, `/usr/bin/sv`. На системах, где
   эти бинарники живут в `/bin` или `/sbin` (Alpine, старые Debian до
   usrmerge, отдельные embedded-сборки), команды не находились, и панель
   молча ломалась: `dnsmasq --test` падал с «не найден», рестарт сервиса
   падал, автоопределение init-системы через `fileExists` возвращало `none`.

2. **Утечка JWT через query-string SSE.** `connectSSE` в `store.js` клал токен
   в URL: `new EventSource('/api/events?token=' + token)`, а `authMiddleware`
   в `auth.go` принимал `c.Query("token")` как fallback. Токен попадал в
   access-логи nginx/Apache, в `Referrer`-заголовок при переходах, в историю
   браузера и в логи любого reverse-proxy перед панелью.

3. **Слабая валидация hostname.** `hostnameRegex = ^[a-zA-Z0-9-.]+$` пропускал
   имена, нарушающие DNS RFC: `-host`, `host-`, `.host`, `host.`, `host..name`,
   `host_name`. dnsmasq такое съедает, но резолверы и многие клиенты
   некорректно обрабатывают — это source-непредсказуемых глюков сети.

Решения обсуждались и принимались в предыдущей сессии (см. план в чате),
реализация — в этой.

---

## Изменение 1: пути бинарников через `$PATH` + флаги

### Подход

Гибрид: флаг оператора → `exec.LookPath` по `$PATH` → fallback на известные
абсолютные пути. `LookPath` автоматически находит бинарник в любом из
`/usr/bin`, `/usr/sbin`, `/bin`, `/sbin`, согласно переменной окружения `$PATH`
дистрибутива. Флаг даёт точечный override для нестандартных инсталляций.

### Новый файл `bins.go`

- Пакетные переменные: `dnsmasqBinPath`, `sudoBinPath`, `systemctlBinPath`,
  `serviceBinPath`, `rcServiceBinPath`, `svBinPath`.
- `binsOnce sync.Once` — резолв идёт ровно один раз; повторные вызовы
  (например, из тестов после `main()`) — no-op. Это развязывает тесты и
  порядок инициализации.
- `resolveBin(flagVal, candidates, fallbacks) string` — общая логика:
  1. Если флаг задан — использует его, но проверяет через `isExecutable`
     (существует + бит executable). Несуществующий флаг-путь логируется в
     stderr, идём дальше по списку.
  2. Перебирает `candidates` через `exec.LookPath` — основная ветка.
  3. Перебирает `fallbacks` — абсолютные пути в порядке предпочтения.
  4. Возвращает `""` если ничего не нашлось (вызывающий решает, фатально ли).
- `resolveBins()` — вызывается из `main()` после `flag.Parse()`, заполняет все
  6 переменных. На пустой `dnsmasqBinPath` печатает предупреждение в stderr.
- Акцессоры-функции `dnsmasqBin()`, `sudoBin()` и т.д. — ленивый резолв, если
  переменная ещё пуста (нужно тестам, которые не запускают `main`).

Fallback-порядок для каждого бинарника:
- `dnsmasq`: `dnsmasq` → `/usr/sbin/dnsmasq`, `/usr/bin/dnsmasq`, `/bin/dnsmasq`, `/sbin/dnsmasq`
- `sudo`: `sudo` → `/usr/bin/sudo`, `/bin/sudo`
- `systemctl`: `systemctl` → `/usr/bin/systemctl`, `/bin/systemctl`
- `service`: `service` → `/usr/sbin/service`, `/usr/bin/service`, `/sbin/service`, `/bin/service`
- `rc-service`: `rc-service` → `/usr/bin/rc-service`, `/bin/rc-service`, `/sbin/rc-service`
- `sv`: `sv` → `/usr/bin/sv`, `/bin/sv`

### `main.go`

- 6 новых флагов: `-dnsmasq-bin`, `-sudo-bin`, `-systemctl-bin`,
  `-service-bin`, `-rc-service-bin`, `-sv-bin`. Все с пустым дефолтом
  («auto-resolved via $PATH if empty»).
- `resolveBins()` вызывается первой строкой `main()` после `flag.Parse()`.

### `system.go`

- Все ~30 `exec.Command("/usr/bin/...")` заменены на
  `exec.Command(sudoBin(), systemctlBin(), ...)` и т.д. во всех пяти Caller'ах:
  `SystemdSystemCaller`, `SystemdUserCaller`, `OpenRCCaller`, `RunitCaller`,
  `SysVinitCaller`. Затронуты методы `IsActive`, `Restart`, `RestartSelf`.
- `detectInitSystem()` — заменил 4 проверки `fileExists("/usr/bin/...")` на
  сравнение `rcServiceBin() != ""`, `systemctlBin() != ""` и т.д. Логика
  функции идентична, изменился только источник «есть ли бинарник в системе».
- `detectSystemCaller()` — два вызова `sudo`/`systemctl` для авто-пробы (не
  root: пробуем sudo → пробуем user-systemd → fallback на sudo) переведены на
  резолвные функции.
- **Удалена** `fileExists(path)` — после миграции не осталось ни одного
  вызова.

### `dnsmasq.go`

- Все 5 вызовов `exec.Command("/usr/bin/dnsmasq", "--test")` (в
  `reloadDnsmasq`, `restoreHistoryVersion`, `writeConfigWithTest`,
  `writeFileRaw`, `restoreBackupZip`) заменены на `exec.Command(dnsmasqBin(),
  "--test")`. Сделано одним `replaceAll` — строки были идентичны.

---

## Изменение 2: SSE-токен через заголовок

### Подход

`EventSource` из коробки не умеет ставить произвольные заголовки — это
ограничение стандарта W3C. Используется библиотека `event-source-polyfill`,
которая реализует тот же интерфейс, но через XHR/fetch под капотом и
поддерживает опцию `headers`. Это самая лёгкая (≈12 КБ) и стабильная
альтернатива; переписывать на `fetch`+`ReadableStream` вручную было бы
дороже и хрупче.

### Фронтенд

- `frontend/package.json`: добавлена зависимость
  `event-source-polyfill: ^1.0.31`. `package-lock.json` обновлён через
  `npm install`.
- `frontend/src/store.js`:
  - Импорт `import { EventSourcePolyfill } from 'event-source-polyfill'`.
  - `connectSSE()` переписан: вместо
    `new EventSource('/api/events?token=' + encodeURIComponent(store.token))`
    теперь
    `new EventSourcePolyfill('/api/events', { headers: { Authorization: 'Bearer ' + store.token } })`.
  - Все обработчики (`arp`, `dnsmasq_status`, `onerror`) без изменений —
    сигнатура `EventSourcePolyfill` полностью совместима с `EventSource`.

### Бэкенд

- `auth.go:authMiddleware`: удалена ветка `else if q := c.Query("token"); q != ""`.
  Теперь принимается только `Authorization: Bearer ...` или `X-API-Key`.
  Оставлен комментарий-обоснование, почему `?token=` остался в `/metrics`.
- `metrics.go:checkMetricsAuth` — **не тронут**. `?token=` для Prometheus
  scrape_url оставлен сознательно: конфиг scrape в Prometheus не позволяет
  задавать кастомные заголовки без sidecar'а, а плодить сущности (специальный
  short-lived токен) за рамками этой задачи.
- `handlers.go:eventsHandler` — без изменений (ему всё равно, откуда токен,
  middleware уже отработала).

### Тесты `dnsmasq_test.go`

- `TestAuthMiddlewareQueryToken` → переименован в
  `TestAuthMiddlewareQueryTokenRejected`. Теперь передаёт валидный JWT через
  `?token=` и ожидает **401**. Комментарий в тесте фиксирует причину удаления
  ветки (логи/referrer/SSE).
- `TestAuthMiddlewareQueryTokenRevoked` — **удалён**. Раньше проверял, что
  отозванный токен через query даёт 401. Теперь **любой** токен через query
  даёт 401 — тест стал избыточным, покрытие не упало.
- `TestAuthMiddlewareQueryTokenBad` — **удалён** по той же причине
  (покрывается `...Rejected`).
- `TestEventsHandlerStreamsSSE` — URL изменён с `/api/events?token=x` на
  `/api/events`. Тест вызывает `eventsHandler` напрямую без middleware, так
  что `?token=` ни на что не влиял, но убрано для чистоты.

---

## Изменение 3: `hostnameRegex` по RFC

### Контекст стандартов

- **RFC 952** (оригинальная спецификация hostname) + **RFC 1123** §2.1
  (релаксация: разрешены ведущие цифры в label).
- **RFC 1034** (общая длина ≤253 символа, структурная валидация).
- **RFC 5891** (IDN-расширения, для нас неактуально — dnsmasq-конфиг в
  Punycode, и regex его уже принимает как ASCII).

Label: 1–63 символа, начинается и заканчивается буквенно-цифровым, внутри
`[a-zA-Z0-9-]`. Полное имя: один и более label'ов через ровно одну точку.

### Новая регулярка

```go
hostnameRegex = regexp.MustCompile(
    `^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?` +
    `(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`,
)
```

Разбор:
- `[a-zA-Z0-9]` — первый символ label'а, всегда буквенно-цифровой.
- `([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?` — опциональная «серединка» до 61 символа
  + обязательный буквенно-цифровой последний символ. Вместе с первым даёт
  диапазон 1–63 символа на label. Опциональность позволяет однобуквенные
  label'ы (`a`, `h1`).
- `(\.[a-zA-Z0-9](...)?)*` — ноль или более дополнительных label'ов через
  ровно одну точку. Две точки подряд не проходят — точка всегда требует
  после себя `[a-zA-Z0-9]`.
- Дефис на границе label'а невозможен: первый и последний символ всегда из
  `[a-zA-Z0-9]`.

Что **намеренно** остаётся валидным:
- Label, начинающийся с цифры (`1host`) — RFC 1123 §2.1 это разрешил.
- Двойной дефис внутри (`a--b`) — исторически зарезервированы только `xn--`
  для Punycode, общий случай разрешён.

### Проверка общей длины

Regex не покрывает «сумма длин label'ов + точки ≤253». Добавлен хелпер
`validHostname(s) bool` рядом с `PluginManifest` в `main.go`:

```go
func validHostname(s string) bool {
    if len(s) == 0 || len(s) > 253 {
        return false
    }
    return hostnameRegex.MatchString(s)
}
```

### Замены вызовов

6 мест заменены с `hostnameRegex.MatchString(x)` на `validHostname(x)`:

- `handlers.go:316` — `bulkEditHandler`, валидация нового hostname после
  transform.
- `handlers.go:426` — `addHostHandler`, валидация одиночного добавления.
- `handlers.go:536` — `bulkAddHostsHandler`, дедуп-цикл.
- `handlers.go:548` — `bulkAddHostsHandler`, цикл проверки конфликтов.
- `handlers.go:579` — `bulkAddHostsHandler`, построение множества новых MAC.
- `dnsmasq.go:823` — `parseCSVHosts`, валидация при CSV-импорте.

### Тесты `dnsmasq_test.go`

Добавлен `TestValidHostname` — таблица из 19 кейсов:

| Категория | Примеры | Ожидание |
|---|---|---|
| Корректные | `host1`, `my-host`, `a`, `a-b-c`, `host.example.com`, `1host`, `h1-h2.h3-h4` | ✓ |
| Пустая строка | `""` | ✗ |
| Ведущий/замыкающий дефис | `-host`, `host-`, `host.name-`, `-a.b` | ✗ |
| Ведущая/замыкающая точка | `.host`, `host.` | ✗ |
| Подряд идущие точки | `host..name` | ✗ |
| Запрещённые символы | `host name` (пробел), `host_name` (underscore) | ✗ |
| Длина >253 | `strings.Repeat("a", 254)` | ✗ |
| Граничный кейс длины | `aaa...(63).bbb...(63).ccc...(63).ddd...(60)` = 253 | ✓ |

---

## Документация

- `обновление.md` — добавлен раздел «⚠️ Обратные несовместимости» с тремя
  пунктами: (1) hostname-валидация, (2) `?token=` удалён из `/api/*`,
  (3) пути бинарников. Описано, на что влияет и что проверить перед
  обновлением.
- `docs/portability-and-validation.md` — новый файл, пользовательская
  документация новых возможностей и изменений (см. отдельный лог-файл для
  деталей реализации).

---

## Изменённые файлы

| Файл | Тип изменения | Что сделано |
|---|---|---|
| `bins.go` | **новый** | резолвер путей + акцессоры |
| `main.go` | изменён | 6 флагов, `resolveBins()`, новый `hostnameRegex`, `validHostname()` |
| `system.go` | изменён | замена ~30 хардкодов, правка `detectInitSystem`/`detectSystemCaller`, удалена `fileExists` |
| `dnsmasq.go` | изменён | 5 путей `dnsmasq` → `dnsmasqBin()`, `hostnameRegex.MatchString` → `validHostname` |
| `handlers.go` | изменён | 5 вызовов `hostnameRegex.MatchString` → `validHostname` |
| `auth.go` | изменён | удалена query-token ветка из `authMiddleware` |
| `dnsmasq_test.go` | изменён | `+TestValidHostname`, `TestAuthMiddlewareQueryToken` → `...Rejected`, удалены 2 избыточных теста, правка URL в SSE-тесте |
| `frontend/package.json` | изменён | `+event-source-polyfill` |
| `frontend/package-lock.json` | изменён | lockfile обновлён |
| `frontend/src/store.js` | изменён | `EventSource` → `EventSourcePolyfill` с заголовком |
| `frontend/dist/*` | перебилд | `npm run build` |
| `обновление.md` | изменён | раздел о несовместимостях |
| `docs/portability-and-validation.md` | **новый** | пользовательская документация |

---

## Проверка

```powershell
# Бэкенд
$env:INTERMASQ_SECRET="test-secret-key-0123456789abcdef0123456789abcdef"
go vet ./...        # чисто
go test .           # PASS (ok intermask)
go build ./...      # OK

# Фронтенд
cd frontend
npm run build       # 115 модулей, dist собран
```

CI-пайплайн `.forgejo/workflows/build.yml` прогоняет те же шаги на
`golang:1.25-alpine` — должен пройти без правок (Alpine как раз был одним из
мотиваторов изменения 1).

## Замеченные, но не тронутые места

- `c.GetHeader("X-API-Key")` в `authMiddleware` принимает `apiKey ==
  string(SecretKey)`. Это известный компромисс («API-ключ = мастер-секрет»),
  отдельная задача — не часть этой сессии.
- `eventsHandler` выставляет `Access-Control-Allow-Origin: *`. С заголовком
  `Authorization` (не cookie) это безопасно — браузер не шлёт CORS-preflight
  для простых GET без cookie-credentials.
- `aliasDomainRegex` в `main.go` всё ещё разрешает `a..b` и `a--b`. Это
  отдельная история — alias-домены могут быть wildcard-паттернами и
 Registrar-специфичными формами, требует самостоятельного обсуждения.
