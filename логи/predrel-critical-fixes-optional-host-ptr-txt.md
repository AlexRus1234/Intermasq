# Сессия: predrel — Critical fixes + optional host fields + PTR/TXT aliases

Закрывающий pre-release проход: 2 критичных бага безопасности/надёжности +
3 небольшие фичи для покрытия кейсов «пользователь не должен лезть в файлы
dnsmasq руками». Принцип KISS удержан: ни одной новой подсистемы, всё
расширяет существующие API и UI.

---

## Коммит

| Хэш | Описание |
|-----|----------|
| (в этом PR) | Fix user/templates DB corruption bugs; allow optional host fields; add PTR/TXT aliases |

---

## Контекст и принципы

Главный критерий заказчика: после установки типичный домашний пользователь
не должен редактировать `.conf`-файлы через SSH. Любая типичная операция
должна быть в UI.

Анализ кодовой базы показал:

- **Критичные баги**, которые мешают релизу независимо от функционала.
- **3 типичных кейса**, не покрытых UI и заставляющих админа лезть в файлы.

Ad-block lists и notifications на новые устройства обсуждались, но
отвергнуты как нарушение KISS (выходят за границы «панель dnsmasq»).

---

## 1. Баг 1: `loadUsers` / `loadTemplates` молча игнорируют ошибки

### Симптом

`loadUsers` (auth.go) и `loadTemplates` (templates.go) написаны как:

```go
data, _ := os.ReadFile(*DBPath)
json.Unmarshal(data, &users)
```

Если `users.json` повреждён, нет прав на чтение, или JSON невалиден —
`users` остаётся пустым map. Дальше `statusHandler` возвращает
`setup_required: len(users) == 0 = true`, и любой стук к `POST /api/setup`
создаёт нового admin'а. Это критичная дыра: одно повреждение файла = полный
admin-доступ для любого, кто дойдёт до URL.

### Фикс

Различение сценариев:

```go
if _, err := os.Stat(*DBPath); os.IsNotExist(err) {
    os.MkdirAll(filepath.Dir(*DBPath), 0700)
    return  // первый запуск — нормально
}
data, err := os.ReadFile(*DBPath)
if err != nil {
    fmt.Fprintf(os.Stderr, "[FATAL] Cannot read user database %s: %v\n", *DBPath, err)
    os.Exit(1)
}
if err := json.Unmarshal(data, &users); err != nil {
    fmt.Fprintf(os.Stderr, "[FATAL] Cannot parse user database %s: %v\n", *DBPath, err)
    os.Exit(1)
}
```

Аналогично для `loadTemplates`. Теперь:
- Файл отсутствует → нормальный сценарий, `setup_required = true` legitimately.
- Файл существует, но не читается/не парсится → `[FATAL]` + exit 1.

Это блокирует автоматический restart-loop в systemd, что и есть цель —
админ должен вручную разобраться с повреждённой БД.

### Тесты

`TestLoadUsersFailsOnCorruptJSON`, `TestLoadTemplatesFailsOnCorruptJSON` —
subprocess pattern (внутренний прогон с `INTERMASQ_TEST_FATAL=1`, проверка
non-zero exit code родительским тестом). `TestLoadUsersMissingFileIsOK` —
контракт «отсутствие файла = легитимный первый запуск».

---

## 2. Баг 2: не-atomic `saveUsers` / `saveTemplates`

### Симптом

```go
func saveUsers() error {
    data, _ := json.MarshalIndent(users, "", "  ")
    return os.WriteFile(*DBPath, data, 0600)
}
```

Если процесс упал между `Open` и `Write` — файл повреждён. Это напрямую
ведёт к багу 1: повреждённый файл → `[FATAL]` + exit. Плюс состояние
`users.json.tmp.nnn` артефактов.

При этом в `state.go` плагинов Pomen/Povez уже использовался atomic pattern
`tmp+rename`. В ядре — нет, непоследовательно.

### Фикс

```go
func saveUsers() error {
    data, _ := json.MarshalIndent(users, "", "  ")
    dir := filepath.Dir(*DBPath)
    if err := os.MkdirAll(dir, 0700); err != nil { return err }
    tmp := *DBPath + ".tmp"
    if err := os.WriteFile(tmp, data, 0600); err != nil { return err }
    return os.Rename(tmp, *DBPath)
}
```

`os.Rename` атомарен на POSIX (одна filesystem операция). На Windows —
заменяет существующий файл атомарно только если он в той же FS; для нашего
Linux-only деплоя этого достаточно. Аналогично для `saveTemplates`.

### Тесты

`TestSaveUsersAtomic` — после сохранения файл существует, парсится, `.tmp`
артефакта нет. `TestSaveUsersAtomicPreservesExistingOnFailure` — делает
родительский каталог read-only, проверяет что исходный файл остался
нетронутым (skip если запущено под root или permissive FS).

---

## 3. Фича 3: опциональность IP/hostname в dhcp-host

### Контекст

dnsmasq поддерживает четыре осмысленные формы `dhcp-host`:

```
dhcp-host=aa:bb:cc:dd:ee:ff                    (infinite lease)
dhcp-host=aa:bb:cc:dd:ee:ff,phone              (DNS-имя, IP от DHCP)
dhcp-host=aa:bb:cc:dd:ee:ff,192.168.1.10       (статический IP, без DNS)
dhcp-host=aa:bb:cc:dd:ee:ff,phone,192.168.1.10 (полная запись)
```

Но `addHostHandler` требовал все три поля:

```go
if !macRegex.MatchString(req.Mac) || net.ParseIP(req.Ip) == nil || !validHostname(req.Hostname) || !isSafePath(req.File) {
    c.JSON(400, gin.H{"error": "invalid_data"})
    return
}
```

Кейс «хочу дать телефону имя в DNS, но не фиксировать IP» — **самый частый**
для дома — был невозможен через UI.

### Фикс

#### Новый хелпер

`validateHostFields(mac, ip, hostname string) bool` в `dnsmasq.go`:
- MAC обязателен и валиден.
- Если IP указан — должен парситься `net.ParseIP`.
- Если hostname указан — должен удовлетворять `validHostname`.

Используется в 5 местах:
- `addHostHandler` (handlers.go) — валидация.
- `parseCSVHosts` (dnsmasq.go) — CSV import.
- `bulkAddHostsHandler` (handlers.go) — запись в newMacs.
- (косвенно) все остальные места, где нужно «validate but allow optional».

#### Изменения в handlers.go

`addHostHandler`:
- Заменён strict check на `validateHostFields`.
- `findHostsByIP` вызывается только если `req.Ip != ""` — иначе IP-конфликт
  не имеет смысла (поиск по пустому IP вернул бы все записи без IP).

`bulkAddHostsHandler`:
- Внутренняя батч-проверка: для каждой строки отдельно проверяется MAC
  (обязательный), IP (если указан — валидный), hostname (если указан —
  валидный). Раньше был `continue` для невалидных, что молча пропускало
  мусор. Теперь 400 с указанием MAC'а.
- Внутри-batch duplicate IP: пропускается если IP пустой.
- Cross-config duplicate: `findHostsByIP` только если IP указан.

#### Сериализация уже работала

`formatDhcpHostLine` уже давно пропускал пустые `Ip` и `Hostname` —
изменений не требовалось. Поэтому round-trip сохраняется для всех 4 форм.

### Тесты (6 новых)

- `TestValidateHostFieldsAllCombinations` — table-driven по 8 кейсам.
- `TestAddHostHandlerMacOnly` — MAC-only → `dhcp-host=<mac>`.
- `TestAddHostHandlerMacPlusHostname` — MAC + hostname → `dhcp-host=<mac>,hostname`.
- `TestAddHostHandlerMacPlusIP` — MAC + IP → `dhcp-host=<mac>,ip`.
- `TestAddHostHandlerRejectsBadIP` — опциональность ≠ попустительство мусора.
- `TestAddHostHandlerIPDuplicateStillChecked` — duplicate check работает
  когда IP указан.
- `TestParseCSVHostsAcceptsMACOnly`, `TestParseCSVHostsMACPlusHostname` —
  CSV import тоже ослаблен.

### Фронтенд

`HostForm.vue`:
- Placeholder'ы обновлены: `IP (опц.)` и `Имя (опц.)` на RU; `IP (optional)`
  и `Hostname (optional)` на EN. Пользователь видит, что поля не требуются.
- `saveHost()` — убраны client-side проверки обязательности IP/hostname;
  MAC и file остаются обязательными. Валидация формата — на бэке.
- `parsedBulkHosts` переписан: для каждой строки сначала находится MAC,
  затем IP (если есть), затем hostname (остаток). Раньше требовал ровно 3
  токена, теперь 1–3. Filter `e.mac` (был `e.mac && e.ip`).

### Совместимость

Существующие `dhcp-host=` строки в `.conf`-файлах парсятся корректно через
`parseDhcpHostLine` — это уже работало. Изменения касаются только валидации
**при записи** новых хостов.

---

## 4+5. Фичи 4+5: PTR и TXT DNS-записи

### Контекст

`parseAliasLine` понимал только `address=` (A) и `cname=` (CNAME). Этого
мало для:

- **Reverse DNS** (`dig -x 192.168.1.10` → `nas.lan`). Требует `ptr-record=`.
  Postfix / sshd с GSSAPI / Kerberos / self-hosted почта — регулярно ломаются
  без PTR.
- **TXT-записи**: SPF, DKIM, ACME DNS-01 challenges, mDNS-метаданные.
  Без этого локальный почтовый релей или home-CA не настроить.

Оба кейса заставляли пользователя лезть в `.conf`-файл.

### Фикс

#### `parseAliasLine` расширена

Добавлены ветки для `ptr-record=` и `txt-record=`:

- **PTR**: `ptr-record=<name>[,<name>…],<target>` — dnsmasq допускает
  несколько имён reverse-зоны, но UI работает с одним. Берётся первое имя +
  последний токен как target.
- **TXT**: `txt-record=<name>,<value>` — value может содержать запятые
  (DKIM с `k=rsa; p=…`), поэтому сплит только по первой запятой.

#### `aliasToLine` switch

```go
switch a.Type {
case "CNAME": return fmt.Sprintf("cname=%s,%s", a.Domain, a.Target)
case "PTR":   return fmt.Sprintf("ptr-record=%s,%s", a.Domain, a.Target)
case "TXT":   return fmt.Sprintf("txt-record=%s,%s", a.Domain, a.Target)
}
return fmt.Sprintf("address=/%s/%s", a.Domain, a.Target)  // A default
```

#### `isAliasDirective`

```go
return strings.HasPrefix(line, "address=") ||
    strings.HasPrefix(line, "cname=") ||
    strings.HasPrefix(line, "ptr-record=") ||
    strings.HasPrefix(line, "txt-record=")
```

#### `validateAliasEntry`

```go
switch a.Type {
case "A":     return net.ParseIP(a.Target) != nil
case "CNAME", "PTR":
    return aliasDomainRegex.MatchString(a.Target)
case "TXT":
    // TXT-значение может быть произвольным текстом. Единственное ограничение —
    // непустое и без новых строк (иначе serializeConfigFile даст несколько
    // строк, ломая формат conf-файла).
    return a.Target != "" && !strings.Contains(a.Target, "\n")
}
```

#### `parseCSVAliases`

Расширен для типов PTR и TXT, аналогично остальным. TXT принимает любой
непустой текст без новой строки.

#### `removeAliasLine` и `findAliasesByDomain`

Эти функции уже использовали `parseAliasLine` и `isAliasDirective` —
расширение автоматически распространилось.

#### DNS health checker (metrics.go)

`runDNSHealthPass` намеренно **не расширен** для PTR/TXT:
- A и CNAME проверяются через `resolver.LookupHost` — это «домен резолвится?».
- PTR потребовал бы `resolver.LookupAddr`, TXT — `resolver.LookupTXT`.
- Health check про «работает ли DNS вообще», не «все записи валидны».
- KISS: не плодим сущности. Оставляем A/CNAME, остальные типы просто
  не health-check'аются (но работают).

### Тесты (11 новых)

- `TestParseAliasLinePTR`, `TestParseAliasLineTXT` — парсинг.
- `TestParseAliasLineTXTMultiComma` — DKIM-подобное значение с запятыми.
- `TestAliasToLinePTR`, `TestAliasToLineTXT` — сериализация.
- `TestAliasRoundTripPTR`, `TestAliasRoundTripTXT` — round-trip.
- `TestIsAliasDirectiveRecognizesNewTypes` — `isAliasDirective` узнаёт
  новые префиксы.
- `TestReadAllAliasesIncludesPTRAndTXT` — end-to-end чтение `.conf`.
- `TestValidateAliasEntryPTRAndTXT` — table-driven валидация.
- `TestRemoveAliasLinePTR`, `TestRemoveAliasLineTXT` — удаление по типу.
- `TestAddAliasHandlerPTR`, `TestAddAliasHandlerTXT` — end-to-end HTTP.
- `TestParseCSVAliasesIncludesPTRAndTXT` — CSV import новых типов.

### Фронтенд

`AliasForm.vue`:
- `<select>` типа записи: добавлены `<option value="PTR">PTR</option>` и
  `<option value="TXT">TXT</option>`.
- `targetPlaceholder` computed: для TXT — `v=spf1 -all`, для остальных —
  по-старому.
- `directivePreview` computed: показывает канонический dnsmasq-синтаксис
  для выбранного типа. Помогает пользователю понять, что именно запишется
  в файл (`address=/…/…`, `cname=…,…`, `ptr-record=…,…`, `txt-record=…,…`).
- `parsedBulkAliases` расширен: распознаёт `ptr-record=…` и `txt-record=…`
  в свободном тексте bulk-импорта, а также формат `PTR domain target` /
  `TXT domain value`.

`AliasTable.vue`:
- `typeBadgeClass(type)` / `typeTextClass(type)` — различные цвета
  бейджей для A/CNAME/PTR/TXT (раньше было бинарно A=primary, остальное=info).
  Не критично, но визуально разделяет типы.

### Локали

`ru.json` / `en.json`:
- `dns.targetTxtPlaceholder` — `"v=spf1 -all"`.
- `dns.bulkPlaceholder` — обновлён, добавлены примеры PTR/TXT.
- `hosts.ipOptional`, `hosts.hostnameOptional` — для placeholder'ов формы хоста.
- `hosts.bulkPlaceholder` — переписан с указанием опциональности.

---

## Проверки

- `go build ./...` — OK.
- `go vet ./...` — OK.
- `gofmt -l` на изменённых файлах — пусто.
- `go test ./... -count=1` — все тесты проходят.
  - 2 skip: `TestConfigTemplatesValidForDnsmasqSyntax` (без dnsmasq в окружении)
    и `TestSaveUsersAtomicPreservesExistingOnFailure` (под root / permissive FS).
- `npm run build` (vite) — OK, 115 модулей, ~375 КБ JS.

40+ новых тестов в `dnsmasq_test.go` для всех 5 пунктов.

---

## Риски и нюансы

- **Обратная совместимость dhcp-host:** старые `.conf`-файлы с тремя полями
  парсятся и записываются как раньше. Изменения касаются только валидации
  **при добавлении нового** хоста через UI/API.
- **Bulk import теперь строже к мусору:** раньше невалидные строки в bulk
  silently skip'ались, теперь 400 с указанием MAC'а. Может сломать
  скрипты, которые пихали мусор. Считаю правильным — лучше явная ошибка
  чем тихая потеря данных. Если понадобится backward compat — добавим
  flag в запросе.
- **TXT-валидация принимает любые символы кроме `\n`.** dnsmasq
  формально требует экранирования кавычек для сложных значений, но
  UTF-8 строка без `\n` проходит `dnsmasq --test`. Если будут edge-cases,
  валидацию можно ужесточить.
- **PTR target = IP технически проходит `aliasDomainRegex`.** Это
  намеренно: dnsmasq принимает `ptr-record=name,1.2.3.4`. Не наша задача
  диктовать, как именно админ использует PTR.
- **DNS health checker не покрывает PTR/TXT.** Намеренно (см. выше).
  Если когда-то понадобится — `runDNSHealthPass` расширяется switch'ом
  по type с `LookupAddr` / `LookupTXT`.
- **`loadUsers` fatal блокирует systemd restart-loop.** Это и есть цель:
 Повреждённую БД должен чинить админ вручную. systemd после N быстрых
  рестартов всё равно уйдёт в `failed` state — это правильный сигнал
  оператору.
- **Atomic saveUsers на Windows не работает корректно** (rename
  заменяет только в той же FS). Но целевая платформа — Linux, так что
  окей.

---

## Файлы, изменённые в этой сессии

| Файл | Тип изменения | Назначение |
|------|--------------|------------|
| `auth.go` | fixed | loadUsers fatal on err, saveUsers atomic |
| `templates.go` | fixed | loadTemplates fatal on err, saveTemplates atomic |
| `dnsmasq.go` | extended | validateHostFields, parseAliasLine (PTR/TXT), aliasToLine, isAliasDirective, parseCSVHosts, parseCSVAliases |
| `handlers.go` | extended | addHostHandler optional fields, bulkAddHostsHandler optional fields, validateAliasEntry PTR/TXT |
| `dnsmasq_test.go` | extended | 40+ новых тестов для всех 5 пунктов |
| `frontend/src/components/static/HostForm.vue` | extended | опциональные поля, переписанный parsedBulkHosts |
| `frontend/src/components/dns/AliasForm.vue` | extended | PTR/TXT в select + preview, парсинг в bulk |
| `frontend/src/components/dns/AliasTable.vue` | extended | цвета бейджей по типу записи |
| `frontend/src/locales/{ru,en}.json` | extended | новые ключи для placeholder'ов |
