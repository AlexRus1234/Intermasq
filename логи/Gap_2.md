# Gap 2 — Playwright E2E: промт на реализацию

**Назначение:** самодостаточный промт для ИИ-ассистента (или будущего себя),
чтобы реализовать первую итерацию UI/E2E-тестов (Gap 2) без двойных
толкований. Прочитай целиком перед стартом; не импровизируй там, где даны
конкретные селекторы/инварианты.

---

## 0. Что это за проект и что делаем

**Intermasq** — веб-панель для dnsmasq. Backend Go (gin), frontend Vue 3 +
Bootstrap 5, embed фронтенда в бинарь через `go:embed`. Репозиторий:
`B:\Repo\Intermasq\Intermasq`, ветка `main`. CI — Forgejo Actions, контейнер
`fedora:44`, runs as root, npm/go/rpm через внутренний прокси **Nora** с
прямым фолбэком в интернет если в Nora нет.

**Цель этой итерации:** поднять Playwright против работающего intermasq-бинарника
и написать **5 spec'ов** (минимальный батч): auth, theme, i18n, hosts-sort
(regression бага A1), host-crud. Это первый заход в L4 (UI); полный список
(20-30 specs, A5/A7/SSE/search) — вторым батчем, НЕ в этой задаче.

**Почему E2E вообще нужен:** smoke.sh (L3) бьёт только HTTP-API. Frontend-only
баги **A1** (дубли строк таблицы при сортировке), **A5** (BulkEditModal),
**A7** (TemplatesModal) HTTP-слоем не поймать — они в Vue-реактивности. A1
попадает в эту итерацию как regression.

---

## 1. ЖЁСТКИЕ ограничения (не нарушать)

1. **Продуктовые исходники не трогать.** Никаких правок в `*.go`,
   `frontend/src/**`, `frontend/package.json`, `frontend/index.html`,
   `frontend/vite.config.js`. Тестовая инфра живёт ТОЛЬКО в `tests/**` и в
   `.forgejo/workflows/build.yml`.
2. **Не лезть в `frontend/package.json`.** Раннер Playwright ставим через
   **отдельный** `tests/e2e/package.json` (см. §4.1) — иначе ломается правило 1
   и возникает двойной source-of-truth для дев-зависимостей.
3. **Не менять продуктовое поведение ради тестируемости.** Если для теста
   «не хватает» data-testid — НЕ добавляй их в `.vue`. Используй CSS/emoji/ARIA
   селекторы (они стабильны, см. §3.6).
4. **Коммиты — только после того, как локально/в CI зелёно.** Не пушить
   «авось заработает».
5. **CI-шаг обязан быть opt-in** (новый input `run_e2e_tests`, default false),
   по образцу `run_perf_tests`. E2E не должен утяжелять дефолтный прогон.

---

## 2. Куда кладём файлы (структура)

```
tests/e2e/
├── package.json                 # @playwright/test (свой, изолированный)
├── playwright.config.ts
├── global-setup.ts              # ждёт сервер, создаёт admin, пишет storageState
├── .gitignore                   # node_modules/ .auth/ test-results/ playwright-report/
└── specs/
    ├── auth.spec.ts             # login UI (fresh context)
    ├── theme.spec.ts            # dark/light toggle + persist
    ├── i18n.spec.ts             # RU↔EN toggle + persist
    ├── hosts-sort.spec.ts       # A1 regression: сортировка не меняет count строк
    └── host-crud.spec.ts        # seed→виден→delete UI→исчез без reload
.forgejo/workflows/build.yml     # +input run_e2e_tests, + шаг (правка workflow — можно)
```

**Почему spec'и под `tests/e2e/`, а не `frontend/tests/`:** Node-резолв
`import '@playwright/test'` идёт ВВЕРХ от файла spec'а. Свой `package.json` в
`tests/e2e/` + `npm ci` туда же создают `tests/e2e/node_modules`, и spec'ы
резолвят раннер локально. `frontend/package.json` остаётся нетронутым (правило
1+2). Это сознательное отклонение от «tests/ — дом bash-тестов»; Playwright —
JS-инструмент и живёт со своим node_modules.

---

## 3. Разведка по приложению (бери как есть, не переоткрывай)

### 3.1 Авторизация и JWT
- JWT хранится в **`localStorage['token']`** (`frontend/src/store.js:28`,
  `AuthScreen.vue:41`). Ключ буквально `'token'`.
- axios-клиент: `baseURL: '/api'`, interceptor добавляет
  `Authorization: Bearer <token>` если `store.token` (`store.js:53-58`).
- `actions.checkStatus()` (`store.js:72`): `GET /api/status` →
  - `setup_required===true` → `store.view='setup'`
  - иначе если есть `token` → `view='dashboard'` + `loadData()`
  - иначе → `view='login'`
- Значения `store.view`: `'loading' | 'login' | 'setup' | 'dashboard' | 'error'`.

### 3.2 Бутстрап приложения
- `App.vue` `onMounted`: применяет тему из `localStorage['theme']`, зовёт
  `checkStatus()`, стартует SSE. Значит при загрузке `/` с уже сейвленным
  `token` приложение само уходит в `dashboard`.

### 3.3 localStorage-ключи (используются в UI)
| Ключ | Значение | Кто пишет |
|---|---|---|
| `token` | JWT | `AuthScreen.submit`, `actions.setToken` |
| `theme` | `'dark'` / `'light'` | `App.toggleTheme`, читается в `onMounted` |
| `locale` | `'ru'` / `'en'` | `App.switchLocale` |

### 3.4 Тема (theme.spec)
- Атрибут **`data-bs-theme`** на `<html>` (`document.documentElement`):
  `'dark'` / отсутствует или `'light'`.
- `App.toggleTheme()` (`App.vue:33`): `getAttribute('data-bs-theme')==='dark'`
  → ставит противоположное + `localStorage.setItem('theme', ...)`.
- `onMounted` (`App.vue:51`): если `localStorage.theme==='dark'` → ставит dark.
- **Детерминизм для теста:** storageState (из globalSetup) сеет ТОЛЬКО `token`.
  `theme` в новом контексте = null → на старте светлая тема (атрибута нет).
  Один клик toggle → `data-bs-theme="dark"` + `localStorage.theme="dark"`.
  Этого достаточно для стабильного assert'а.

### 3.5 Локаль (i18n.spec)
- `locale.value` ∈ `{'ru','en'}`. Пункт меню переключения
  (`App.vue:89`): `🌐 {{ locale === 'ru' ? 'English' : 'Русский' }}`.
- **Инвариант без привязки к дефолтной локали:** после клика по 🌐-пункту
  подпись этого пункта меняется (`English`↔`Русский`) и `localStorage.locale`
  становится непустым. Не зависим от того, ru или en был дефолтным.

### 3.6 Селекторы (стабильные, не по i18n-тексту)
| Что | Селектор | Примечание |
|---|---|---|
| Меню (только когда залогинен) | `.dropdown-toggle` | `v-if="store.token"` (`App.vue:73`) — индикатор dashboard |
| Пункт «локаль» | `.dropdown-item` с текстом `🌐` | `hasText: '🌐'` |
| Пункт «тема» | `.dropdown-item` с текстом `🌓` | `hasText: '🌓'` |
| Пункт «logout» | `.dropdown-item.text-danger` | единственный `.text-danger` в меню (`App.vue:94`) |
| Логин: username | `input.form-control` nth=0 | `AuthScreen.vue:8` |
| Логин: password | `input.form-control` nth=1 (type=password) | `AuthScreen.vue:9` |
| Кнопка submit (login/setup) | `.btn-primary` | единственная на экране auth |
| Сортируемые `<th>` | `th` с текстом `IP` / `MAC` / `Hostname` | `HostTable.vue:18-20`, `@click="sortBy(...)"` |
| Ячейка MAC в строке | `tbody tr td.font-monospace` | `HostTable.vue:35` |
| Кнопка удаления в строке | `button.btn-outline-danger` | `HostTable.vue:51` (`✕`) |
| Строка таблицы по MAC | `tbody tr` `hasText: <mac>` | |

### 3.7 Баг A1 (hosts-sort regression)
- Корень: `HostTable.vue:27` — `<tr v-for="h in sortedHosts" :key="h.mac">`.
  `:key="h.mac"` не уникален в некоторых сценариях → при пересортировке Vue
  переиспользует DOM и строки визуально дублируются.
- **Для regression-теста механизм НЕ важен.** Инвариант: после N кликов
  сортировки количество строк таблицы не меняется. Сеем K уникальных хостов,
  кликаем `th:has-text("IP")` 3 раза, после каждого проверяем count.
- **Изоляция от мусора:** считай ТОЛЬКО строки с нашим префиксом MAC
  (`td.font-monospace` `hasText: '<prefix>'`), не все строки — бэкенд общий
  между spec'ами, могут остаться чужие хосты.

### 3.8 API-эндпоинты (нужны globalSetup + seed'ам)
Все кроме `/status`/`/setup`/`/login` требуют `Authorization: Bearer <jwt>`.

| Метод | Путь | Тело / заметки |
|---|---|---|
| GET | `/api/status` | `{setup_required, dnsmasq_active,...}`. Без auth. |
| POST | `/api/setup` | `{username,password}` → `{token}`. **403 если уже настроено.** Без auth. |
| POST | `/api/login` | `{username,password}` → `{token}`. **Rate-limit 10/мин** (`auth.go`). Без auth. |
| GET | `/api/hosts` | массив HostEntry |
| POST | `/api/hosts` | `{mac,ip,hostname,file,tags?}` → 200 / 409 dup / 400. **`file` обязан быть внутри conf-dir** (`isSafePath`). |
| DELETE | `/api/hosts/:mac?file=<abs path>` | 200 / 404. |

Админ-учётка в CI: `admin` / `pass1234` (как в smoke.sh, `tests/lib/state.sh`).
`INTERMASQ_SECRET` уже в env джоба.

### 3.9 Native confirm() в UI
`HostTable.deleteHost` (`HostTable.vue:147`) и `bulkDelete` зовут браузерный
`confirm()`. Playwright по умолчанию их **отменяет**. Обязательно:
`page.on('dialog', d => d.accept())` перед кликом удаления.

---

## 4. Спецификация файлов

### 4.1 `tests/e2e/package.json`
```json
{
  "name": "intermasq-e2e",
  "private": true,
  "type": "module",
  "scripts": { "test:e2e": "playwright test" },
  "devDependencies": { "@playwright/test": "^1.49.0" }
}
```
Версия `^1.49.0` — Nora разрулит с прямым фолбэком. НЕ добавлять `playwright`
отдельно — `@playwright/test` тянет всё нужное.

### 4.2 `tests/e2e/playwright.config.ts`
- `testDir: './specs'`
- `baseURL` = `process.env.E2E_BASE_URL || 'http://localhost:18083'`
- **`workers: 1`, `fullyParallel: false`** — общий бэкенд (один conf-dir, один
  файл хостов); параллелизм даст гонки.
- `retries: 0`
- `reporter: [['list'], ['html', { open: 'never' }]]`
- `globalSetup: './global-setup.ts'`
- `use.storageState: './.auth/storageState.json'` (сеет `token` для всех spec'ов)
- `use.trace: 'retain-on-failure'`
- `projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]`

### 4.3 `tests/e2e/global-setup.ts`
Назначение: дождаться сервера, гарантировать существование admin, добыть JWT,
записать `storageState` с `localStorage.token`.

Алгоритм:
1. Wait-for-server: в цикле до ~30с `fetch(baseURL+'/api/status')` пока не `ok`.
2. `request.newContext({ baseURL })`.
3. `GET /api/status` → `body.setup_required`?
   - true → `POST /api/setup {username,password}` → `token`
   - false → `POST /api/login {username,password}` → `token`
   (setup вернёт 403 при повторе — поэтому ветвление по `setup_required`.)
4. `assert token` непустой.
5. Записать файл `.auth/storageState.json` **напрямую** (без запуска браузера):
```json
{
  "cookies": [],
  "origins": [
    { "origin": "<baseURL>", "localStorage": [ { "name": "token", "value": "<jwt>" } ] }
  ]
}
```
   `origin` должен точно совпадать с baseURL (включая порт).
6. `ctx.dispose()`.

Учётка: `username: process.env.ADMIN_USER || 'admin'`,
`password: process.env.ADMIN_PASS || 'pass1234'`. `fetch` — глобальный в Node
18+, в CI Node 22 (ок).

### 4.4 `tests/e2e/.gitignore`
```
node_modules/
.auth/
test-results/
playwright-report/
```

### 4.5 spec: `auth.spec.ts` (login UI, fresh context)
- `test.use({ storageState: { cookies: [], origins: [] } })` — опционаут
  общего storageState, чтобы проверить именно экран логина.
- `page.goto('/')` → приложение в `view='login'` (admin уже создан globalSetup).
- Заполнить: `input.form-control` nth(0) = admin, nth(1) = pass1234.
- Клик `.btn-primary` → редирект в dashboard.
- Assert dashboard: `.dropdown-toggle` виден (меню `v-if="store.token"`).
- Logout: клик `.dropdown-toggle` → клик `.dropdown-item.text-danger`.
- Assert: снова экран логина → `.btn-primary` виден.

### 4.6 spec: `theme.spec.ts`
- Обычный storageState (залогинен).
- `goto('/')` → ждать `.dropdown-toggle`.
- Запомнить `html[data-bs-theme]` (null = light).
- Открыть меню → клик `.dropdown-item` `hasText:'🌓'`.
- Assert: `html` приобрёл `data-bs-theme="dark"` (с null → dark).
- Assert: `localStorage.getItem('theme') === 'dark'` (через `page.evaluate`).

### 4.7 spec: `i18n.spec.ts`
- `goto('/')` → ждать `.dropdown-toggle`.
- Открыть меню, взять `.dropdown-item` `hasText:'🌐'`, запомнить `innerText`.
- Кликнуть его.
- Assert: `localStorage.getItem('locale')` непустой.
- Снова открыть меню, взять ту же подпись → она **изменилась**
  (`English`↔`Русский`).

### 4.8 spec: `hosts-sort.spec.ts` (A1 regression)
- **beforeAll:** через `request.newContext` залогиниться (`/api/login`) →
  получить `apiToken`. Для каждого из 5 MAC'ов префикса `aa:11:11:11:11:01..05`
  сделать `POST /api/hosts` с `{mac, ip: '10.99.<i>.2', hostname:'sort<i>',
  file: <CONF_DIR>/e2e-sort.conf}`, заголовок `Authorization: Bearer <apiToken>`.
  На 409 (уже есть) — игнорировать. `CONF_DIR` из env (см. §5).
- `goto('/')` → ждать таблицу (static-таб по умолчанию).
- `prefix = 'aa:11:11:11:11'`; selector =
  `page.locator('tbody tr td.font-monospace', { hasText: prefix })`.
- `await expect(selector).toHaveCount(5, {timeout:15000})`.
- Цикл 3 раза: `page.locator('th',{hasText:'IP'}).click()` →
  `await expect(selector).toHaveCount(5)`.
- Инвариант: count строк нашего префикса не меняется от пересортировки.

### 4.9 spec: `host-crud.spec.ts`
- **beforeAll:** login via API → `apiToken`. Засеять один хост
  `aa:22:33:44:55:01` (IP `10.99.50.2`, file `<CONF_DIR>/e2e-crud.conf`).
- `goto('/')` → ждать `.dropdown-toggle`.
- `row = page.locator('tbody tr', { hasText: 'aa:22:33:44:55:01' })`.
- `await expect(row).toBeVisible({timeout:15000})`.
- `page.on('dialog', d => d.accept())` ← ОБЯЗАТЕЛЬНО (native confirm).
- `row.locator('button.btn-outline-danger').click()`.
- `await expect(row).toBeHidden({timeout:10000})` — таблица обновилась через
  `actions.loadData()` БЕЗ full page reload (URL остался `/`).

---

## 5. CI-интеграция (`.forgejo/workflows/build.yml`)

1. В блок `on.workflow_dispatch.inputs` добавить:
```yaml
      run_e2e_tests:
        description: "Run Playwright E2E (tests/e2e)? Installs chromium; opt-in."
        required: true
        default: false
        type: boolean
```
2. Новый шаг **после** perf, **перед** «Show binary info»:
```yaml
      - name: L4 — Playwright E2E (Gap 2, opt-in)
        if: success() && github.event.inputs.run_e2e_tests == 'true'
        run: |
          mkdir -p /tmp/e2e-conf /tmp/e2e-history
          ./intermasq-ci \
            -port 18083 \
            -conf-dir /tmp/e2e-conf \
            -db /tmp/e2e-users.json \
            -audit-log /tmp/e2e-audit.log \
            -history-dir /tmp/e2e-history \
            -templates /tmp/e2e-templates.json \
            -leases /tmp/e2e-leases \
            -arp-file tests/fixtures/arp-sample.txt \
            -init-system=none \
            -ci-mode=true &
          E2E_PID=$!
          for i in $(seq 1 10); do
            if curl -sf http://localhost:18083/api/status >/dev/null 2>&1; then break; fi
            sleep 1
          done
          cd tests/e2e
          npm ci
          npx playwright install --with-deps chromium
          set +e
          E2E_BASE_URL=http://localhost:18083 CONF_DIR=/tmp/e2e-conf \
            ADMIN_USER=admin ADMIN_PASS=pass1234 npx playwright test
          PW_RC=$?
          set -e
          kill $E2E_PID 2>/dev/null || true
          wait $E2E_PID 2>/dev/null || true
          if [ "$PW_RC" -ne 0 ]; then
            echo "::error::playwright returned $PW_RC"
            exit 1
          fi
```
Заметки:
- `--with-deps` ставит OS-либы chromium через dnf (root в CI — ок).
- Браузер-бинарь качается напрямую; Nora закрывает только npm-пакет.
- `CONF_DIR` пробрасывается в spec'и (для `file` под conf-dir → `isSafePath`).
- Своя инстанция на `:18083`, отдельный conf-dir — не конфликтует со smoke/perf.

---

## 6. Грабли (читай перед запуском)

1. **node resolution:** spec'и обязаны лежать под `tests/e2e/`, где есть свой
   `node_modules`. Если положить в корневой `tests/e2e` БЕЗ местного package.json
   — `import '@playwright/test'` не резолвится.
2. **workers/parallel:** только `workers:1` + `fullyParallel:false`. Иначе гонки
   на общем файле хостов и rate-limit `/api/login` (10/мин).
3. **native dialogs:** без `page.on('dialog', d=>d.accept())` удаление молча
   отменится — тест зависнет на `toBeHidden`.
4. **count по всему `tbody tr`** может включать empty-state row (с `colspan`),
   когда хостов 0. Считай именно `td.font-monospace` с твоим префиксом.
5. **MAC-изоляция:** у каждого spec'а свой префикс
   (`aa:11:11...` для sort, `aa:22:33...` для crud). Не переиспользуй `aa:bb:cc`
   из `gen-hosts.sh` — будет 409 dup (findHostsByMac сканит весь conf-dir, см.
   лог `gap5-6-perf-and-plugins.md` — этот баг уже был в perf.sh).
6. **`file` под conf-dir:** `isSafePath` требует абсолютный путь внутри
   `-conf-dir`. Собирай через `${process.env.CONF_DIR}/e2e-*.conf`.
7. **rate-limit /api/login (10/мин):** globalSetup логинится 1 раз. Spec'ы в
   `beforeAll` тоже логинятся — при 5 spec'ах это 6 логинов за прогон, в лимит
   укладываются. Не логинься в `beforeEach`.
8. **default locale неизвестен** → НЕ завязывайся на i18n-текст. Только
   классы/ARIA/emoji (§3.6) и инварианты (для i18n — смена подписи пункта).
9. **storageState origin** должен совпадать с baseURL до порта — иначе `token`
   не применится к контексту.
10. **frontend/dist должен быть собран** до старта intermasq (бинарь embed'ит
    `frontend/dist/*`). В CI шаг «Build frontend» уже идёт раньше — порядок ок.

---

## 7. Как проверять

**Локально (WSL/Linux):**
```bash
export INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXX"
go build -o /tmp/intermasq-e2e .
./intermasq-ci... # на Windows локально нет; этот шаг реален только в CI/Linux
```
Локально на Windows intermasq не стартует продуктивно (init-system, пути) —
поэтому основная проверка **в CI** через `run_e2e_tests=true`.

**CI (основная):** запустить workflow с `run_e2e_tests=true`. Все 5 spec'ов
должны PASS. Если падает на bootstrap (chromium не скачался) — смотреть
доступность `playwright.azureedge.net` из контейнера; Nora+direct должны
справиться.

**Что НЕ должно сломаться:** дефолтный прогон (без `run_e2e_tests`) остаётся
зелёным и не длиннее — e2e opt-in.

---

## 8. Приёмка (definition of done)

- [ ] Файлы из §2 созданы, продуктовые исходники (§1) не тронуты.
- [ ] `cd tests/e2e && npm ci && npx playwright test` — 5/5 PASS в CI
      (input `run_e2e_tests=true`).
- [ ] A1 regression (`hosts-sort.spec`) зелёный — count строк стабилен после
      3 кликов сортировки.
- [ ] Дефолтный CI-прогон (`run_e2e_tests=false`) не изменился по времени/цвету.
- [ ] `логи/Gap_2.md` (этот файл) актуален; после реализации — короткий
      session-лог `логи/gap2-playwright-bootstrap.md` по конвенции
      (контекст → что сделано → результат).
- [ ] `tests/ROADMAP.md` и `логи/duis.md`: L4 Playwright — «первая итерация
      закрыта (5 specs)», A1 под regression; A5/A7/SSE/search — второй батч.

---

## 9. ВНЕ области этой задачи (явно)

- A5 (BulkEditModal), A7 (TemplatesModal), SSE-live, search/filter, tags badge,
  config editor — **второй батч**, после того как бутстрап устоится.
- Любой рефакторинг продуктовых исходников под тестируемость (data-testid и
  т.п.) — запрещён (§1).
- Покрытие 99%, fuzzing, real VM — другие gap'ы, не этот.
- Мутации, browser-matrix — enterprise, не для v1.0.

---

## 10. Порядок исполнения (рекомендация)

1. `tests/e2e/package.json` + `.gitignore`.
2. `playwright.config.ts` + `global-setup.ts`.
3. `auth.spec.ts` — самый простой, проверит что бутстрап+storageState живой.
4. `theme.spec.ts`, `i18n.spec.ts` — не требуют seed'а.
5. `hosts-sort.spec.ts` (A1) — требует seed-хелпера.
6. `host-crud.spec.ts` — добавит dialog-handling.
7. Правка `build.yml` (§5).
8. Коммит+пуш, прогон в CI с `run_e2e_tests=true`, фикс замечаний.
9. Session-лог + актуализация ROADMAP/duis.

На каждом шаге — прогон в CI, не копить. Если bootstrap-шаг (chromium) падает
по причине сети — это блокер №1, решать до написания остальных spec'ов.
