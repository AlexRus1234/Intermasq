# predrel-test-remediation — P2: deep-refactor prerequisites

**Дата:** 2026-08-03
**Скоуп:** `predrel-test-remediation.md` (родительский план) → фаза P2
(`predrel-test-remediation-p2.md`, промт, 11 задач).
**Коммиты:** `8a489f3` (Трек A — Go), `04ad007` (Трек B — smoke/Playwright).
**Результат:** `go vet`/`go build`/`go test ./...`/`go test -race` зелёные на
Windows; `bash -n` чистый; `vite build` чистый; `playwright --list` парсит
все 34 specs. Linux-gated кейсы (smoke против запущенного binary, Playwright
против intermasq-ci:18083) гоняются оператором на CI Fedora 44.

## Контекст

P2 = бывший P1 triage — обязательно **до** рефакторинга `system.go`,
`backup.go`, `metrics.go`, `audit.go`, `sse.go`, `auth.go` и до разделения
`package main` на подпакеты. Без закрытия P2 глубокий проход даёт регрессии,
которые тесты не замечают из-за слабых ассертов или утечки состояния. P1
(критичное) закрыт в `7a84a2e` (см. `-p1-exec.md`).

## Что сделано

### Трек A — Go (коммит `8a489f3`)

**P2.5 — `sysCaller` в `withSandboxFlags`** (`setup_test.go:46-98`). В struct
`orig` добавлено поле `sysCaller SystemCaller`, сохранение в setup,
восстановление в `t.Cleanup`. Любой тест, вызывающий `setupServer()` (а она
мутирует `sysCaller` через `initSystemCaller`, `system.go:347`), больше не
утекает resolved-caller'ом в следующие тесты. Иллюстрационный тест
`TestWithSandboxFlags_RestoresSysCaller` пинит это sentinel-ом
`&SystemdSystemCaller{UseSudo:true}` (ненулевой размер → надёжная pointer-
identity; zero-size `NoneCaller` не годится — такие указатели могут
алиаситься по Go spec). Ручные сохранения в `goroutines_test.go`,
`handlers_test.go`, `linux_test.go`, `new_features_test.go` **не тронуты**
(нулевая хрупкость, ниже риск).

**P2.6 — `withSandboxFlags` в `TestSetupServer_HistoryDirFail`**
(`setup_test.go:221`). Первой строкой добавлен `withSandboxFlags(t)`;
дублирующее ручное нейтрализование горутин убрано (sandbox уже это делает).
Защищает от `loadTemplates()` (`templates.go:37,41`) → `os.Exit(1)` на
битом хостовом `/etc/intermasq/templates.json`, который без sandbox убивал
весь тест-бинарник.

**P2.7 — `TestMetricsHandler_AllMetricNames`** (`metrics_test.go`). E2E-тест
`metricsHandler` целиком, пинит все 9 канонических имён метрик. Использованы
**существующие** хелперы `signTestJWT` + inline `SecretKey` (а не
несуществующие `setupMetricsGlobals`/`setTestSecret`/`makeToken` из промта);
вызов через `newMetricsContext` → `gin.CreateTestContext` (а не
`metricsHandler(w, req)`, что не компилируется — сигнатура
`func(c *gin.Context)`). Заполнены `dnsHealth` (две `intermasq_domain_*`
серии эмитятся только в этом цикле, `metrics.go:80-87`) и `sysCaller =
&NoneCaller{}` (без него `checkDnsmasqStatus` nil-deref'ит без `setupServer`).

**P2.8 — `audit_test.go` (новый)**. `TestWriteAudit_Concurrent` — 100
горутин, все строки валидный JSON с ожидаемыми полями + Timestamp.
`TestWriteAudit_ReadRoundtrip` — single write → `auditHandler` GET,
проверка полей. `*AuditLogPath` redirected + restored через `t.Cleanup`
(`withSandboxFlags` его не снапшотит — это путь write-only, не setup-путь).
Заполняет пробел: `writeAudit` не имел прямого теста, конкурентная запись
не проверялась (read-путь `auditHandler` уже покрыт в `handlers_test.go:1239`).

**P2.9 — Fuzz-фикс тавтологии** (`fuzz_test.go:179-189`). Недостижимые
пусто-строковые ветки (`l.Ip == ""`/`l.Mac == ""` — `strings.Fields` не даёт
пустых токенов) заменены на format-инвариант. **Важная адаптация:** промт
предлагал безусловный MAC/IP-format чек, но `parseLeasesContent` по дизайну
принимает мусор (см. шапку `fuzz_test.go`) и возвращает записи для любой
строки с ≥3 токенами — безусловный чек стрелял по garbage-seeds (`"a b c"` →
`{Ip:"c",Mac:"b"}`). Финальный вариант: format-ассерты применяются **только
когда Mac-токен MAC-образный** (содержит `:` или `-`) — это ловит целевую
мутацию `Ip=fields[0]` на реалистичных seeds и сохраняет panic-покрытие
мусорных seeds. `normalizeMAC` + `macRegex` уже применяются так же в
`FuzzParseDhcpHostLine:65`.

### Трек B — smoke + Playwright (коммит `04ad007`)

**P2.1 — `check_length` хелпер + обёртка существующих вычислений.** В
`tests/lib/http.sh` добавлен `check_length(desc, min, [jq_expr])` (default
`length`, для вложенных — `.files | length`). Обёрнуты уже-вычисляемые, но
не-ассерченные counts: 22 (`/api/hosts`), 40 (`/api/config .files`), 50
(`/api/history .versions`), 30 (новый `GET /api/aliases` после POST'ов). CSV-
экспорты 25/34 — inline `wc -l` (CSV не JSON, `jq` не применяется).
Known-empty-in-CI эндпоинты (42 `/api/templates/ranges`, 83 `/api/leases`) —
явный комментарий вместо бессмысленного ассерта. 83 `/api/hosts/next-ip` —
non-empty `.ip` content-ассерт (детерминированный). 70-audit и 83 ARP уже
имели length-ассерты — **не тронуты**.

**P2.2 — self-seed суитов 26/27.** Перед bulk-операцией каждый суит
пересоздаёт целевой хост POST'ом в `$FILE`. Использованы **реальные**
значения (а не ошибочные из промта): 26 → `aa:bb:cc:dd:ee:10` (csv1,
10.0.0.20); 27 → `aa:bb:cc:dd:ee:11` (csv2, 10.0.0.21 — с правильным
prefix `10.0.0`, т.к. ip_transform его заменяет; и hostname `csv2`, т.к.
`bulkEditHandler` валидирует hostname даже без трансформа). Сеял в `$FILE`
(источник bulk-операций), а не в отдельный файл — иначе bulk не нашёл бы
хост. `200|409` оба приняты (409 = уже создан 23-м).

**P2.3 — `reload-ui.spec.ts`.** `waitForResponse` больше не фильтрует по
`status === 200` (вис 30с на 400); матчит любой `/api/reload` с timeout
15с, статус ассертится отдельно.

**P2.4 — skip-guards.** `plugins-iframe` — runtime-подсчёт 🧩 dropdown-items
(ноль → skip, вместо locator-timeout). `discovery-tab` — probe
`/api/new-devices` через `apiLogin()` + `page.request.get`; пусто → skip.
Runtime-детект выбран вместо env-var (`PLUGINS_ENABLED`) — не требует правок
CI `build.yml`.

**P2.10 — `users-tab.spec.ts` self-delete.** Заменён dialog-count
(`>=2`) на реальный HTTP-ассерт: `waitForResponse(DELETE /api/users/admin)` →
`expect(resp.status()).toBe(400)` + `body.error` contains `cannot_delete_self`
(`handlers_users.go:68`). `page.on('dialog', d.accept())` сохранён (тест
уже его имел — в отличие от сломанного примера в промте, где хендлер был
пропущен).

**P2.11 — `history-modal.spec.ts` + `HistoryModal.vue`.** На `<tr>`添加лен
`:data-version="v.version"`. Spec выбирает строку с лексикографически
минимальным `data-version` (= старейший snapshot = pre-GONE состояние
`{KEEP}`) — order-independent независимо от newest-first рендера.

## Расхождения с промт-планом (исправлено по ходу)

Верификация explore-агентом до старта нашла неточности в 5 из 11 задач.
Полный разбор выше; кратко:

1. **P2.1:** в 6 из 9 сьютов `jq 'length'`/`wc -l` **уже вычислялось, но
   только эхалось** без `check`. Реальная работа — обернуть, а не «добавить
   с нуля». `70-audit.sh` и `83-discovery.sh` ARP уже имели length-ассерты.
2. **P2.2:** MAC в промте (`ee:10:00:00:00:01`) ≠ реальный
   (`aa:bb:cc:dd:ee:10`); bulk-edit требует hostname=`csv2`+IP=`10.0.0.21`;
   сеять надо в `$FILE` (источник bulk), не в отдельный файл.
3. **P2.7:** хелперы `setupMetricsGlobals`/`setTestSecret`/`makeToken` **не
   существуют**; сигнатура `metricsHandler(w, req)` не компилируется.
4. **P2.8:** `auditHandler` read-путь уже тестируется (`handlers_test.go:1239`);
   реальный пробел — direct `writeAudit` + конкурентная запись + round-trip.
5. **P2.9:** `parseLeasesContent` принимает мусор → безусловный format-чек
   стрелял по garbage-seeds; заменён на gate по MAC-образному токену.
6. **P2.10:** пример фикса в промте пропускал `page.on('dialog', ...)`;
   реальный тест уже его имел.
7. **P2.11:** версии — timestamps `^\d{8}-\d{6}(-\d+)?$`, не номера; порядок
   **уже** newest-first (`history.go:208-210`); селектор промта `/^1$/` не
   сматчил бы ничего; направление риска в промте указано наоборот.

P2.3 и P2.4 — допущения подтверждены дословно.

## Верификация

Локально (Windows):
- `go vet ./...` — чисто.
- `go build ./...` — чисто.
- `go test ./... -count=1` (`INTERMASQ_SECRET` задан) — **ok** 16.5с.
- `go test ./... -race -count=1` (`CGO_ENABLED=1`) — **ok** 125.6с, без data
  races.
- Мутации проверены и откатаны:
  - P2.7: `intermasq_hosts_total`→`_host_total` → тест FAIL с указанием метрики.
  - P2.8: убрать `O_APPEND` → `TestWriteAudit_Concurrent` FAIL (1 строка
    вместо 100).
  - P2.9: `fields[2]`→`fields[0]` → `FuzzParseLeasesContent` FAIL на
    `net.ParseIP`.
- `bash -n` на всех тронутых suite'ах + `tests/lib/http.sh` — exit 0.
- `vite build` (frontend) — ok, 121 modules, 5.35с.
- `npx playwright test --list` — парсит 34 теста в 29 файлах (включая все
  5 тронутых specs).

CI (Fedora 44, оператор прогоняет отдельно):
- `BASE=http://localhost:18081 ./tests/smoke.sh` — length-ассерты (P2.1)
  честно ловят пустоту; 26/27 зелёные при пропуске 23 (P2.2).
- `cd tests/e2e && npx playwright test` — все specs PASS; без mock-плагина
  и/или ARP-fixture plugins-iframe/discovery-tab skip'аются (P2.4).
- `go test -run '^$' -fuzz='^FuzzParseLeasesContent$' -fuzztime=30s .` — no
  crash (опционально, по желанию оператора).

## Изменённые файлы

```
Трек A (8a489f3):
 audit_test.go   | 121 ++++++++++++++++++++++++++++++++++++++++++++++++++++ (new)
 fuzz_test.go    |  25 +++++++++++++++------
 metrics_test.go |  64 +++++++++++++++++++++++++++++++++++++++++
 setup_test.go   |  51 +++++++++++++++++++++++++++---------
 4 files changed, 241 insertions(+), 18 deletions(-)

Трек B (04ad007):
 frontend/src/components/history/HistoryModal.vue |   2 +-
 tests/e2e/specs/discovery-tab.spec.ts            |  17 ++++++++
 tests/e2e/specs/history-modal.spec.ts            |  18 +++++++-
 tests/e2e/specs/plugins-iframe.spec.ts           |  18 +++++++-
 tests/e2e/specs/reload-ui.spec.ts                |  11 ++++-
 tests/e2e/specs/users-tab.spec.ts                |  27 +++++++----
 tests/lib/http.sh                                |  24 ++++++++++
 tests/suites/22-hosts-delete-list.sh             |   7 ++-
 tests/suites/25-hosts-csv-export.sh              |   5 +-
 tests/suites/26-hosts-bulk-move.sh               |  13 +++++-
 tests/suites/27-hosts-bulk-edit.sh               |  12 +++++
 tests/suites/30-aliases-happy.sh                 |   7 +++
 tests/suites/34-aliases-csv.sh                   |   4 +-
 tests/suites/40-config-files.sh                  |   6 +-
 tests/suites/42-templates-hosts.sh               |   4 +-
 tests/suites/50-safety-backup-history.sh         |   7 ++-
 tests/suites/83-discovery.sh                     |  14 ++++++
 17 files changed, 172 insertions(+), 24 deletions(-)
```

## Что осталось (вне этой сессии)

- **P3** (`predrel-test-remediation-p3.md`) — 9 polish-задач до v1.0
  release: A11 isSafePath direct test, strict-fake dnsmasq, инвертированные
  комментарии, mutation-friendly селекторы, покрытие endpoints. ~1 день.
- **Продуктовый security-аудит** (вне тестовых промтов): JWT alg-confusion в
  `auth.go:214`, plugin trust boundary в `main.go:131-193`, X-Forwarded-For
  в `rateLimitMiddleware`, `hash, _ :=` в `handlers.go:47`.
- **A15** — оставлен KNOWN-CONDITIONAL на dnsmasq 2.80 по решению оператора
  (см. P1-exec).
