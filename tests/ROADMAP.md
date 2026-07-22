# Test Coverage Roadmap

Где мы сейчас, куда идём, что нужно добавить чтобы дойти до 95-100%.

---

## Текущее состояние (после CI-автоматизации сессии)

| Слой | Coverage | Статус |
|---|---|---|
| L1 — Go unit | ~85% pure logic | ✓ стабильный (2685 строк тестов) |
| L2 — Go integration (httptest) | ~30% handlers | △ только существующие, новые не добавлены |
| L3 — smoke.sh | ~60% API | ✓ стабильный (~80 проверок) |
| L4 — Playwright UI | 0% | ✗ не реализован |
| L5 — Real VM (init/dnsmasq) | 0% | ✗ не реализован |

**Суммарно: ~60-65%** функционала покрыто автоматизированными тестами.

---

## Что мешает дойти до 80%

### Gap 1: HTTP handlers без integration tests (~15%)

**Сценарии:** endpoint-ы, которые smoke.sh не гоняет:
- `POST /api/hosts/bulk-move`
- `POST /api/hosts/bulk-edit`
- `POST /api/aliases/bulk`
- `POST /api/aliases/csv` (import)
- `GET /api/aliases/csv` (export)
- `PUT /api/config` (visual editor)
- `GET /api/history/diff`
- `POST /api/history/restore`
- `POST /api/backup/restore`
- `GET /api/plugins`
- `GET /api/templates/:id` (DELETE)
- `GET /api/templates/ranges`
- `GET /api/leases`, `/api/arp`, `/api/new-devices`

**Решение:** расширить smoke.sh. ~2-3 часа работы. Каждая endpoint =
3-5 проверок (happy + edge + error).

### Gap 2: UI behavior (~20%)

**Сценарии:** всё что делает Vue-реактивность и browser-side logic:
- A1 — дубли строк при сортировке (Vue key collision)
- A5 — bulk-edit modal behavior
- A7 — templates UI
- i18n переключение
- dark/light темы
- Live SSE updates
- Поиск и сортировка в таблице

**Решение:** Playwright. ~2 дня на bootstrap + 20-30 spec-тестов.

### Gap 3: Edge cases (~5%)

**Сценарии:**
- Concurrent writes на одном файле (race)
- Very long hostnames (64+)
- Unicode в hostname/domain
- IPv6 IP parsing
- Empty `.conf` файлы
- Файлы только с комментариями
- Network прерывания во время reload

**Решение:** Go integration tests через httptest с моками. ~1 день.

---

## Что нужно для 95%

После закрытия Gap 1-3: ~85% coverage. Чтобы дотянуть до 95%, нужно:

### Gap 4: Real init-system integration (~5%)

**Что не покрыто:**
- `detectInitSystem()` чтение `/proc/1/comm`
- Реальные exec.Command("systemctl", ...) calls
- Systemd-user vs system caller detection
- OpenRC, runit, sysvinit callers
- `sudo systemctl restart dnsmasq` через sudoers

**Решение:** L5 — nightly job на persistent test VM. Скрипт:
1. Snap VM к чистому состоянию (Proxmox API или virsh)
2. Установить intermasq-ci как systemd-unit
3. Прогнать smoke.sh с `-init-system=systemd`
4. Проверить что dnsmasq реально рестартует
5. Повторить с systemd-user, openrc, runit (container per init)
6. Отчёт в Slack/email

Время: 1-2 дня на bootstrap, дальше работает само.

### Gap 5: Performance/stress (~3%)

**Что не покрыто:**
- Время ответа при 200+ хостах
- 50 параллельных SSE клиентов
- 10 одновременных reload
- Memory leaks при длительной работе

**Решение:**
- `tests/fixtures/gen-hosts.sh` — генератор .conf с N хостами
- `hey` или `wrk` для load testing
- Контейнер с sleep 86400 + monitor RSS

Время: 0.5-1 день.

### Gap 6: Plugin system (~2%)

**Что не покрыто:**
- Plugin discovery из `/etc/intermasq/plugins/`
- Plugin manifest parsing
- Unix-socket proxy
- Plugin lifecycle (crash → no restart, supervised?)

**Решение:**
- Mock plugin в `tests/fixtures/plugins/hello/`
- Test plugin: shell-скрипт, открывает сокет, отвечает "hello"
- smoke.sh: проверяет что `/api/plugins` показывает плагин, что
  `/plugins/hello/` проксируется

Время: 0.5 дня.

---

## Что нужно для 100% (в реальности 98-99%)

Теоретический 100% coverage требует:

- **Mutation testing** — `go-mutesting` или аналоги, проверяют что
  тесты ловят изменения в коде
- **Fuzzing** — `go test -fuzz` для парсеров (`parseDhcpHostLine`,
  `parseArpContent`, `parseLeases`)
- **Compatibility matrix** — тесты на разных версиях dnsmasq (2.80,
  2.89, 2.90+)
- **Cross-distro** — Fedora, Debian, Alpine, Ubuntu, OpenSUSE
- **Browser matrix** — Chrome, Firefox, Safari, Edge
- **Real device testing** — phones с random MAC, IoT devices, etc.

Это уже для enterprise-grade, для pre-release v1.0 избыточно.

---

## План по приоритетам

| Приоритет | Задача | Время | Дельта coverage |
|---|---|---|---|
| **P0** | Расширить smoke.sh (Gap 1) | 2-3 часа | +15% |
| **P0** | Пофиксить баги A1-A4 + A12-A13 | 2-3 часа | (не coverage, но чистит красноту) |
| **P1** | Playwright (Gap 2) | 2 дня | +20% |
| **P1** | Go integration edge cases (Gap 3) | 1 день | +5% |
| **P2** | L5 Real VM nightly (Gap 4) | 1-2 дня | +5% |
| **P2** | Performance testing (Gap 5) | 0.5-1 день | +3% |
| **P3** | Plugin system tests (Gap 6) | 0.5 дня | +2% |
| **P4** | Fuzzing, mutation, matrix | неделя | +5% |

После P0+P1+фиксов: **~85%**.
После P2: **~90-93%**.
После P3+P4: **~95-98%**.

---

## Concrete next steps

### На этой неделе
1. Пофиксить A1, A2, A3, A4 (по `tests/bugreport/bugs.md`)
2. Расширить smoke.sh: bulk-move, bulk-edit, aliases/bulk, history diff/restore, backup restore
3. После каждого фикса — удалить ID из `tests/known-bugs.txt`, обновить check, прогнать pipeline

### В следующие 2 недели
1. Пофиксить A5, A6, A8, A12, A13
2. Поставить Playwright + chromium в CI
3. Написать первые 10 Playwright specs (A1 regression + auth flow + i18n)

### В течение месяца
1. Настроить persistent test VM (Proxmox или отдельный хост)
2. nightly cron job — прогон smoke.sh с `-init-system=systemd`
3. Отчёт о regression в init-system коде

### На дистанции (v1.0 release)
1. После всех фиксов known-bugs = 0, smoke.sh = 100% pass
2. L5 nightly стабильно зелёный
3. Playwright покрывает все UI-критичные пути
4. Coverage report через `go test -cover -coverprofile=cover.out`
5. Benchmarks через `go test -bench`

---

## Метрики "когда готово к v1.0 release"

- [ ] `tests/known-bugs.txt` пустой (или содержит только wontfix'ы)
- [ ] smoke.sh: 0 Fail, 0 Known-fail, 0 Skipped, ~80+ Pass
- [ ] L1+L2 Go test coverage ≥ 70% (`go test -cover ./...`)
- [ ] Playwright: 20+ spec'ов, все зелёные
- [ ] L5 nightly: 7 дней без красноты
- [ ] Все баги из `tests/bugreport/bugs.md` либо FIXED, либо WONTFIX с rationale
- [ ] CHANGELOG.md обновлён
- [ ] README обновлён (installation, configuration, troubleshooting)
