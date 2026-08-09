# Raw-редактор конфигов и RBAC в UI

Два пользовательских изменения поверх рефакторинга бекенда: текстовый
(raw) редактор `.conf`-файлов, который раньше был только на уровне API, и
роль-зависимое поведение интерфейса (RBAC). Плюс мелкая инфра-
зачистка — единый logout и локальный `Makefile`.

---

## 1. Raw-редактор конфигураций

### Что было

Бекенд давно умел читать и писать `.conf`-файлы как plain-text через
`GET/PUT /api/files/:name` (см. `docs/new-features.md` §1), но во фронтенде
существовал только **визуальный** редактор директив (`DnsmasqConfig.vue`).
Он удобен для известных ключей, но не может представить произвольный
текст — комментарии, нестандартный порядок, неподдерживаемые директивы.
Raw-поверхность существовала только в smoke-тестах и L1/L2 Go-тестах.

### Что появилось

В тулбаре конфиг-вкладки появился переключатель **🎨 Visual / 📝 Raw**
(виден только при выбранном файле). В raw-режиме — `<textarea>` с
monospace, содержимое которой PUT'ится на `PUT /api/files/:name`.

- Переключение Visual → Raw подгружает свежий текст файла (`GET /api/files/:name`).
- Переключение обратно — перечитывает снапшот (`GET /api/config`), чтобы
  визуальный редактор и индикатор `.bak` увидели изменения.
- Смена файла в raw-режиме перезагружает содержимое textarea.
- Общий кнопка 💾 Save диспетчеризует: в visual → `PUT /api/config`
  (сериализация директив), в raw → `PUT /api/files/:name` (plain text).

### Гарантии безопасности (те же, что у визуального пути)

Оба пути записи идут через `internal/dnsmasq/write.go`:

| Этап | Функция | Поведение |
|------|---------|-----------|
| Проверка пути | `IsSafePath` | отказ, если файл вне `-conf-dir` |
| Бэкап | `CreateLocalBackup` | `.bak` + snapshot в versioned history |
| Валидация | `dnsmasq --test --conf-file=<path>` | реальная проверка синтаксиса |
| Откат при ошибке | `restoreLocalBackup` | без повторного test (иначе старый валидный конфиг не вернётся) |
| Аудит | `audit.WriteAudit` | `config_write_raw` для raw, `config_update` для visual |

При провале `--test` хендлер возвращает `400 {"error":"dnsmasq_test_failed",
"detail":"<вывод dnsmasq>"}`; UI показывает detail в alert, чтобы было
видно, **какую** строку отверг dnsmasq. См. фикс A13 (`writeFileRaw`
действительно тестирует записанный файл) и A14 (проверка argv).

### Доступ

- `GET /api/files/:name` — `auth` (любой аутентифицированный пользователь).
- `PUT /api/files/:name` — `admin` (`AdminMiddleware`).

Поэтому переключатель Raw скрыт для не-adminов (`v-if="store.isAdmin"`) —
читать raw они могли бы, но сохранить нет смысла (403).

---

## 2. RBAC в UI (роль-зависимые элементы)

### Контекст

Бекенд разделяет маршруты `/api` на две группы (`internal/webapi/register.go`):

- `auth` — любой аутентифицированный пользователь (`auth.Middleware`).
- `admin` — только администраторы (`auth.AdminMiddleware`).

`MakeToken` (`internal/auth/auth.go:368`) **уже** кладёт claim `role` в
JWT. До этого изменения фронтенд роль не декодировал — обычный
пользователь видел admin-кнопки и при клике получал 403 с непонятным
alert'ом.

### Что появилось

В `store.js` добавлены геттеры на reactive-сторе:

```js
get role()    { return decodeRole(this.token) }   // 'admin' | 'user'
get isAdmin() { return this.role === 'admin' }
```

`decodeRole` читает claim `role` из payload JWT **без проверки подписи** —
бережно относимся к старым/битым токенам (дефолт `'user'`, чтобы сессия
деградировала безопасно: admin-контролы скрыты, а не наоборот). Геттеры
реактивны: шаблоны, зависящие от `store.isAdmin`, обновляются при
login/logout/смене роли без ручной проводки.

### Что скрыто для не-adminов (`v-if="store.isAdmin"`)

10 точек, каждая соответствует admin-маршруту в `register.go`:

| UI-элемент | Backend | Файл |
|------------|---------|------|
| 🔄 Apply (reload) | `POST /api/reload` | `App.vue` |
| 📤 Restore (backup/restore) | `POST /api/backup/restore` | `App.vue`, `SafetyTab.vue` |
| 🔄 Restart Intermasq | `POST /api/restart-self` | `App.vue` (меню) |
| 👥 Вкладка Users | `GET/POST/DELETE /api/users` | `App.vue` |
| ⏪ Rollback | `POST /api/rollback` | `StaticView.vue`, `DnsAliasesView.vue`, `DnsmasqConfig.vue` |
| 📝 Raw-toggle | `PUT /api/files/:name` | `DnsmasqConfig.vue` |
| ⏪ History restore | `POST /api/history/restore` | `HistoryModal.vue` |

**Видны всем аутентифицированным:** Backup (download, `GET /api/backup`),
CSV-экспорт, вкладки Static/DNS/Discovery/Config/Safety, audit-лог,
история версий (просмотр + diff), управление шаблонами хостов
(`POST/DELETE /api/templates` — это `auth`, не `admin`).

### Source of truth — бекенд

`AdminMiddleware` остаётся авторитетом: подделанный client-side `role`
даёт только «неправильный» UI, но не даёт привилегий — каждый admin-запрос
перепроверяется сервером. Это намеренный выбор: фронтенд доверяет токену
лишь для решения, что рендерить.

---

## 3. Прочие зачистки

### Единый logout

Раньше было два пути:
- `store.logout()` — локальная очистка, использовался на 401.
- `system.logoutRequest()` — `POST /api/logout` + очистка, был привязан к меню.

Оба слиты в один канонический `actions.logout()`: fire-and-forget POST на
revocation (чёрный список jti), затем локальная очистка. Инлайн-обработчик
401 в `config.js` тоже теперь зовёт его — единая точка выхода из сессии.

### `Makefile`

CI (`.forgejo/workflows/build.yml`) и так пересобирает `frontend/dist`
перед `go build`. Локальный `go build` молча въезжал со стейлым embed
(`//go:embed frontend/dist/*`). Теперь:

```sh
make build    # npm run build (с авто npm install) → go build
make clean    # удалить frontend/dist и бинарь
```

Зеркалит порядок шагов CI — локальный бинарь больше не embed'ит устаревший
bundle.

---

## 4. Тесты

| Уровень | Где | Что покрывает |
|---------|-----|---------------|
| L1 (Go unit) | `internal/webapi/*_test.go`, `internal/dnsmasq/*_test.go` | raw-хендлеры и `ReadFileRaw`/`WriteFileRaw` — **без изменений**, были полностью покрыты ранее |
| L3 (smoke) | `tests/suites/40-config-files.sh`, `81-path-traversal.sh` | GET/PUT raw + A13 + path-traversal — **без изменений** |
| L4 (Playwright) | `tests/e2e/specs/config-raw.spec.ts` | raw UI: валидный контент → PUT 200, невалидный → PUT 400 + rollback. Skip снят (раньше был permanent-skip — нет UI; теперь UI есть) |
| L4 (Playwright) | `tests/e2e/specs/rbac-admin-controls.spec.ts` | non-admin пользователь: admin-контролы отсутствуют в DOM, auth-любого-юзера контролы рендерятся |

RBAC-спек создаёт пользователя с дефолтной ролью (`POST /api/users` →
`RoleUser`), логинится им в отдельном контексте и ассертит `toHaveCount(0)`
на admin-элементах. Бекенд не менялся — L1/L3 не затронуты.
