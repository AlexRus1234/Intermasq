# Hardening sweep — промт на закрытие остаточных gap'ов

**Назначение:** самодостаточный промт для ИИ-ассистента (или будущего себя),
чтобы за одну сессию закрыть 4 остаточные задачи тестового покрытия и
харденинга. По формату — продолжение `логи/Bugfix_sweep.md` (который закрыл
баги A1/A2/A3/A4/A6/A8/A12). Специфика/история — в `tests/ROADMAP.md` и
`tests/bugreport/bugs.md`; ИСПОЛНЯЕМЫЙ промт — здесь. Прочитай ЦЕЛИКОМ
перед стартом; не импровизируй там, где даны конкретные файлы/фиксы.

---

## 0. Что за проект и где мы

**Intermasq** — веб-панель для dnsmasq. Backend Go 1.25 (gin), frontend
Vue 3 + Bootstrap 5, embed фронтенда через `go:embed`. Репо:
`B:\Repo\Intermasq\Intermasq`, ветка `main`. CI — Forgejo Actions, контейнер
`fedora:44`, runs as root, npm/go/rpm через внутренний прокси Nora.

**Состояние на старте (после Bugfix sweep 2026-07-28):**
- L1+L2 Go: **65.6%** coverage (`go test -cover ./...`), 241+ тестов, `-race` чист.
- L3 smoke.sh: **0 Fail / 0 Known-fail** (7 багов закрыто, в `known-bugs.txt`
  остался только A11 с пометкой wontfix/hardening).
- L4 Playwright: 33 spec'а (31 pass + 2 skip). Mutation-pass пройден.
- L5 Real VM: 0% (Gap 4, отдельная задача — НЕ этой сессии).
- `tests/known-bugs.txt`: только A11.

**Цель сессии:** закрыть 4 остаточные задачи (в порядке ROI):
1. **Fuzzing парсеров** (P2, ~+2-3% coverage, 0.5 дня).
2. **A11 path-traversal hardening** (LOW, defense-in-depth, снимает wontfix).
3. **Усилить 2 слабых Playwright spec'а** (`hosts-sort`, `auth`) —
   mutation-pass нашёл, что они проходят даже при сломанном коде.
4. **2 infra-spec'а** (`setup-screen`, `sse-live`) — сейчас `test.skip`,
   требуют CI-инфра изменения.

---

## 1. ЖЁСТКИЕ ограничения (не нарушать)

1. **Перед пушем ЛЮБОЙ Go-правки — `go vet ./...` ОБЯЗАТЕЛЬНО** (не только
   `go build`). CI гоняет vet и режет unreachable code и др.; локальный
   `go build` это не ловит (был красный прогон из-за ранних `return`).
2. **Не ломай существующие тесты.** После каждой задачи локально:
   `$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXX"; go test ./... -race -count=1`.
3. **Продуктовые правки — строго в объёме фикса.** Никакого рефакторинга
   сверх сказанного. Особенно: A11 НЕ меняет публичное поведение
   эндпоинтов (все 9 smoke-векторов в `81-path-traversal.sh` должны
   остаться зелёными с теми же статусами).
4. **Fuzz-тесты — это `func FuzzXxx(t *testing.T, f fuzzing.X)`** (Go 1.18+
   native). НЕ тащить `go-fuzz`/`daprand`/внешние fuzz-фреймворки. Корпус —
   в `testdata/corpus/<FuzzName>/` (по файлу на кейс).
5. **Fuzz oracle = «не паникует + структурные инварианты на успех».** Не
   пытайся строить эталонный вывод — парсеры принимают мусор как
   legitimate input (озвращают `ok=false` или пустую коллекцию). Цель —
   ловить panic, бесконечные циклы, race, nil-deref.
6. **Playwright-спеки работают против ОДНОГО сервера :18083** (workers:1,
  `tests/e2e/playwright.config.ts:18`). setup-screen и sse-live infra-варианты
   требуют ДОПОЛНИТЕЛЬНЫХ инстансов/фикстур — меняется CI yml, НЕ конфиг.
7. **CI changes — в `.forgejo/workflows/build.yml`**, opt-in input по
   образцу `run_e2e_tests` (строки 26-27, 210-263). Не делай fuzz/e2e
   дефолтным — это удлиняет пайплайн.
8. **Коммиты — после `go vet` + `go test` зелёного.** Пуш — по просьбе
   оператора (по умолчанию — в конце сессии). CI подтверждает.
9. **Синхронизируй документы:** `tests/ROADMAP.md` (отметь задачи закрытыми),
   `tests/bugreport/bugs.md` (A11 → FIXED с rationale), session-лог
   `логи/hardening-sweep.md` в конце.

---

## 2. Задачи (исполняемый список)

По убыванию ROI. Для каждой: файл(ы) → корень → фикс → тест → knock-on.

### T1. Fuzzing парсеров (~+2-3% coverage, P2)

**Целевые функции (4 парсера):**

| Функция | Файл:строка | Сигнатура | Готова к fuzz? |
|---|---|---|---|
| `parseDhcpHostLine` | `dnsmasq.go:116` | `(raw, file string) (HostEntry, bool)` | ✓ pure |
| `parseArpContent` | `arp_leases.go:43` | `(content string) map[string]bool` | ✓ pure |
| `parseAliasLine` | `aliases.go:57` | `(line, file string, hasBak bool) (DnsAliasEntry, bool)` | ✓ pure |
| `parseLeasesContent` | `arp_leases.go` (НОВАЯ) | `(content string) []LeaseEntry` | нужен рефакторинг (см. ниже) |

**T1.1. Рефакторинг `parseLeases` (ОБЯЗАТЕЛЬНО перед fuzz).**

Текущий `parseLeases` (`arp_leases.go:58`) читает файл напрямую через
`os.Open(*LeasesPath)` — не fuzzится. Вынести тело в чистую функцию:

```go
// parseLeasesContent parses the textual content of a dnsmasq.leases file
// (whitespace-separated: timestamp MAC IP [hostname] [client-id]).
func parseLeasesContent(content string) []LeaseEntry {
	leases := []LeaseEntry{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 {
			l := LeaseEntry{Ip: fields[2], Mac: fields[1]}
			if len(fields) > 3 {
				l.Hostname = fields[3]
			}
			leases = append(leases, l)
		}
	}
	return leases
}

func parseLeases() []LeaseEntry {
	data, err := os.ReadFile(*LeasesPath)
	if err != nil {
		return []LeaseEntry{}
	}
	return parseLeasesContent(string(data))
}
```

Поведение `parseLeases()` должно остаться идентичным (тот же формат вывода).
Существующие тесты (`dnsmasq_test.go` — поиск по `parseLeases`) это подтвердят.

**T1.2. Fuzz-функции.** Новый файл `fuzz_test.go` (или добавить в конец
`dnsmasq_test.go` — на твоё усмотрение, но отдельный файл чище). По одной
`FuzzXxx` на парсер:

```go
package main

import "testing"

// FuzzParseDhcpHostLine гарантирует, что парсер не паникует на любом
// вводе, и что успешный результат round-trip'ит: MAC валиден по macRegex
// и formatDhcpHostLine(entry) содержит entry.Mac.
func FuzzParseDhcpHostLine(f *testing.F) {
	// Seed corpus — реальные и краевые случаи.
	seeds := []string{
		"dhcp-host=aa:bb:cc:dd:ee:ff,nas,10.0.0.1",
		"dhcp-host=aa:bb:cc:dd:ee:ff",
		"dhcp-host=aa-bb-cc-dd-ee-ff,10.0.0.1",
		"dhcp-host:11:22:33:44:55:66,set:iot",
		"dhcp-host=",
		"not-a-dhcp-host-line",
		"",
		"dhcp-host=" + strings.Repeat(",", 1000),
	}
	for _, s := range seeds { f.Add(s, "/etc/dnsmasq.d/x.conf") }
	f.Fuzz(func(t *testing.T, raw, file string) {
		entry, ok := parseDhcpHostLine(raw, file)
		if !ok { return }
		// Инварианты на успех:
		if !macRegex.MatchString(normalizeMAC(entry.Mac)) {
			t.Errorf("parsed MAC %q fails macRegex (raw=%q)", entry.Mac, raw)
		}
		if entry.File != file { t.Errorf("File not propagated: got %q want %q", entry.File, file) }
		// Round-trip: отформатированная строка как минимум содержит MAC.
		out := formatDhcpHostLine(entry)
		if !strings.Contains(out, entry.Mac) {
			t.Errorf("formatDhcpHostLine round-trip lost MAC: %q", out)
		}
	})
}
```

Аналогично:
- `FuzzParseArpContent(content string)` — инвариант: ключи возвращённой
  map непустые и lowercase. Seed: реальный `/proc/net/arp` (см.
  `tests/fixtures/arp-sample.txt`), пустая строка, только заголовок,
  строки с мусорными полями, длина полей < 4, флаги != "0x2".
- `FuzzParseAliasLine(line, file string, hasBak bool)` — инвариант: если
  `ok`, то `aliasToLine(entry)` перетранслируется в эквивалентный entry
  (тип+домен+target совпадают; file = file или file+"|has_bak"). Seed: все 4
  типа (address=/cname=/ptr-record=/txt-record=), wildcard `address=/#/`,
  malformed, TXT с quoted value, multiline, пустой.
- `FuzzParseLeasesContent(content string)` — инвариант: каждый lease
  имеет непустые `Ip` и `Mac`, `len(fields)>=3`-семантика соблюдена. Seed:
  реальный leases-файл, пустой, одна строка без полей, строки с >4 полей
  (client-id), unicode в hostname.

**T1.3. Seed corpus в `testdata/corpus/`.** Для каждого Fuzz-теста создать
директорию `testdata/corpus/<FuzzName>/` с файлами-seed'ами (по одному на
кейс, имена — любые уникальные). Go's fuzz engine их подхватит. Несколько
краевых: пустой ввод, очень длинная строка (10k+), только-разделители,
unicode, NUL-байты, CR/LF/CR-LF микс.

**T1.4. CI интеграция (ОПЦИОНАЛЬНО, opt-in input).** По образцу
`run_e2e_tests` (`build.yml:26-27`) добавить input `run_fuzz_tests` и шаг:

```yaml
      - name: Fuzz parsers (Gap 6, opt-in, time-bounded)
        if: success() && github.event.inputs.run_fuzz_tests == 'true'
        run: |
          for target in FuzzParseDhcpHostLine FuzzParseArpContent FuzzParseAliasLine FuzzParseLeasesContent; do
            echo "::group::fuzz $target (30s)"
            go test -run='^$' -fuzz="^${target}$" -fuzztime=30s ./...
            echo "::endgroup::"
          done
```

`-run='^$'` отключает обычные тесты, `-fuzztime=30s` — бюджет на каждый
target. Без явного `-fuzz` (дефолтный прогон) fuzz-тесты работают как
обычные unit-тесты по seed-корпусу — это БЕСПЛАТНО добавляет ~20-30 кейсов
к `go test ./...`.

**Регрессия:** `go test ./... -count=1` прогонит seed corpus как unit-тесты.
`go test -fuzz=FuzzParseDhcpHostLine -fuzztime=60s -run='^$' ./...` —
локальный fuzz (60s на target). Если найдёт crash — фикс парсера + кейс в
`testdata/corpus/<FuzzName>/` (Go сохранит автоматически в
`testdata/fuzz/<FuzzName>/`).

**Knock-on:** рефакторинг `parseLeases` может зацепить тесты, которые
мокают `os.Open(*LeasPath)`. Проверь `rg "parseLeases|LeasesPath" *_test.go`.

---

### T2. A11 path-traversal hardening (LOW, defense-in-depth)

**Контекст:** большинство векторов уже закрыто через `isSafePath`
(`dnsmasq.go:51`). Smoke `tests/suites/81-path-traversal.sh` покрывает 9
векторов — **все зелёные**. Это не активная уязвимость, а hardening:
привести 2 «дырявых» call site'а к тому же паттерну, что и остальные 22.

**T2.1. Уязвимые call sites (2 шт).**

`getFileHandler` и `putFileHandler` (`handlers_config.go:197-241`) —
единственные эндпоинты, принимающие путь через URL-параметр `:name`
(`c.Param("name")`) и НЕ вызывающие `isSafePath` после `filepath.Join`:

```go
func getFileHandler(c *gin.Context) {
	name := c.Param("name")
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || filepath.Ext(name) != ".conf" {
		c.JSON(403, gin.H{"error": "access_denied"})
		return
	}
	path := filepath.Join(*ConfigDir, name)  // <-- НЕТ isSafePath(path)
	content, err := readFileRaw(path)
	...
}
```

Текущая защита — substring-чек на `/`/`\` + extension `.conf`. Это
работает (сепараторов нет → `Join` не может вылезти за `ConfigDir`), но:
- (a) это defense-by-coincidence, а не единый chokepoint;
- (b) если кто-то позже ослабит substring-чек или добавит aliasing —
  `isSafePath` поймает;
- (c) Go's `net/http` чистит `..` до router'а, но это framework-level
  защита (см. коммент в `81-path-traversal.sh:13-21`), её может не быть
  в не-HTTP attack vectors (плагины через Unix-сокет).

**Фикс** — добавить `isSafePath(path)` после `filepath.Join`, вернуть 403
`access_denied` (тот же статус/тело, что и substring-чек, чтобы smoke
не зависел от того, КАКАЯ именно защита сработала):

```go
path := filepath.Join(*ConfigDir, name)
if !isSafePath(path) {
	c.JSON(403, gin.H{"error": "access_denied"})
	return
}
```

В ОБОИХ хендлерах (`getFileHandler` и `putFileHandler`). После substring-
фильтра и до `readFileRaw`/`writeFileRaw`.

**Регрессия:** новый L2-тест в `handlers_test.go` — `TestGetFileHandlerRejectsUnsafePath`
и `TestPutFileHandlerRejectsUnsafePath`. Создать mock через прямый вызов
handler'а с `c.Params = gin.Params{{Key: "name", Value: ".."}}` (substring
поймает первым), плюс кейс с валидным-на-первый-взгляд именем, которое
после Join уходит за пределы (если возможно придумать — иначе ограничиться
substring cases). Главное —.lock что 403 возвращается.

**Knock-on / проверка:**
- Smoke `81-path-traversal.sh` — все 9 векторов должны остаться с теми же
  статусами (404 для Go-cleaned `..`, 403 для PUT non-.conf, 400 для
  host/alias file=/etc/*, 403 для aliases traversal, 400 для history).
- `isSafePath` в `readFileRaw`/`writeFileRaw` (`dnsmasq.go:60,70,90`) —
  double-check, они уже проверяют. Получится triple-check (handler +
  isSafePath-after-Join + readFileRaw/writeraw) — это норм, defense-in-depth.

**T2.2. Обновить `tests/bugreport/bugs.md` (A11 → FIXED)** и **удалить A11
из `tests/known-bugs.txt`** (там сейчас единственная строка). Если после
удаления файл пуст — оставь заголовочный комментарий (см. начальные строки
`known-bugs.txt`), чтобы структура сохранилась.

---

### T3. Усилить 2 слабых Playwright spec'а

Mutation-pass (см. `логи/gap2-finish.md` Блок C) показал: эти 2 spec'а
проходят даже со сломанным кодом — то есть они guard'ы, а не regressions.

**T3.1. `tests/e2e/specs/hosts-sort.spec.ts` — добавить assert порядка.**

Сейчас (43 строки): сеет 5 хостов `aa:11:11:11:11:01..05` с IP
`10.99.{1..5}.2`, кликает `th IP` 3 раза, после каждого клика проверяет
`toHaveCount(5)`. Если сортировка сломана (например, A1-регрессия вернулась
и строки дублируются), count всё ещё может быть 5 если дубли фильтруются —
тест проходит впустую.

**Фикс:** после кликов проверять ПОРЯДОК. Поведение сортировки
(`HostTable.vue:82-95`):
- `sortKey=ref('ip')`, `sortAsc=ref(true)` (начальное: ascending по IP).
- Клик по тому же ключу → toggle `sortAsc`. Клик по новому → `sortAsc=true`.

Assert-блок (после seed-блока):

```ts
// Извлечь MAC postfix (последние 2 цифры) из каждой строки в порядке отображения.
const visibleOrder = async () => {
  const cells = await page.locator('tbody tr td.font-monospace', { hasText: PREFIX }).all()
  return Promise.all(cells.map((c) => c.textContent())).then((ts) =>
    ts.map((t) => t!.slice(-2)),
  )
}

// 1) Клик по IP при начальном sortKey='ip', sortAsc=true → toggles to descending.
await page.locator('th', { hasText: 'IP' }).click()
expect(await visibleOrder()).toEqual(['05', '04', '03', '02', '01'])

// 2) Снова IP → ascending.
await page.locator('th', { hasText: 'IP' }).click()
expect(await visibleOrder()).toEqual(['01', '02', '03', '04', '05'])

// 3) Клик по Hostname (новый ключ → sortAsc=true → ascending по hostname sort1..sort5).
await page.locator('th', { hasText: 'Hostname' }).click()
expect(await visibleOrder()).toEqual(['01', '02', '03', '04', '05'])

// 4) Снова Hostname → descending.
await page.locator('th', { hasText: 'Hostname' }).click()
expect(await visibleOrder()).toEqual(['05', '04', '03', '02', '01'])
```

Оставить existing count-asserts как pre-condition (5 строк есть). Обновить
коммент в шапке spec'а: теперь это regression, не только guard.

**Knock-on / риски:** `page.locator('tbody tr td.font-monospace', { hasText: PREFIX })`
— этот селектор берёт ЯЧЕЙКИ MAC (`.font-monospace` = MAC column,
`HostTable.vue:35`). Если в таблице есть leftover-хосты от других спек с тем
же prefix'ом — порядок сломается. Seed-prefix `aa:11:11:11:11` достаточно
уникален; но если в CONF_DIR остались хвосты от прежних прогонов, добавить
фильтр по `cleanPath(h.file) === '<CONF_DIR>/e2e-sort.conf'` (search-filter
по имени файла не делается — придётся полагаться на prefix уникальность).
Если mutation-pass (Блок C) ронял именно `hosts-sort` мутацией `applyConfig`
— убедись, что новый order-assert тоже падает на этой мутации.

**T3.2. `tests/e2e/specs/auth.spec.ts` — добавить assert 401 после logout.**

Сейчас (31 строки): login → dashboard visible → logout → `.btn-primary`
visible. Проблема: `.btn-primary` рендерится И на login, И (возможно) на
dashboard (если там есть primary-кнопки) → assertion слабый.

**Фикс:** после клика logout дополнительно:
1. Проверить, что `localStorage.getItem('token') === null` (`store.js`
   `logout()` удаляет токен).
2. Проверить, что следующий API-запрос возвращает 401:

```ts
// После .dropdown-item.text-danger click и подтверждения login-screen:

// Сильный assert: токен должен быть удалён из localStorage.
const token = await page.evaluate(() => localStorage.getItem('token'))
expect(token).toBeNull()

// Сильный assert: следующий API-запрос без токена → 401.
const r = await page.evaluate(() => fetch('/api/hosts').then((x) => x.status))
expect(r).toBe(401)
```

Заменить (или дополнить) существующий `.btn-primary visible`-assert. Если
оставляешь `.btn-primary` — убедись, что он scoped до auth-screen
(`AuthScreen.vue`), а не падает на dashboard.

**Knock-on / риски:** `auth.spec` использует `test.use({ storageState: ... })`
с пустым стейтом (строка 10). После logout token удаляется — это уже в
page context, не в исходном storageState. `fetch('/api/hosts')` идёт без
Authorization header → 401. Если axios-перехватчик в `store.js` пытается
refresh — его нет (JWT без refresh-токена), так что 401 гарантируется.

---

### T4. 2 infra-spec'а (`setup-screen`, `sse-live`) — разблокировать

Сейчас оба `test.skip` с комментарием «нужна CI-инфра». Это требует правок в
`.forgejo/workflows/build.yml` (opt-in L4 шаг, строки 210-263) — НЕ в
`playwright.config.ts` (там остаётся 1 проект chromium, 1 worker).

**T4.1. `tests/e2e/specs/setup-screen.spec.ts` — 2-я инстанция :18084.**

Текущий spec (18 строк) скипует тест. Чтобы разблокировать:

**Шаг 1 — CI yml.** В L4 шаге (`build.yml:228-263`) ПОСЛЕ основного
сервера :18083 и ДО `npx playwright test`, поднять второй инстанс на
`:18084` со СВЕЖИМ `-db` (чтобы `/api/status` вернул `setup_required:true`):

```yaml
          # 2-я инстанция для setup-screen spec (fresh user DB → setup_required).
          rm -f /tmp/e2e-setup-users.json
          ./intermasq-ci \
            -port 18084 \
            -conf-dir /tmp/e2e-conf-setup \
            -db /tmp/e2e-setup-users.json \
            -init-system=none -ci-mode=true &
          E2E_SETUP_PID=$!
          mkdir -p /tmp/e2e-conf-setup
          for i in $(seq 1 10); do
            if curl -sf http://localhost:18084/api/status >/dev/null 2>&1; then break; fi
            sleep 1
          done
```

В export-строке перед `npx playwright test` добавить `E2E_SETUP_BASE_URL=http://localhost:18084`.
Перед `kill $E2E_PID` в cleanup — `kill $E2E_SETUP_PID 2>/dev/null || true`.
Также `rm -rf /tmp/e2e-conf-setup` в начале для повторяемости.

**Шаг 2 — spec.** Заменить `test.skip(...)` на реальный тест, читающий
`E2E_SETUP_BASE_URL` (если env не задан — `test.skip` с понятной причиной,
чтобы локальные запуски не падали):

```ts
import { test, expect } from '@playwright/test'

const SETUP_URL = process.env.E2E_SETUP_BASE_URL

test('setup-screen: first-run admin setup', async ({ browser }) => {
  test.skip(!SETUP_URL, 'needs E2E_SETUP_BASE_URL (2nd intermasq instance :18084)')
  const page = await browser.newPage({ baseURL: SETUP_URL })
  await page.goto('/')
  // AuthScreen.vue в setup-mode показывает 2 input.form-control + .btn-primary.
  await page.locator('input.form-control').nth(0).fill('setupadmin')
  await page.locator('input.form-control').nth(1).fill('setuppass1234')
  await page.locator('.btn-primary').click()
  // После setup токен выдаётся → редирект на dashboard.
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })
  await page.close()
})
```

Обрати внимание: тест создаёт СОБСТВЕННЫЙ page (не использует fixture-ный
`page`) с явным `baseURL: SETUP_URL`, потому что дефолтный baseURL
конфига = `http://localhost:18083`. Использование `browser.newPage`
обходит globalSetup storageState (который для :18083).

**Knock-on / риски:**
- Пароль `setuppass1234` — должен пройти любые парольные проверки в
  `setupHandler` (`handlers.go`). Проверь там минимальную длину/сложность.
- setupHandler может требовать, чтобы `setup_required==true` — это даёт
  fresh `-db` (несуществующий файл). Если `loadUsers()` создаёт пустой
  файл при старте, то `/api/status` может показать `setup_required:false`
  — тогда тест зависнет на dashboard-assert. Проверь логику `statusHandler`
  (`handlers.go`) — счётчик `len(users)` под `usersMu.RLock()`.
- Имя `setupadmin` НЕ должно конфликтовать с `admin` на основном сервере
  (это разные -db файлы, но визуально путает — лучше `setupadmin`).

**T4.2. `tests/e2e/specs/sse-live.spec.ts` — writable -arp-file.**

Текущий spec (30 строк) — упрощённый (200 + content-type). Полный вариант
требует, чтобы intermasq запустился с WRITABLE `-arp-file` (сейчас
`tests/fixtures/arp-sample.txt` read-only), spec mid-test дописывает
ARP-строку и наблюдает новую 🟢 через SSE delta.

**Шаг 1 — CI yml.** Заменить в L4 шаге:
```yaml
            -arp-file tests/fixtures/arp-sample.txt \
```
на копирование в writable location:
```yaml
          cp tests/fixtures/arp-sample.txt /tmp/e2e-arp.txt
          ./intermasq-ci \
            ... \
            -arp-file /tmp/e2e-arp.txt \
```
В export-строку добавить `ARP_FILE=/tmp/e2e-arp.txt`.

**Шаг 2 — spec.** Сохранить текущий упрощённый assertion (200 +
content-type) как первый test, ДОБАВИТЬ второй — full variant:

```ts
import { test, expect } from '@playwright/test'
import { appendFileSync } from 'node:fs'

const ARP_FILE = process.env.ARP_FILE

test('sse-live: /api/events streams under Bearer auth', async ({ page }) => {
  // ... (существующий упрощённый тест — без изменений)
})

test('sse-live: appended ARP entry surfaces as new online dot', async ({ page }) => {
  test.skip(!ARP_FILE, 'needs ARP_FILE env (writable -arp-file)')

  const token = await apiLogin()
  // Seed host whose MAC is NOT in arp-sample → в UI он изначально 🔴.
  const NEW_MAC = '99:88:77:66:55:01'
  await seedHosts(token, [{
    mac: NEW_MAC, ip: '10.99.99.1', hostname: 'sse-target', file: `${CONF_DIR}/e2e-sse.conf`,
  }])

  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Стартовый assert: наша строка показывает 🔴 (offline).
  const offlineDot = page.locator('tr', { hasText: NEW_MAC }).locator('span.text-muted')
  await expect(onlineDot).toBeVisible({ timeout: 15000 })

  // Мутируем ARP-файл: дописываем NEW_MAC как 0x2 (reachable).
  appendFileSync(ARP_FILE!,
    `10.99.99.1     0x1         0x2         ${NEW_MAC}     *        eth0\n`)

  // SSE-бродкастер опрашивает каждые 5s (sse.go:78). Ждём до 20s (4 цикла)
  // пока 🟢 не появится.
  const onlineDot = page.locator('tr', { hasText: NEW_MAC }).locator('span.text-success')
  await expect(onlineDot).toBeVisible({ timeout: 20000 })
})
```

**Knock-on / риски:**
- `appendFileSync` меняет файл; это загрязняет `/tmp/e2e-arp.txt` между
  прогонами — CI создаёт его свежим каждый раз (`cp`), но локально надо
  чистить. Можно в `test.afterAll` откатить (прочитать, вырезать строку),
  но проще положиться на CI-пересоздание.
- `NEW_MAC = '99:88:77:66:55:01'` — НЕ должен пересекаться с arp-sample
  (там `11:22:33:44:55:0X` и `aa:bb:cc:dd:ee:01`). Уникальный prefix обязателен.
- 5s SSE polling — явно засечь в `timeout: 20000` (4 цикла, безопасно).
- `span.text-success` / `span.text-muted` — селекторы из `HostTable.vue:32-33`
  (🟢/🔴 online-indicator). Если хост не показывается в активной вкладке
  (пагинация?) — подожди загрузки: после `goto('/')` таблица рендерится по
  `actions.loadData()` (`App.vue:54`).
- Если несколько спеков мутируют ARP-файл параллельно — конфиг требует
  `workers:1` (уже так), так что гонки нет.

---

## 3. Верификация (после каждой задачи + финальная)

**Локально (Windows):**
- `go vet ./...` — ЧИСТО (обязательно).
- `$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXX"; go test ./... -race -count=1` — зелёный.
- T1: `go test -run='^Fuzz' -count=1 ./...` (seed corpus как unit-тесты).
- T1 (опционально): `go test -fuzz=FuzzParseDhcpHostLine -fuzztime=30s -run='^$' ./...` × 4 target'а.
- T2: `go test -run "TestGetFileHandler|TestPutFileHandler" -count=1 ./...`.
- T3/T4: локально НЕ запустить (нужен Linux + dnsmasq + chromium) —
  полагайся на CI L4 opt-in шаг.

**CI (основная для T3/T4):**
- Дефолтный прогон: `go test` зелёный, `go vet` чист, smoke.sh 0 Fail/0
  Known-fail (T2 не должен сломать 9 векторов в `81-path-traversal.sh`).
- Opt-in `run_e2e_tests=true`: 33 spec'а → 35 spec'ов (2 unskip'нуто),
  все зелёные. Из них `hosts-sort` и `auth` усилены (T3).
- Opt-in `run_fuzz_tests=true` (если добавишь): 4 fuzz target'а × 30s,
  без crash.

---

## 4. Приёмка (definition of done)

- [ ] **T1:** 4 `FuzzXxx` функции (после рефакторинга `parseLeases` →
      `parseLeasesContent`) + seed corpus в `testdata/corpus/`. Seed corpus
      проходит как unit-тесты. Опционально: opt-in CI input.
- [ ] **T2:** `getFileHandler` и `putFileHandler` вызывают `isSafePath`
      после `filepath.Join`. Smoke `81-path-traversal.sh` — те же 9
      статусов. A11 → FIXED в `bugs.md`, удалён из `known-bugs.txt`.
- [ ] **T3:** `hosts-sort.spec.ts` asserts порядка (4 клика, 4 порядка).
      `auth.spec.ts` asserts `localStorage.token===null` + 401 на /api/hosts.
      Оба остаются зелёными.
- [ ] **T4:** `setup-screen.spec.ts` и `sse-live.spec.ts` — `test.skip`
      заменён на реальные тесты, читающие env (`E2E_SETUP_BASE_URL` / `ARP_FILE`).
      CI yml поднимает 2-ю инстанцию :18084 и writable ARP_FILE. Оба зелёные
      в opt-in L4.
- [ ] `go vet ./...` и `go test ./... -race` зелёные.
- [ ] `tests/ROADMAP.md`: P2 (fuzzing, A11) + Gap 2 «Усилить 2 spec'а» +
      «infra-specs» отмечены закрытыми.
- [ ] Session-лог `логи/hardening-sweep.md` (контекст → по-задачно: фикс →
      верификация → результат).

---

## 5. Порядок исполнения (рекомендация)

1. **T2 (A11 hardening)** — 15 мин, 2 call site'а + 2 L2-теста, снимает
   wontfix с known-bugs. Быстрый старт, самая дешёвая задача.
2. **T1 (Fuzzing)** — 0.5 дня: рефакторинг parseLeases → 4 FuzzXxx →
   seed corpus. Опционально CI input. Самый высокий coverage ROI.
3. **T3 (Playwright усиление)** — 1 час: 2 spec'а, локально можно
  syntax-check (npm install в tests/e2e + npx tsc --noEmit если есть
   tsconfig; иначе полагайся на CI).
4. **T4 (infra-specs)** — 2-3 часа: правки CI yml + 2 spec'а. Самая
   трудоёмкая, зависит от L4 opt-in прогона (медленный feedback).

На каждой задаче — `go vet` (для T1/T2) + коммит. Для T3/T4 коммит после
сборки без TS-ошибок; финальную уверенность даст CI L4 прогон.

---

## 6. ВНЕ области этой сессии

- **Gap 4 (L5 Real VM nightly)** — отдельная задача, см. `tests/ROADMAP.md`
  «Что осталось → Gap 4». Нужна persistent VM, не локально.
- **Go coverage → 70%** — fuzzing добавит ~2-3%, но это не дотянёт до 70%
  (сейчас 65.6%). Остаток — edge-case тесты в `bins.go`, `reloadDnsmasq`,
  `startDNSHealthChecker`. Отдельная сессия.
- **A10 (Discovered-devices IP)** — feature gap, отдельный PR.
- **README / CHANGELOG** — документация, отдельная задача.
- **Enterprise-grade** (mutation testing, compatibility matrix, cross-distro,
  browser matrix) — post-v1.0.
- Любой рефакторинг продуктовых исходников сверх описанного — запрещён
  (исключение: `parseLeases` refactor в T1.1 — необходим для fuzz).
- Если найдёшь НОВЫЙ баг в ходе fuzz (crash) — зафиксируй в
  `tests/bugreport/bugs.md` с новым ID (A14+) и `tests/known-bugs.txt`,
  почини, добавь regression-тест. Не глушай fuzz, чтобы спрятать баг.
