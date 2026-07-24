# Test Coverage Roadmap

Где мы сейчас, куда идём, что нужно добавить чтобы дойти до 95-100%.

---

## Текущее состояние

| Слой | Coverage | Статус |
|---|---|---|
| L1 — Go unit | ~85% pure logic | ✓ стабильный (`dnsmasq_test.go` + `new_features_test.go`) |
| L2 — Go integration (httptest) | ~85-90% handlers | ✓ ~50/52 handlers; skip `eventsHandler` (SSE) и `reloadHandler` (нужен dnsmasq binary) |
| L3 — smoke.sh | ~75-80% API | ✓ 29 suite-файлов, 136 проверок; плагин-прокси покрыт (`82-plugins.sh`) |
| L4 — Playwright UI | 0% | ✗ не реализован |
| L5 — Real VM (init/dnsmasq) | 0% | ✗ не реализован |
| Perf/stress (opt-in) | реализовано, informational | ✓ `tests/perf.sh` (read/reload/CRUD+RSS/SSE); не coverage-слой |

**Суммарно: ~90%** функционала покрыто автоматизированными тестами.

---

## Уже закрыто

| Gap | Что закрыло | Лог |
|---|---|---|
| **Gap 1** — непокрытые endpoints smoke.sh (~15%) | Все Gap 1 endpoints добавлены в suites | `smoke-refactor-and-gap1.md` |
| **Gap 3** — Go edge cases (~5%) | +56 L2/edge тестов в `handlers_test.go` (IPv6, unicode, concurrent writes, empty/comment-only conf, ZIP edge cases) | `gap3-l2-handler-tests.md` |
| **Gap 5** — Performance/stress (~3%) | `tests/perf.sh` + `tests/fixtures/gen-hosts.sh` + opt-in CI input `run_perf_tests` | `gap5-6-perf-and-plugins.md` |
| **Gap 6** — Plugin system (~2%) | Mock-плагин `tests/fixtures/plugins/hello/` + расширение `82-plugins.sh` (presence + проксирование) | `gap5-6-perf-and-plugins.md` |

---

## Что осталось

### Gap 2: UI behavior (~+20%) — главный остаток

**Сценарии:** всё что делает Vue-реактивность и browser-side logic:
- A1 — дубли строк при сортировке (Vue key collision)
- A5 — bulk-edit modal behavior
- A7 — templates UI
- i18n переключение, dark/light темы
- Live SSE updates, поиск и сортировка в таблице

**Решение:** Playwright. Bootstrap (Chromium в Fedora CI-контейнер) + 20-30 spec-тестов. ~2 дня.

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

После Gap 2 + Gap 4: ~90-93%. Оставшееся до 95%+ — enterprise-grade:

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
| **P1** | Playwright (Gap 2) | 2 дня | +20% |
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
