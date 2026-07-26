# Gap 2 — финал: промт на реализацию (батч 4 + фиксы A5/A13 + mutation-pass)

**Назначение:** самодостаточный промт для ИИ-ассистента (или будущего себя),
чтобы закрыть L4 Playwright до практического 100% за одну сессию без двойных
толкований. Прочитай ЦЕЛИКОМ перед стартом; не импровизируй там, где даны
конкретные селекторы/инварианты/пути. Это преемник `Gap_2.md` (батч 1) —
старый промт удалён, история батчей 1–3 живёт в `логи/gap2-*.md`.

---

## 0. Что это за проект и где мы сейчас

**Intermasq** — веб-панель для dnsmasq. Backend Go (gin), frontend Vue 3 +
Bootstrap 5, embed фронтенда в бинарь через `go:embed`. Репозиторий:
`B:\Repo\Intermasq\Intermasq`, ветка `main`. CI — Forgejo Actions, контейнер
`fedora:44`, runs as root, npm/go/rpm через внутренний прокси **Nora** с
прямым фолбэком в интернет.

**Состояние L4 на старте этой сессии: 25 spec'ов, 25/25 зелёных** (CI input
`run_e2e_tests`). Структура (ВСЁ уже есть — НЕ пересоздавай):

```
tests/e2e/
├── package.json + package-lock.json   # @playwright/test ^1.49 (изолированно)
├── playwright.config.ts                # workers:1, fullyParallel:false, baseURL из env
├── global-setup.ts                     # ждёт сервер, setup|login, пишет storageState
├── .gitignore                          # node_modules/ .auth/ test-results/ playwright-report/
├── lib/
│   ├── api.ts                          # BARREL: export * from './api-auth'; './api-hosts'
│   ├── api-auth.ts                     # BASE_URL, CONF_DIR, apiLogin()
│   └── api-hosts.ts                    # SeedHost, seedHosts(), deleteHostApi()
└── specs/                              # 21 файл, 25 тестов (см. логи gap2-batch3-phaseV.md)
```

CI-шаг «L4 — Playwright E2E» уже в `.forgejo/workflows/build.yml` (opt-in
`run_e2e_tests`, default false), инстанция на `:18083`, свой conf-dir
`/tmp/e2e-conf`. **Не трогай CSS-deps-часть шага — она уже отлажена**
(см. §4 п.2).

**Цель этой сессии — 3 блока работ:**
1. **Фиксы A5 и A13** (правки ПРОДУКТОВЫХ исходников — это разрешено и
   требуется; см. §1).
2. **Батч 4 — 8 spec'ов** (audit/plugins-iframe/i18n-api-error/
   config-template-fill/config-directive/config-raw/setup-screen/sse-live).
3. **Mutation-pass** — разовый, на throwaway-ветке, для эмпирической
   уверенности, что тесты реально ловят регрессии.

---

## 1. ЖЁСТКИЕ ограничения (не нарушать)

1. **Продуктовые правки РАЗРЕШЕНЫ только в объёме фиксов A5 и A13** (§5).
   Любые другие изменения `*.go`/`frontend/src/**`/`frontend/package.json`
   — запрещены. НЕ добавляй `data-testid` в `.vue` (тесты работают на
   CSS/emoji/placeholder-селекторах; см. §3).
2. **Не лезь в `frontend/package.json`.** Раннер Playwright изолирован в
   `tests/e2e/package.json`. Новые хелперы — в `tests/e2e/lib/` (баррель
   `api.ts` переэкспортирует; в батчах 3 добавлены `api-auth.ts`,
   `api-hosts.ts`; если нужно — добавь `api-aliases.ts` и строку в баррель).
3. **CI-шаг L4 не утяжеляй и не делай не-opt-in.** Правки `build.yml`
   разрешены ТОЛЬКО если для sse-live/setup-screen нужна infra-правка
   (§6, §8) — и тогда только внутри opt-in шага.
4. **Коммиты — только после `--list` + (для фиксов) зелёного CI.**
5. **MAC-изоляция:** каждый spec — свой префикс, не коллизирующий с
   arp-fixture (`11:22:33:44:55:01..02`, `aa:bb:cc:dd:ee:01`,
   `60:3d:61:28:89:5c`, `00:00:00:00:00:00`) и с существующими спеками
   (`aa:11:11:11:11`, `aa:22:33:44:55`, `aa:33:44:55:66`,
   `aa:44:55:66:77:88`, `aa:55:66:77:88`, `aa:66:77:88:99`,
   `aa:77:88:99:aa`, `aa:88:11:22:33`, `aa:99:11:22:33`,
   `aa:b1:00:00:00`, `aa:b2:00:00:00`, `aa:c1:00:00:00`,
   `aa:d1:00:00:00`, `aa:d2:00:00:00`). Новые — начиная с `aa:e1:...`,
   `aa:e2:...` и т.д.
6. **`workers:1`, `fullyParallel:false`** — уже в конфиге, НЕ меняй.

---

## 2. Живой UI: точка входа и навигация (НЕ переоткрывай, бери как есть)

- Точка входа: `frontend/index.html` → `/src/main.js` → `src/App.vue`.
- **`frontend/App.vue` (в корне frontend/) — МЁРТВЫЙ код** (старая версия до
  слияния LeasesTab в DiscoveredTab). Никем не импортируется. НЕ читай его,
  ориентируйся только на `frontend/src/App.vue`.
- Табы в живом UI (App.vue, кнопки в `.btn-group`): static / 🌐 dns /
  🔍 discovery / ⚙️ config / 🛡️ safety / 👥 users. **Отдельного leases-таба
  нет** — discovery покрывает и arp-newDevices, и newLeases.
- `localStorage`: `token` (JWT), `theme` ('dark'/'light'), `locale` ('ru'/'en').
- `store.view`: 'loading' | 'login' | 'setup' | 'dashboard' | 'error'.

---

## 3. Селекторы и контракты (консолидировано из батчей 1–3)

### 3.1. API (для `request.newContext({ baseURL: BASE_URL })`)
**ВСЕ пути — с `/api`-префиксом** (baseURL = host без пути! см. §4 п.1):

| Метод | Путь | заметка |
|---|---|---|
| GET | `/api/status` | без auth |
| POST | `/api/setup` | `{username,password}` → `{token}`. 403 если уже настроено |
| POST | `/api/login` | `{username,password}` → `{token}`. Rate-limit 10/мин, сброс на успехе |
| GET/POST/DELETE | `/api/hosts`, `/api/hosts/:mac?file=<abs>` | `file` обязан быть внутри `-conf-dir` (`isSafePath`) |
| GET | `/api/aliases` / POST `/api/aliases` / POST `/api/aliases/delete` | `{type,domain,target,file}` |
| GET | `/api/backup` | → ZIP (blob) |
| POST | `/api/backup/restore` | multipart `file` |
| GET | `/api/history?file=<abs>` / `/api/history/diff?file&from&to` / POST `/api/history/restore` | |
| POST | `/api/rollback` | `{file}` → revert из `.bak` |

Учётка CI: `admin` / `pass1234`. `INTERMASQ_SECRET` уже в env джоба.

### 3.2. Селекторы (стабильные)
- **dashboard-индикатор:** `.dropdown-toggle` (`v-if="store.token"`).
- **табы:** `.btn-group button` hasText `🌐`/`🔍`/`⚙️`/`👥`.
- **search-инпут** (тулбар App.vue): `.col-md > input.form-control` (placeholder i18n → структурно).
- **формы (HostForm/AliasForm):** карточка `.card.p-3.shadow-sm`. **ВСЕ input-селекторы скаупить к этой карточке** — иначе search-инпут перебивает `nth(0)` (§4 п.4).
- **HostForm:** MAC `input[placeholder="MAC (aa:bb...)"]` (захардкожен!); tags `input.form-control.font-monospace`; importMode `select.form-select-sm` (`.first()` — их два в single-mode); file-input `.input-group:has(.btn-success) input.form-control`; Add `.input-group .btn-success`; IP-инпут `.input-group:has(button:has-text("🎲")) input.form-control`; Save (edit-mode) `.input-group .btn-warning`.
- **HostTable:** `th` hasText `IP`/`MAC`/`Hostname` (сортировка); row `tbody tr` hasText `<mac>`; MAC-cell `td.font-monospace`; delete `button.btn-outline-danger`; edit ✏️ `button.btn-outline-secondary`; bulk-bar `.bg-danger .btn-group` hasText `📦`/`✏️`/`🗑️`; file-cell (в 'all'-view) `td.small.text-muted`.
- **Модалки:** `.modal-content`; заголовки по эмодзи (`⚙️` templates, `📦` move, `✏️` edit, `🕒` history); HistoryModal version-row `tbody tr` first; diff `≠` `button.btn-outline-primary`; restore `⏪` `button.btn-outline-warning`; diff-content `pre.history-diff`.
- **StaticView per-file:** `.nav-link` hasText `<basename>.conf`; rollback `⏪` `.btn-outline-warning` hasText `⏪`; history `🕒` `.btn-outline-secondary` hasText `🕒`.
- **DnsmasqConfig:** new-file link `.nav-link.text-success`; name-input `input[placeholder="filename.conf"]` (захардкожен); create `＋` `.card.border-success .btn-success`; delete `🗑` `.d-flex.gap-2 .btn-outline-danger`.
- **UsersTab:** create-карта = `.card.shadow-sm` `filter({ has: '.btn-success' })`; username `input[type="text"]`; password `input[type="password"]`; delete `🗑` `.btn-outline-danger` + confirm.
- **DiscoveredTab:** newDevices в `.card.border-warning`; MAC в `td.font-monospace.fw-bold`; ➕ `button.btn-outline-primary`.

### 3.3. localStorage-ключи — см. §2.

---

## 4. ГРАБЛИ (ОБЯЗАТЕЛЬНО прочесть — это стоит 5 красных CI-прогонов в батчах 1–3)

1. **`/api`-префикс.** Playwright `request.newContext({ baseURL })` использует
   host-only baseURL (`http://localhost:18083`, БЕЗ `/api`). Любой API-путь
   в хелперах/спеках должен быть `/api/...`. Это падало ДВАЖДЫ: `/backup`
   (→404) и `deleteHostApi` `/hosts/:mac` (→404 silent no-op, спал 3 фазы).
   **Перед коммитом: `grep -rn "ctx\.\(get\|post\|delete\)" tests/e2e/lib` и
   убедись, что каждый путь начинается с `/api/`.**

2. **Chromium-deps на Fedora.** `npx playwright install --with-deps` умеет
   только apt → на fedora:44 падает на `apt-get: command not found`. Решение
   в `build.yml` уже есть (не пересоздавай): `dnf install` явный список
   chromium-либ БЕЗ gtk3 (gtk3 тянет openh264 с недоступного кодек-зеркала
   `codecs.fedoraproject.org`/`ciscobinary.openh264.org`). Рабочая строка:
   ```
   dnf install -y --setopt=install_weak_deps=False \
     nss nspr atk at-spi2-atk cups-libs libdrm libxkbcommon \
     libX11 libXcomposite libXdamage libXext libXfixes libXrandr \
     mesa-libgbm pango alsa-lib libxshmfence
   ```
   затем `npx playwright install chromium` (БЕЗ `--with-deps`).

3. **Диалоги (alert/confirm).** Продукт массово использует браузерные
   `alert()` (success/error) и `confirm()`. Включай их accept ДО клика:
   `page.on('dialog', d => d.accept())`.
   - **Двойные диалоги:** некоторые действия дают confirm + затем alert на
     результате (напр. `deleteUser` на отказе → `cannot_delete_self` →
     `alert(msg)`). Если тест резолвится раньше второго диалога → Playwright
     рвёт страницу с pending-alert → краш `dialog.accept: Target page has
     been closed`. Фикс-паттерн: счётчик + `await expect.poll(() => dialogs,
     {timeout:10000}).toBeGreaterThanOrEqual(2)`.

4. **Скаупинг селекторов.** `.form-control` на dashboard — это и search
   (тулбар), и инпуты форм. Позиционные `nth(N)` без скаупа бьют search.
   Скауп к `.card.p-3.shadow-sm`. `.btn-warning` тоже неоднозначен
   (тулбарная «🔄 Применить» + HostForm Save) → `.input-group .btn-warning`.
   Emoji-кнопки (`✏️`,`⏪`,`⚙️`) матчат несколько мест → всегда hasText +
   скауп к контейнеру.

5. **`.bak` и history-версия создаются 2-м write.** `createLocalBackup`
   (history.go:259) зовёт `saveHistory` + пишет `.bak` из ТЕКУЩЕГО
   состояния; `saveHistory` — no-op для отсутствующего/пустого файла. Поэтому
   1-й write в свежий файл не даёт ни `.bak`, ни версии. Для rollback/history
   spec'ов — **ceй 2 хоста в один файл**: 2-й write оставит `.bak` =
   1-хост состояние и version1 = 1-хост снапшот.

6. **`restoreBackupZip` MERGES, не wipe.** Перезаписывает только файлы,
   указанные в zip (старое → `.restore.bak`), не удаляет остальные. Поэтому
   backup-restore-spec безопасен для чужих файлов.

7. **storageState origin** должен точно совпадать с baseURL (включая порт),
   иначе `token` не применится (уже сделано в global-setup.ts — не трогать).

8. **`--list` запускай из `tests/e2e/`** (через workdir), иначе «0 tests» —
   это ложная тревога, не поломка.

---

## 5. Блок A — фиксы A5 и A13 (правки ПРОДУКТОВЫХ исходников)

### A5 (HIGH, frontend) — BulkEditModal крашится при открытии
**Root cause:** `frontend/src/components/static/BulkEditModal.vue:67`:
```js
const host = store_hosts.find(x => x.mac === h.mac)   // store_hosts = store (reactive object), у него НЕТ .find
```
**Фикс (1 строка):**
```js
const host = store_hosts.hosts.find(x => x.mac === h.mac)
```
**После фикса:** в `tests/e2e/specs/bulk-ops.spec.ts` тест `bulk-edit:
IP prefix transform...` помечен `test.fail(...)` — **сними `.fail`** (сделай
обычным `test(...)`), удали комментарий про A5. Playwright при фикс-коммите
сообщит «expected to fail but passed» — это норма, это сигнал что фикс сработал.

### A13 (HIGH, backend) — `dnsmasq --test` гоняется без `--conf-file=<path>`
**Root cause:** в 3 функциях записи одиночного файла `dnsmasq --test`
тестирует дефолтный конфиг системы, а не записываемый файл:
- `dnsmasq.go:77` — `writeFileRaw`
- `dnsmasq.go:97` — `writeConfigWithTest`
- `history.go:245` — `restoreHistoryVersion`

**Канонический паттерн уже есть в репозитории** — `dnsmasq_test.go:1882`:
```go
cmd := exec.Command(dnsmasqBin(), "--test", "--conf-file="+tmp)
```
**Фикс (3 строки, одинаковая замена):**
```go
testCmd := exec.Command(dnsmasqBin(), "--test", "--conf-file="+path)
```
(в restoreHistoryVersion переменная `filePath`, не `path` — подставь её).

**НЕ ТРОГАЙ** `sse.go:110` (reloadDnsmasq) и `backup.go:119`
(restoreBackupZip) — они тестируют конфиг-дир в целом, смена флага меняет
семантику reload/restore и может сломать reload-ui/backup-restore-спеки и
smoke.sh. Это отдельная задача, не A13.

**После фикса A13:**
- Удали `A13` из `tests/known-bugs.txt`.
- В smoke.sh найди check с тегом `A13` и обнови ожидание (было KNOWN-fail →
  должно стать 200/успехом). Smoke-логика: пока ID в known-bugs.txt — failure
  жёлтый; убрал ID → проверь, что check реально зелёный.
- A13-фикс **разблокирует** config-directive и config-raw spec'и (§6).

### Верификация фиксов
- `go vet ./...` и `gofmt -l .` (CI это проверяет).
- `go test ./... -race -count=1` — существующие тесты не должны сломаться;
  `dnsmasq_test.go` уже покрывает writeFileRaw, проверь что A13-фикс не
  ломает `TestWriteFileRaw*`.
- CI: прогон с `run_e2e_tests=true` (bulk-edit без `.fail` должен PASS) и
  обычный прогон (smoke.sh должен остаться зелёным после удаления A13).

---

## 6. Блок B — батч 4 (8 spec'ов)

Все spec'и — в `tests/e2e/specs/`. Хелперы — `../lib/api`. Шаблон:
beforeAll (apiLogin + seed если нужно) → goto → ждать `.dropdown-toggle` →
действия → assert. Диалоги — §4 п.3.

### 6.1. `audit-tab.spec.ts` (низкий риск)
- Сделай через API host-add (seedHosts) → в UI открой 🛡️ safety-таб →
  AuditTab рендерит список. Assert: в списке есть запись `add` (или
  `config_update` и т.п.) — `tbody tr` hasText `add`.
- API audit: `GET /api/audit` (массив). AuditTab компонент: `safety/AuditTab.vue`
  (прочитай перед написанием — селекторы оттуда).

### 6.2. `plugins-iframe.spec.ts` (низкий риск)
- В CI уже установлен mock-плагин `hello` (см. шаг "Build & install mock
  plugin" в build.yml — кладёт в `/etc/intermasq/plugins/hello/`).
- UI: в user-меню (`.dropdown-toggle`) есть пункт `🧩 Hello Plugin` → клик →
  открывается overlay с `<iframe src="/plugins/hello/">`. Assert: `iframe`
  visible + `iframe.contentFrame()` отвечает (подожди загрузку).
- Примечание: `/api/plugins` уже покрыт smoke (`82-plugins.sh`); здесь только UI.

### 6.3. `i18n-api-error.spec.ts` (низкий риск)
- Сей host `aa:e1:00:00:00:01` через API. В UI открой static-таб. Открой
  user-меню, переключи локаль на EN (🌐) — `localStorage.locale='en'`.
- Открой HostForm, введи МАК `aa:e1:00:00:00:01` (дубликат) → Add → toast
  с переводом `mac_duplicate`. Assert: toast содержит английский текст
  (напр. "already exists" — проверь точную строку в `frontend/src/locales/en.json`,
  ключи `alert.*` / `translateApiError` в `i18n.js`). Не завязывайся на RU.
- Toast container: `.components/ToastContainer.vue` — прочитай селектор.

### 6.4. `config-template-fill.spec.ts` (средний, после A13 не обязателен)
- Config-таб → `+` new file → name=`e2e-tpl-fill.conf`, template=`basic-dhcp`
  (или другой из `GET /api/config/templates`) → ＋.
- Assert: таб появился + контент файла наполнен (напр. `dhcp-range`).
  Через `GET /api/files/e2e-tpl-fill.conf` (raw) или в UI открыть файл и
  увидеть директивы. Этот spec НЕ зовёт save → работает и до A13.

### 6.5. `config-directive.spec.ts` (СРЕДНИЙ, ЗАБЛОКИРОВАНО до фикса A13)
- Создай файл через UI (empty template), выбери его, добавь директиву
  (напр. `port=5353` через "+ add directive"), Save (`.btn-primary` hasText 💾)
  + confirm.
- **До A13:** save гоняет `--test` на дефолтном конфиге → проходит
  (дефолт валиден), НО не проверяет сам файл → spec формально зелёный, но
  ничего не тестирует. **После A13:** `--test --conf-file=<file>` реально
  валидирует → spec становится честным. Запускай ТОЛЬКО после A13.
- Negative: директива с invalid value (напр. `port=abc`) → save →
  `dnsmasq_test_failed` → toast/Alert, файл откатился. Assert: файл не
  изменился (директивы нет).

### 6.6. `config-raw.spec.ts` (СРЕДНИЙ, ЗАБЛОКИРОВАНО до A13)
- `PUT /api/files/<name>` (raw) через UI? В UI raw-режима НЕТ (только API).
  Поэтому spec: через `PUT /api/files/e2e-raw.conf` записать валидный
  контент → assert `GET /api/files/e2e-raw.conf` = тот же контент. Потом
  invalid (пустой `dhcp-host=`) → 400 `dnsmasq_test_failed`, файл откатился.
- Это скорее API-level spec, НО тестирует writeFileRaw-путь (A13). Запускай
  после A13. По сути дублирует smoke — оцени целесообразность (можно skip).

### 6.7. `setup-screen.spec.ts` (ВЫСОКИЙ, infra)
- Нужен ИЗОЛИРОВАННЫЙ пустой user-DB. globalSetup уже создал admin на
  `/tmp/e2e-users.json`. Варианты:
  - **(а) Smoke-вариант (рекомендую):** отдельный `test.use({ storageState:
    пустой })`, потом через `request` POST `/api/setup` на ЧИСТЫЙ db... но
    db уже занят. НЕ получится в той же инстанции.
  - **(б) Правка CI:** поднять **вторую** инстанцию `intermasq-ci` на `:18084`
    с `-db /tmp/e2e-setup-users.json` (fresh) и `-ci-mode`, и в spec ходить
    на `E2E_SETUP_BASE_URL`. Setup-экран → заполнить (`input.form-control`
    nth0/1) → `.btn-primary` → редирект в dashboard.
- Если infra-вариант (б) слишком тяжёл — пометь spec `test.skip` с комментом
  «needs isolated user-DB; see setup-screen in Gap_2_finish.md» и двигайся.
  Auth-flow (login) уже покрыт `auth.spec`.

### 6.8. `sse-live.spec.ts` (ВЫСОКИЙ, infra)
- Вещатель (`sse.go:73` `startSSEBroadcaster`) шлёт ТОЛЬКО дельты
  (`if arpJSON != lastArp`), интервал 5с. arp-fixture статичен → после
  первого пуши тишина. Плюс приложение грузит arp через REST (`loadData`),
  не SSE → «🟢 появился» НЕ доказывает SSE.
- **Нужна CI-правка:** скопировать fixture в writable путь
  (`-arp-file /tmp/e2e-arp.txt` для e2e-инстанции), тогда spec mid-test
  дописывает строку через `fs.appendFileSync` (Node) → ждём (≤10с) что
  конкретный host получил 🟢 через SSE-push (а не через initial REST —
  поэтому сеем host с MAC, которого изначально НЕТ в arp, потом дописываем
  его → 🟢 появляется только через SSE-дельту).
- Альтернатива (проще, слабее): smoke «`EventSource('/api/events')`
  коннектится + readyState OPEN под Bearer». Реализуй этот если полный
  вариант не идёт.

### CI-правки для 6.7/6.8 (ЕСЛИ делаешь infra-варианты)
Внутри opt-in шага L4 в `build.yml`: для sse — сменить `-arp-file` на
`/tmp/e2e-arp.txt` и скопировать туда fixture в начале шага; для setup —
поднять 2-ю инстанцию `:18084`. НЕ выносить в дефолтный прогон.

---

## 7. Блок C — mutation-pass (разовый, throwaway-ветка)

**Цель:** эмпирически доказать, что спеки ловят регрессии, а не «зелено от
того, что мёртвые». После `deleteHostApi`-бага (спал 3 фазы) — не паранойя.

**Процесс:**
1. `git checkout -b mutation-test` от `main`.
2. Для каждой мутации в таблице — внеси ОДНУ правку продукта, коммит
   (`mutation: <id>`), пушь, прогон CI `run_e2e_tests=true`.
3. Ожидаемый результат: краснеют ровно spec'и из колонки «должны упасть»
   (и только они). Если мутация НЕ роняет ожидаемый spec → spec слабый,
  допили assertion.
4. После всех — `git checkout main; git branch -D mutation-test` (НЕ мержить).
   Forgejo сам удалит remote-ветку при необходимости (или удали через UI/`gh`).

**Таблица мутаций:**

| мутация (продукт) | файл:что сделать | должны упасть (spec) |
|---|---|---|
| `addHostHandler` всегда 500 | `handlers_hosts.go` в конец `return` 500 | host-add-ui, host-tags, host-edit-ui, bulk-import-text, csv-import, rollback-ui (seed), history-modal (seed), search-filter (seed), bulk-ops (seed) |
| сломать `sortBy` (no-op) | `HostTable.vue:92` `sortBy` → пустое тело | hosts-sort |
| `addAliasHandler` всегда 500 | `handlers_aliases.go` | dns-aliases-add |
| `reloadDnsmasq` всегда error | `sse.go:109` `return fmt.Errorf(...)` первой строкой | reload-ui (waitForResponse 200 таймаут) |
| logout не чистит token | `api/system.js:97` `logoutRequest` → убрать `store.token=''` | auth (logout → `.btn-primary` не виден) |
| `deleteHostHandler` no-op (не удаляет) | `handlers_hosts.go` — не писать файл | host-crud, rollback/history (если assert удалённого) |

Достаточно 4–5 мутаций. Если каждая роняет ровно ожидаемые — уверенность
получена. Любая мутация, не роняющая ничего → красный флаг, усилить spec.

---

## 8. CI (`.forgejo/workflows/build.yml`)

Шаг «L4 — Playwright E2E» уже корректен. Менять только если:
- setup-screen: добавить 2-ю инстанцию `:18084` (§6.7).
- sse-live: `-arp-file /tmp/e2e-arp.txt` + копирование fixture (§6.8).
Всё внутри opt-in (`if run_e2e_tests == 'true'`). Дефолтный прогон НЕ трогать.

---

## 9. Проверка

**Локально (Windows, `tests/e2e/`):**
- `npm ci` — lockfile валиден (не должен меняться).
- `npx playwright test --list` — должно быть **25 + N новых = ~33 теста**
  (N зависит от того, сколько из 6.5–6.8 реально добавлено).
- `grep -rn "ctx\.\(get\|post\|delete\)" tests/e2e/lib` — все пути с `/api`.

**CI (основная):**
- Прогон `run_e2e_tests=true`: 25 + новые, все PASS, bulk-edit БЕЗ `.fail`
  (после A5), known-bugs.txt БЕЗ A13.
- Прогон дефолтный: smoke.sh зелёный (после удаления A13 — проверь что
  smoke-check для A13 стал ожидать успех и реально зелёный).

**Что НЕ должно сломаться:** go test (`-race`), gofmt, smoke.sh (кроме
снятого A13), дефолтный тайминг CI.

---

## 10. Приёмка (definition of done)

- [ ] A5 пофиксен (1 строка), bulk-edit spec без `.fail`, PASS.
- [ ] A13 пофиксен (3 строки), убран из `known-bugs.txt`, smoke-check обновлён,
      go test зелёный.
- [ ] Батч 4: добавлены spec'и из §6 (минимум audit/plugins-iframe/
      i18n-api-error/config-template-fill; config-directive/raw после A13;
      setup-screen/sse-live — или реализованы, или `test.skip` с комментом).
- [ ] `npx playwright test --list` — ~30–33 теста, `--list` зелёный.
- [ ] CI `run_e2e_tests=true` — все PASS (0 known-fail в L4).
- [ ] Mutation-pass: на `mutation-test`-ветке 4–5 мутаций роняют ровно
      ожидаемые spec'и; ветка удалена, в main не мержена.
- [ ] Session-лог `логи/gap2-finish.md` (контекст → A5/A13 фиксы → батч 4 →
      mutation-pass → результат). ROADMAP.md + duis.md обновлены (L4 → финал,
      A5/A13 → FIXED и убраны из баг-таблицы).

---

## 11. Порядок исполнения (рекомендация)

1. **A5 + A13 фиксы** (§5) — коммит, прогон (e2e + дефолтный smoke). Это
   разблокирует config-directive/raw и снимет `.fail`/KNOWN-fail.
2. **Простые батч-4 spec'и** (§6.1–6.4): audit, plugins-iframe,
   i18n-api-error, config-template-fill. Коммит, `--list`, push, CI.
3. **config-directive/raw** (§6.5–6.6, после A13).
4. **setup-screen / sse-live** (§6.7–6.8) — оценить сложность; если infra
   тяжело — `test.skip` с комментом и двигаться.
5. **Mutation-pass** (§7) на throwaway-ветке.
6. **Документация:** session-лог + ROADMAP + duis.

На каждом шаге — CI, не копить. Если bootstrap-шаг падает по сети
(chromium/hello-plugin) — это блокер №1, решать до остальных spec'ов.

---

## 12. ВНЕ области этой задачи

- Правки `system.go` (init-system), `bins.go`, горутины — это **Gap 4** (real VM).
- Fuzzing парсеров — отдельная задача.
- Любой рефакторинг продуктовых исходников сверх A5/A13 — запрещён (§1).
- Мутации в main, browser-matrix, real-device — enterprise, не сюда.
