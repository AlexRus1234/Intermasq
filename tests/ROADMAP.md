# Test Coverage Roadmap

Где мы сейчас, куда идём, что нужно добавить чтобы дойти до 95-100%.

---

## Текущее состояние

| Слой | Coverage | Статус |
|---|---|---|
| L1+L2 Go (unit + httptest) | **82.7%\*** (CI Linux) / 75.6% (Windows) | один package `main` → L1/L2 совместно не делятся. Парсеры/handler'ы ≥80%; остаточный разрыв сосредоточен в init-system/bootstrap/goroutine-коде (см. сноску + Gap 4) |
| L3 — smoke.sh | ~75-80% API | ✓ 34 suite-файла, 155 проверок (после predrel-test-remediation-P3: +5 suites — `28` apply-template / `44` leases-to-static / `84` restart-self / `85` reload / `86` events-sse; P2.1 — `check_length`/`check_lines` length-ассерты на GET-эндпоинтах); плагин-прокси покрыт (`82-plugins.sh`). Иная метрика — доля эндпоинтов, не строки |
| L4 — Playwright UI | 34 теста (33 pass + 1 permanent-skip) | ✓ **финал**: батч 1+2 + фазы А,Б,В + Блок A (A5/A13 FIXED) + батч 4 + mutation-pass пройден. Hardening sweep (2026-07-29) добил: усилены 2 слабых spec'а (`hosts-sort`, `auth`) и разблокированы 2 infra-spec'а (`setup-screen` 2-я инстанция `:18084`, `sse-live` writable ARP). Единственный skip — `config-raw` (дублирует smoke, постоянный) |
| L5 — Real VM (init/dnsmasq) | opt-in `run_l5_vm_tests` в build.yml | ✓ реализован (`логи/l5-nightly-bootstrap.md`): `tests/l5/provision.sh`+`vm-check.sh` валидны на Arch/systemd **и** Alpine/OpenRC (PASS=16/16 каждая: detect→real init, реальный рестарт dnsmasq + RestartSelf по смене PID, root + rootless/sudo). Бинарник из того же прогона, без Packages. **Post-reboot stable** (обе ВМ пережили рестарт) |
| Perf/stress (opt-in) | реализовано, informational | ✓ `tests/perf.sh` (read/reload/CRUD+RSS/SSE); не coverage-слой |

> **\*** `82.7%` (CI Linux) / `75.6%` (Windows) — измерено
> `go test "./..." -count=1 -coverprofile coverage.out` (Coverage sweep A+B+C+D
> → Quality sweep Этап 3, 2026-08-01). Дельта CI/Windows = Linux-gated
> fake-binary/dnsmasq тесты (`linux_test.go`), что skip'аются на Windows.
> ~17% непокрытых строк сосредоточены в: `system.go` (init-system exec — это и
> есть **Gap 4**), `bins.go` (резолв linux-бинарных `sudo`/`systemctl`/`service`/
> `rc-service`/`sv`), `main.go` (`main`/`loadPlugins` — bootstrap), `sse.go`
> (`startSSEBroadcaster`/`reloadDnsmasq` — горутина + dnsmasq exec),
> `metrics.go` (`startDNSHealthChecker`/`runDNSHealthPass`). Дотянуть до ~99% в
> текущем окружении **нереально** — нужно закрывать Gap 4 (real VM) +
> рефакторить bootstrap (правка исходников).;

**Суммарно:** Go-покрытие 82.7% (CI Linux) / 75.6% (Windows); L3 API ~75-80%
(иная метрика); L4 — 33 pass + 1 permanent-skip; L5 — реализован (opt-in
`run_l5_vm_tests`, PASS=16/16 на Arch+Alpine, post-reboot stable). Метрики разных
слоёв не суммируются в одно число.

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
| **Gap 2** (3-й батч, фаза А) — UI coverage | +5 specs (host-edit-ui/bulk-delete/templates-modal[A7 smoke]/users-tab[create+delete, delete-self]). Хелпер разбит: `lib/api.ts` → barrel + `api-auth.ts` + `api-hosts.ts`. | `gap2-batch3-phaseA.md` |
| **Gap 2** (3-й батч, фаза Б) — UI coverage | +4 specs (dns-aliases-add/bulk-import-text/csv-import/reload-ui). All form-input selectors scoped to the form card so the toolbar search box can't shadow them. | `gap2-batch3-phaseB.md` |
| **Gap 2** (3-й батч, фаза В) — UI coverage | +5 tests/4 specs (rollback-ui/history-modal/discovery-tab/backup-restore-ui[download+restore]). 2 writes/file для `.bak`+version; restore = merge (безопасно для других спеков). | `gap2-batch3-phaseV.md` |
| **Gap 2** (финал, Блок A) — продуктовые фиксы A5 + A13 | A5: `BulkEditModal.vue` `store_hosts.find` → `.hosts.find` (1 строка), `test.fail` снят. A13: `writeFileRaw`/`writeConfigWithTest`/`restoreHistoryVersion` → `dnsmasq --test --conf-file=<path>` (3 строки); A13 убран из `known-bugs.txt`; smoke-чек `40-config-files` стал честным 400. A3/A4-хосты изолированы в `19-bugs.conf` (не отравляют `10-static.conf` для restore-валидации). | `gap2-blockA-a5a13-fixes.md` |
| **Gap 2** (финал, Блок B) — батч 4 Playwright | +6 реализованных specs (audit-tab/plugins-iframe/i18n-api-error/config-template-fill/config-directive[A13 validation]/sse-live[simplified]) + 2 infra-skip (config-raw дублирует smoke, setup-screen нужна 2-я инстанция :18084). 25→33 теста (31 pass + 2 skip). Селекторы выведены из реальных компонентов. | `gap2-finish.md` |
| **Hardening sweep** (2026-07-29) — T1+T2+T3+T4 | T1: fuzzing 4 парсеров (`fuzz_test.go`, рефакторинг `parseLeases`→`parseLeasesContent`). T2: A11 path-traversal defense-in-depth (`isSafePath` после `filepath.Join`). T3: усилены `hosts-sort` (assert порядка) + `auth` (`token===null`+401). T4: разблокированы `setup-screen` (2-я инстанция `:18084`) + `sse-live` (writable ARP). known-bugs.txt пуст на момент sweep; позже Quality sweep Этап 2 (2026-07-31) добавил A14/A15 как version-conditional, A14 закрыт в predrel-test-remediation-P1 (2026-08-02). | `hardening-sweep.md` |
| **Coverage sweep A+B+C+D** (2026-07-29) — statement-coverage | Go statement-coverage 65.6% → 81.3% на CI. Block B: `linux_test.go` + `fakeDnsmasq` seam (`dnsmasqBinPath`) для dnsmasq-зависимых success-путей. Block C: `setupServer()` extraction. Block D: fake init-system бинарники (`system_callers_test.go`). Block A: pure-helper тесты. | `coverage-sweep.md` |
| **Quality sweep Этап 4** (2026-07-31) — Go mutation-testing | 12 ручных мутаций: 9 killed, 2 survived→regressed (R2/R3 regression-тесты в `dnsmasq_test.go`, коммит `f8fd404`), 1 equivalent. Mutation score 81.8% (до R2/R3). | `quality-sweep.md` |
| **Quality sweep Этап 1** (2026-07-31) — Fuzz opt-in CI | Opt-in CI-шаг `run_fuzz_tests` (build.yml, по образцу `run_e2e_tests`): 4 `FuzzXxx` × 30s real `-fuzz`. Прогон ~2m54s — **no crash found**; `testdata/fuzz/` пуст. Дефолтный CI не затронут. | `quality-sweep.md` |
| **Quality sweep Этап 2** (2026-07-31) — dnsmasq compat matrix | Opt-in CI-шаг `run_compat_matrix`: build-from-source 3 версий dnsmasq (2.80/2.86/2.90) × smoke.sh. 2.90=139/139, 2.86=138/139, 2.80=137/139. Найдены 2 реальных бага: **A14** (`backup.go:119` `--test` без `--conf-file=`) + **A15** (2.80 static.conf) — зарегистрированы как known, products-код не тронут. | `quality-sweep.md` |
| **Quality sweep Этап 3** (2026-08-01) — Handler success-ветки | Success/feature-тесты для ~10 handler'ов из карты §3 (products не тронут). 4 Windows-coverable доведены ≥80% (`historyDiffHandler`→100%, `rollbackHandler`→90%, `changePasswordHandler`→85%, `resolveAliasesTargetFile`→87.5%); +3 Linux-gated handler 400-ветки (`fakeDnsmasq(1)`). **CI 81.3% → 82.7%**, коммит `1837a67`. | `quality-sweep.md` |

---

## Что осталось

### Gap 2: UI behavior — ЗАКРЫТО (34 теста, 33 pass + 1 permanent-skip)

**Закрыто (батч 1+2 + фазы А,Б,В + Блок A + Блок B, 33 specs):** Playwright против
`intermasq-ci` в CI (Fedora 44, opt-in `run_e2e_tests`). Батч 1: auth/theme/i18n/
hosts-sort (A1 guard)/host-crud. Батч 2: host-add-ui/host-tags/search-filter/
bulk-ops (bulk-move + bulk-edit)/config-files + seed-хелпер. Фаза А: host-edit-ui/
bulk-delete/templates-modal (A7 smoke)/users-tab. Фаза Б: dns-aliases-add/
bulk-import-text/csv-import/reload-ui. Фаза В: rollback-ui/history-modal/
discovery-tab/backup-restore-ui. Блок A: продуктовые фиксы A5+A13 (см. ниже).
Блок B (батч 4): audit-tab/plugins-iframe/i18n-api-error/config-template-fill/
config-directive (A13 validation)/sse-live (simplified) + 2 infra-skip
(config-raw, setup-screen). Хелпер разбит на `lib/api-auth.ts` + `lib/api-hosts.ts`
+ barrel `lib/api.ts`. См. логи `gap2-playwright-bootstrap.md`,
`gap2-batch2-ui-coverage.md`, `gap2-batch3-phaseA.md`, `gap2-batch3-phaseB.md`,
`gap2-batch3-phaseV.md`, `gap2-blockA-a5a13-fixes.md`, `gap2-finish.md`.

**A5 + A13 FIXED (Блок A, `логи/gap2-finish.md`):** A5 — `BulkEditModal.vue:67`
`store_hosts.find(...)` → `store_hosts.hosts.find(...)` (TypeError в `preview`
computed, модалка не открывалась); `test.fail` снят. A13 —
`writeFileRaw`/`writeConfigWithTest`/`restoreHistoryVersion` теперь гоняют
`dnsmasq --test --conf-file=<path>` (валидация записанного файла, а не
default-конфига); A13 убран из `known-bugs.txt`, smoke-чек стал честным 400.
См. `логи/gap2-blockA-a5a13-fixes.md`.

**Добито в Hardening sweep (2026-07-29, `логи/hardening-sweep.md`):**
- **T3 — 2 слабых spec'а усилены** (найдено mutation-pass Блок C): `hosts-sort` —
  assert видимого ПОРЯДКА после кликов сортировки (был только count-guard);
  `auth` — assert `localStorage.token===null` + `fetch('/api/hosts')→401` после
  logout (был слабый `.btn-primary visible`).
- **T4 — 2 infra-spec'а разблокированы:** `setup-screen` (2-я инстанция `:18084`
  со свежим `-db` → `setup_required:true`) и `sse-live` full-вариант (writable
  `/tmp/e2e-arp.txt`, append ARP → 🟢 через SSE delta). Правки в
  `.forgejo/workflows/build.yml` (L4 шаг).
- **mutation-pass ВЫПОЛНЕН** (Блок C, `логи/gap2-mutation-pass.md`): 4 frontend-
  мутации (`applyConfig` / `addAlias` / `deleteHost` / A5-revert) роняют ровно
  `reload-ui` / `dns-aliases-add` / `host-crud` / `bulk-edit`, без коллатерала.

**Решение:** Playwright, расширение `tests/e2e/specs/`. Единственный остаточный
skip — `config-raw` (постоянный, дублирует smoke `40-config-files.sh`).

### Gap 4: Real init-system integration (~+5%) — ЗАКРЫТО

**Что закрыто (функционально, на живых PID 1):**
- `detectInitSystem()` чтение `/proc/1/comm` → `systemd` / `openrc`
- Реальные `exec.Command("systemctl"/"rc-service", …)` calls (root + sudo)
- Systemd-user vs system caller detection (`os.Getuid`)
- `sudo systemctl/rc-service restart dnsmasq` через sudoers (rootless-режим)
- `RestartSelf()` через реальную init-систему

**Решение (реализовано):** opt-in галочка `run_l5_vm_tests` в `build.yml` (НЕ
отдельный nightly-файл/cron, без Packages). На 2 persistent ВМ (Arch/systemd,
Alpine/openrc) поднимаются по 2 инстанса intermasq — root (`UseSudo=false`) и
rootless `intermasq`-user+sudoers (`UseSudo=true`). `tests/l5/provision.sh`
(идемпотентный: br-l5, изолированный dnsmasq, nft restrictive) + `vm-check.sh`
(detect + реальный рестарт dnsmasq + RestartSelf по смене PID). runit/sysvinit —
post-v1.0 (ниша).

**Результат:** PASS=16/16 на каждой ВМ, validated end-to-end через реальный
runner; **post-reboot stable** (обе ВМ пережили рестарт — nft/services/binary
корректно восстанавливаются). Гонять по факту правок в init-путях
(`system.go`/`bins.go`/`main.go`). См. `логи/l5-nightly-bootstrap.md`,
настройки ВМ — `tests/l5/vm-setup.md`, ход теста — `tests/l5/test-flow.md`.

> **Note on `system_callers_test.go`:** unit-тесты с fake-бинарниками на PATH
> дают statement-coverage для `SystemCaller` (sudoDispatch, argv-construction,
> output-parsing), но **НЕ** дают функциональной уверенности в реальных
> `systemctl`/`rc-service`/`sv`/`service` семантиках — это vanity-покрытие
> (см. шапку `system_callers_test.go`: «цифра coverage растёт, доверие — нет»).
> Для реальной проверки init-перезагрузки — **только L5 real-VM** (Gap 4,
> opt-in `run_l5_vm_tests`). **При рефакторинге `system.go` полагаться
> ТОЛЬКО на L5**, не на `system_callers_test.go`.

### Fuzzing (~+2-3%) — ЗАКРЫТО (Hardening sweep, T1)

**Реализовано:** `fuzz_test.go` — 4 native `FuzzXxx` (`FuzzParseDhcpHostLine`,
`FuzzParseArpContent`, `FuzzParseAliasLine`, `FuzzParseLeasesContent`) после
рефакторинга `parseLeases` → чистая `parseLeasesContent`. Seed corpus через
`f.Add` (compile-checked, работает как subtest'ы в дефолтном `go test`).
**Opt-in CI-шаг `run_fuzz_tests` ДОБАВЛЕН** (Quality sweep Этап 1,
2026-07-31): 4 target'а × 30s real `-fuzz` на одиночном пакете `.`; прогон
~2m54s — **no crash found**, `testdata/fuzz/` пуст. Дефолтный CI не затронут
(opt-in). См. `логи/quality-sweep.md` (раздел «Этап 1»).

---

## Что нужно для 95-100% (в реальности 98-99%)

Go statement-coverage сейчас **82.7%\*** (CI Linux) / 75.6% (Windows).
Реалистичный потолок **в текущем окружении практически достигнут** (~83%):
`system.go` callers закрыты fake-бинарниками на PATH (Coverage sweep D),
`reloadDnsmasq`/DNS-health — Linux-gated exec-тестами (Coverage sweep B).
Остаток (~17%) — потолок без закрытия Gap 4 + правки исходников:

- **`main()`/`loadPlugins()`** — bootstrap, не юнит-тестируем без рефакторинга
  исходников (вынос логики в тестируемые функции).
- **`detectInitSystem` + реальное init-взаимодействие** — это **Gap 4 (real
  VM)**; в Fedora-контейнере PID 1 не systemd. Fake-бинарники (Coverage sweep
  D) дали statement-%, но не реальную уверенность — функциональное покрытие
  (Gap 4 на VM) ценнее (снимает критику «vanity-покрытие»).
- **Фоновые горутины** (`startSSEBroadcaster`, `cleanBlacklistLoop`) — partial.

То есть **~99% statement-coverage недостижимо без закрытия Gap 4 + правки
исходников**. Дальнейший ROI обрывается (Quality sweep Этап 3 подтвердил:
добивание success-веток дало +1.4%, а error-500 хвост — нулевой ROI).

Оставшееся сверх реалистичного потолка — enterprise-grade:

- ~~**Mutation testing**~~ — ✅ ВЫПОЛНЕНО (Quality sweep Этап 4): 12 ручных
  мутаций, 9 killed / 2 survived→regressed (R2/R3) / 1 equivalent.
- ~~**Compatibility matrix**~~ — ✅ ВЫПОЛНЕНО (Quality sweep Этап 2): opt-in CI
  с build-from-source 2.80/2.86/2.90; найдены A14/A15.
- **Cross-distro** — Fedora, Debian, Alpine, Ubuntu, OpenSUSE (compat-matrix
  покрывает version-axis через source-build; distro-контейнеры нуждаются в
  docker-in-docker, недоступном на runner'е).
- **Browser matrix** — Chrome, Firefox, Safari, Edge
- **Real device testing** — phones с random MAC, IoT devices

Для pre-release v1.0 избыточно.

---

## План по приоритетам

| Приоритет | Задача | Время | Дельта coverage |
|---|---|---|---|
| **P0** | Пофиксить баги A1-A4 + A12 (A5 + A13 — FIXED в Блоке A) | 2-3 часа | (чистит красноту known-bugs) |
| **P0✓** | **Bugfix sweep (2026-07-28) — закрыто:** A1, A2, A3, A4, A6, A8, A12 → FIXED с regression-тестами. См. `логи/bugfix-sweep.md`. | готово | smoke 0 Fail / 0 Known-fail |
| **P0✓** | **Hardening sweep (2026-07-29) — A11 закрыто:** `getFileHandler`/`putFileHandler` (`handlers_config.go`) получили `isSafePath` после `filepath.Join` (defense-in-depth); regression-тесты `TestGetFileHandlerRejectsUnsafePath` / `TestPutFileHandlerRejectsUnsafePath`. На момент sweep `tests/known-bugs.txt` был пуст от A-багов; позже Quality sweep Этап 2 (2026-07-31) добавил A14/A15 (version-conditional на dnsmasq 2.80/2.86), из них A14 закрыт в predrel-test-remediation-P1 (2026-08-02). См. `логи/hardening-sweep.md`. | готово | known-bugs.txt: чист на ≥2.86, содержит A15 conditional на 2.80 |
| **P1** | Playwright (Gap 2) — **ФИНАЛ** ✓ (34 tests: 33 pass + 1 permanent-skip `config-raw`); A5/A13 FIXED; батч 4 закрыт; mutation-pass пройден; 2 слабых spec'а (`hosts-sort`, `auth`) усилены; 2 infra-spec'а (`setup-screen`, `sse-live`) разблокированы (Hardening sweep, 2026-07-29) | готово | UI-покрытие закрыто полностью |
| **P2✓** | L5 Real VM (Gap 4) — **реализовано**: opt-in галочка `run_l5_vm_tests` в `build.yml` (не отдельный файл, без cron/автозапуска, без Packages) + `tests/l5/provision.sh` (idempotent, авто-detect systemd/openrc, 2 инстанса root+rootless, nft restrictive) + `tests/l5/vm-check.sh`. Валидировано вживую на Arch/systemd **и** Alpine/OpenRC (PASS=16/16 каждая, post-reboot stable). См. `логи/l5-nightly-bootstrap.md`. | готово | функциональная уверенность (не statement-%) |
| **P2✓** | Fuzzing для парсеров (Hardening sweep + Quality sweep Этап 1) — закрыто: 4 `FuzzXxx` в `fuzz_test.go` (seed corpus) **+ opt-in CI-шаг `run_fuzz_tests`** (4×30s real `-fuzz`, no crash). См. `логи/quality-sweep.md`. | готово | ~+2-3% |
| **P2✓** | Quality sweep Этап 4 — Go mutation-testing (2026-07-31): 12 мутаций, 9 killed / 2 survived→regressed (R2/R3) / 1 equivalent. См. `логи/quality-sweep.md`. | готово | качество тестов (не %) |
| **P2✓** | Quality sweep Этап 2 — dnsmasq compat matrix (2026-07-31): opt-in CI build-from-source 2.80/2.86/2.90; найдены A14/A15 (known). См. `логи/quality-sweep.md`. | готово | версионная уверенность |
| **P2✓** | Quality sweep Этап 3 — Handler success-ветки (2026-08-01): success/feature-тесты для ~10 handler'ов; CI 81.3% → **82.7%**, все целевые ≥80%. См. `логи/quality-sweep.md`. | готово | +1.4% |

---

## Метрики "когда готово к v1.0 release"

- [x] `tests/known-bugs.txt` пустой на целевой dnsmasq ≥2.86; ИЛИ содержит только version-conditional баги с явной причиной (нынешний статус: A15 — KNOWN-CONDITIONAL на dnsmasq 2.80, dhcp-host tag-set strictness; A14 FIXED в predrel-test-remediation-P1, 2026-08-02)
- [x] smoke.sh: 0 Fail / 0 Known-fail на целевой dnsmasq ≥2.86 (139/139 CLEAN PASS); A15 KNOWN-fail срабатывает только на dnsmasq 2.80 (compat-matrix, opt-in)
- [x] L1+L2 Go test coverage ≥ 70% (`go test -cover ./...`) — **82.7% на CI Linux** / 75.6% Windows (Coverage sweep A+B+C+D + Quality sweep Этап 3, `логи/quality-sweep.md`)
- [x] Playwright: 20+ spec'ов, все зелёные (33 pass + 1 permanent-skip `config-raw`, покрыт smoke)
- [x] L5: post-reboot stable — обе ВМ пережили рестарт, `run_l5_vm_tests` green (PASS=16/16); opt-in, гоняется по факту правок в init-путях (`system.go`/`bins.go`/`main.go`)
- [x] `tests/perf.sh`: 0 hard failures на дефолтных порогах
- [x] Все баги из `tests/bugreport/bugs.md` либо FIXED, либо WONTFIX с rationale
- [ ] CHANGELOG.md обновлён
- [ ] README обновлён (installation, configuration, troubleshooting)
