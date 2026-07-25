# Gap 2 — Playwright E2E, батч 3 фаза А (UI coverage, +5 specs)

**Дата:** 2026-07-25
**Gap:** 2 (L4 — UI/E2E), продолжение `gap2-batch2-ui-coverage.md`
**Коммиты:** `8e04be3` (5 specs + split helper), `75128fb` (2 CI-фикса селекторов)
**Результат:** CI `run_e2e_tests=true` — **16/16 passed**

## Контекст

После батча 1+2 (11 specs) основной UI всё ещё имел дыры: редактирование
хоста, bulk-delete, шаблоны (A7), users-вкладка. Цель батча 3 — добить
до ~24 specs, разбитыми на фазы А (низкий риск) → Б (средний) → В (выше).
План зафиксирован в `C:\Users\alexr\AppData\Local\Temp\opencode\l4-batch3-plan.md`.
Фаза А = 5 specs, которые переиспользуют уже знакомые паттерны (HostForm,
модалки, confirm-диалоги).

## Что сделано

### Инфра — разбивка хелпера

`tests/e2e/lib/api.ts` разбит, чтобы не рос одним файлом:
- `lib/api-auth.ts` — `BASE_URL`, `CONF_DIR`, `apiLogin()`.
- `lib/api-hosts.ts` — `SeedHost`, `seedHosts()`, `deleteHostApi()`.
- `lib/api.ts` — barrel (`export * from './api-auth'; export * from './api-hosts'`).

Импорты спеков (`../lib/api`) не менялись — barrel держит путь стабильным.
В фазе Б добавится `lib/api-aliases.ts`.

### 5 новых spec'ов (+5 → 16)

| spec | что проверяет | селекторы / нюансы |
|---|---|---|
| `host-edit-ui` | ✏️ на строке → edit-mode → сменить IP → Save → IP-ячейка обновилась без reload | IP-инпут через `.input-group:has(button:has-text("🎲"))`; Save `.input-group .btn-warning` |
| `bulk-delete` (в `bulk-ops.spec`) | отметить 3 (prefix `aa:99:11:22:33:0[1-3]`) → 🗑️ → confirm → 0 строк | `.bg-danger .btn-group` 🗑️ + `page.on('dialog')` |
| `templates-modal` (A7 smoke) | ⚙️ в HostForm → создать шаблон → в списке → ✕ + confirm → исчез | hostname_pattern по захардкоженному `placeholder="device-{NNN}"`; остальные позиционно |
| `users-tab` (create+delete) | create `e2euser` → в списке → delete → исчез | create-карта через `filter({has: '.btn-success'})`; username = единственный `input[type="text"]` |
| `users-tab` (delete-self) | delete `admin` → отказ (`cannot_delete_self`), строка остаётся | ждём `≥2` диалогов (confirm + error alert) |

## Грабли (один красный CI-прогон до зелёного)

14 passed, 2 failed — оба точечно:

1. **`host-edit-ui`: strict mode violation.** `.btn-warning` матчил 2
   элемента — HostForm Save (edit-mode) И тулбарная «🔄 Применить»
   (`App.vue` `applyConfig` тоже `.btn-warning`). Фикс: `.input-group .btn-warning`
   (Save в форме, тулбар не в input-group).
2. **`users-tab` (delete-self): `dialog.accept: page has been closed`.**
   `deleteUser()` на отказе шлёт **второй** `alert(msg)` (`api/system.js:78`).
   Тест резолвился (admin-строка видна) быстрее, чем alert стрелял →
   Playwright рвал страницу с pending-alert → краш хендлера. Фикс: счётчик
   диалогов + `expect.poll(() => dialogs).toBeGreaterThanOrEqual(2)` —
   ждём confirm И error-alert, потом assert. Тест 1 (create+delete) не
   падал, т.к. успех → нет alert (один диалог).

## Результат

CI (Forgejo, `fedora:44`, `run_e2e_tests=true`): **16/16 passed**. Все
фаза-А потоки (edit/bulk-delete/templates/users-CRUD/delete-self) зелёные.
bulk-edit остаётся `test.fail` (A5, без изменений).

## Что дальше

- **Фаза Б (4):** dns-aliases-add, bulk-import-text, csv-import, reload-ui.
  Потребует `lib/api-aliases.ts` (seedAlias) и file-upload (`setInputFiles`)
  для csv.
- **Фаза В (4):** rollback-ui, history-modal, discovery-tab, backup-restore-ui
  (последний — самый сложный: dynamic file input → `page.on('filechooser')`).
- Потом батч 4 (6 нишевых + 2 жёстких).

## Локальная проверка (механическая)

- `npx playwright test --list` — **16 tests in 13 files**, разбивка хелпера
  резолвится (barrel → api-auth + api-hosts).
