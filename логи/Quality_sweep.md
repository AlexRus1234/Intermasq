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

## Этап 3 — Handler success-ветки ✅ ВЫПОЛНЕН (2026-08-01)

Добавлены success/feature-тесты для непокрытых веток handler'ов из карты §3
(продуктовый код не тронут — правки только в `handlers_test.go`,
`dnsmasq_test.go`, `linux_test.go`). 4 Windows-coverable handler'а доведены
≥80%: `historyDiffHandler` 44→100%, `rollbackHandler` 70→90%,
`changePasswordHandler` 50→85% (real bcrypt), `resolveAliasesTargetFile`
50→87.5% (empty-creates-default). +3 Linux-gated handler-level 400-ветки через
`fakeDnsmasq(1)` (reuse Coverage sweep B): putFile (A13 rollback),
updateConfig, restoreBackup — `dnsmasq_test_failed → 400 + .bak rollback`.
Остальные пункты карты (writeConfigWithTest / restoreHistoryVersion /
putFile / updateConfig / historyRestore / restoreBackup success) уже покрыты
в Coverage sweep B (`linux_test.go`, Linux-only). **CI coverage 81.3% →
82.7%** (до ~85% не дотянули — ROI-обрыв по §3; все целевые handler'ы ≥80%).
Коммит `1837a67`. Полную сводку и delta-таблицу см. в `логи/quality-sweep.md`
(раздел «Этап 3»). Дефолтный CI не затронут; продуктовый Go-код не тронут.

---

## Этап ВМ — L5 Real VM (Gap 4) ✅ ВЫПОЛНЕН (2026-08-02)

Закрыт единственный нулевой слой (L5) — **функционально**, не цифрой: `detectInitSystem()`
и реальные init-рестарты проверены на живых systemd и openrc (PID 1).

Реализация (по итогам корректировок оператора — **НЕ** отдельный nightly-файл/cron,
**без** зависимости от Packages):
- **2 persistent ВМ:** Arch/systemd (172.20.5.18) + Alpine/openrc (172.20.5.19).
  runit/sysvinit — post-v1.0.
- **Opt-in галочка `run_l5_vm_tests` в `build.yml`** (как `run_e2e_tests`): ручной
  `workflow_dispatch`, без автозапуска. Бинарник `./intermasq-ci` берётся из того же
  прогона (scp в `/tmp/` + `mv` — нельзя писать в работающий binary).
- **`tests/l5/provision.sh`** (idempotent, авто-detect init): изолированный bridge
  `br-l5` (`10.5.0.0/24`), dnsmasq только на `10.5.0.1:53` (`bind-interfaces`+`no-resolv`),
  nftables restrictive (policy drop + SSH:22/lo/br-l5; файл per-init: `.conf`/`.nft`),
  **2 инстанса** intermasq — root (`UseSudo=false`) и rootless user `intermasq`+sudoers
  (`UseSudo=true`).
- **`tests/l5/vm-check.sh`**: assert `[INIT] System:` + реальный рестарт dnsmasq (смена
  PID + `dig`-проб) + RestartSelf (смена PID) — для обоих инстансов.

**Результат (через реальный runner):** обе ВМ **PASS=16/16** (root + rootless/sudo в
каждой). Найдено и пофиксено 7 инфра-багов (Type=dbus stall, dnsmasq wildcard leak,
Alpine conf.d/log/nft-файл, ETXTBSY при scp) — все в `provision.sh`, продуктовый код
не тронут. Фейк-тесты D оставлены (быстрые regression-guards); L5 — функциональный слой
сверху. Полную сводку, артефакты и баг-таблицу см. `логи/l5-nightly-bootstrap.md`;
настройки ВМ — `tests/l5/vm-setup.md`, ход теста — `tests/l5/test-flow.md`.
Post-reboot stable (обе ВМ пережили рестарт). Soak-метрика переформулирована под
opt-in: гонять L5 по факту правок в init-путях, метрика в `tests/ROADMAP.md` тикнута.

---

## Финальное (после всех 5 этапов)

- [x] Обновить `tests/ROADMAP.md`: метрики (coverage ≥70% ✓; L5 реализован ✓,
      post-reboot stable, метрика тикнута; «остальные enterprise» — post-v1.0).
- [x] Session-лог — этап ВМ: `логи/l5-nightly-bootstrap.md` (сводка, артефакты,
      баг-таблица, результаты PASS=16/16). Этапы 1/2/4 — в `логи/quality-sweep.md`.
- [x] Этот файл — referencia; правки закоммичены по этапам.