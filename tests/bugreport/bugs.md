# Intermasq — баг-репорт v1

Полный список известных багов. Каждый баг имеет:
- **ID** (A1-A13)
- **severity** (CRITICAL / HIGH / MEDIUM / LOW / FEATURE)
- **status** (OPEN / FIXED / WONTFIX)
- файлы для починки
- regression-тест в `tests/smoke.sh` (если применимо)

Источник `tests/known-bugs.txt` синхронизирован с этим файлом — если
добавляешь/удаляешь баг там, обновляй и здесь.

---

## Сводка

| ID | Severity | Component | Status | Regression test |
|---|---|---|---|---|
| A1 | CRITICAL | frontend (HostTable.vue) | FIXED | Playwright `hosts-sort.spec.ts` (guard) |
| A2 | CRITICAL | backend (aliases.go) | FIXED | smoke.sh: `A2: duplicate A same file → 409` |
| A3 | HIGH | backend (main.go macRegex) | FIXED | smoke.sh: `A3: zero MAC rejected` |
| A4 | HIGH | backend (validation) | FIXED | smoke.sh: `A4: dash-MAC handled` |
| A5 | HIGH | frontend (BulkEditModal.vue) | FIXED | был Playwright `test.fail` (Блок A), `.fail` снят |
| A6 | MEDIUM | backend (handlers_hosts.go) | FIXED | smoke.sh: `Bulk JSON response has count field` |
| A7 | MEDIUM | frontend (TemplatesModal.vue) | OPEN | UI проверен вручную, не баг |
| A8 | MEDIUM | backend (metrics.go) | FIXED | smoke.sh: `A8: 401 has body` |
| A10 | LOW | backend (arp_leases.go) | OPEN | feature gap, не regression test |
| A11 | LOW | security (handlers_*.go) | FIXED | smoke.sh: path traversal battery; L2 `TestGetFileHandlerRejectsUnsafePath` / `TestPutFileHandlerRejectsUnsafePath` |
| A12 | HIGH | backend (main.go aliasDomainRegex) | FIXED | smoke.sh: `A12: Add TXT with underscore domain` |
| A13 | HIGH | backend (dnsmasq.go writeFileRaw) | FIXED | smoke.sh: `PUT with invalid dnsmasq syntax → 400` (стал честным) |

**Итого:** 7 из 9 багов закрыты в Bugfix sweep (2026-07-28): A1, A2, A3, A4,
A6, A8, A12 → FIXED. Ранее A5 + A13 уже закрыты (Блок A). A11 закрыт в
Hardening sweep (2026-07-29) как defense-in-depth. Остаются: A7 — не
баг (UI-layout), A10 — feature gap (отдельный PR). Все smoke-tagged
баги убраны из `tests/known-bugs.txt` → smoke.sh ожидаемо 0 Fail / 0
Known-fail. Regression-тесты добавлены в `dnsmasq_test.go` и
`handlers_test.go`; A1 покрыт существующим Playwright guard
`hosts-sort.spec.ts`. Логи сессий: `логи/bugfix-sweep.md`,
`логи/hardening-sweep.md`.

---

## A1 — Дублирование строк таблицы при сортировке

> **Status: FIXED** (Bugfix sweep, 2026-07-28). Минимальный фикс:
> `:key="h.mac + '|' + (h.file||'')"` в `HostTable.vue:27`. Ключ теперь
> уникален: `h.file` различается между `.conf`-файлами, а суффикс `|has_bak`
> (из `getHostsHandler`) делает уникальными bak/non-bak варианты одного MAC.
> Опциональный `has_bak`-рефакторинг сознательно не сделан (вне объёма фикса).
> Regression: существующий Playwright `hosts-sort.spec.ts` (guard — count строк
> стабилен при сортировке) остаётся зелёным.

**Severity:** CRITICAL
**Component:** `frontend/src/components/static/HostTable.vue:27`

**Симптом:** при клике на заголовок MAC/IP/Hostname количество строк в
таблице кратно растёт (2 → 4 → 8 → 16). После F5 сбрасывается.

**Корень:**
```vue
<tr v-for="h in sortedHosts" :key="h.mac">
```

Ключ `:key="h.mac"` **не уникален**, потому что:
1. Тот же MAC может быть записан в нескольких `.conf` файлах
2. `getHostsHandler` в `handlers_hosts.go:48` мутирует `entry.File`
   через `|has_bak`-суффикс, плодя дубликаты с разных итогов

Vue требует уникальных ключей в `v-for`. При неуникальных ключах
reconciliation падает в undefined behavior — дубль DOM при каждой
перерисовке.

**Фикс:**
```vue
<tr v-for="h in sortedHosts" :key="h.mac + '|' + (h.file||'')">
```

Дополнительно: `getHostsHandler` мутирует `entry.File` через `|has_bak`-
суффикс — анти-паттерн (пачкать данные маркером состояния). Лучше
вернуть отдельное поле `has_bak bool` в `HostEntry`, а в UI показывать
индикатором. Это автоматически исправит и баг с ключом.

**Regression test:** Нужен Playwright — кликнуть на сортировку 3 раза,
проверить что count строк не изменился.

---

## A2 — Дубликаты DNS-alias можно добавлять

> **Status: FIXED** (Bugfix sweep, 2026-07-28). `addAliasHandler` теперь
> вызывает `findAliasesByDomain(domain, "", "")` без exclude — существующая
> запись в том же файле корректно считается конфликтом → 409. Knock-on:
> second-delete в `32-aliases-delete.sh` теперь честно возвращает 404.
> Regression: `TestAddAliasHandlerDuplicateRejected` +
> `TestDeleteAliasHandlerSecondDeleteNotFound` в `dnsmasq_test.go`.

**Severity:** CRITICAL
**Component:** `handlers_aliases.go:76` + `aliases.go:176-189`

**Симптом:** "Duplicate domain+type → можно добавить полную копию"

**Корень:**
```go
conflicts := findAliasesByDomain(req.Domain, req.Type, req.File)
```

`findAliasesByDomain` **исключает** запись с matching `type+file`:
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
без exclude. Update-flow в коде пока нет, так что exclude бесполезен
и его можно просто убрать. Проверить `bulkAddAliasesHandler:139` и
`importAliasesCSVHandler:287` — там уже вызывается правильно.

**Knock-on эффект:** smoke test `Delete alias again → 404` сейчас падает
как KNOWN(A2) — после фикса A2 second delete правильно вернёт 404.

**Regression test:** `tests/smoke.sh` — `A2: duplicate A same file → 409`.

---

## A3 — MAC `00:00:00:00:00:00` принимается

> **Status: FIXED** (Bugfix sweep, 2026-07-28). `validateHostFields` теперь
> отвергает zero/broadcast MAC через `strings.EqualFold` blacklist (после
> нормализации). Regression: cases в `TestValidateHostFieldsAllCombinations` +
> e2e `TestAddHostHandlerRejectsZeroBroadcastMAC`; smoke `A3: zero/broadcast MAC
> rejected → 400` зелёные.

**Severity:** HIGH
**Component:** `main.go:78`

**Симптом:** zero-MAC и broadcast-MAC сохраняются без ошибки.

**Корень:**
```go
macRegex = regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`)
```

Регекс пропускает broadcast/zero MAC. dnsmasq на релоаде такое либо
режет, либо (что хуже) тихо принимает и ломает DHCP.

**Фикс:** добавить чёрный список MAC в `validateHostFields`:
```go
if strings.EqualFold(mac, "00:00:00:00:00:00") ||
   strings.EqualFold(mac, "ff:ff:ff:ff:ff:ff") {
    return false
}
```

**Regression test:** `tests/smoke.sh` — `A3: zero MAC rejected` и
`A3: broadcast MAC rejected`.

---

## A4 — MAC с `-` сохраняется, dnsmasq потом падает

> **Status: FIXED** (Bugfix sweep, 2026-07-28). Добавлен `normalizeMAC()`
> (`-`→`:`), вызывается на входе `addHostHandler`, `bulkAddHostsHandler`,
> `parseCSVHosts` и дефенсивно внутри `validateHostFields`. Записанный файл
> всегда содержит canonical colon-форму. Regression: `TestNormalizeMAC`,
> `TestAddHostHandlerDashMACNormalized`, `TestParseCSVHostsNormalizesDashMAC`
> + table-cases; smoke `A4: dash-MAC normalised → 200` (+ file colon-form
> check) зелёный.

**Severity:** HIGH
**Component:** `main.go:78`, `dnsmasq.go:149`

**Симптом:** `dhcp-host=aa-bb-cc-dd-ee-02,...` сохраняется → dnsmasq
`--test` падает на reload → вся кнопка "Применить" не работает.

**Корень:** `macRegex` принимает и `:`, и `-`. `formatDhcpHostLine`
пишет MAC as-is. dnsmasq принимает только `:` (man 6 bytes separated
by colons).

**Фикс:** нормализовать MAC к `:`-форме в `validateHostFields` (или в
точке входа — `addHostHandler`, `bulkAddHostsHandler`,
`importCSVHandler`). Заодно это улучшит консистентность `findHostsByMac`.

```go
mac = strings.ReplaceAll(mac, "-", ":")
```

**Regression test:** `tests/smoke.sh` — `A4: dash-MAC handled`.

---

## A5 — Bulk-edit: модалка либо не реагирует, либо no_hosts

> **Status: FIXED** (Блок A, коммит `7cd0e1d`, 2026-07-26). Фактический root
> cause оказался **другим** — см. ниже «Фактический фикс». Гипотезы ниже
> (HostTable watcher / tooltip) НЕ понадобились; они оставлены как исходный
> контекст симптома. Лог: `логи/gap2-blockA-a5a13-fixes.md`.

**Severity:** HIGH
**Component:** `frontend/src/components/static/HostTable.vue:4`, `BulkEditModal.vue:1`

**Симптом:**
- "edit - ничего не происходит" с выбранными чекбоксами
- "стоит мне убрать все галочки и появляются поля для заполнения, но
  там я постоянно получаю nohost"

**Корень:**
1. `HostTable.vue:4` — панель действий показывается только
   `v-if="selectedHosts.length > 0"`. Убрал все галочки → панель
   пропала. Если `BulkEditModal` остался открытым при изменении
   `selectedHosts`, он остаётся с `props.hosts=[]` → сабмит →
   `no_hosts` от бэка.
2. `BulkEditModal.vue:88` `canSubmit` требует хотя бы одного из полей
   IP-transform или hostname-transform. Если оставить все пустым —
   кнопка disabled, визуально никакой обратной связи нет.

**Фикс:**
- В `HostTable.vue` добавить `watch(selectedHosts)` → если пусто,
  форсировать `showEdit=false; showMove=false`.
- В `BulkEditModal.vue` добавить tooltip на disabled-кнопке:
  `:title="!canSubmit ? 'Заполните хотя бы одно поле' : ''"`.

**Regression test:** Нужен Playwright — открыть модалку, снять чекбоксы,
убедиться модалка закрылась.

**Фактический фикс (Блок A, `7cd0e1d`):** L4-репродюсер `bulk-ops.spec`
показал, что модалка падала раньше, чем доходило до `no_hosts` — TypeError в
`preview` computed: `BulkEditModal.vue:67` звал `store_hosts.find(...)`, а
`store_hosts` это reactive-объект `store` (без `.find`). Фикс = 1 строка:
`store_hosts.hosts.find(...)`. `test.fail` снят, тест зелёный. Гипотезы
выше (watcher/tooltip) не применялись — корень был проще.

---

## A6 — Bulk-import показывает "импортировано 0" / JSON без count

> **Status: FIXED** (Bugfix sweep, 2026-07-28). `bulkAddHostsHandler` теперь
> возвращает `{"status":"ok","count":N}`, инконсистентность с CSV-путём устранена.
> Regression: augmented `TestBulkAddHostsHandler_Success` (assert `.count == N`).

**Severity:** MEDIUM
**Component:** `handlers_hosts.go:269` (`bulkAddHostsHandler`)

**Симптом:** `POST /api/hosts/bulk` возвращает `{"status":"ok"}` без
`count` поля. CSV-путь возвращает count корректно.

**Корень:** инконсистентность между `bulkAddHostsHandler` (нет count) и
`importCSVHandler` (есть count). UI для JSON-mode импорта использует
клиентский count (`parsedBulkHosts.length`), но alert менее понятен.

**Фикс:** добавить `"count": len(hosts)` в ответ `bulkAddHostsHandler`:
```go
c.JSON(200, gin.H{"status": "ok", "count": len(req.Hosts)})
```

**Regression test:** `tests/smoke.sh` — `Bulk JSON response has count field`.

---

## A7 — Templates UI не соответствует чек-листу

**Severity:** MEDIUM (не баг, UI-layout)
**Component:** `frontend/src/components/static/TemplatesModal.vue:27-39`

**Симптом пользователя:** "У меня вообще по-другому колонки: name,
вторая выпадающая пустая, третья device-{NNN}, четвертая путь..."

**Анализ:** это **не баг**, это сам UI. `TemplatesModal.vue:27-39`:
- col: `form.name`
- col: select для `form.ip_range` (становится выпадашкой при наличии
  dhcp-ranges)
- col: `form.hostname_pattern` с placeholder `device-{NNN}`
- col: `form.target_file` с placeholder `/etc/dnsmasq.d/hosts.conf`

**Фикс (optional):** переработать UI в 2 колонки вместо 4, подписать
поля явно (не placeholder'ами).

**Regression test:** Не нужен — UI-layout вопрос.

---

## A8 — /metrics на `curl` без `-i` выглядит "пустым"

> **Status: FIXED** (Bugfix sweep, 2026-07-28). `metricsHandler` теперь
> вызывает `c.AbortWithStatusJSON(401, gin.H{"error": "auth_required"})` вместо
> bare `AbortWithStatus(401)`. Regression: augmented `TestMetricsHandler_NoAuth_401`
> в `handlers_test.go` (assert non-empty body + "auth_required"); smoke `A8: 401
> has body` зелёный.

**Severity:** MEDIUM (UX)
**Component:** `metrics.go:60`

**Симптом:**
```bash
$ curl http://localhost:8081/metrics
$
```

**Корень:** `metricsHandler` без auth делает
`c.AbortWithStatus(401)` — без body. curl по умолчанию не печатает
статус-код и не считает 4xx ошибкой.

**Фикс:**
```go
c.AbortWithStatusJSON(401, gin.H{"error": "auth_required"})
```

Поможет и Prometheus-у давать более понятный `last_error` в UI.

**Regression test:** `tests/smoke.sh` — `A8: 401 has body`.

---

## A10 — Discovered-devices не показывает IP

**Severity:** LOW (feature gap)
**Component:** `arp_leases.go:43` (`parseArpContent`)

**Симптом:** Discovery показывает MAC и vendor, но не IP. Пользователю
непонятно "что это за устройство".

**Корень:** `parseArpContent` хранит только `map[mac]bool`, IP
выбрасывает:
```go
activeMacs[strings.ToLower(fields[3])] = true
```

OUI-vendor работает только для зарегистрированных вендоров — для
рандомизированных MAC (телефоны) не покажет ничего.

**Фикс:** расширить `parseArpContent` → возвращать
`map[mac]struct{IP, Mask}`. UI покажет IP рядом с MAC — сразу понятно,
где оно в сети. Заодно `ArpTab.vue` сможет сортировать/фильтровать по IP.

**Regression test:** smoke.sh можно расширить — проверить что
`/api/arp` возвращает IP-адреса.

---

## A11 — Path-traversal battery

> **Status: FIXED** (Hardening sweep, 2026-07-29). Defense-in-depth: в
> `getFileHandler` и `putFileHandler` (`handlers_config.go`) добавлен вызов
> `isSafePath(path)` после `filepath.Join(*ConfigDir, name)` — те же 403 +
> `access_denied`, что и substring-фильтр. Поведение эндпоинтов не изменилось
> (все 9 smoke-векторов в `81-path-traversal.sh` остаются с теми же
> статусами); substring-фильтр на `/`/`\` сегодня срабатывает первым для
> любого достижимого через URL traversal-входа, а `isSafePath`-после-Join —
> страховочный слой на случай будущего ослабления фильтра или нового call
> site'а. Теперь оба хендлера повторяют единый chokepoint-паттерн остальных
> 22 call site'ов. Regression: `TestGetFileHandlerRejectsUnsafePath`
> (`dnsmasq_test.go`), `TestPutFileHandlerRejectsUnsafePath`
> (`handlers_test.go`). A11 удалён из `tests/known-bugs.txt`. Лог:
> `логи/hardening-sweep.md`.

**Severity:** LOW (большинство векторов закрыто)

Большинство path-traversal векторов **закрыто** корректно через
`isSafePath`. smoke.sh проверяет 8 разных:
- POST /api/hosts с file=/etc/passwd → 400 ✓
- DELETE /api/hosts/:mac?file=/etc/passwd → 400 ✓
- POST /api/aliases с file=../../../tmp/x.conf → 403 ✓
- GET /api/files/<traversal> → 404 (Go HTTP чистит путь до router)
- PUT /api/files/passwd (non-.conf) → 403 ✓
- POST /api/history/restore с file=/etc/hosts → 400 ✓
- GET /api/history?file=/etc/shadow → 400 ✓
- Hostname с newline → 400 ✓

**Известная особенность:** Go's `net/http` server автоматически чистит
пути с `..` до того, как маршрут матчится в Gin. Так
`/api/files/../../etc/passwd` становится `/etc/passwd` и не доходит до
`getFileHandler`. Это **defence in depth**, а не замена явному чеку.

**Regression test:** `tests/smoke.sh` — path traversal секция.

---

## A12 — aliasDomainRegex отвергает подчёркивание

> **Status: FIXED** (Bugfix sweep, 2026-07-28). Регекс ослаблен до
> `^[a-zA-Z0-9_]([a-zA-Z0-9-._]*[a-zA-Z0-9_])?$` — теперь принимает `_dmarc.local`,
> `_sip._tcp`, DKIM/ACME challenge names. Regression: `TestAliasDomainRegexUnderscore`
> в `dnsmasq_test.go`; smoke `A12: Add TXT with underscore domain → 200` зелёный.

**Severity:** HIGH
**Component:** `main.go:80`

**Симптом:** `POST /api/aliases {"domain":"_dmarc.local",...}` возвращает
400. То же для `_sip._tcp`, DKIM/ACME challenge names.

**Корень:**
```go
aliasDomainRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-.]*[a-zA-Z0-9])?$`)
```

Регекс требует alphanumeric первый символ. `_` не входит. DNS RFC
позволяет underscore в owner names для SRV/TXT/etc (особо актуально
для DMARC, DKIM, ACME DNS-01 challenges).

**Фикс:**
```go
aliasDomainRegex = regexp.MustCompile(`^[a-zA-Z0-9_]([a-zA-Z0-9-._]*[a-zA-Z0-9_])?$`)
```

Или лучше — полная per-type валидация (A/CNAME строже, TXT/PTR свободнее).

**Regression test:** `tests/smoke.sh` — `A12: Add TXT with underscore domain`.

---

## A13 — writeFileRaw dnsmasq --test не тестит записанный файл

> **Status: FIXED** (Блок A, коммит `7cd0e1d`, 2026-07-26). Применён вариант
> с `--conf-file=<path>` (канонический паттерн из `dnsmasq_test.go:1882`) в
> `writeFileRaw`, `writeConfigWithTest`, `restoreHistoryVersion`. A13 убран из
> `known-bugs.txt`; smoke-чек стал честным 400. Лог: `логи/gap2-blockA-a5a13-fixes.md`.
> `reloadDnsmasq` (sse.go) и `restoreBackupZip` (backup.go) намеренно оставлены
> с bare `--test` — отдельная задача (см. `логи/gap2-finish.md`).

**Severity:** HIGH
**Component:** `dnsmasq.go:65-80` (`writeFileRaw`, `writeConfigWithTest`)

**Симптом:** записываешь через `PUT /api/files/30-test.conf` невалидный
синтаксис (`port=abc`), dnsmasq --test пропускает, файл сохраняется,
на следующем reload'е dnsmasq падает.

**Корень:**
```go
testCmd := exec.Command(dnsmasqBin(), "--test")
```

Без аргументов dnsmasq тестирует **свой default config** (обычно
`/etc/dnsmasq.conf` + `conf-dir=/etc/dnsmasq.d`). Файл по пути
`path` (который мы только что записали, например `/tmp/conf/...` в CI
или `/etc/dnsmasq.d/...` в проде) **не включается** в тест.

В проде это обычно работает потому что `/etc/dnsmasq.d/` входить в
`conf-dir` по умолчанию. В CI с `-conf-dir /tmp/conf` — нет, потому
что intermasq-флаг не передаётся в dnsmasq.

**Фикс:**
```go
testCmd := exec.Command(dnsmasqBin(), "--test", "--conf-dir="+filepath.Dir(path))
// или:
testCmd := exec.Command(dnsmasqBin(), "--test", "--conf-file="+path)
```

`--conf-dir` предпочтительнее (тестирует и взаимодействие нескольких
файлов). Если в каталоге конфликты между файлами — `--test` поймает.

**Regression test:** `tests/smoke.sh` — `A13: PUT with invalid dnsmasq syntax`.

---

## Приоритеты починки

> **Bugfix sweep (2026-07-28):** A1, A2, A3, A4, A6, A8, A12 закрыты за одну
> сессию с regression-тестами. См. `логи/bugfix-sweep.md`. Оригинальная
> приоритизация ниже сохранена как историческая справка.

По убыванию ROI (impact × лёгкость фикса):

1. **A1** — 30 минут фикса, самый заметный пользователю баг
2. **A2** — 30 минут, data corruption
3. **A3 + A4** — 1 час (оба в MAC validation), dnsmasq reload failure
4. **A12** — 5 минут (regex tweak), ломает DMARC/DKIM
5. **A13** — 10 минут (one arg to dnsmasq --test), validation hole
6. **A6** — 30 минут (consistency)
7. **A8** — 5 минут (UX)
8. **A5** — 1 час (UI polish)
9. **A10** — 1 час (feature gap, separate PR)

После A1-A4 + A12-A13 проект можно показывать чужим людям без стыда.
A5-A10 — polish для v1.0 release.
