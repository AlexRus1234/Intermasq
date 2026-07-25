# Gap 2 — Playwright E2E bootstrap (первая итерация)

**Дата:** 2026-07-25
**Gap:** 2 (L4 — UI/E2E)
**Спека:** `логи/Gap_2.md`
**Коммиты:** `e76a902` (реализация), `3dd91d1` + `26eccb7` (фиксы депов)
**Результат:** CI `run_e2e_tests=true` — **5/5 PASS**

## Контекст

L4 (UI) был единственным крупным нулём в покрытии. Frontend-баги A1
(дубли строк при сортировке), A5 (BulkEditModal), A7 (TemplatesModal) и
вообще вся браузерная логика (тема, i18n, SSE, CRUD без reload) HTTP-слоем
(smoke.sh) не ловятся — нужна реальная голова браузера. Цель итерации —
поднять Playwright против живого `intermasq-ci` и написать минимальный
батч из 5 spec'ов. Полный список (20-30 specs, A5/A7/SSE/search) — вторым
батчем.

## Что сделано

### Структура (всё новое, продуктовые исходники не тронуты)

```
tests/e2e/
├── package.json + package-lock.json   # изолированный @playwright/test (^1.49 → 1.62)
├── playwright.config.ts                # workers:1, fullyParallel:false
├── global-setup.ts                     # wait-server, setup|login, storageState
├── .gitignore                          # node_modules/ .auth/ test-results/ playwright-report/
└── specs/
    ├── auth.spec.ts        login → dashboard → logout (fresh context)
    ├── theme.spec.ts       🌓 → data-bs-theme="dark" + persist
    ├── i18n.spec.ts        🌐 → смена подписи пункта + persist
    ├── hosts-sort.spec.ts  A1 guard: count строк префикса стабилен после 3 кликов
    └── host-crud.spec.ts   seed → виден → ✕ (dialog accept) → исчез без reload
```

Раннер изолирован своим `package.json` в `tests/e2e/` — `frontend/package.json`
не правился. `storageState` сеет только `localStorage.token`; `theme`/`locale`
в свежем контексте = null → детерминированные asserts (не завязаны на дефолтную
локаль/тему). MAC-префиксы изолированы per-spec (`aa:11:11:11:11` для sort,
`aa:22:33:44:55:01` для crud), чтобы не конфликтовать ни с `gen-hosts.sh`
(`aa:bb:cc:...`), ни между собой на общем conf-dir.

### CI (opt-in)

`.forgejo/workflows/build.yml`: новый input `run_e2e_tests` (default false) +
шаг «L4 — Playwright E2E» после perf. Своя инстанция `intermasq-ci` на `:18083`
со своим conf-dir (`/tmp/e2e-conf`) — не конфликтует со smoke (`18081`) и perf
(`18082`). Дефолтный прогон не изменился.

### Грабли на пути (два красных CI-прогона до зелёного)

1. **`--with-deps` не умеет dnf.** `npx playwright install --with-deps` знает
   только apt (Ubuntu/Debian). На Fedora 44 он фолбэчится на ubuntu-депы и
   падает на `apt-get: command not found` (exit 127). Фикс: shared-lib депы
   chromium ставим сами через `dnf`, бинарник браузера качаем без `--with-deps`
   (`3dd91d1`). Спек (§5) тут ошибался — предполагал, что `--with-deps`
   разрулит через dnf.

2. **`gtk3` тянул недоступный кодек-реп.** Первая попытка поставить депы через
   `gtk3` (рассчитывал, что он транзитивно даст atk/pango/cairo) утянула весь
   десктоп-стек: pipewire, polkit, libcamera, **openh264**… `openh264` лежит
   на внешнем кодек-зеркале (`codecs.fedoraproject.org` /
   `ciscobinary.openh264.org`), недоступном из контейнера (SSL reset + 403),
   и весь `dnf install` падал на нём. Фикс: выкинул `gtk3`, ставлю напрямую
   `atk at-spi2-atk` (ATK-слой) + минимальный набор X/gbm/alsa/pango либ,
   с `--setopt=install_weak_deps=False` чтобы убрать рекомендованные пакеты
   и их кодек-репы (`26eccb7`). Headless chromium полный gtk3 не нужен.

## Результат

CI (Forgejo, `fedora:44`, `run_e2e_tests=true`): **5/5 PASS**.

- `auth` — login+logout через UI в fresh context.
- `theme` — toggle ставит `data-bs-theme="dark"` + `localStorage.theme`.
- `i18n` — подпись 🌐-пункта меняется + `localStorage.locale` непустой.
- `hosts-sort` (A1 guard) — count строк префикса `aa:11:11:11:11` = 5
  стабилен после 3 кликов сортировки по IP.
- `host-crud` — seed-хост удаляется через ✕ (после `page.on('dialog')`),
  строка исчезает без full reload.

## Замечание про A1

`hosts-sort.spec` — это **guardrail**, а не воспроизведитель бага: с
уникальными MAC (как в тесте) `:key="h.mac"` коллизий не даёт, так что
оригинальный A1 с уникальными ключами может и не воспроизвестись. Тест
ловит регрессию «сортировка ломает рендер строк» (дубли/потери), но не
гарантирует поимку исходного механизма. Если понадобится точный
reproducer — отдельная задача (seed с дублирующим MAC через прямой
запуск/правку файлов, в обход API-валидации).

## Что НЕ в этой итерации (второй батч)

A5 (BulkEditModal), A7 (TemplatesModal), SSE live-updates, search/filter,
tags badge, config editor — отдельной задачей после устоявшегося бутстрапа.

## Локальная проверка (механическая)

- `npm ci` в `tests/e2e/` — lockfile валиден (3 пакета: `@playwright/test`,
  `playwright`, `playwright-core`).
- `npx playwright test --list` — 5/5 тестов найдены, TS + конфиг
  компилируются (без сервера/браузера/globalSetup).
- Прогон specs реален только в CI — на Windows `intermasq` продуктивно не
  стартует (init-system/пути).
