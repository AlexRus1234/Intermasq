# P3 — polish тестовой инфраструктуры до v1.0 (бывший P2)

**Статус:** ✓ ВЫПОЛНЕНО (2026-08-04).
**Подробный лог:** [`predrel-test-remediation-p3-exec.md`](./predrel-test-remediation-p3-exec.md).
Ниже — исходный промт (для справки).

---

# P3 — polish тестовой инфраструктуры до v1.0 (бывший P2)

**Цель:** закрыть находки, которые **не блокируют рефакторинг**, но снижают
читаемость, поддерживаемость или дают чрезмерно хрупкие тесты. Стоит сделать
до v1.0 release, но не критично для самого факта рефакторинга.

**Трудоёмкость:** ~1 день (9 задач).
**Применяемость:** до v1.0 release, после (или в процессе) P1/P2.

---

## Задача P3.1 — A11 discriminating vector для path-traversal defense

**Контекст:** regression-тесты A11 (`TestGetFileHandlerRejectsUnsafePath`
`dnsmasq_test.go:1648`, `TestPutFileHandlerRejectsUnsafePath`
`handlers_test.go:511`) проверяют векторы вида `../etc/evil.conf`,
`..\evil.conf`, `../../etc/dnsmasq.conf`. Но все они содержат `/` или `\`, и
**substring-фильтр** в `handlers_config.go:199/209/223/229` режет их раньше,
чем доходит до A11-фикса (`isSafePath` after `filepath.Join`). Откат A11-слоя
оставляет тесты зелёными.

**Сложность:** в `handlers_config.go` substring-фильтр **по построению**
покрывает все достижимые через URL traversal-входы. A11-слой — defense-in-
depth, и **внешний** вектор, который обходит substring, но ловится isSafePath,
по всей видимости **не существует** по дизайну (это и есть смысл DiD).

**Подход:**
- **Вариант A (recommended):** признать это явно в комментариях тестов:
  ```go
  // NOTE: these vectors are caught by the substring filter BEFORE the A11
  // isSafePath-after-Join layer fires. The A11 layer is defense-in-depth:
  // if the substring filter is ever weakened (e.g. to allow some / in
  // names), isSafePath is the second gate. There is no external HTTP
  // vector that bypasses substring but is caught by isSafePath by design.
  // To directly test the A11 layer, see TestIsSafePath_AfterJoin (below).
  ```
- **Вариант B (если хочется реально pin A11-логику):** добавить **unit-тест
  isSafePath напрямую**:
  ```go
  func TestIsSafePath_AfterJoin(t *testing.T) {
      orig := *ConfigDir
      *ConfigDir = "/tmp/conf"
      defer func() { *ConfigDir = orig }()

      cases := []struct{ path string; want bool }{
          {"/tmp/conf/10.conf", true},
          {"/tmp/conf/../etc/passwd", false},        // traversal
          {"/tmp/conf_dangerous/x", false},          // sibling, not inside
          {"/tmp/conf2/x", false},                   // prefix-collision
          {"/tmp/conf", true},                       // dir itself OK
      }
      for _, c := range cases {
          joined := filepath.Join(*ConfigDir, c.path)  // simulate the production Join
          if got := isSafePath(joined); got != c.want {
              t.Errorf("isSafePath(%q) = %v, want %v", joined, got, c.want)
          }
      }
  }
  ```
  Это тестирует A11-слой напрямую, без зависимости от substring-фильтра.

**Как верифицировать:**
1. На фиксе: тест PASS.
2. Мутация: в `isSafePath` (`dnsmasq.go:51-55`) заменить `strings.HasPrefix(cleanPath, cleanDir+...)` на `strings.HasPrefix(cleanPath, cleanDir)` (убрать разделитель) → тест FAIL на `/tmp/conf_dangerous/x` (prefix-collision case). Откатить.

**Acceptance criteria:** A11-слой `isSafePath` тестируется напрямую, с
комментарием, объясняющим, почему внешние векторы не покрывают этот слой.

---

## Задача P3.2 — Success-fake dnsmasq, который реально валидирует контент

**Контекст:** `fakeDnsmasq(t, 0)` в `linux_test.go` — это `#!/bin/sh\nexit 0`,
принимающий **любой** контент, включая мусор, который реальный dnsmasq
отверг бы. Поэтому success-path тесты (`TestWriteConfigWithTest_Success`,
`TestUpdateConfigHandler_Success`, etc.) проверяют wiring, но не валидацию.

**Файлы:**
- `linux_test.go` — рядом с `fakeDnsmasq` добавить:
  ```go
  // fakeDnsmasqStrict создаёт sh-скрипт, который читает конфиг из
  // --conf-file=<path> аргумента и выходит 1 если в нём найден маркер
  // invalid. Используется для тестов, где важно, что dnsmasq реально
  // валидирует содержание (а не просто вызывается).
  func fakeDnsmasqStrict(t *testing.T) string {
      t.Helper()
      dir := t.TempDir()
      binPath := filepath.Join(dir, "dnsmasq")
      // Имитирует валидацию: находит `# INVALID` в конфиге → exit 1
      script := `#!/bin/sh
conf=""
for arg in "$@"; do
    case "$arg" in
        --conf-file=*) conf="${arg#--conf-file=}" ;;
    esac
done
if [ -n "$conf" ] && [ -f "$conf" ]; then
    if grep -q '# INVALID' "$conf"; then
        echo "dnsmasq: invalid config"
        exit 1
    fi
fi
exit 0
`
      if err := os.WriteFile(binPath, []byte(script), 0755); err != nil {
          t.Fatalf("write strict fake dnsmasq: %v", err)
      }
      return binPath
  }
  ```
- Добавить тест, использующий strict-fake для проверки wiring + валидации:
  ```go
  func TestWriteConfigWithTest_StrictFakeRejectsInvalid(t *testing.T) {
      if runtime.GOOS == "windows" { t.Skip("linux-gated") }
      withDnsmasqBin(t, fakeDnsmasqStrict(t))
      // ... setup conf path inside ConfigDir ...
      err := writeConfigWithTest(confPath, []byte("# INVALID\ndhcp-host=..."))
      if err == nil {
          t.Errorf("expected dnsmasq --test to reject 'INVALID' marker")
      }
      if !strings.Contains(err.Error(), "dnsmasq_test_failed") {
          t.Errorf("expected dnsmasq_test_failed error, got: %v", err)
      }
  }
  ```

**Как верифицировать:**
1. На фиксе: тест PASS.
2. Мутация: в `writeConfigWithTest` (`dnsmasq.go:89`) убрать вызов `dnsmasq
   --test` (просто записать и вернуть nil) → тест FAIL. Откатить.

**Acceptance criteria:** wiring + content-валидация проверяются совместно.

---

## Задача P3.3 — Починить инвертированный комментарий про A8 в 80-metrics.sh

**Контекст:** `tests/suites/80-metrics.sh:11-15`:
```bash
if [ "$METRICS_NOAUTH_BODY_SIZE" -gt 2 ]; then
    check "A8: 401 has body (currently empty)" 1 1 A8 || true   # PASS
else
    check "A8: 401 has body (currently empty)" 1 0 A8 || true   # FAIL
fi
```
PASS-ветка требует body **non-empty** (>2 bytes), но описание чека и
комментарий говорят «(currently empty)». Это инверсия: если держатель кода
прочитает комментарий и «починит» тест, поменяв сравнение — он сломает
зелёный прогон. Сам A8 уже FIXED (`metrics.go:62` использует
`AbortWithStatusJSON` с body), A8 нет в `known-bugs.txt`.

**Файлы:**
- `tests/suites/80-metrics.sh:11-15` — выровнять описание с поведением:
  ```bash
  # A8 was: /metrics returned 401 with empty body. Fixed in metrics.go:62
  # (AbortWithStatusJSON now writes {"error":"auth_required"}).
  # Assert: body is non-empty (>2 bytes) AND contains "auth_required".
  if [ "$METRICS_NOAUTH_BODY_SIZE" -gt 2 ]; then
      BODY_CONTAINS=$(body | grep -c 'auth_required' || true)
      check "A8: 401 body non-empty AND contains auth_required" 1 "$BODY_CONTAINS"
  else
      check "A8: 401 body non-empty AND contains auth_required" 1 0
  fi
  ```
- Заодно убрать `|| true` на этих checks (A8 уже FIXED, не should-fail).

**Как верифицировать:**
1. На фиксе: smoke PASS.
2. Мутация: в `metrics.go:62` вернуть `c.AbortWithStatus(401)` (empty body) →
   A8-чек FAIL (не KNOWN-fail). Откатить.

**Acceptance criteria:** комментарий соответствует поведению; A8 —
honest regression (не KNOWN-fail).

---

## Задача P3.4 — 11-auth-ratelimit.sh: RL_BLOCKED-aware assertion

**Контекст:** `tests/suites/11-auth-ratelimit.sh:11-17`:
```bash
if [ "$RL_BLOCKED" = "1" ]; then
    check "..." 429 "$S" || true
else
    # ... "Mark as known-issue rather than hard fail"
    check "..." 429 "$S" || true      # IDENTICAL call, no bug_id
fi
```
Обе ветки идентичны (без bug_id). Если rate-limiter не срабатывает за цикл
(медленный CI / протухшее окно / параллельный successful login сбросил
счётчик), `$S` = 401 → hard FAIL без маскировки.

**Файлы:**
- `tests/suites/11-auth-ratelimit.sh:11-17` — разделить логику:
  ```bash
  if [ "$RL_BLOCKED" = "1" ]; then
      # Rate-limiter tripped within the loop — expected 429
      check "ratelimit blocks after N attempts" 429 "$S"
  else
      # Rate-limiter did NOT trip within the loop (slow CI, window expired).
      # Don't assert 429 (would fail); instead assert 401 (auth failed) as
      # the minimum sane behaviour.
      check "ratelimit slow-CI fallback: 429 OR 401" 401 "$S"
      echo "  note: rate-limiter did not trip within $N attempts (env-dependent)"
  fi
  ```

**Как верифицировать:**
1. На быстром CI: rate-limiter срабатывает → ветка `if`, 429 PASS.
2. На медленном CI / при локальном прогоне (если rate-limit окно протухло):
   ветка `else`, 401 PASS с информационной пометкой.

**Acceptance criteria:** suite не флапает на медленном CI.

---

## Задача P3.5 — 20-hosts-happy.sh: `grep -c || echo 0` даёт `"0\n0"`

**Контекст:** `tests/suites/20-hosts-happy.sh:29` (тот же паттерн в
`tests/suites/31-aliases-bugs.sh:8`):
```bash
LINES=$(grep -c "^dhcp-host=" "$FILE" || echo 0)
check "..." 4 "$LINES"
```
`grep -c` печатает `0` и exit 1, потом `echo 0` допечатывает ещё `0` →
`LINES` становится `"0\n0"`. Последующее сравнение `4 = "0\n0"` падает с
искажённым значением в сообщении. Сегодня маскируется тем, что 4 add succeed.

**Файлы:**
- `tests/suites/20-hosts-happy.sh:29`:
  ```bash
  LINES=$(grep -c "^dhcp-host=" "$FILE" || true)
  ```
  `grep -c` в случае no-match печатает `0` (на stdout) и exit 1; `|| true`
  подавляет ненулевой exit code, **не** допечатывая. `LINES` будет чистое
  `0`.
- Аналогично `tests/suites/31-aliases-bugs.sh:8` и любые другие вхождения
  этого паттерна (grep'нуть `grep -c.*echo 0` в `tests/`).

**Как верифицировать:**
1. На фиксе: smoke PASS.
2. Мутация: временно сломать `addHostHandler` возвращать 400 → `LINES = 0`,
   check-сообщение показывает чистый `0`, не `"0\n0"`. Откатить.

**Acceptance criteria:** failure-сообщения содержат корректные значения.

---

## Задача P3.6 — audit-tab.spec.ts: матчить MAC + действие, не только MAC

**Контекст:** `tests/e2e/specs/audit-tab.spec.ts:35-36`:
```typescript
const row = page.locator('tbody tr', { hasText: MAC })
await expect(row).toBeVisible({ timeout: 10000 })
```
`seedHosts` трактует 409 как success — если хост уже существует от
предыдущего прогона (state-leakage между test-runs), новый audit-entry не
пишется, но старая строка с тем же MAC всё ещё матчится. Spec проходит
вакуумно.

**Файлы:**
- `tests/e2e/specs/audit-tab.spec.ts:35-36` — добавить в `hasText` часть
  действия или timestamp-паттерн:
  ```typescript
  // Match MAC AND a recent action (e.g. "add" with a relative timestamp).
  // The audit table renders entries like "<time> <user> add <mac> <file>"
  // — pinning both ensures we caught THIS run's entry, not a stale one.
  const row = page.locator('tbody tr', {
    hasText: MAC,
  }).filter({
    hasText: /just now|\d+s ago|\d+m ago|add/i,
  })
  await expect(row).toBeVisible({ timeout: 10000 })
  ```
  Альтернатива (надёжнее) — до seed записать count строк, после — assert
  count увеличился ровно на 1:
  ```typescript
  const before = await page.locator('tbody tr', { hasText: MAC }).count()
  await seedHosts([{ mac: MAC, ... }])
  await page.reload()
  await expect.poll(async () =>
    page.locator('tbody tr', { hasText: MAC }).count()
  ).toBe(before + 1)
  ```

**Как верифицировать:**
1. На фиксе: spec PASS.
2. Мутация: во `writeAudit` (`audit.go:51`) return без записи → spec FAIL на
   count-delta. Откатить.

**Acceptance criteria:** spec ловит регрессию в writeAudit, а не только
«таблица рисуется».

---

## Задача P3.7 — templates-modal.spec.ts: `.nth()` → placeholder-якоря

**Контекст:** `tests/e2e/specs/templates-modal.spec.ts:27-30`:
```typescript
await modal.locator('input.form-control').nth(0).fill(NAME)
await modal.locator('input.form-control').nth(1).fill('10.99.99.0/24')
await modal.locator('input[placeholder="device-{NNN}"]').fill('e2e-{NNN}')
await modal.locator('input.form-control').nth(3).fill(`${CONF_DIR}/e2e-tpl.conf`)
```
Позиционные `.nth()` хрупки: если `dhcpRanges` станет непустым в e2e-env,
`<input>` для `ip_range` заменяется на `<select>`, индексы плывут, и поля
заполняются не туда молча.

**Файлы:**
- `tests/e2e/specs/templates-modal.spec.ts:27-30` — заменить на якорные
  селекторы:
  ```typescript
  await modal.locator('input[placeholder*="name" i]').fill(NAME)
  await modal.locator('input[placeholder="10.99.99.0/24"], select').first().fill('10.99.99.0/24')  // или .selectOption() если select
  await modal.locator('input[placeholder="device-{NNN}"]').fill('e2e-{NNN}')
  await modal.locator('input[placeholder*="hosts.conf" i]').fill(`${CONF_DIR}/e2e-tpl.conf`)
  ```
  Лучше всего — добавить `data-testid` в `TemplatesModal.vue` на каждый input:
  ```vue
  <input data-testid="tpl-name" ...>
  <input data-testid="tpl-ip-range" ...>
  <input data-testid="tpl-hostname-pattern" ...>
  <input data-testid="tpl-target-file" ...>
  ```
  и в spec использовать их — самый устойчивый к UI-рефакторингу подход.

**Как верифицировать:**
1. На фиксе: spec PASS.
2. Мутация: в `TemplatesModal.vue` поменять порядок input'ов → spec всё
   ещё PASS (потому что не зависит от позиции). Откатить.

**Acceptance criteria:** spec не зависит от порядка элементов в DOM.

---

## Задача P3.8 — Покрыть непокрытые endpoints в smoke-сьютах

**Контекст:** 6 эндпоинтов не имеют ни одного smoke-чека (только perf/L5 или
совсем ничего):
- `POST /api/hosts/apply-template` (`applyTemplateHandler`)
- `POST /api/leases/to-static` (`bulkLeaseToStaticHandler`)
- `GET /api/aliases` (`getAliasesHandler`) — покрывается P2.1 частично; всё
  равно добавить отдельный позитивный чек
- `POST /api/restart-self` (`inline main.go:262`)
- `POST /api/reload` (`reloadHandler`)
- `GET /api/events` (SSE) (`eventsHandler`)

**Файлы (предлагаемое распределение):**
- `tests/suites/28-hosts-apply-template.sh` (NEW):
  ```bash
  # POST /api/hosts/apply-template: валидный template ID + N → 200, count > 0
  ```
- `tests/suites/44-leases-to-static.sh` (NEW):
  ```bash
  # POST /api/leases/to-static: с fixture leases → 200 + count
  # ... (нужнаleases fixture, см. tests/fixtures/)
  ```
- `tests/suites/30-aliases-happy.sh` — расширить: после POST добавить
  `S=$(GET "$JWT" "/api/aliases")` + length-чек (P2.1).
- `tests/suites/91-restart-self.sh` (NEW) — рискованный, рестарт убивает
  сервер в не-CI режиме. Делать только с `-init-system=none`:
  ```bash
  # POST /api/restart-self: в ci-mode должно вернуть 200 + status:restarting
  # В не-ci-mode это убьёт сервер, поэтому гоняется только если detect_ci_mode.
  if [ "$CI_MODE" = "1" ]; then
      S=$(POST "$JWT" "/api/restart-self")
      check "POST /api/restart-self returns 200 in ci-mode" 200 "$S"
  fi
  ```
- `tests/suites/92-reload.sh` (NEW) или расширить `00-preflight.sh`:
  ```bash
  S=$(POST "$JWT" "/api/reload")
  check "POST /api/reload returns 200 or 400 (no dnsmasq)" "$S" 200 "$S"
  # 400 тоже допустимо (no dnsmasq installed) — loosen check
  ```
- `tests/suites/93-events-sse.sh` (NEW):
  ```bash
  # GET /api/events: подключиться, дождаться первого event'а за 10s, дисконнект
  timeout 10 curl -sN -H "Authorization: Bearer $JWT" "$BASE/api/events" | head -n 1 | grep -q "event:" \
      && check "SSE /api/events emits initial event" 1 1 \
      || check "SSE /api/events emits initial event" 1 0
  ```

**Как верифицировать:**
1. Каждый новый suite проходит.
2. `tests/smoke.sh` интегрирует их автоматически (по lexical-порядку NN-).

**Acceptance criteria:** все эндпоинты из `main.go:250-316` имеют хотя бы один
smoke-чек.

---

## Задача P3.9 — system_callers_test.go: оставить честную vanity-маркировку

**Контекст:** `system_callers_test.go` имеет в шапке честный комментарий
«цифра coverage растёт, доверие — нет». Это правильно. Задача — не чинить
(там нечего чинить), а **зафиксировать в ROADMAP**, что для реального
тестирования SystemCaller нужно гнать L5 VM (Gap 4), а unit-тесты с
fake-бинарниками дают только statement-coverage.

**Файлы:**
- `tests/ROADMAP.md` — в разделе «Что осталось» (строка ~60) или рядом с
  Gap 4 (строка ~102) добавить примечание:
  ```
  > **Note on system_callers_test.go:** unit-тесты с fake-бинарниками на PATH
  > дают statement-coverage для SystemCaller (sudoDispatch, argv-construction,
  > output-parsing), но НЕ дают функциональной уверенности в реальных
  > systemctl/rc-service/sv семантиках. Для последнего — только L5 real-VM
  > (Gap 4, opt-in run_l5_vm_tests). При рефакторинге system.go полагаться
  > ТОЛЬКО на L5, не на system_callers_test.go.
  ```

**Как верифицировать:** N/A (документационная правка).

**Acceptance criteria:** при будущем рефакторинге `system.go` разработчик
явно предупреждён, что unit-тесты там — vanity, и нужно гнать L5.

---

## Задача P3.10 — Финальная верификация фазы P3

После задач P3.1–P3.9 прогнать полный цикл:

1. **Go:**
   ```bash
   go vet ./...
   go test ./... -count=1
   go test ./... -race -count=1
   ```
2. **Bash smoke:**
   ```bash
   BASE=http://localhost:18081 ./tests/smoke.sh
   ```
   Новые suites (28/44/91/92/93) PASS, остальные не сломаны.
3. **Playwright:**
   ```bash
   cd tests/e2e && npx playwright test
   ```
   audit-tab / templates-modal specs PASS.
4. **Коммит:** `predrel-test-remediation-P3: polish + endpoint coverage`.

**Acceptance criteria фазы P3:** тесты устойчивы к UI/config refactor'ам,
комментарии соответствуют поведению, все эндпоинты покрыты. Готов к v1.0
release.
