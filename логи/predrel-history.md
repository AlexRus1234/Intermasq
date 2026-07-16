# Сессия: predrel — Многоуровневый роллбэк (история версий конфигов)

## Контекст

Реализована многоуровневая история версий конфигурационных файлов dnsmasq.
Раньше `createLocalBackup` создавал единственный `.bak`-файл, который
перезаписывался при каждом изменении — откат возможен был только на один
шаг назад. Теперь параллельно с `.bak` ведётся архив из N последних
состояний каждого файла в `/etc/intermasq/history/`, с REST API для
просмотра списка версий, получения diff и восстановления любой версии.

Старые файлы документации (`docs/*.md`) намеренно не тронуты — пользователь
обновит их сам. Этот файл — журнал сессии (как другие в `логи/`).

---

## Коммит

| Хэш | Описание |
|-----|----------|
| (в этом PR) | Add multi-level version history for dnsmasq configs |

---

## Принятые решения

Выбрано в диалоге с пользователем до реализации:

- **Сосуществование с `.bak`:** `.bak` (быстрый undo на 1 шаг) **оставлен как
  есть**. History — отдельный механизм для осознанного отката к конкретной
  версии. Семантика не слита, `POST /api/rollback` не менялся.
- **Хранилище:** `/etc/intermasq/history/` (уже доступно для записи — там же
  `users.json`, `audit.log`, `templates.json`). Настраивается флагом
  `-history-dir`.
- **Глубина:** по умолчанию 10 версий на файл, флаг `-history-depth`.
  Ротация — удаление старейших по mtime при превышении.
- **Имена файлов истории:** sha256-хэш пути конфига (первые 8 байт, hex) +
  версия `YYYYMMDD-HHMMSS[-N]`. Суффикс `-N` добавляется при коллизии
  (несколько снимков в одну секунду). Оригинальный путь не восстанавливается
  из имени — безопасность через необратимость.
- **Версия — строгий regex** `^\d{8}-\d{6}(-\d+)?$`, никакие path-сепараторы
  не допускаются. Path traversal через `version` невозможен.
- **Восстановление** идёт через `dnsmasq --test`: при провале теста
  pre-restore содержимое возвращается на диск, ошибка возвращается клиенту.
  Перед восстановлением текущее состояние само сохраняется в history —
  undo restore доступен.
- **Точка фиксации истории:** внутри `createLocalBackup` — одна строка
  `saveHistory(filePath)` в начале. Все 15 точек вызова в `handlers.go`
  автоматически начали фиксировать историю, дифф минимален.
- **Diff:** собственная реализация LCS-based unified diff, без внешних
  зависимостей. Достаточно для коротких конфигов.
- **Аудит:** новое действие `"restore"`, в `AuditEntry` добавлено поле
  `Version` (omitempty).

---

## 1. Бэкенд — движок версий (`dnsmasq.go`)

### Новые функции

- `historyFilePrefix(filePath) string` — sha256(путь)[:8] + `_`.
- `historyFileName(filePath, version) string` — `<prefix><version>.bak`.
- `nextHistoryVersion(filePath) string` — `YYYYMMDD-HHMMSS`, при коллизии
  с существующим файлом добавляет `-2`, `-3`, …
- `saveHistory(filePath)` — копирует текущее содержимое файла в history,
  затем `rotateHistory`. No-op если файл не существует, пуст, вне
  `ConfigDir`, или `HistoryDir` пуст. Ошибки логируются, не возвращаются
  (history — best-effort, не должна блокировать запись).
- `rotateHistory(filePath)` — удаляет старейшие версии сверх `HistoryDepth`.
- `listHistory(filePath) ([]HistoryEntry, error)` — список версий, **новейшие
  первыми**.
- `readHistoryVersion(filePath, version) ([]byte, error)` — содержимое
  конкретной версии.
- `restoreHistoryVersion(filePath, version) error` — `saveHistory` → запись
  содержимого версии → `dnsmasq --test` → при провале откат pre-restore.
- `unifiedDiff(a, b, headerA, headerB) string` — LCS-based line diff в
  unified-стиле, без внешних зависимостей.

### Регистрация

- `var historyVersionRegex = regexp.MustCompile(`^\d{8}-\d{6}(-\d+)?$`)`.
- `isSafeHistoryPath` делегирует в `isSafePath` (history только для файлов
  внутри `ConfigDir`).
- `ensureHistoryDir()` — `os.MkdirAll(*HistoryDir, 0750)`.

### Изменённые функции

- `createLocalBackup(filePath)` — в начало добавлен вызов `saveHistory`.
  Семантика `.bak` не изменилась.
- `restoreHistoryVersion` — при провале `dnsmasq --test` восстанавливает
  **pre-restore** содержимое (исправлен первоначальный баг, когда
  перезаписывалось тем же контентом версии).

---

## 2. Бэкенд — флаги запуска (`main.go`)

| Флаг | По умолчанию | Назначение |
|------|--------------|------------|
| `-history-dir` | `/etc/intermasq/history` | Директория для версий |
| `-history-depth` | `10` | Сколько версий на файл хранить |

`ensureHistoryDir()` вызывается после `flag.Parse()`.

---

## 3. Бэкенд — API (`handlers.go`, `main.go`)

### Новые эндпоинты

| Метод | Путь | Описание | Auth |
|-------|------|----------|------|
| `GET` | `/api/history?file=<path>` | Список версий файла | Да |
| `GET` | `/api/history/diff?file=<path>&from=<v>&to=<v\|current>` | Unified diff | Да |
| `POST` | `/api/history/restore` | Восстановить версию | Да |

### `historyListHandler`

- Query-параметр `file` (абсолютный путь внутри `ConfigDir`).
- 400 `file_required` если пусто.
- 400 `invalid_path` если вне `ConfigDir` (`isSafePath`).
- 200 `{ versions: [{version, timestamp, size}] }`.

### `historyDiffHandler`

- Query: `file`, `from` (обязательный), `to` (опциональный; `""` или
  `"current"` — сравнение с текущим файлом на диске).
- 400 `params_required` если `file`/`from` пусты.
- 404 `version_not_found` / `current_not_found`.
- 200 `{ diff: "<unified text>" }`.

### `historyRestoreHandler`

- Тело: `HistoryRestoreReq{ File, Version }`.
- Под `mu.Lock` (как `rollbackHandler`).
- 400 `params_required` / `invalid_path`.
- 500 `restore_error` с `detail` (включая вывод `dnsmasq --test`).
- `writeAudit` с `Action:"restore"`, `Version` заполнен.
- 200 `{ status: "restore_ok" }`.

### Роутинг

В `main.go` рядом с `/rollback`:

```go
auth.POST("/rollback", rollbackHandler)
auth.GET("/history", historyListHandler)
auth.GET("/history/diff", historyDiffHandler)
auth.POST("/history/restore", historyRestoreHandler)
auth.POST("/reload", reloadHandler)
```

---

## 4. Бэкенд — модели и аудит

### `models.go`

- `HistoryRestoreReq{ File, Version string }`.

### `audit.go`

- В `AuditEntry` добавлено `Version string json:"version,omitempty"`.
- При restore пишется `Action:"restore"`, `Version: req.Version`.

---

## 5. Бэкенд — тесты (`dnsmasq_test.go`)

8 новых тестов (все проходят, `go test ./...` зелёный):

- `TestSaveHistoryCreatesVersion` — создаётся ровно одна версия.
- `TestSaveHistoryNoOpForMissingFile` — для несуществующего файла версий нет.
- `TestSaveHistoryRejectsUnsafePath` — путь вне `ConfigDir` игнорируется.
- `TestRotateHistoryKeepsDepth` — при `HistoryDepth=3` и 5 сохранениях
  остаётся 3 версии.
- `TestReadHistoryVersionInvalid` — `../escape` и `not-a-date` отвергаются.
- `TestListHistorySortedNewestFirst` — сортировка по убыванию версии.
- `TestUnifiedDiffAddsAndRemoves` — diff корректно показывает `+`/`-`.
- `TestUnifiedDiffEmptyA` — пустой «before» → все строки добавлены.

Хелпер `setupHistoryEnv(t)` создаёт temp `ConfigDir` и `HistoryDir`,
ставит `HistoryDepth=10`.

---

## 6. Фронтенд — state (`store.js`)

В `store` добавлены:

- `history: []` — список версий текущего файла.
- `historyDiff: ''` — текст diff для отображения.

Новые actions:

- `loadHistory(file)` — `GET /api/history?file=`, заполняет `store.history`,
  сбрасывает `historyDiff`.
- `loadHistoryDiff(file, from, to)` — `GET /api/history/diff`, заполняет
  `store.historyDiff`.
- `restoreHistory(file, version)` — `POST /api/history/restore`, alert
  успеха/ошибки.

---

## 7. Фронтенд — компонент `HistoryModal.vue`

Новый файл `frontend/src/components/history/HistoryModal.vue`:

- Модалка в стиле `TemplatesModal`/`BulkMoveModal` (Bootstrap modal-dialog).
- Props: `show`, `file`.
- Emits: `close`, `restored`.
- При открытии (`watch(props.show)`) загружает список версий.
- Таблица: версия (code), размер, действия.
- Кнопка `≠` — diff против текущего файла (`to: 'current'`).
- Кнопка `⏪` — восстановление с `confirm()`.
- Diff рендерится в `<pre class="history-diff">` с тёмным фоном.
- Локализация через `$t('history.*')`.

---

## 8. Фронтенд — точки запуска

Кнопка `🕒 История` добавлена рядом с существующей `⏪ Откат` в трёх
вкладках:

- `StaticView.vue` — видна при выбранном файле (не «Все файлы»).
- `DnsAliasesView.vue` — аналогично.
- `DnsmasqConfig.vue` — видна при выбранном файле (всегда, даже без `.bak`).

`HistoryModal` монтируется в каждом из трёх компонентов, emits `restored`
триггерят `actions.loadData()` / `actions.loadConfig()`.

---

## 9. Фронтенд — локали

### `ru.json` / `en.json`

Новая секция `history`:

```
title, icon, iconTooltip, empty, loading, version, size, bytes,
actions, diffVsCurrent, restore, diffTitle
```

Новые ключи:
- `confirm.restore` — подтверждение восстановления.
- `alert.restoreSuccess`, `alert.restoreError`.
- `alert.historyLoadError`, `alert.historyDiffError`.
- `api.restore_error`.
- `audit.action_restore`.

### `AuditTab.vue`

`restore` добавлен в `actionClass` рядом с `rollback` → `bg-warning text-dark`.

---

## 10. Документация

- `README.md` / `README.en.md`:
  - Фича «Бэкап и откат» расширена упоминанием многоуровневой истории.
  - Таблица API: 3 новых эндпоинта.
  - Таблица флагов: `-history-dir`, `-history-depth`.
- `docs/*.md` — **не тронуты** по просьбе пользователя (обновит сам).

---

## Проверки

- `go build ./...` — OK.
- `go vet ./...` — OK.
- `go test ./... -count=1` — все тесты (включая 8 новых) проходят.
- `npm run build` (vite) — OK, 108 модулей, ~336 КБ JS.

---

## Риски и нюансы

- **Конкуренция:** `saveHistory` вызывается из `createLocalBackup`, который
  всегда под `mu.Lock` в handlers — гонок нет.
- **Размер history:** 10 версий × десятки КБ = копейки. При больших конфигах
  `HistoryDepth` настраивается флагом.
- **`dnsmasq --test` при restore обязателен** — иначе можно положить DNS
  восстановлением версии с синтаксической ошибкой.
- **Path traversal:** version-параметр строго через regex, file — через
  `isSafePath`. Имена history-файлов — через sha256-хэш, не через
  оригинальный путь.
- **Коллизия имён** при нескольких снимках в одну секунду решается суффиксом
  `-N` (тест `TestRotateHistoryKeepsDepth` это проверяет).
