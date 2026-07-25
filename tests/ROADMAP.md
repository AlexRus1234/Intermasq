# Test Coverage Roadmap

Где мы сейчас, куда идём, что нужно добавить чтобы дойти до 95-100%.

---

## Текущее состояние

| Слой | Coverage | Статус |
|---|---|---|
| L1+L2 Go (unit + httptest) | **65.6%\*** (измерено) | один package `main` → L1/L2 совместно не делятся. Парсеры/handler'ы 80-100%; разрыв сосредоточен в init-system/bootstrap/goroutine-коде (см. сноску) |
| L3 — smoke.sh | ~75-80% API | ✓ 29 suite-файлов, 136 проверок; плагин-прокси покрыт (`82-plugins.sh`). Иная метрика — доля эндпоинтов, не строки |
| L4 — Playwright UI | 11 specs | ◐ батч 1+2 закрыты (auth/theme/i18n/hosts-sort/host-crud/host-add-ui/host-tags/search-filter/bulk-ops/config-files). A5 воспроизведён (`test.fail`); остаток — SSE-live + A7-smoke + true A5-фикс |
| L5 — Real VM (init/dnsmasq) | 0% | ✗ не реализован |
| Perf/stress (opt-in) | реализовано, informational | ✓ `tests/perf.sh` (read/reload/CRUD+RSS/SSE); не coverage-слой |

> **\*** `65.6%` — измерено `go test -cover ./...` (241 тест, package `main`).
> Раньше в доках фигурировали оценки «~85-90%», но то был подсчёт «handler'ов с
> хотя бы одним тестом», а не statement-coverage. ~34% непокрытых строк
> сосредоточены в: `system.go` (init-system exec — это и есть **Gap 4**),
> `bins.go` (резолв linux-бинарных `sudo`/`systemctl`/`service`/`rc-service`/`sv`),
> `main.go` (`main`/`loadPlugins` — bootstrap), `sse.go` (`startSSEBroadcaster`/
> `reloadDnsmasq` — горутина + dnsmasq exec), `metrics.go` (`startDNSHealthChecker`/
> `runDNSHealthPass`). Дотянуть до ~99% в текущем окружении **нереально** — нужно
> закрывать Gap 4 (real VM) + рефакторить bootstrap (правка исходников).;

**Суммарно:** Go-покрытие 65.6%\* (измерено); L3 API ~75-80% (иная метрика);
L4/L5 — 0%. Метрики разных слоёв не суммируются в одно число.

---

## Уже закрыто

| Gap | Что закрыло | Лог |
|---|---|---|
| **Gap 1** — непокрытые endpoints smoke.sh (~15%) | Все Gap 1 endpoints добавлены в suites | `smoke-refactor-and-gap1.md` |
| **Gap 3** — Go edge cases (~5%) | +56 L2/edge тестов в `handlers_test.go` (IPv6, unicode, concurrent writes, empty/comment-only conf, ZIP edge cases) | `gap3-l2-handler-tests.md` |
| **Gap 5** — Performance/stress (~3%) | `tests/perf.sh` + `tests/fixtures/gen-hosts.sh` + opt-in CI input `run_perf_tests` | `gap5-6-perf-and-plugins.md` |
| **Gap 6** — Plugin system (~2%) | Mock-плагин `tests/fixtures/plugins/hello/` + расширение `82-plugins.sh` (presence + проксирование) | `gap5-6-perf-and-plugins.md` |
| **Gap 2** (1-я итерация) — Playwright bootstrap | `tests/e2e/` (изолированный `@playwright/test`, `global-setup`, 5 specs: auth/theme/i18n/hosts-sort/host-crud) + opt-in CI input `run_e2e_tests`. A1 под regression-guard. | `gap2-playwright-bootstrap.md` |
| **Gap 2** (2-й батч) — UI coverage | +5 specs (host-add-ui/host-tags/search-filter/bulk-ops[bulk-move+bulk-edit]/config-files) + общий seed-хелпер `tests/e2e/lib/api.ts`. A5 пойман репродюсером (`test.fail`, root cause pinned). | `gap2-batch2-ui-coverage.md` |

---

## Что осталось

### Gap 2: UI behavior (~+20%) — 2 батча закрыты, остаток = SSE + A5-фикс

**Закрыто (батч 1+2, 11 specs):** Playwright против `intermasq-ci` в CI
(Fedora 44, opt-in `run_e2e_tests`). Батч 1: auth/theme/i18n/hosts-sort
(A1 guard)/host-crud. Батч 2: host-add-ui/host-tags/search-filter/bulk-ops
(bulk-move + bulk-edit)/config-files + общий seed-хелпер
`tests/e2e/lib/api.ts`. См. `логи/gap2-playwright-bootstrap.md` и
`логи/gap2-batch2-ui-coverage.md`.

**A5 воспроизведён (бонус батча 2):** bulk-edit-спек оказался точным
репродюсером A5 — `BulkEditModal.vue:67` звёт `store_hosts.find(...)`
вместо `store_hosts.hosts.find(...)` → TypeError в computed → модалка не
рендерится. Помечен `test.fail()` (CI зелёный); фикс в проде = одна
строка, после него снять `.fail`.

**Осталось (3-й батч / мелочь):**
- SSE-live: либо smoke «EventSource на `/api/events` коннектится», либо
  полноценный тест с мутацией arp-файла (вещатель шлёт только дельты, а
  arp-fixture статичен — нужна правка CI: копировать fixture в writable
  путь и дописывать строку mid-test).
- A7 (TemplatesModal) — UI-smoke (по duis.md это cosmetic, не баг).
- true A5-фикс (1 строка в `BulkEditModal.vue`) → снять `test.fail`.

**Решение:** Playwright, расширение `tests/e2e/specs/`. ~0.5-1 день.

### Gap 4: Real init-system integration (~+5%)

**Что не покрыто:**
- `detectInitSystem()` чтение `/proc/1/comm`
- Реальные `exec.Command("systemctl", ...)` calls
- Systemd-user vs system caller detection
- OpenRC, runit, sysvinit callers
- `sudo systemctl restart dnsmasq` через sudoers (rootless-режим)

**Решение:** L5 — nightly job на persistent test VM.
1. Snap VM к чистому состоянию (Proxmox API или virsh)
2. Установить intermasq-ci как systemd-unit
3. Прогнать smoke.sh с `-init-system=systemd`
4. Проверить что dnsmasq реально рестартует
5. Повторить с systemd-user, openrc, runit (container per init)
6. Отчёт

Время: 1-2 дня на bootstrap, дальше работает само.

### Fuzzing (~+2-3%)

**Где:** Go built-in fuzzing (`go test -fuzz`).
**Цели:** `FuzzParseDhcpHostLine`, `FuzzParseArpContent`, `FuzzParseLeases`, `FuzzParseAliasLine`.

---

## Что нужно для 95-100% (в реальности 98-99%)

Go statement-coverage сейчас **65.6%\***. Реалистичный потолок **в текущем
окружении — ~80-85%**: остаточные unit-тесты (+3-5%), `system.go` callers через
fake-бинарники на PATH (+8-12%, но тест против моков), Linux-gated exec-тесты
для `reloadDnsmasq`/DNS-health (+3-5%). Дальше — потолок:

- **`main()`/`loadPlugins()`** — bootstrap, не юнит-тестируем без рефакторинга
  исходников (вынос логики в тестируемые функции).
- **`detectInitSystem` + реальное init-взаимодействие** — это **Gap 4 (real
  VM)**; в Fedora-контейнере PID 1 не systemd.
- **Фоновые горутины** (`startSSEBroadcaster`, `cleanBlacklistLoop`) — partial.

То есть **~99% statement-coverage недостижимо без закрытия Gap 4 + правки
исходников**. При этом statement-% `system.go` через фейки даёт число, но не
реальную уверенность — функциональное покрытие (Gap 4 на VM) тут ценнее.

Оставшееся сверх реалистичного потолка — enterprise-grade:

- **Mutation testing** — `go-mutesting`, проверяют что тесты ловят мутации
- **Compatibility matrix** — разные версии dnsmasq (2.80, 2.89, 2.90+)
- **Cross-distro** — Fedora, Debian, Alpine, Ubuntu, OpenSUSE
- **Browser matrix** — Chrome, Firefox, Safari, Edge
- **Real device testing** — phones с random MAC, IoT devices

Для pre-release v1.0 избыточно.

---

## План по приоритетам

| Приоритет | Задача | Время | Дельта coverage |
|---|---|---|---|
| **P0** | Пофиксить баги A1-A4 + A12-A13 | 2-3 часа | (чистит красноту known-bugs) |
| **P1** | Playwright (Gap 2) — батч 1+2 ✓ (11 specs), остаток SSE+A7+A5-фикс | +0.5 дня | основное UI-покрытие закрыто; SSE/true-A5 — 3-й батч |
| **P2** | L5 Real VM nightly (Gap 4) | 1-2 дня | +5% |
| **P2** | Fuzzing для парсеров | 0.5 дня | +2-3% |

---

## Метрики "когда готово к v1.0 release"

- [ ] `tests/known-bugs.txt` пустой (или содержит только wontfix'ы)
- [ ] smoke.sh: 0 Fail, 0 Known-fail, 0 Skipped, ~140+ Pass
- [ ] L1+L2 Go test coverage ≥ 70% (`go test -cover ./...`)
- [ ] Playwright: 20+ spec'ов, все зелёные
- [ ] L5 nightly: 7 дней без красноты
- [ ] `tests/perf.sh`: 0 hard failures на дефолтных порогах
- [ ] Все баги из `tests/bugreport/bugs.md` либо FIXED, либо WONTFIX с rationale
- [ ] CHANGELOG.md обновлён
- [ ] README обновлён (installation, configuration, troubleshooting)
