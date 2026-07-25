# Gap 2 — Playwright E2E, батч 2 (UI coverage + A5 reproducer)

**Дата:** 2026-07-25
**Gap:** 2 (L4 — UI/E2E), продолжение `логи/gap2-playwright-bootstrap.md`
**Коммиты:** `558f8fd` (5 specs + helper + refactor), `7ec2b62` (A5 → `test.fail`)
**Результат:** CI `run_e2e_tests=true` — **11/11 (10 passed + 1 expected-fail = A5)**

## Контекст

Батч 1 (`логи/gap2-playwright-bootstrap.md`) поднял Playwright-инфру и 5
базовых спеков. Остальное browser-side поведение (CRUD-create, tags, search,
bulk-операции, config editor) всё ещё без E2E-покрытия. Цель батча — закрыть
основную UI-функциональность и заодно попытаться поймать A5 (BulkEditModal)
честным репродюсером. Полный план и оценка рисков — в разговоре перед стартом
(поиск-инпут без stable-селектора, config-save gated by `dnsmasq --test`
(A13), SSE упирается в статичный arp-fixture).

## Что сделано

### Инфра

- **`tests/e2e/lib/api.ts`** — общий seed-хелпер: `apiLogin()`,
  `seedHosts(token, [...])` (идемпотентно, 409 игнорируется),
  `deleteHostApi(...)`. Убрал дубли login+seed из sort/crud.
- Рефакторнул `hosts-sort.spec` и `host-crud.spec` на хелпер (поведение
  идентично, `--list` подтверждает).

### 5 новых spec'ов

| spec | что проверяет | ключевые селекторы |
|---|---|---|
| `host-add-ui` | добавить через HostForm → строка появилась без reload | MAC-инпут `placeholder="MAC (aa:bb...)"` (захардкожен, не i18n); file-инпут через `.input-group:has(.btn-success)` |
| `host-tags` | `set:iot,tag:guest` → бейджи `span.badge` | tags-инпут `.form-control.font-monospace` |
| `search-filter` | фильтр по префиксу (3→3→0) | search-инпут структурно `.col-md > input.form-control` (placeholder i18n) |
| `bulk-ops` (move) | чекбоксы → 📦 → customTarget → файл-ячейка сменилась | bulk-бар `.bg-danger .btn-group button hasText 📦`; модалка `.modal-content` |
| `bulk-ops` (edit) | чекбоксы → ✏️ → IP-prefix transform → IP сменился | bulk-кнопка ✏️; old/new-prefix `input.form-control` nth 0/1 |
| `config-files` | config-таб → `+` new file → вкладка → 🗑 delete → исчезла | new-file `placeholder="filename.conf"` (захардкожен); create `.card.border-success .btn-success` |

**Изоляция:** per-spec MAC-префиксы без коллизий с arp-fixture (`aa:bb:cc`,
`11:22:33`) и между собой. Каждый host-зависимый спек пишет в свой `.conf`.
Все селекторы — CSS/emoji/placeholder, без `data-testid` (продуктовые
исходники не тронуты).

### CI-интеграция

Без изменений — шаг «L4 — Playwright E2E» (opt-in `run_e2e_tests`) уже на
месте после батча 1; новые спеки подхватываются автоматически.

## Бонус: A5 пойман точным репродюсером

`bulk-ops.spec` (тест bulk-edit) упал на `expect(modal).toBeVisible()` после
клика ✏️ — модалка не появилась. Разбор показал root cause A5:

```js
// frontend/src/components/static/BulkEditModal.vue:65-67
const preview = computed(() => {
  return props.hosts.slice(0, 5).map(h => {
    const host = store_hosts.find(x => x.mac === h.mac)   // ← BUG
```

`store_hosts` — это реактивный `store` (`import { store as store_hosts }`),
у которого **нет** метода `.find`. Должно быть `store_hosts.hosts.find(...)`.
`computed preview` бросает `TypeError` при первом доступе (`v-if="preview.length > 0"`
в шаблоне), Vue рендер модалки абортится → `.modal-content` не появляется.
Поэтому bulk-**move** прошёл (там `store.hosts.map` написано корректно), а
bulk-**edit** — нет.

**Фикс в проде — одна строка:**
```diff
- const host = store_hosts.find(x => x.mac === h.mac)
+ const host = store_hosts.hosts.find(x => x.mac === h.mac)
```

По философии проекта (баги собираем, не чиним в тестовой работе) продуктовые
исходники не правил. Тест помечен **`test.fail()`** — CI зелёный, а семантика
зеркалит smoke.sh KNOWN-fail: когда A5 пофиксят, тест «пройдёт» → Playwright
сообщит «expected to fail but passed» → напомнит снять `.fail` (и комментарий).

## Результат

CI (Forgejo, `fedora:44`, `run_e2e_tests=true`): **11 tests, 10 passed +
1 expected-fail (A5)**. Все новые UI-потоки (add/tags/search/bulk-move/
config-create-delete) зелёные.

## Что НЕ в этом батче (3-й батч)

- **SSE-live.** Главный затык: `startSSEBroadcaster` (sse.go:73) шлёт только
  дельты (`if arpJSON != lastArp`), а arp-fixture статичен → после первого
  пуши тишина. Плюс приложение грузит arp через REST (`loadData`), а не SSE,
  так что «🟢 появился» не доказывает SSE. Варианты: (а) smoke
  «EventSource на `/api/events` коннектится + auth» (просто, но слабо), или
  (б) полноценный тест с мутацией arp-файла (правка CI: копировать fixture в
  writable `/tmp`-путь и дописывать строку mid-test → ждать дельту).
- **A7 (TemplatesModal)** — UI-smoke; по `duis.md` это cosmetic, не баг.
- **true A5-фикс** (1 строка) → снять `test.fail` с bulk-edit-теста.

## Локальная проверка (механическая)

- `npm ci` — без изменений (lockfile не тронут).
- `npx playwright test --list` — **11 tests in 10 files**, импорт `../lib/api`
  резолвится, TS/конфиг компилируются.
