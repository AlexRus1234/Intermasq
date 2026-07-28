# Bugfix sweep — промт на закрытие известных багов

**Назначение:** самодостаточный промт для ИИ-ассистента (или будущего себя),
чтобы закрыть открытые баги за одну сессию без двойных толкований. Это
продолжение закрытого L4-промта (Gap 2 Playwright финал — работа завершена,
промт удалён); итог L4 — в `логи/gap2-finish.md`. Специфика/история багов — в
`tests/bugreport/bugs.md`; ИСПОЛНЯЕМЫЙ промт — здесь. Прочитай ЦЕЛИКОМ
перед стартом; не импровизируй там, где даны конкретные файлы/фиксы.

---

## 0. Что это за проект и где мы

**Intermasq** — веб-панель для dnsmasq. Backend Go (gin), frontend Vue 3 +
Bootstrap 5, embed фронтенда через `go:embed`. Репо:
`B:\Repo\Intermasq\Intermasq`, ветка `main`. CI — Forgejo Actions, контейнер
`fedora:44`, runs as root, npm/go/rpm через внутренний прокси Nora.

**Состояние тестов на старте:**
- L1+L2 Go: 65.6% coverage (`go test -cover ./...`).
- L3 smoke.sh: ~138 проверок, **9 known-fail** (A2, A3, A4, A6, A8, A11, A12).
- L4 Playwright: 33 spec'а (31 pass + 2 skip) — **ФИНАЛ**, mutation-pass пройден.
- `tests/known-bugs.txt`: A2, A3, A4, A6, A8, A11, A12 (7 ID). A5 и A13 уже
  FIXED и убраны (Блок A, `логи/gap2-blockA-a5a13-fixes.md`).

**Цель сессии:** починить открытые баги с regression-тестами
(A1, A2, A3, A4, A6, A8, A12; A11 — опционально), убрать их из known-bugs,
довести smoke до **0 Fail / 0 Known-fail**.

---

## 1. ЖЁСТКИЕ ограничения (не нарушать)

1. **Перед пушем ЛЮБОЙ Go-правки — `go vet ./...` ОБЯЗАТЕЛЬНО** (не только
   `go build`). CI гоняет vet и режет unreachable code и др.; локальный
   `go build` это не ловит (уже был красный прогон из-за ранних `return`).
2. **smoke.sh known-bugs-механика:** ID в `tests/known-bugs.txt` = «ожидается
   FAIL». Починил баг → **удали ID из known-bugs.txt** → соответствующий check
   начинает ожидать успех. check уже кодирует ИСПРАВЛЕННОЕ поведение (400/409/
   count/body), просто был tagged known-fail → после снятия ID должен стать
   зелёным. Если не стал — правь check.
3. **Синхронизируй `tests/bugreport/bugs.md`:** статус OPEN → FIXED + коммит.
   Сводка-таблица и «Итого» — тоже.
4. **Не ломай существующие тесты.** После каждого бага локально:
   `go vet ./...`, затем `go test ./... -race -count=1` (нужен env
   `INTERMASQ_SECRET=ci-test-secret-32-bytes-pad-XXXXXX`, иначе `init()` в
   `main.go` падает). Фронтенд-правки (A1) — `npm run build` в `frontend/`.
5. **Продуктовые правки — строго в объёме фикса.** Никакого рефакторинга сверх
   сказанного (исключение — A1, где `:key`-фикс и опциональный `has_bak`-
   рефакторинг; см. §2.A1).
6. **A3/A4-изоляция:** в `tests/suites/21-hosts-bugs.sh` невалидные MAC уже
   пишутся в `19-bugs.conf` (не в `10-static.conf`) — **не revert'ить**. После
   фикса A3/A4 API вернёт 400 → check зелёный, хост не пишется; изоляция
   остаётся безвредной.
7. **Backend-фиксы видны L2 go test ПЕРЕД smoke/e2e.** Учитывай: если фикс
   меняет handler-поведение, `handlers_test.go` (1457 строк) реагирует первым.
8. **Коммиты — после `go vet` + `go test` зелёного.** Пуш — по просьбе
   оператора; CI (smoke) подтверждает.
9. **Backend smoke-чеки живут в `tests/suites/NN-*.sh`** (лексический порядок):
   A2/A3/A4 — в `20-*`/`21-*`/`30-*`; A6 — `24-hosts-bulk.sh`; A8 — `80-metrics.sh`;
   A11 — `81-path-traversal.sh`; A12 — `31-aliases-bugs.sh`.

---

## 2. Баги (исполняемый список)

По убыванию ROI. Для каждого: файл → корень → фикс → regression-test → knock-on.

### A1 (CRITICAL, frontend) — дубли строк таблицы при сортировке
- **Файл:** `frontend/src/components/static/HostTable.vue` (`:key` в `v-for`).
- **Корень:** `:key="h.mac"` не уникален — тот же MAC в нескольких `.conf` +
  `getHostsHandler` (`handlers_hosts.go:48`) мутирует `entry.File` суффиксом
  `|has_bak`, плодя дубли. Vue требует уникальных ключей → reconciliation даёт
  дубль DOM при каждой перерисовке (2→4→8→16 строк, сбрасывается F5).
- **Фикс:** `:key="h.mac + '|' + (h.file||'')"`. Опционально (правильнее):
  вернуть отдельное поле `has_bak bool` в `HostEntry` вместо суффикса `|has_bak`
  в File — это убирает и анти-паттерн, и коллизию ключей.
- **Regression:** Playwright `hosts-sort.spec.ts` (guard: count строк стабилен
  при сортировке). Это guard, не reproducer — он ПАССУЕТ сейчас и должен
  оставаться зелёным после фикса. Усиление (опционально): assert порядка.
- **Важно:** фронтенд-правка → пересобрать (`cd frontend && npm run build`) для
  локальной проверки; CI пересобирает сам.

### A2 (CRITICAL, backend) — DNS-alias дубликаты можно добавлять
- **Файл:** `handlers_aliases.go:76` + `aliases.go` `findAliasesByDomain`.
- **Корень:** `findAliasesByDomain(domain, type, file)` **исключает** запись с
  matching `type+file`. Логика «не считай сам себя конфликтом» имеет смысл для
  edit-flow, но edit-flow в коде НЕТ → для add получается ровно наоборот:
  существующая запись в том же файле исключается → 0 конфликтов → дубль пишется.
- **Фикс:** в `addAliasHandler` звать БЕЗ exclude (или убрать exclude вообще —
  update-flow отсутствует). Проверить `bulkAddAliasesHandler` и
  `importAliasesCSVHandler` — там уже корректно.
- **Regression:** smoke `A2: duplicate A same file → 409` + `A2: file has
  exactly 1 nas.local A record` (обе в `30-aliases-happy.sh` / `31-aliases-bugs.sh`).
- **Knock-on:** smoke `Delete again → 404` (tagged A2, `32-aliases-delete.sh`)
  после фикса реально вернёт 404 (дубля нет → second delete мажет) → check
  зелёный. Сними A2 с этих проверок.

### A3 (HIGH, backend) — zero/broadcast MAC принимаются
- **Файл:** `main.go:78` (`macRegex`) + `validateHostFields`.
- **Корень:** регекс пропускает `00:00:00:00:00:00` и `ff:ff:ff:ff:ff:ff`.
- **Фикс:** в `validateHostFields` чёрный список:
  ```go
  if strings.EqualFold(mac, "00:00:00:00:00:00") ||
     strings.EqualFold(mac, "ff:ff:ff:ff:ff:ff") { return false }
  ```
- **Regression:** smoke `A3: zero MAC rejected → 400` + `A3: broadcast MAC
  rejected → 400` (`21-hosts-bugs.sh`).

### A4 (HIGH, backend) — MAC с `-` сохраняется, dnsmasq падает на reload
- **Файл:** `main.go:78` + точка входа (`addHostHandler`/`bulkAddHostsHandler`/`importCSVHandler`).
- **Корень:** `macRegex` принимает и `:`, и `-`; `formatDhcpHostLine` пишет
  as-is. dnsmasq принимает только `:`. Сохранённый `aa-bb-...` → `--test` fail.
- **Фикс:** нормализовать `-`→`:` в `validateHostFields` (или в каждой точке
  входа): `mac = strings.ReplaceAll(mac, "-", ":")`. Заодно консистентность
  `findHostsByMac`. smoke-check A4 ожидает 400 ИЛИ нормализацию — проверь, что
  именно он ждёт (см. `21-hosts-bugs.sh`), и подгони фикс/чек.
- **Regression:** smoke `A4: dash-MAC handled` (`21-hosts-bugs.sh`).

### A12 (HIGH, backend) — aliasDomainRegex отвергает `_` (ломает DMARC/DKIM)
- **Файл:** `main.go:80` (`aliasDomainRegex`).
- **Корень:** `^[a-zA-Z0-9](...)?$` — `_` не алфавитно-цифровой → `_dmarc.local`,
  `_sip._tcp`, DKIM/ACME имена режутся.
- **Фикс:**
  ```go
  aliasDomainRegex = regexp.MustCompile(`^[a-zA-Z0-9_]([a-zA-Z0-9-._]*[a-zA-Z0-9_])?$`)
  ```
  (опционально per-type: A/CNAME строже, TXT/PTR свободнее).
- **Regression:** smoke `A12: Add TXT with underscore domain → 200`
  (`31-aliases-bugs.sh`).

### A6 (MEDIUM, backend) — bulk JSON без `count` в ответе
- **Файл:** `handlers_hosts.go` `bulkAddHostsHandler`.
- **Корень:** инконсистентность с CSV-путём (там `count` есть). Ответ
  `{"status":"ok"}` без count.
- **Фикс:** `c.JSON(200, gin.H{"status": "ok", "count": len(req.Hosts)})`.
- **Regression:** smoke `Bulk JSON response has count field` (`24-hosts-bulk.sh`).

### A8 (MEDIUM, backend/UX) — `/metrics` 401 с пустым телом
- **Файл:** `metrics.go:60` (`metricsHandler` без auth).
- **Корень:** `c.AbortWithStatus(401)` — без body. curl без `-i` выглядит пусто;
  Prometheus `last_error` невнятный.
- **Фикс:** `c.AbortWithStatusJSON(401, gin.H{"error": "auth_required"})`.
- **Regression:** smoke `A8: 401 has body` (`80-metrics.sh`).

### A11 (LOW, security) — path-traversal battery
- **Статус:** PARTIAL — большинство векторов закрыто через `isSafePath`.
  smoke `81-path-traversal.sh` покрывает 8 векторов, все проходят.
- **Особенность:** Go `net/http` чистит `..` до router — defence in depth, не
  замена явному чеку.
- **Действие:** опционально — hardening (явные проверки везде, где полагаешься
  на path-cleaning). На known-fail не влияет. Можно пропустить.

---

## 3. Верификация (после каждого бага + финальная)

**Локально (Windows):**
- `go vet ./...` — ЧИСТО (обязательно!).
- `$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXX"; go test ./... -race -count=1` — existing тесты не должны сломаться; `handlers_test.go` pierwszy видит backend-фиксы.
- A1: `cd frontend && npm run build` — собирается.
- smoke локально НЕ запустить (нужен dnsmasq + linux) — полагайся на CI.

**CI (основная):**
- Дефолтный прогон: smoke.sh → **0 Fail, 0 Known-fail** (по мере снятия ID),
  go test зелёный, gofmt чистый.
- L4 (opt-in `run_e2e_tests=true`) — не обязателен для backend-багов; A1
  (frontend) — прогнать, чтобы убедиться что UI не сломался.

---

## 4. Приёмка (definition of done)

- [ ] A1, A2, A3, A4, A6, A8, A12 пофикшены (A11 — опционально/hardening).
- [ ] `tests/known-bugs.txt` пуст (или содержит только wontfix с rationale).
- [ ] `tests/bugreport/bugs.md`: статусы FIXED + коммиты, сводка/Итого обновлены.
- [ ] smoke.sh (CI default): **0 Fail, 0 Known-fail, ~138 Pass**.
- [ ] `go vet ./...` и `go test ./... -race` зелёные.
- [ ] Session-лог `логи/bugfix-sweep.md` (контекст → по-бажно: фикс → верификация
      → результат). ROADMAP P0/«Что осталось» обновлены.

---

## 5. Порядок исполнения (рекомендация)

1. **A12** (5 мин, regex) → A8 (5 мин, AbortWithStatusJSON) — быстрый старт,
   снимают 2 known-fail, греют контур.
2. **A6** (30 мин, count-поле) — простое.
3. **A3 + A4** (вместе, MAC-validation) — нормализация + blacklist; связаны.
4. **A2** (CRITICAL, aliases) — убрать exclude; проверь knock-on (Delete→404).
5. **A1** (CRITICAL, frontend) — `:key` + опционально `has_bak`-рефакторинг;
   прогон e2e.
6. **(опционально) A11** hardening.

На каждом шаге — `go vet` + `go test` + коммит. Не копить. CI между шагами.

---

## 6. ВНЕ области этой сессии

- **A7** — не баг (UI-layout TemplatesModal).
- **A10** — feature gap (Discovered-devices IP), отдельный PR.
- **A5, A13** — уже FIXED.
- **Gap 4 (L5 real VM)**, fuzzing парсеров, усиление `hosts-sort`/`auth` —
  отдельные задачи, см. `tests/ROADMAP.md` «Что осталось».
- Любой рефакторинг продуктовых исходников сверх описанных фиксов — запрещён.
