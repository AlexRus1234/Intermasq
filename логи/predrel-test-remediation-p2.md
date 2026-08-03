# P2 — починки перед глубоким рефакторингом (бывший P1) ✓ ВЫПОЛНЕНО

**Статус:** ✓ ВЫПОЛНЕНО (2026-08-03, коммиты `8a489f3` Трек A + `04ad007`
Трек B, в `main`).
**Подробный лог:** [`predrel-test-remediation-p2-exec.md`](./predrel-test-remediation-p2-exec.md).

**Цель:** устранить находки, которые **не критичны для текущего зелёного CI**,
но станут препятствием при рефакторинге init/backup/metrics/audit/sse/system
доменов. Без закрытия P2 глубокий проход даст регрессии, которые тесты не
заметят из-за слабых ассертов или утечки состояния.

**Трудоёмкость:** ~1.5 дня (11 задач).
**Применяемость:** обязательно до рефакторинга `system.go`, `backup.go`,
`metrics.go`, `audit.go`, `sse.go`, `auth.go` (rate-limit/blacklist), до
разделения `package main` на подпакеты.

**Зависимости:** P1 желательно закрыт (особенно P1.3.a — `sysCaller` leak
фиксится здесь в задаче P2.5, но если P1.3 не сделано, тесты P2 всё равно
работают — просто менее изолированно).

---

## Задача P2.1 — Добавить `jq 'length'` нижние границы на GET-эндпоинты

**Контекст:** today большинство smoke-сьютов проверяют только HTTP 200 на
GET-эндпоинтах (`/api/hosts`, `/api/config`, `/api/history`, `/api/audit`,
`/api/leases`, `/api/new-devices`, `/api/aliases`, `/api/templates/ranges`,
`/api/templates`, оба CSV-экспорта). Если хендлер начнёт возвращать пустой
массив `[]` с 200 — все сьюты проходят. Рефакторинг read/parse-пути
остаётся без защиты.

**Файлы и конкретные точки:**

| Suite | Строка | Эндпоинт | Известное состояние к моменту чека | Минимум |
|-------|--------|----------|------------------------------------|---------|
| `tests/suites/22-hosts-delete-list.sh` | `:10-11` | `GET /api/hosts` | после suite 20 (4 хоста) + 23 (CSV) | `>= 4` |
| `tests/suites/40-config-files.sh` | `:19-22` | `GET /api/config` | после создания тестовых файлов | `>= 2` файлов |
| `tests/suites/42-templates-hosts.sh` | `:34-35` | `GET /api/templates/ranges` | без dhcp-range в CI — пусто | `>= 0` (но с явным `known empty`) |
| `tests/suites/50-safety-backup-history.sh` | `:9-12` | `GET /api/history` | после mutations | `>= 1` |
| `tests/suites/70-audit.sh` | `:4-7` | `GET /api/audit` | после всех предыдущих writes | `>= 5` |
| `tests/suites/83-discovery.sh` | `:7-10` | `GET /api/leases` | без реального dnsmasq в CI — пусто | `known empty` |
| `tests/suites/83-discovery.sh` | `:26-29` | `GET /api/new-devices` | зависит от arp fixture | `>= 1` (CI) |
| `tests/suites/83-discovery.sh` | `:32-35` | `GET /api/hosts/next-ip` | для CIDR 10.99.0.0/24 | `1` запись с полем `ip` |
| `tests/suites/30-aliases-happy.sh` | после add | `GET /api/aliases` | **NEW**: добавить GET после POST, чтобы list также покрывался | `>= 1` |
| `tests/suites/25-hosts-csv-export.sh` | `:4-7` | `GET /api/hosts/csv` | после всех добавлений | `>= 1` строка данных (после header) |
| `tests/suites/34-aliases-csv.sh` | `:6-9` | `GET /api/aliases/csv` | после добавления alias | `>= 1` строка данных |

**Подход:**
- Для каждого пункта добавить `check`-ассерт вида:
  ```bash
  COUNT=$(body | jq 'length' 2>/dev/null || echo -1)
  check "<name> body has >=N items" 1 $([ "$COUNT" -ge N ] && echo 1 || echo 0)
  ```
  или завести хелпер `check_length "<name>" "<endpoint>" "<min>"` в
  `tests/lib/http.sh`.
- Для эндпоинтов, которые **ожидаемо пусты** в CI (leases без реального
  dnsmasq, templates/ranges без dhcp-range), — явный комментарий `# known
  empty in CI` и `check ... 0 0` (что 0 == 0), либо пропуск.

**Как верифицировать:**
1. Временно сломать `getHostsHandler` возвращать `c.JSON(200, []HostEntry{})` →
   сьюты 22/25 должны FAIL на length-ассерте. Откатить.
2. Аналогично для других эндпоинтов.

**Acceptance criteria:** каждый GET-эндпоинт с данными имеет хотя бы один
length-ассерт, который падает при возврате пустоты.

---

## Задача P2.2 — Self-seed в суитах 26/27 (убрать зависимость от 23)

**Контекст:** `tests/suites/26-hosts-bulk-move.sh:9-13` и
`tests/suites/27-hosts-bulk-edit.sh:11-16` ожидают, что хосты `ee:10` и
`ee:11` уже созданы суитом `tests/suites/23-hosts-csv.sh:7-9`. Если 23
пропустится или упадёт, 26/27 падают с запутанным «moved=0» / «updated=0» —
не указывающим на корень.

**Файлы:**
- `tests/suites/26-hosts-bulk-move.sh` — перед bulk-move запросом
  пересоздать целевой хост:
  ```bash
  # Self-seed: not depending on 23-hosts-csv to have created ee:10.
  POST "$JWT" "/api/hosts" '{"mac":"ee:10:00:00:00:01","ip":"10.99.10.2","hostname":"move26","file":"'"$CONF_DIR"'/26-bulkmove.conf"}' >/dev/null || true
  ```
  Использовать **свой** файл `26-bulkmove.conf` и свой MAC-суффикс, чтобы
  не конфликтовать с другими суитами. Аналогично для bulk-edit.
- `tests/suites/27-hosts-bulk-edit.sh` — то же с MAC `ee:11` (или новым
  уникальным) и своим файлом `27-bulkedit.conf`.

**Альтернатива (менее инвазивная):** добавить в начало каждого суита
комментарий-маркер `# DEPENDS-ON: 23-hosts-csv.sh` и проверку, что целевой
хост существует; если нет — `skip_suite "depends on 23"`. Но это слабее
self-seed.

**Как верифицировать:**
1. Временно пропустить 23 (закомментировать строку в `tests/smoke.sh:48`
   или переименовать файл) → 26/27 должны PASS (благодаря self-seed).
   Откатить.
2. Полный прогон smoke — 26/27 остаются зелёными.

**Acceptance criteria:** 26/27 не зависят от состояния, созданного другими
сьютами.

---

## Задача P2.3 — `waitForResponse` с timeout в reload-ui.spec.ts

**Контекст:** `tests/e2e/specs/reload-ui.spec.ts:17-20`:
```typescript
const [resp] = await Promise.all([
  page.waitForResponse((r) => r.url().includes('/api/reload') && r.status() === 200),
  page.locator('.btn-warning', { hasText: '🔄' }).click(),
])
```
Если `applyConfig` вернёт 400 (например, `dnsmasq --test` упал), предикат
`r.status() === 200` не сматчится, `waitForResponse` висит 30s (default test
timeout), затем fails с opaque-сообщением.

**Файл:**
- `tests/e2e/specs/reload-ui.spec.ts:17-20` — переписать:
  ```typescript
  const [resp] = await Promise.all([
    page.waitForResponse(
      (r) => r.url().includes('/api/reload'),
      { timeout: 15000 }
    ),
    page.locator('.btn-warning', { hasText: '🔄' }).click(),
  ])
  expect(resp.status()).toBe(200)
  ```
  Так 400 выдаст быстрый и понятный `expect(resp.status()).toBe(200)` с
  реальным статусом в сообщении.

**Как верифицировать:**
1. На фиксе: spec PASS.
2. Мутация: временно сломать reload (например, подсунуть кривой конфиг в
   fixture) → spec должен быстро (≤15s) упасть с осмысленным сообщением про
   статус. Откатить.

**Acceptance criteria:** spec не висит 30s при 400, выдаёт понятную ошибку.

---

## Задача P2.4 — Skip-guards для plugins-iframe и discovery-tab specs

**Контекст:** `tests/e2e/specs/plugins-iframe.spec.ts` (весь) и
`tests/e2e/specs/discovery-tab.spec.ts` (весь) падают с locator-timeout, если
CI-окружение не настроило mock-плагин (`tests/fixtures/plugins/hello/`) и
ARP-fixture (`-arp-file=tests/fixtures/arp-sample.txt`). При локальном прогоне
разработчика или в урезанной матрице это red. Другие env-зависимые specs
(`setup-screen.spec.ts`, `sse-live.spec.ts` full-вариант) — правильно имеют
`test.skip(...)`. Эти два — нет.

**Файлы:**
- `tests/e2e/specs/plugins-iframe.spec.ts` — в начале test() добавить:
  ```typescript
  test('plugins iframe shows hello output', async ({ page, browser }) => {
    // Mock plugin is installed by CI build.yml (Gap 6 step). Skip if absent.
    const plugins = await page.evaluate(() => (window as any).store?.plugins ?? [])
    test.skip(plugins.length === 0, 'no plugins loaded — requires mock plugin installed')
    // ... rest of test
  })
  ```
  Или ещё проще — `test.skip(!process.env.PLUGINS_ENABLED, 'requires PLUGINS_ENABLED=1')`
  и в CI build.yml передать `PLUGINS_ENABLED=1` в env запуска Playwright.

- `tests/e2e/specs/discovery-tab.spec.ts` — добавить проверку, что
  `/api/new-devices` возвращает непустой массив; если пусто — skip:
  ```typescript
  test('discovery tab shows unknown ARP device', async ({ page }) => {
    const resp = await page.request.get('/api/new-devices')
    test.skip(resp.status() !== 200, '/api/new-devices unavailable')
    const devices = await resp.json()
    test.skip(!Array.isArray(devices) || devices.length === 0, 'no new devices — requires arp fixture')
    // ... rest of test
  })
  ```

**Как верифицировать:**
1. Запуск Playwright **без** mock-плагина и **без** ARP-fixture → оба specs
   SKIP (жёлтый), остальные PASS.
2. Запуск **с** mock-плагином и ARP-fixture (CI-режим) → оба PASS.

**Acceptance criteria:** specs skip'аются в урезанном окружении, не падают
red.

---

## Задача P2.5 — Сохранение/рестор `sysCaller` в withSandboxFlags

**Контекст:** `setup_test.go:46-98` (`withSandboxFlags`) сохраняет/ресторит
10+ глобалов (`DBPath`, `TemplatesPath`, `HistoryDir`, `ConfigDir`, `ArpPath`,
`LeasesPath`, `InitSystem`, `SystemdScope`, `PluginsDir`, `SocketsDir`,
`loadedPlugins`, `startSSEBroadcasterFn`, `startDNSHealthCheckerFn`). Но
**не** `sysCaller` (`system.go:345`). После `setupServer() → initSystemCaller()`
`sysCaller` остаётся `NoneCaller` (или что InitSystem разрезолвил) до конца
тест-бинарника. Сейчас это маскируется тем, что `goroutines_test.go` сохраняет
`sysCaller` руками, но это хрупкая связь.

**Файлы:**
- `setup_test.go:46` — в структуру `orig` добавить поле `sysCaller SystemCaller`,
  в setup сохранить `orig.sysCaller = sysCaller`, в cleanup восстановить
  `sysCaller = orig.sysCaller`:
  ```go
  func withSandboxFlags(t *testing.T) string {
      t.Helper()
      orig := struct {
          // ... existing fields ...
          sysCaller        SystemCaller
      }{
          // ... existing ...
          sysCaller:        sysCaller,
      }
      // ... existing setup ...
      t.Cleanup(func() {
          // ... existing restores ...
          sysCaller = orig.sysCaller
      })
      return dir
  }
  ```

**Как верифицировать:**
1. После правки — `go test ./... -count=1` зелёный.
2. С `-race` — `go test ./... -race -count=1` — без новых data races.
3. Добавить тест-иллюстрацию: `TestSysCaller_RestoredAfterSetup` — вызвать
   `withSandboxFlags`, потом `setupServer()`, потом в cleanup проверить, что
   `sysCaller` вернулся к original значению.

**Acceptance criteria:** любой тест, меняющий `sysCaller` через `setupServer`,
не утекает в следующие. Драйвер для `t.Parallel()` (когда будет разделение
пакетов) становится безопаснее.

---

## Задача P2.6 — Обернуть `TestSetupServer_HistoryDirFail` в withSandboxFlags

**Контекст:** `setup_test.go:215-248` (`TestSetupServer_HistoryDirFail`) НЕ
вызывает `withSandboxFlags` — только нейтрализует горутины и перенаправляет
`*HistoryDir`. Это оставляет `DBPath`, `TemplatesPath`, `ConfigDir`, `ArpPath`,
`LeasesPath`, `PluginsDir` в дефолтных значениях (`/etc/intermasq/users.json`,
`/etc/intermasq/templates.json`, `/etc/dnsmasq.d`, `/proc/net/arp`, ...).
`loadTemplates()` (`templates.go:37,41`) вызывает `os.Exit(1)` на битом файле
— если на хосте есть кривой `/etc/intermasq/templates.json`, тест убивает весь
бинарник.

**Файлы:**
- `setup_test.go:215-248` — первая строка теста:
  ```go
  func TestSetupServer_HistoryDirFail(t *testing.T) {
      withSandboxFlags(t)   // ADD: redirects all paths to t.TempDir(), neutralizes goroutines
      // ... existing logic (override *HistoryDir to a blocker path) ...
  }
  ```
  После `withSandboxFlags` перенаправить `*HistoryDir` на
  `/nonexistent-blocker/path` уже поверх sandbox-пути — тест продолжает
  проверять ту же ошибку, но в изоляции.

**Как верифицировать:**
1. Создать на хосте (временно) `/etc/intermasq/templates.json` с битым JSON.
2. До фикса: `go test -run TestSetupServer_HistoryDirFail` убивает процесс.
3. После фикса: тест PASS в sandbox, не трогая реальные пути.
4. Удалить `/etc/intermasq/templates.json`.

**Acceptance criteria:** тест изолирован и не зависит от состояния хоста.

---

## Задача P2.7 — End-to-end тест metricsHandler (все 9 имён метрик)

**Контекст:** `metrics_test.go` покрывает форматеры (`boolToFloat`,
`writeSimpleMetric`, `writeLabeledMetric`) и auth-gate `checkMetricsAuth` (8
кейсов). Но сам `metricsHandler` (`metrics.go:60-91`) целиком не тестируется.
7 канонических имён метрик + 2 labeled-серии могут быть переименованы/typo'нут
без падения тестов.

**Файлы:**
- `metrics_test.go` — добавить тест:
  ```go
  func TestMetricsHandler_AllMetricNames(t *testing.T) {
      setupMetricsGlobals(t)   // existing helper
      setTestSecret(t)
      // ... setup state so all metrics have non-zero data:
      //   - create 1 host in ConfigDir
      //   - call reloadDnsmasq() once (to bump Reloads counter)
      //   - call writeFileRaw with invalid content (to bump TestFailures)
      // ...

      req := httptest.NewRequest("GET", "/metrics", nil)
      req.Header.Set("Authorization", "Bearer "+makeToken("admin"))
      w := httptest.NewRecorder()
      metricsHandler(w, req)

      body := w.Body.String()
      required := []string{
          "intermasq_hosts_total",
          "intermasq_leases_active",
          "intermasq_arp_online_total",
          "intermasq_dnsmasq_active",
          "intermasq_reloads_total",
          "intermasq_dnsmasq_test_failures_total",
          "intermasq_uptime_seconds",
          "intermasq_domain_up",
          "intermasq_domain_resolve_seconds",
      }
      for _, name := range required {
          if !strings.Contains(body, name) {
              t.Errorf("metricsHandler: missing metric %q in body", name)
          }
      }
  }
  ```

**Как верифицировать:**
1. На фиксе: тест PASS.
2. Мутация: переименовать `intermasq_hosts_total` → `intermasq_host_total` в
   `metrics.go` → тест FAIL с указанием отсутствующей метрики. Откатить.

**Acceptance criteria:** любое переименование/typo в канонических именах
метрик роняет тест.

---

## Задача P2.8 — Конкурентный тест writeAudit

**Контекст:** `audit.go:26-51` (`writeAudit`) делает
`O_APPEND|O_CREATE|O_WRONLY` + единый `f.Write`. Атомарность small-write на
POSIX гарантирована, но: 1) нет ни одного теста `writeAudit` напрямую; 2)
`auditHandler` (round-trip) не тестируется; 3) конкурентная запись
непроверена.

**Файлы:**
- Создать новый файл `audit_test.go` (или добавить в `goroutines_test.go`):
  ```go
  func TestWriteAudit_Concurrent(t *testing.T) {
      withSandboxFlags(t)
      *AuditLogPath = filepath.Join(t.TempDir(), "audit.log")

      const N = 100
      var wg sync.WaitGroup
      for i := 0; i < N; i++ {
          wg.Add(1)
          go func(idx int) {
              defer wg.Done()
              writeAudit(AuditEntry{
                  User:   fmt.Sprintf("user%d", idx),
                  Action: "test_action",
              })
          }(i)
      }
      wg.Wait()

      // Read back via auditHandler-equivalent or directly
      data, err := os.ReadFile(*AuditLogPath)
      if err != nil {
          t.Fatalf("read audit log: %v", err)
      }
      lines := strings.Split(strings.TrimSpace(string(data)), "\n")
      if len(lines) != N {
          t.Fatalf("expected %d audit lines, got %d", N, len(lines))
      }
      // Validate all lines are well-formed JSON with expected fields
      for i, line := range lines {
          var entry AuditEntry
          if err := json.Unmarshal([]byte(line), &entry); err != nil {
              t.Errorf("line %d: invalid JSON: %v (line=%q)", i, err, line)
          }
          if entry.Action != "test_action" {
              t.Errorf("line %d: expected action test_action, got %q", i, entry.Action)
          }
      }
  }
  ```

**Как верифицировать:**
1. На фиксе: тест PASS (100 строк, все валидный JSON).
2. С `-race`: `go test -race -run TestWriteAudit_Concurrent` — без data
   races.
3. Мутация: убрать `O_APPEND` flag → должны появиться перемешанные/битые
   строки → тест FAIL на JSON-валидации. Откатить.

**Acceptance criteria:** конкурентная запись в audit не теряет записи и не
портит JSON.

---

## Задача P2.9 — Починить тавтологию в FuzzParseLeasesContent

**Контекст:** `fuzz_test.go:179-189` (`FuzzParseLeasesContent`) утверждает,
что `l.Ip == ""` и `l.Mac == ""` никогда не случаются. Но `parseLeasesContent`
(`arp_leases.go:67`) ставит `l.Ip = fields[2]` и `l.Mac = fields[1]`, где
`fields` из `strings.Fields` — никогда не yields пустые токены, при `len(fields)
>= 3`. Оба empty-ветки **недостижимы по построению**. Fuzz-тест не ловит
ничего, кроме panic.

**Файлы:**
- `fuzz_test.go:179-189` — заменить тавтологию на реальные invariant:
  ```go
  for _, l := range leases {
      // MAC должен быть валидным MAC-форматом (после normalizeMAC)
      m := normalizeMAC(l.Mac)
      if !macRegex.MatchString(m) {
          t.Errorf("FuzzParseLeasesContent: lease MAC %q not a valid MAC (input=%q)", l.Mac, data)
      }
      // IP должен парситься net.ParseIP
      if net.ParseIP(l.Ip) == nil {
          t.Errorf("FuzzParseLeasesContent: lease IP %q not a valid IP (input=%q)", l.Ip, data)
      }
  }
  ```
  Теперь fuzz ловит реальные регрессии (например, если парсер начнёт
  возвращать мусор вместо IP/MAC).

**Как верифицировать:**
1. На фиксе: `go test -run FuzzParseLeasesContent` PASS (seed-корпус
   проходит).
2. Real fuzz: `go test -run '^$' -fuzz='^FuzzParseLeasesContent$' -fuzztime=30s .` — no crash.
3. Мутация: в `parseLeasesContent` заменить `l.Ip = fields[2]` на `l.Ip =
   fields[0]` (timestamp вместо IP) → seed-тест FAIL на `net.ParseIP`-чекe.
   Откатить.

**Acceptance criteria:** fuzz-тест реально валидирует формат вывода парсера,
не тавтологичен.

---

## Задача P2.10 — users-tab self-delete: реальный assert 400

**Контекст:** `tests/e2e/specs/users-tab.spec.ts:49-55` утверждает только,
что ≥2 dialogs сработали И admin-строка ещё видна. Если `deleteUser()` во
фронтенде замутировать на no-op (не вызывать API), тест всё равно проходит.

**Файлы:**
- `tests/e2e/specs/users-tab.spec.ts:49-55` — заменить на реальную проверку
  ответа:
  ```typescript
  test('admin cannot delete self', async ({ page }) => {
    // ... navigate to users tab ...
    const adminRow = page.locator('tbody tr', { hasText: 'admin' })

    // Wait for the DELETE request and assert the backend returns 400
    const [resp] = await Promise.all([
      page.waitForResponse(
        (r) => r.url().includes('/api/users/admin') && r.request().method() === 'DELETE',
        { timeout: 10000 }
      ),
      // Accept both confirm dialogs (delete + the cannot-delete-self warning)
      page.locator('tbody tr', { hasText: 'admin' }).locator('button.btn-outline-danger').click(),
    ])
    expect(resp.status()).toBe(400)
    const body = await resp.json()
    expect(body.error).toContain('cannot_delete_self')

    // Admin row must still be present
    await expect(adminRow).toBeVisible({ timeout: 10000 })
  })
  ```

**Как верифицировать:**
1. На фиксе: spec PASS.
2. Мутация: во фронтенде (например, `frontend/src/api/users.js`)
  `deleteUser` → no-op (return без fetch) → spec FAIL на waitForResponse
  timeout. Откатить.
3. Мутация: backend `deleteUserHandler` принимает self-delete (убрать 400) →
   spec FAIL на `expect(resp.status()).toBe(400)`. Откатить.

**Acceptance criteria:** spec реально проверяет, что backend отклонил
self-delete.

---

## Задача P2.11 — history-modal: пин order через метку версии, не .first()

**Контекст:** `tests/e2e/specs/history-modal.spec.ts:36` использует
`page.locator('tbody tr').first()` для выбора версии 1. Это работает только
если history отсортирован oldest-first (что недокументировано). Любой рефактор
history, перевернувший порядок (newest-first — частый паттерн для backup-lists)
роняет spec по неправильной причине.

**Файлы:**
- `tests/e2e/specs/history-modal.spec.ts:36` — вместо `.first()` выбрать по
  известной метке версии. Изучить `HistoryModal.vue` — какие идентификаторы
  версий отображаются (обычно timestamp или номер). Селектор:
  ```typescript
  // Выбрать строку версии 1 (самую старую) по явной метке, не по позиции
  const versionRow = page.locator('tbody tr', {
    has: page.locator('code', { text: /^1$/ }),   // или timestamp / hash первой версии
  }).first()   // .first() теперь безопасен — фильтр по содержимому, не по позиции
  ```
- Если в UI нет явной метки версии — добавить её (data-attribute):
  `HistoryModal.vue` → `<tr :data-version="v.index">` → селектор
  `tbody tr[data-version="1"]`. Это самое чистое решение.

**Как верифицировать:**
1. На фиксе: spec PASS.
2. Мутация: в backend (`history.go`) перевернуть порядок возврата версий
  (newest-first) → spec всё равно PASS (потому что выбирает по метке, а не по
  позиции). Откатить.

**Acceptance criteria:** spec не падает при изменении порядка отображения
history.

---

## Задача P2.12 — Финальная верификация фазы P2

После задач P2.1–P2.11 прогнать полный цикл:

1. **Go:**
   ```bash
   go vet ./...
   go test ./... -count=1
   go test ./... -race -count=1
   ```
   Новые тесты (metricsHandler e2e, writeAudit concurrent, sysCaller restore)
   PASS.
2. **Fuzz:**
   ```bash
   go test -run '^$' -fuzz='^FuzzParseLeasesContent$' -fuzztime=30s .
   ```
   No crash.
3. **Bash smoke:**
   ```bash
   BASE=http://localhost:18081 ./tests/smoke.sh
   ```
   Все length-ассерты (P2.1) проходят, 26/27 зелёные при пропуске 23 (P2.2).
4. **Playwright:**
   ```bash
   cd tests/e2e && npx playwright test
   ```
   specs PASS. Без fixture — plugins-iframe/discovery-tab skip'аются (P2.4).
5. **Коммит:** `predrel-test-remediation-P2: deep-refactor prerequisites`.

**Acceptance criteria фазы P2:** тесты изолированы, ассерты реальны, env-
зависимости skip'аются. Готов к глубокому рефакторингу system/backup/metrics/
audit/sse доменов.
