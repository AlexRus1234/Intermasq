# Баг-репорт + план автоматизации (после ручного тестирования v3)

Собрано из пометок в `manual-testing-checklist.md` после первого прохода.
Код НЕ трогал — только диагноз по чтению.

---

## A. Подтверждённые баги (по приоритету)

### A1. 🔴 CRITICAL — Дублирование строк таблицы при сортировке

**Симптом:** при клике на заголовок MAC/IP/Hostname количество строк в
таблице кратно растёт (2 → 4 → 8 → 16). После F5 сбрасывается до исходного.

**Пример из лога:**
```
🟢 60:3d:61:28:89:5c 172.20.5.78   dwada23332  yadr00x05.conf
🟢 60:3d:61:28:89:5c 172.20.200.2  yandexST    iot172x20x200.conf
🟢 60:3d:61:28:89:5c 172.20.5.78   dwada23332  yadr00x05.conf
🟢 60:3d:61:28:89:5c 172.20.200.2  yandexST    iot172x20x200.conf
... (ещё 4 строки)
```

**Корень:** `frontend/src/components/static/HostTable.vue:27`:
```vue
<tr v-for="h in sortedHosts" :key="h.mac">
```
Ключ `:key="h.mac"` **не уникален**, потому что тот же MAC может быть
записан в нескольких `.conf` файлах (или, что ещё вероятнее, одна и та
же запись возвращается сервером с суффиксом `|has_bak` и без него —
`getHostsHandler` в `handlers_hosts.go:48` мутирует `entry.File`).

Vue требует уникальных ключей в `v-for`. При неуникальных ключах
reconciliation падает в undefined behavior — на практике дубль DOM при
перерисовке после изменения `sortKey` / `sortAsc`.

**Фикс (когда дойдём):**
```vue
<tr v-for="h in sortedHosts" :key="h.mac + '|' + (h.file||'')">
```
Дополнительно: `getHostsHandler` мутирует `entry.File` через `|has_bak`-
суффикс — это анти-паттерн (пачкать данные маркером состояния). Лучше
вернуть отдельное поле `has_bak bool` в `HostEntry`, а в UI показывать
индикатором. Это автоматически исправит и баг с ключом.

---

### A2. 🔴 CRITICAL — Дубликаты DNS-alias можно добавлять

**Симптом:** "Duplicate domain+type → можно добавить полную копию"

**Корень:** `handlers_aliases.go:76` + `aliases.go:176-189`:
```go
conflicts := findAliasesByDomain(req.Domain, req.Type, req.File)
```
А `findAliasesByDomain` **исключает** запись с matching `type+file`:
```go
if a.Type == excludeType && cleanAliasFile(a.File) == excludeFile {
    continue
}
```
Логика исключения имеет смысл для PUT/edit-флоу ("не считай сам себя
конфликтом"). Для add-флоу получается ровно наоборот: существующая
запись в том же файле с тем же type **исключается из конфликтов** →
функция возвращает 0 конфликтов → `addAliasHandler` пропускает duplicate-
check и appending строку.

**Фикс:** для POST (add) вызывать `findAliasesByDomain(domain, "", "")` —
без exclude. Разделение add vs edit в этом месте сделано через
`addAliasHandler` / гипотетический `updateAliasHandler`, но update
в коде вообще нет — так что exclude бесполезен и его можно просто
убрать. Заодно проверь `bulkAddAliasesHandler:139` и
`importAliasesCSVHandler:287` — там `findAliasesByDomain(a.Domain, "", "")`
уже вызывается правильно, так что баг только в single-add.

---

### A3. 🟠 HIGH — MAC `00:00:00:00:00:00` принимается

**Симптом:** "применилось" (сохранилось в файл).

**Корень:** `main.go:78`:
```go
macRegex = regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`)
```
Регекс пропускает broadcast/zero MAC. dnsmasq на релоаде такое либо
режет, либо (что хуже) тихо принимает и ломает DHCP.

**Фикс:** добавить чёрный список MAC в `validateHostFields`:
```go
if mac == "00:00:00:00:00:00" || mac == "FF:FF:FF:FF:FF:FF" { return false }
```
Или расширить регекс (уродливо). Лучше отдельная проверка.

---

### A4. 🟠 HIGH — MAC с `-` сохраняется, dnsmasq потом падает

**Симптом:** "ошибка перезапуска" после `dhcp-host=aa-bb-cc-dd-ee-02,...`.

**Корень:** тот же `macRegex` принимает и `:`, и `-`. `formatDhcpHostLine`
пишет MAC as-is. dnsmasq принимает только `:` (man-страница: "six bytes
separated by colons"), `--test` фейлится, весь reload фейлится.

**Фикс:** нормализовать MAC к `:`-форме в `validateHostFields` (или
в точке входа — `addHostHandler`, `bulkAddHostsHandler`,
`importCSVHandler`). Заодно это поможет консистентности `findHostsByMac`.

---

### A5. 🟠 HIGH — Bulk-edit: модалка либо не реагирует, либо no_hosts

**Симптом пользователя:**
- "edit - ничего не происходит" (с выбранными чекбоксами)
- "стоит мне убрать все галочки и появляются поля для заполнения,
  но там я постоянно получаю nohost"

**Анализ:**
1. `HostTable.vue:4` — панель действий показывается только
   `v-if="selectedHosts.length > 0"`. Убрал все галочки → панель
   пропала. "Появляются поля для заполнения" — пользователь, скорее
   всего, видит `BulkEditModal` в каком-то висящем состоянии (модалка
   не закрылась). `BulkEditModal.vue:1` — `<div v-if="show" ...>` —
   закрывается через `@close`. Если пользователь меняет выбор хостов
   пока модалка открыта, она остаётся открытой с уже невалидным
   `props.hosts=[]` → сабмит → `no_hosts` от бэка.
2. "ничего не происходит" при выбранных — `canSubmit` (строка 88)
   требует хотя бы одного из полей IP-transform или hostname-transform.
   Если оставить все пустым — кнопка disabled, визуально никакой
   обратной связи нет.

**Фикс:**
- В `HostTable.vue` добавить `watch(selectedHosts)` → если пусто,
  форсировать `showEdit=false; showMove=false`.
- В `BulkEditModal.vue` добавить `disabled`-тултип или
  `:title="!canSubmit ? 'Заполните хотя бы одно поле' : ''"` на
  кнопке Apply.
- В **README/чeклисте** уточнить, что нужно выбрать хосты → открыть
  модалку → заполнить IP **или** hostname transform → Apply.

---

### A6. 🟡 MEDIUM — Bulk-import показывает "импортировано 0"

**Симптом:** "испортирует, но каждый раз пишет что импортировано 0,
хотя на деле все ок".

**Гипотезы (нужны детали от пользователя):**
1. Пользователь использует CSV-режим в `HostForm.vue` (третья вкладка).
   `importCSV` в `api/hosts.js:95` показывает
   `t('alert.csvImportSuccess', { count: res.data.count })`.
   Бэк `importCSVHandler` возвращает `{"status":"ok","count":len(hosts)}`
   после **фильтрации** через `validateHostFields`. Если в CSV MAC'и
   в формате `aa-bb-...` (см. A4) или невалидные hostname — они
   тихо отфильтровываются, count=0, но статус 200 (проверка на 0
   делается ДО фильтрации через `len(records)`, не `len(hosts)`).
   **Точный диагноз:** `parseCSVHosts` (`dnsmasq.go:392-415`) проверяет
   каждую строку через `validateHostFields` и тихо skip'ает
   невалидные. Затем `importCSVHandler:372` проверяет `len(hosts)==0`
   → должен бы вернуть `csv_empty`, но если прошла хотя бы одна
   строка — returns 200 с count=N. Если count=0 значит все строки
   отфильтрованы → д.б. `csv_empty`. **Противоречие** — нужно
   воспроизвести с конкретным входом.
2. Пользователь использует text-режим (`parsedBulkHosts`).
   `HostForm.vue:265` показывает `t('alert.importSuccess', { count:
   parsedBulkHosts.value.length })`. Этот count — клиентский, д.б.
   точным.
3. Пользователь использует standalone `BulkImport.vue` — но тот
   просто emits текст без своего alert'а.

**Что сделать:** спросить пользователя, какой именно режим (single /
text / csv) использовался и что за данные вставлял. И добавить в alert
явное разделение: `"parsed: N, written: M"`.

---

### A7. 🟡 MEDIUM — Templates UI не соответствует чек-листу

**Симптом пользователя:**
"У меня вообще по другому колонки: name, вторая выпадающая пустая,
третья device-{NNN}, четвертая путь /etc/dnsmasq,d/hosts.conf".

**Анализ:** это **не баг**, это сам UI. `TemplatesModal.vue:27-39`:
- col-md-6: `form.name`
- col-md-6: select для `form.ip_range` (становится выпадашкой, если
  есть `store.dhcpRanges`; пустая, потому что dhcp-range в .conf
  файлах не задан → `store.dhcpRanges=[]`)
- col-md-6: `form.hostname_pattern` с placeholder `device-{NNN}`
- col-md-6: `form.target_file` с placeholder `/etc/dnsmasq.d/hosts.conf`

**Fix:** обновить чек-лист (готов сделать), либо переработать UI в
2 колонки вместо 4 и подписать поля явно (не placeholder'ами).

---

### A8. 🟡 MEDIUM — /metrics на `curl` без `-i` выглядит "пустым"

**Симптом:**
```
[root@SHLZ00 ~]# curl http://localhost:8081/metrics
[root@SHLZ00 ~]#
```

**Анализ:** это **не баг**, а ожидаемое поведение. `metricsHandler`
без auth делает `c.AbortWithStatus(401)` — без body. curl по умолчанию
не печатает статус-код и не считает 4xx ошибкой. С `curl -i` или
`curl -v` будет видно `HTTP/1.1 401`.

Тем не менее, для UX оператора стоит добавить тело:
```go
c.AbortWithStatusJSON(401, gin.H{"error": "auth_required"})
```
Поможет и Prometheus-у давать более понятный `last_error` в UI.

---

### A9. 🟢 LOW — Export CSV экспортирует всё

**Симптом:** "он вообще все экспортирует. надо добавить ещё возможность
локального экспорта".

**Анализ:** `exportCSVHandler` правильно делает своё дело (все .conf
из ConfigDir). Запрос на фичу: добавить query-parameter `?file=...`
для экспорта одного файла. Маленькая правка.

---

### A10. 🟢 LOW — Discovered-devices: "не знаю что это"

**Симптом:** устройства показаны, но что это за устройства — непонятно.

**Анализ:** `getNewDevices` возвращает `{mac, ip, vendor}`. **IP не
заполнен**, потому что ARP-парсер `parseArpContent` хранит только
`map[mac]bool`, IP выбрасывается. OUI-vendor работает только для
зарегистрированных вендоров — для рандомизированных MAC (телефоны)
не покажет ничего осмысленного.

**Фикс:** расширить `parseArpContent` → возвращать `map[mac]struct{IP,
Mask}` (или хотя бы IP). UI покажет IP рядом с MAC — сразу понятно,
где оно в сети. Заодно `getArpTable()` тоже вернёт IP, и `ArpTab.vue`
сможет сортировать/фильтровать по IP.

---

### A11. 🟢 LOW — Path-traversal чек-лист: "как это делать?"

Не баг, просто пользователь не знает как тестировать. Конкретные
curl-команды — см. раздел C ниже, и они идеальный кандидат на
автоматизацию (раздел D).

---

## B. Не протестировано (и почему)

| Что | Почему | Что нужно |
|---|---|---|
| 6.2–6.5. Config editor | Нет `.conf` файлов с реальной конфигурацией | Применить template `basic-dhcp` через UI, потом тыкать |
| 7.2 Diff/Restore | "Это мне по ssh лезть?" — да, history-файлы в `/etc/intermasq/history/` | Скрипт в D2 сделает сам |
| 8. tcpdump/SSE | `tcpdump: command not found` | `dnf install tcpdump` или использовать `ss -tnp` |
| 12.4 Portability | Один тестовый сервер, не Alpine | Alpine container в CI (раздел D5) |
| 13. Plugins | Частично — есть один рабочий | Простой hello-world плагин в `tests/fixtures/` |
| 15. Path traversal | Пользователь не знает curl | Готовый скрипт, см. C |
| 17. Performance | "не могу" — нет 200 хостов | Fixture-генератор, см. D6 |
| 18. Logs | Не тыкал | Автоматически проверяется в D2 |

---

## C. Готовые curl-команды для path-traversal (раздел 15)

Сохранить как `tests/manual/path-traversal.sh`, запускать после логина.
Каждая команда должна вернуть 4xx и **не** изменить состояние системы.

```bash
#!/usr/bin/env bash
set -u
BASE="${BASE:-http://localhost:8081}"
SECRET="${INTERMASQ_SECRET:?must be set}"

# Получаем JWT
TOKEN=$(curl -s -X POST "$BASE/api/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"pass1234"}' | jq -r .token)

echo "=== 1. POST /api/hosts с file=/etc/passwd ==="
curl -s -o /dev/null -w "HTTP %{http_code}\n" -X POST "$BASE/api/hosts" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"mac":"aa:bb:cc:dd:ee:ff","hostname":"x","file":"/etc/passwd"}'
# Ожидается: 400 (invalid_data) или 403 (access_denied)

echo "=== 2. DELETE /api/hosts/:mac?file=/etc/passwd ==="
curl -s -o /dev/null -w "HTTP %{http_code}\n" -X DELETE \
  "$BASE/api/hosts/aa:bb:cc:dd:ee:00?file=/etc/passwd" \
  -H "Authorization: Bearer $TOKEN"
# Ожидается: 400/403

echo "=== 3. POST /api/aliases с file=../../../tmp/x.conf ==="
curl -s -o /dev/null -w "HTTP %{http_code}\n" -X POST "$BASE/api/aliases" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"A","domain":"x.test","target":"10.0.0.1","file":"../../../tmp/x.conf"}'
# Ожидается: 403

echo "=== 4. GET /api/files/..%2F..%2Fetc%2Fpasswd ==="
curl -s -o /dev/null -w "HTTP %{http_code}\n" \
  "$BASE/api/files/..%2F..%2Fetc%2Fpasswd" \
  -H "Authorization: Bearer $TOKEN"
# Ожидается: 403

echo "=== 5. PUT /api/files/passwd (non-.conf) ==="
curl -s -o /dev/null -w "HTTP %{http_code}\n" -X PUT "$BASE/api/files/passwd" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"content":"x"}'
# Ожидается: 403

echo "=== 6. POST /api/history/restore с file=/etc/hosts ==="
curl -s -o /dev/null -w "HTTP %{http_code}\n" -X POST "$BASE/api/history/restore" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"file":"/etc/hosts","version":"20240101-000000"}'
# Ожидается: 400 (invalid_path)

echo "=== 7. POST /api/hosts с hostname=со \\n (newline injection) ==="
curl -s -o /dev/null -w "HTTP %{http_code}\n" -X POST "$BASE/api/hosts" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  --data-binary $'{"mac":"aa:bb:cc:dd:ee:01","hostname":"a\\ndhcp-host=evil","file":"/etc/dnsmasq.d/test.conf"}'
# Ожидается: 400 (invalid_hostname)

echo "=== 8. CSV с embedded quote ==="
printf 'mac,ip,hostname\naa:bb:cc:dd:ee:02,10.0.0.2,"x,y"\n' > /tmp/t.csv
curl -s -o /dev/null -w "HTTP %{http_code}\n" -X POST "$BASE/api/hosts/csv" \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@/tmp/t.csv" -F "target_file=/etc/dnsmasq.d/test.conf"
# Ожидается: 200 (csv reader корректно парсит quoted)
rm -f /tmp/t.csv
```

---

## D. Стратегия автоматизации (всего чек-листа)

Главная идея: **4 уровня автоматизации, каждый со своей ценностью**.

| Уровень | Инструмент | Что ловит | Время на старте | Поддержка |
|---|---|---|---|---|
| D1 | Go `httptest` | API-контракты, validation, path-traversal, race | 1 день | низкая |
| D2 | Bash + curl smoke | Полный e2e на реальном Linux | 0.5 дня | низкая |
| D3 | Playwright (browser) | UI-баги (сортировка, модалки, реактивность) | 2 дня | средняя |
| D4 | Fixtures + stress | Performance, 200 хостов | 0.5 дня | низкая |
| D5 | Container e2e (Alpine) | Portability, init-system edge cases | 1 день | средняя |

**Принцип слоёв:** D1 падает за 5 сек на каждом `go test`. D2 — за 30 сек
на тестовом сервере перед коммитом. D3 — за 2-5 мин в CI на каждый PR.
D4/D5 — по расписанию (раз в неделю / перед релизом).

---

### D1. Go integration tests через `httptest` (самое важное)

Расширение существующего `new_features_test.go` и `dnsmasq_test.go`.

**Ценность:** ловит баги A2, A3, A4, A6, A11, плюс 80% пунктов
чек-листа разделов 1, 2, 3, 5, 6 (API-часть), 9, 15.

**Идея:** поднять `gin.Engine` через `go gin` + `httptest.NewRequest`,
вместо реального HTTP-сервера. Моки на `sysCaller` и `dnsmasq --test`.

**Новый файл `tests/integration_test.go`, готовая структура:**

```go
func setupTestServer(t *testing.T) (*gin.Engine, string) {
    tmpDir := t.TempDir()
    *ConfigDir = tmpDir + "/conf"
    *DBPath = tmpDir + "/users.json"
    *AuditLogPath = tmpDir + "/audit.log"
    *HistoryDir = tmpDir + "/history"
    os.MkdirAll(*ConfigDir, 0755)
    SecretKey = []byte("test-secret-32-bytes-test-secret-32")

    gin.SetMode(gin.TestMode)
    r := gin.New()
    // ... (копия роутинга из main.go, вынесенная в helper)
    return r, tmpDir
}

func TestAddHostRejectsZeroMac(t *testing.T) {
    r, _ := setupTestServer(t)
    // ... login to get token
    body := `{"mac":"00:00:00:00:00:00","ip":"10.0.0.1","file":"/etc/dnsmasq.d/x.conf"}`
    req := httptest.NewRequest("POST", "/api/hosts", strings.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+token)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    if w.Code != 400 { t.Fatalf("expected 400, got %d", w.Code) }
}
```

**Что писать в первую очередь (по нашим багам):**
1. `TestAddHostRejectsZeroMac` (A3)
2. `TestAddHostRejectsBroadcastMac` (A3)
3. `TestAddHostRejectsDashSeparator` или `TestAddHostNormalizesDashMac` (A4)
4. `TestAddAliasRejectsDuplicateSameFile` (A2)
5. `TestPathTraversal_Hosts_FileOutsideConfDir` (A11.1)
6. `TestPathTraversal_HistoryRestore_OutsideConfDir` (A11.6)
7. `TestPathTraversal_Files_NonConfExtension` (A11.5)
8. `TestMetricsReturns401WithoutAuth` (A8 — фикс спецификации)
9. `TestSortKeysUnique` — отдельный unit-test на Vue-стороне невозможен,
   но можно Assert на уровне API: каждый `entry.File` в `/api/hosts`
   уникален с `entry.Mac` (fix A1 — переход на `has_bak` boolean).

**Прогон:** `go test ./... -count=1 -race`. Существующий `dnsmasq_test.go`
(2255 строк) — отличный референс по стилю.

**Оценка:** ~1 день на 30 базовых integration-тестов, потом ещё
добавлять по мере нахождения новых багов.

---

### D2. Bash + curl smoke-скрипт для реального сервера

**Ценность:** один скрипт, который прогоняет 90% "внешних" проверок
на тестовом сервере за 30 секунд. Запускаешь перед каждым релизом.

**Файл `tests/smoke.sh`** (готов к написанию):

```bash
#!/usr/bin/env bash
set -u
BASE="${BASE:-http://localhost:8081}"
SECRET="${INTERMASQ_SECRET:?}"
PASS="${ADMIN_PASS:-pass1234}"

PASS_CTR=0; FAIL_CTR=0
ok()   { echo "✓ $1"; PASS_CTR=$((PASS_CTR+1)); }
fail() { echo "✗ $1 — $2"; FAIL_CTR=$((FAIL_CTR+1)); }
check() { # check desc expected_status actual_status
    if [ "$2" = "$3" ]; then ok "$1"; else fail "$1" "got $3, want $2"; fi
}

# Setup
TOKEN=$(curl -s -X POST "$BASE/api/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$PASS\"}" | jq -r .token)
[ "$TOKEN" = "null" ] && { echo "Login failed"; exit 1; }

# Status
S=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/api/status")
check "GET /api/status (no auth)" 200 "$S"

# Metrics without auth
S=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/metrics")
check "GET /metrics without auth → 401" 401 "$S"

# Metrics with X-API-Key
S=$(curl -s -o /dev/null -w "%{http_code}" -H "X-API-Key: $SECRET" "$BASE/metrics")
check "GET /metrics with X-API-Key → 200" 200 "$S"

# Add host
curl -s -X POST "$BASE/api/hosts" \
    -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -d '{"mac":"aa:bb:cc:dd:ee:01","ip":"10.0.0.11","hostname":"t1","file":"/etc/dnsmasq.d/smoke.conf"}' \
    | jq -e .status >/dev/null && ok "Add host"

# Add same MAC again
S=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/api/hosts" \
    -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -d '{"mac":"aa:bb:cc:dd:ee:01","ip":"10.0.0.99","file":"/etc/dnsmasq.d/smoke.conf"}')
check "Add duplicate MAC → 409" 409 "$S"

# Path traversal
S=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/api/hosts" \
    -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -d '{"mac":"aa:bb:cc:dd:ee:02","file":"/etc/passwd"}')
check "Path traversal file=/etc/passwd" 400 "$S"
[ "$S" = "403" ] && ok "(или 403, тоже валидно)"

# ... (всего ~50 проверок из чек-листа)

echo
echo "Pass: $PASS_CTR, Fail: $FAIL_CTR"
[ "$FAIL_CTR" = 0 ] && exit 0 || exit 1
```

**Что покрыть (по разделам чек-листа):**
- 0, 1, 2 — status, setup, login (rate-limit, JWT, X-API-Key, logout)
- 3 — add/edit/delete/duplicate/CSV/move/edit (+ все bug-regression)
- 5 — A/CNAME/PTR/TXT + duplicate (A2)
- 6 — file create/delete, raw PUT, template-fill
- 7 — backup, restore, history list/diff/restore
- 9 — users CRUD
- 10 — audit entries присутствуют после действий
- 11 — metrics 4 способами auth
- 12 — reload + restart-self (с `-ci-mode=true`, чтобы не убить процесс)

**Не покрыть bash'ем** (нужен browser): разделы 8 (SSE), 13 (UI),
14 (i18n), 16 (UI-сортировка), 17 (perf) — отдельно D3/D4.

**Оценка:** 0.5–1 день на полную версию скрипта.

---

### D3. Playwright E2E для UI-багов

**Ценность:** единственный способ ловить баги типа **A1** (дубли строк),
**A5** (модалки), **A7** (templates UI). Go-тесты это не видят.

**Стек:**
- Node.js (уже есть в CI: `node:22-bookworm`)
- `@playwright/test`
- Запуск: `npx playwright test` против запущенного бинарника intermasq

**Структура:**
```
tests/e2e/
├── package.json
├── playwright.config.js
├── fixtures/
│   └── clean-state.ts       # перед каждым тестом: чистит users.json, conf-dir
└── specs/
    ├── auth.spec.js          # login, rate-limit, logout
    ├── static-hosts.spec.js  # add, edit, delete, sort (regression для A1!)
    ├── bulk-ops.spec.js      # move, edit (regression для A5)
    ├── dns-aliases.spec.js   # A/CNAME, duplicate (regression для A2)
    ├── config-editor.spec.js # file create/delete, raw edit
    ├── history.spec.js       # diff modal, restore
    ├── i18n-theme.spec.js    # RU/EN, dark/light
    └── plugins.spec.js       # discovery + iframe
```

**Самый ценный тест (regression A1):**
```javascript
test('sort by IP does not duplicate rows', async ({ page }) => {
  await page.goto('/')
  // login ...
  const rowCountBefore = await page.locator('tbody tr').count()
  await page.click('th:has-text("IP")')  // sort
  await page.click('th:has-text("IP")')  // reverse
  await page.click('th:has-text("IP")')  // again
  const rowCountAfter = await page.locator('tbody tr').count()
  expect(rowCountAfter).toBe(rowCountBefore)  // A1 regression
})
```

**Запуск в CI:** добавить stage в `.forgejo/workflows/build.yml`:
```yaml
- name: E2E tests
  run: |
    ./intermasq-ci &   # запустить в фоне с -ci-mode=true
    cd tests/e2e && npm ci && npx playwright install --with-deps
    npx playwright test
```

**Оценка:** 1.5–2 дня на 20–30 spec-тестов с базовыми page object'ами.

---

### D4. Fixture generator для performance

**Ценность:** раздел 17 чек-листа ("200 хостов"). Генерирует `.conf`
с N хостами, потом `ab`/`wrk`/`hey` на `/api/hosts`.

**Файл `tests/fixtures/gen-hosts.sh`:**
```bash
#!/usr/bin/env bash
N="${1:-200}"
OUT="${2:-/etc/dnsmasq.d/perf.conf}"
{
  echo "# generated $(date)"
  for i in $(seq 1 "$N"); do
    MAC=$(printf "02:00:00:%02x:%02x:%02x" $((i/65536%256)) $((i/256%256)) $((i%256)))
    IP="10.0.$((i/256)).$((i%256))"
    HN="host-$i"
    echo "dhcp-host=$MAC,$HN,$IP"
  done
} > "$OUT"
echo "Generated $N hosts in $OUT"
```

**Нагрузка:**
```bash
./tests/fixtures/gen-hosts.sh 200
INTERMASQ_SECRET=x ./intermasq-ci &
hey -n 1000 -c 10 -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/hosts
```

**Оценка:** 0.5 дня.

---

### D5. Container e2e (Alpine, разные init-системы)

**Ценность:** 12.4 (portability). Завести в CI отдельный job:
```yaml
test-alpine:
  container: alpine:latest
  steps:
    - run: apk add dnsmasq sudo openrc
    - run: ./intermasq -dnsmasq-bin=$(which dnsmasq) -init-system=none &
    - run: ./tests/smoke.sh
```

Вариант посложнее: rootless Podman + systemd контейнер для тестирования
`-init-system=systemd` (по плану в `планов/дальнейшее.md:21-23`).

**Оценка:** 1 день на Alpine (just works), 2-3 дня на полноценный
systemd-in-container.

---

## E. Рекомендуемый порядок работ

Не всё сразу. По убыванию ROI:

| Приоритет | Задача | Время | Ловит |
|---|---|---|---|
| **P0** | Фикс A1 (key на v-for) | 30 мин | Самый заметный пользователю баг |
| **P0** | Фикс A2 (alias duplicate) | 30 мин | Data corruption |
| **P0** | Фикс A3+A4 (MAC validation) | 1 час | dnsmasq reload failure |
| **P1** | D2 bash smoke-скрипт | 4 часа | Регрессия 80% чек-листа перед релизом |
| **P1** | Фикс A5 (bulk-edit modal UX) | 1 час | Пользовательская путаница |
| **P1** | Фикс A6 (alert count) | 30 мин | Пользовательская путаница |
| **P2** | D1 Go integration tests | 1 день | Долгосрочная защита от регрессий |
| **P2** | D4 fixture generator | 2 часа | Performance testing |
| **P2** | Фикс A8 (401 body) | 5 мин | UX оператора |
| **P2** | Фикс A10 (ARP возвращает IP) | 1 час | Discovered-devices usability |
| **P3** | D3 Playwright E2E | 2 дня | UI-регрессии |
| **P3** | D5 Alpine container test | 1 день | Portability |
| **P3** | Обновление чек-листа (после фиксов) | 1 час | A7, A11 |

---

## F. Что делать прямо сейчас

1. **Принять решения по A1–A5** (fix now / defer). Жду гринлайт.
2. **Уточнить A6** — какой режим импорта использовался (single / text
   / CSV), что за данные вставлял. Можно просто приложить скриншот
   или текст до/после.
3. **Запустить C** (path-traversal скрипт) на тестовом сервере —
   должны быть все 4xx, ни одного 200/500. Любой 200 или 500 = дырка.
4. **Решить**, кто пишет D2 (bash smoke). Готов взять на себя — это
   даст самый быстрый ROI из всего списка.

После фиксов A1–A4 + D2 — релиз v3 уже можно показывать чужим людям.
Остальное накатываем инкрементально.
