# Quality sweep — 5 независимых промтов (mutation → fuzz → compat → ветки → VM)

**Это файл-сборник из 5 самодостаточных промтов.** Каждая новая сессия делает
**ОДИН** этап. Найди свой этап по заголовку ниже и читай **только §0 (общий
заголовок) + свой раздел**. Остальные разделы НЕ читай — это экономия токенов
и запросов к файлам. Каждый раздел уже содержит нужный контекст инлайн.

**Порядок исполнения:** 4 → 1 → 2 → 3 → ВМ
(4 = Go mutation-testing; 1 = fuzz opt-in CI; 2 = dnsmasq compat-matrix;
3 = handler success-ветки → ~85%; ВМ = L5 Real VM nightly).

---

## §0. Общий заголовок (читают ВСЕ сессии)

**Проект:** Intermasq — веб-панель для dnsmasq. Backend Go 1.25 (gin), frontend
Vue 3, embed через `go:embed`. Репо `B:\Repo\Intermasq\Intermasq`, ветка `main`.
CI — Forgejo Actions, контейнер `fedora:44`, runs as root, Go 1.26 tarball,
npm/go/rpm через прокси Nora. Один package `main`.

**Текущее состояние (после Coverage sweep A+B+C+D, 2026-07-29):**
- Go statement-coverage **81.3% на CI** (Linux), 74.9% локально (Windows;
  дельта = Linux-gated fake-binary/dnsmasq тесты, что skip'аются на Windows).
- Smoke **139/139 CLEAN PASS**, 0 Fail / 0 Known-fail. `tests/known-bugs.txt` пуст.
- L4 Playwright: 33 pass + 1 permanent-skip. L5 Real VM: **0% (Gap 4, открыт)**.
- Баги A1–A8,A10–A13: FIXED/WONTFIX. Fuzz: 4 `FuzzXxx` в `fuzz_test.go` (seed
  corpus через `f.Add`; real `-fuzz` НЕ гоняется — opt-in CI-шаг отложен, это
  этап 1 ниже).

**Жёсткие ограничения (для каждого этапа):**
1. Перед пушем ЛЮБОЙ Go-правки — `go vet ./...` ОБЯЗАТЕЛЬНО (CI режет
   unreachable code; локальный `go build` не ловит).
2. **Локально (Windows) — только `gofmt -l` + `go vet`/`go test`.** Smoke/
   Playwright/`-fuzz`/Docker/VM — только CI или спецсреда. Не чини «под Windows»
   то, что Linux-only.
3. Не ломай существующие тесты. После этапа: `$env:INTERMASQ_SECRET=
   "ci-test-secret-32-bytes-pad-XXXXXXXXXX"; go test "./..." -count=1`.
4. **Продуктовые правки — строго в объёме этапа.** Рефакторинг сверх — НЕТ
   (если этап не требует seam-injection, как в Coverage sweep).
5. Коммиты — после `go vet`+`go test` зелёного. Пуш — по просьбе оператора.
6. **PowerShell-quirks:** `go test "./..."` (кавычки обязательно); `go tool
   cover -func coverage.out` (через пробел, не `=`); `>` пишет UTF-16 — не
   перенаправляй go-вывод через `>`, гоняй в stdout.
7. CI opt-in inputs живут в `.forgejo/workflows/build.yml` по образцу
   `run_e2e_tests` (строки ~26-30 объявление, ~210-296 шаг). Не делай новые
   шаги дефолтными — opt-in, чтобы не удлинять пайплайн.

**Экономия токенов:** твой этап уже содержит ссылки `file:line` и команды.
Читай исходник **только** той функции/файла, которую сейчас правишь (Read с
узким offset/limit). Не перечитывай `логи/` прошлых sweep'ов. Один замер
`cover` — только если этап измеряет coverage.

---

## Этап 4 — Go mutation-testing ✅ ВЫПОЛНЕН (2026-07-31)

12 мутаций: **9 killed, 2 survived→regressed** (R2/R3 в `dnsmasq_test.go`,
коммит `f8fd404`), 1 equivalent. Полную матрицу и артефакты см. в
`логи/quality-sweep.md` (раздел «Этап 4»).

---

## Этап 1 — Fuzz opt-in CI + реальный прогон ✅ ВЫПОЛНЕН (2026-07-31)

Добавлен opt-in CI-шаг `run_fuzz_tests` (build.yml:31-35, 133-159) по образцу
`run_e2e_tests`: 4 target'а (`FuzzParseDhcpHostLine`, `FuzzParseArpContent`,
`FuzzParseAliasLine`, `FuzzParseLeasesContent`) × 30s в реальном `-fuzz` режиме
на одиночном пакете `.` (`./...` роняет go с "fuzz testing requires a single
package" — фикс в `6a5fdad`). Реальный прогон ~2m54s: **no crash found in
4×30s**, regression-корпус `testdata/fuzz/` пуст (норма — парсеры толерантны к
мусору). Коммиты: `a2b0edc` (input+step), `6a5fdad` (fix `.` + verbose triage),
`bc7ab0c` (лог). Полную сводку см. в `логи/quality-sweep.md` (раздел «Этап 1»).
Дефолтный CI не затронут (opt-in); продуктовый код не тронут.

---

## Этап 2 — dnsmasq compatibility matrix ✅ ВЫПОЛНЕН (2026-07-31)

Добавлен opt-in CI-шаг `run_compat_matrix` (build.yml:36-40, 230-342):
build-from-source 3 версий dnsmasq (`2.80`/`2.86`/`2.90`) из upstream
tarballs внутри того же fedora:44 контейнера (docker-in-docker недоступен),
сборка с `COPTS="-std=gnu17 -Wno-error=..."` (gcc 15 / C23 toolchain-фикс
для K&R callback declarations в dnsmasq ≤2.90). Каждая версия прогоняет
полный `tests/smoke.sh` с `-dnsmasq-bin` + чистым per-version conf-dir.

**Результаты:** 2.90 = 139/139 CLEAN PASS; 2.86 = 138/139 (1 known);
2.80 = 137/139 (2 known); fedora:44 system = 2.92rel2 (default L3).
Матрица нашла **2 реальных intermasq-бага** (по решению оператора
зарегистрированы как known, products-код не тронут):
- **A14** — `backup.go:119` `restoreBackupZip` зовёт `dnsmasq --test` без
  `--conf-file=`/`--conf-dir=` (валидирует default path, не восстановленные
  файлы); fail на ≤2.86.
- **A15** — dnsmasq 2.80 отвергает восстановленный `10-static.conf` на
  `--test --conf-file=` (точная причина требует stderr триажа).

Боковая находка: bash `read` не парсил последнюю строку `known-bugs.txt`
без trailing `\n` — захардкодил `init_state` через `|| [ -n "$_line" ]`
(`tests/lib/state.sh`).

Коммиты: `8328ceb` (input+step), `3ba97a6`/`d2ee080`/`f75511b`/`42d5a4f`
(build-flag итерации), `39f8c20` (drop `::group::`), `7ad7d39`
(register A14+A15), `b27ecfb` (trailing-newline fix). Полную сводку,
артефакты и version map см. в `логи/quality-sweep.md` (раздел «Этап 2»).
Дефолтный CI не затронут (opt-in); продуктовый Go-код не тронут.

---

## Этап 3 — Handler success-ветки (довести coverage до ~85%)

**Цель:** добить непокрытые **success/feature-ветки** (не error-500 хвост) в
~5-6 handler'ах, где непокрытый путь = реальный feature, а не `return 500`.
Цель: с 81.3% до ~85%. Не гнать дальше — ROI обрывается.

**Карта (post A+B+C+D, локально Windows; на CI выше из-за B/D):**
```
handlers_config.go:221  putFileHandler            20.0%  → success write + rollback на невалидный синтаксис (A13)
handlers_config.go:22   updateConfigHandler      50.0%  → success serialize+test
handlers_safety.go:147  restoreBackupHandler     18.2%  → success unzip+restore
handlers_safety.go:64   historyDiffHandler       44.0%  → unified-diff логика (history.go:unifiedDiff 94%)
handlers_safety.go:100  historyRestoreHandler    50.0%  → success restore
handlers_safety.go:16   rollbackHandler          70.0%  → reload-обратный путь
history.go:229          restoreHistoryVersion     0.0%  → Linux+dnsmasq (success restore)
dnsmasq.go:89           writeConfigWithTest       0.0%  → Linux+dnsmasq (success+rollback)
handlers_users.go:90    changePasswordHandler    50.0%  → success + wrong-old-pass
handlers_aliases.go:22 resolveAliasesTargetFile 50.0%  → empty-file branch
```
**Seam (из Coverage sweep):** `dnsmasqBinPath` (`bins.go`) — записываемая; для
success-веток с `dnsmasq --test` положи fake `dnsmasq` скрипт (exit 0 на
`--test`), `dnsmasqBinPath=tmp/dnsmasq`, `chmod 0755`, `runtime.GOOS!="windows"`
guard → success покрыт без реального dnsmasq. Для error-ветки (rollback) fake
dnsmasq exit 1 → проверь `rollbackFile`/`.bak`.

**Где писать:** `dnsmasq_test.go` (history/aliases/config-snapshot домен),
`handlers_test.go` (handler httptest). НЕ дублируй существующие success-тесты
(напр. `getFileHandler` уже 84% — там только добей 403/iso path). Для каждого
handler: success-200 path + один-два feature-specific edge.
**Что НЕ делать:** не покрывай хвост `if err!=nil {c.JSON(500);return}` ради
цифры — ROI нулевой. Только success/feature-ветки из карты выше.

**Верификация:** `go test "./..." -count=1 -coverprofile coverage.out` →
`go tool cover -func coverage.out` — целевые handler'ы ≥80%, total ~84-86%.
`go vet` чист, существующие тесты зелёные.
**Knock-on:** fake-dnsmasq-хелпер уже должен быть в Coverage sweep B
(`dnsmasq_test.go`) — переиспользуй, не дублируй. Запиши delta % в лог.

---

## Этап ВМ — L5 Real VM nightly (Gap 4, СДЕЛАТЬ ПОСЛЕДНИМ)

**Цель:** единственный нулевой слой (L5 = 0%). Закрыть функциональную дыру,
которую Coverage sweep D (fake-init бинарники) только «закрасил цифрой», не
дав реальной уверенности: проверить что `sudo systemctl restart dnsmasq`
ДЕЙСТВИТЕЛЬНО рестартит dnsmasq на живом systemd/openrc/runit, и `detectInitSystem`
правильно детектит на реальной VM.

**Что не покрыто (system.go, 0% на Windows, partial на CI через fakes):**
- `detectInitSystem()` чтение `/proc/1/comm` на реальной VM (systemd/runit/init)
- реальные `exec.Command("systemctl"/"service"/"rc-service"/"sv", ...)` calls
- systemd-user vs system caller detection (`os.Getuid`)
- `sudo systemctl restart dnsmasq` через sudoers (rootless-режим)
- OpenRC, runit, sysvinit callers

**Решение — nightly job на persistent test VM.** Это infra-задача (1-2 дня на
bootstrap, дальше работает само):
1. **Persistent VM** (Proxmox API или libvirt/virsh) с snapshot → откат к
   чистому состоянию перед прогоном.
2. **Установить intermasq-ci как systemd-unit** на VM (или запустить вручную с
   `-init-system=systemd`).
3. **Прогнать `tests/smoke.sh` с `-init-system=systemd`** → assert dnsmasq
   реально рестартует (не просто `exec.Command` exit 0, а проверка что порт/
   lease обновляется).
4. **Проверить reload/restart-self** через `/api/reload` + restart-self API →
   убедиться что dnsmasq/intermasq реально перезапускаются.
5. **Повторить** с `systemd-user`, `openrc`, `runit` (контейнер/VM per init —
   openrc лучше Alpine-VM, runit — Void/Artix).
6. **Отчёт:** nightly лог, 7 дней без красноты → тикнуть v1.0-метрику
   «L5 nightly: 7 дней без красноты» в `tests/ROADMAP.md`.

**Где живёт:** новый `.forgejo/workflows/l5-nightly.yml` (отдельный schedule
`cron`, не в `build.yml`) либо внешний runner с доступом к VM API. intermasq-ci
бинарник берётся из Forgejo Packages (артефакт основного build.yml) или
собирается на VM.
**Верификация:** nightly зелёный ≥1 прогона per init-system; smoke через `-init-
system=systemd`/`openrc`/`runit` проходит; dnsmasq реально рестартует (не mock).
**Knock-on:** это снимает критику «D = vanity»: fake-бинари дали statement-%, а
L5 даёт реальную уверенность для тех же system.go путей. НЕ удаляй fake-тесты
D — они быстрые regression-guards; L5 — функциональный nightly-слой сверху.
**Запиши в лог:** какая VM-инфра выбрана, какие init проверены, результаты, кто
держит nightly.

---

## Финальное (после всех 5 этапов)

- [ ] Обновить `tests/ROADMAP.md`: метрики (coverage ≥70% ✓ уже; L5 nightly 7
      дней — после этапа ВМ; «остальныеenterprise» — post-v1.0).
- [ ] Session-лог `логи/quality-sweep.md` — по-этапно: что сделано, verify,
      артефакты (mutation-таблица, fuzz crash'и, compat-карта, delta %, VM-infra).
- [ ] Не забыть: этот файл (`Coverage_sweep.md`-стиль) — referencia; реальные
      правки коммитить по этапам.