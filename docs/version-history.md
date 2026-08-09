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

# Многоуровневая история версий конфигов (Version History)

Начиная с v3.0 в Intermasq добавлена многоуровневая история версий
конфигурационных файлов dnsmasq. Раньше `createLocalBackup` создавал
единственный `.bak`-файл, перезаписываемый при каждом изменении — откат
был возможен только на один шаг. Теперь параллельно ведётся архив из N
последних состояний каждого файла в `/etc/intermasq/history/`, с REST API
для просмотра списка версий, получения diff и восстановления любой версии.

---

## Что было раньше

- Перед каждым изменением `.conf`-файла вызывался `createLocalBackup`,
  который писал `file + ".bak"`.
- `.bak` перезаписывался при каждом следующем изменении — более ранняя
  история терялась.
- Откат — только `POST /api/rollback`, восстанавливает `.bak` (последнее
  состояние до текущего).
- В UI — кнопка `⏪ Откат` рядом с именем файла при наличии `.bak`.

## Что появилось

- **Архив версий** в `/etc/intermasq/history/` (настраивается флагом
  `-history-dir`). Хранится до N последних состояний каждого файла
  (по умолчанию 10, флаг `-history-depth`).
- **REST API:**
  - `GET /api/history?file=<path>` — список версий файла.
  - `GET /api/history/diff?file=<path>&from=<v>&to=<v|current>` —
    unified diff между двумя версиями или версией и текущим файлом.
  - `POST /api/history/restore` — восстановить файл из версии.
- **UI:** модалка «🕒 История» с таблицей версий, diff-просмотром и
  кнопкой восстановления. Доступна из трёх вкладок: «Статика», «DNS»,
  «Настройки dnsmasq».
- **Аудит:** новое действие `"restore"`, в `AuditEntry` появилось поле
  `Version`.
- **`.bak` не удалён** — это по-прежнему быстрый undo на один шаг.
  History — отдельный механизм для осознанного отката к конкретной версии.

---

## Как хранятся версии

- **Расположение:** `/etc/intermasq/history/` (флаг `-history-dir`).
- **Имя файла:** `<sha256-хэш-пути>_<версия>.bak`, где:
  - sha256 берётся от очищенного абсолютного пути конфига, первые 8 байт
    в hex — префикс, общий для всех версий одного файла.
  - версия — `YYYYMMDD-HHMMSS` в UTC, при коллизии (несколько снимков в
    одну секунду) добавляется суффикс `-2`, `-3`, …
- **Ротация:** при превышении `-history-depth` старейшие по mtime версии
  удаляются.
- **Безопасность:**
  - Версия валидируется regex `^\d{8}-\d{6}(-\d+)?$` — никакие
    path-сепараторы не допускаются, path traversal через `version`
    невозможен.
  - Путь файла проверяется через `isSafePath` (внутри `-conf-dir`).
  - Оригинальный путь конфига не восстанавливается из имени history-файла
    (необратимый хэш).

---

## Флаги запуска

| Флаг | По умолчанию | Назначение |
|------|--------------|------------|
| `-history-dir` | `/etc/intermasq/history` | Директория для версий |
| `-history-depth` | `10` | Сколько последних версий каждого файла хранить |

Директория создаётся автоматически при старте (`os.MkdirAll`, режим `0750`).

---

## REST API

Все эндпоинты требуют заголовок `Authorization: Bearer <token>`.

### `GET /api/history?file=<path>`

Список сохранённых версий файла, **новейшие первыми**.

**Query:**
- `file` — абсолютный путь к `.conf`-файлу внутри `-conf-dir`.

**Ответ 200:**
```json
{
  "versions": [
    { "version": "20260716-143012", "timestamp": "20260716-143012", "size": 412 },
    { "version": "20260716-135501", "timestamp": "20260716-135501", "size": 387 }
  ]
}
```

**Ошибки:**
- `400 file_required` — пустой `file`.
- `400 invalid_path` — путь вне `-conf-dir`.
- `500 history_error` — ошибка чтения директории.

### `GET /api/history/diff?file=<path>&from=<v>&to=<v|current>`

Unified diff между двумя версиями или между версией и текущим файлом.

**Query:**
- `file` — путь к файлу.
- `from` — версия (обязательный).
- `to` — версия или `"current"`/`""` для сравнения с текущим файлом на
  диске.

**Ответ 200:**
```json
{ "diff": "--- file (@20260716-135501)\n+++ file (@current)\n-dhcp-host=aa:bb:cc:dd:ee:ff,1.2.3.4,old\n+dhcp-host=aa:bb:cc:dd:ee:ff,1.2.3.5,new\n" }
```

**Ошибки:**
- `400 params_required` — пустые `file`/`from`.
- `400 invalid_path` — путь вне `-conf-dir`.
- `404 version_not_found` — версия не найдена.
- `404 current_not_found` — текущий файл не существует.

### `POST /api/history/restore`

Восстановить файл из указанной версии.

**Тело:**
```json
{ "file": "/etc/dnsmasq.d/hosts.conf", "version": "20260716-135501" }
```

**Поведение:**
1. Текущее состояние файла сохраняется в history (чтобы можно было undo).
2. Файл перезаписывается содержимым версии.
3. Запускается `dnsmasq --test`:
   - При успехе — файл остаётся, пишется аудит `Action:"restore"`.
   - При провале — восстанавливается pre-restore содержимое, возвращается
     ошибка.

**Ответ 200:**
```json
{ "status": "restore_ok" }
```

**Ошибки:**
- `400 params_required` — пустые `file`/`version`.
- `400 invalid_path` — путь вне `-conf-dir`.
- `500 restore_error` — ошибка (включая провал `dnsmasq --test`), поле
  `detail` содержит вывод теста.

---

## UI

### Точка запуска

Кнопка `🕒 История` добавлена рядом с существующей `⏪ Откат` в трёх
вкладках:

- **Статика** (`StaticView.vue`) — при выбранном конкретном файле (не
  «Все файлы»).
- **DNS** (`DnsAliasesView.vue`) — аналогично.
- **Настройки dnsmasq** (`DnsmasqConfig.vue`) — при выбранном файле,
  даже если у него нет `.bak`.

### Модалка «История версий файла»

Компонент `frontend/src/components/history/HistoryModal.vue`.

- При открытии загружает список версий через `GET /api/history`.
- Таблица: версия (как code), размер в байтах, действия.
- **`≠`** — получить diff выбранной версии против текущего файла.
  Diff отображается в `<pre>` под таблицей.
- **`⏪`** — восстановить версию с подтверждением. При успехе модалка
  закрывается, родительский компонент перезагружает данные.
- Локализация: `history.*`, `confirm.restore`, `alert.restoreSuccess`,
  `alert.restoreError`.

---

## Взаимодействие с `.bak`

| Механизм | Что хранит | Когда создаётся | Эндпоинт |
|----------|-----------|-----------------|----------|
| `.bak` | 1 последнее состояние до текущего изменения | Перед каждой записью в `createLocalBackup` | `POST /api/rollback` |
| history | N последних состояний | Там же, внутри `createLocalBackup` (через `saveHistory`) | `GET/POST /api/history*` |

Оба механизма сосуществуют:
- `.bak` — быстрый undo одной последней операции (один клик, без выбора
  версии).
- history — осознанный откат к конкретной версии из архива (с diff и
  подтверждением).

При restore из history текущее состояние **также сохраняется** в history,
поэтому undo restore доступен — в списке версий появится свежая запись.

---

## Аудит

В `AuditEntry` добавлено поле:

| Поле | Тип | Описание |
|------|-----|----------|
| `version` | `string` (omitempty) | Версия, восстановленная через `POST /api/history/restore` |

Новое действие: `"restore"` (в UI `AuditTab.vue` окрашено в жёлтый, как
`rollback`).

Пример записи:
```json
{
  "timestamp": "2026-07-16T14:30:12Z",
  "user": "admin",
  "action": "restore",
  "file": "/etc/dnsmasq.d/hosts.conf",
  "version": "20260716-135501"
}
```

---

## Внутренняя реализация (кратко)

- `saveHistory(filePath)` — копирует текущее содержимое файла в history,
  вызывает `rotateHistory`. No-op для несуществующего/пустого файла, пути
  вне `ConfigDir`, пустого `HistoryDir`. Ошибки логируются, не
  возвращаются (history — best-effort, не блокирует запись).
- `rotateHistory(filePath)` — удаляет старейшие по mtime версии сверх
  `-history-depth`.
- `listHistory(filePath)` — glob по префиксу, сортировка по убыванию
  версии.
- `readHistoryVersion(filePath, version)` — чтение конкретной версии с
  валидацией `version` через regex.
- `restoreHistoryVersion(filePath, version)` — `saveHistory` → запись
  версии → `dnsmasq --test` → откат при провале.
- `unifiedDiff(a, b, headerA, headerB)` — LCS-based line diff без внешних
  зависимостей.
- `createLocalBackup` — в начало добавлен вызов `saveHistory`, поэтому все
  существующие точки записи (15 вызовов в `handlers.go`) автоматически
  фиксируют историю.

Тесты: 8 новых в `dnsmasq_test.go` (создание версии, no-op для
несуществующего файла, reject unsafe path, ротация, invalid version,
сортировка, diff, пустой before).
