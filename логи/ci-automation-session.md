# Сессия: CI-автоматизация и тесты

**Дата:** 21-22 июля 2026
**Ветка:** `main` (была `predrel-v3`, в процессе слилась)
**Коммитов:** 14

## Контекст

После первой QA-сессии (`логи/manual-testing-checklist.md`,
`логи/predrel-v3-bugs-and-automation.md`) стало ясно: каждый раз
прокликивать ~250 пунктов чек-листа руками — безумие. Нужен pipeline,
который прогоняет тесты автоматически после сборки.

Цель сессии: привести repo в вид, в котором каждый push в main можно
одной кнопкой протестировать через Forgejo Actions.

## Что было сделано

### 1. Веточная гигиена

**До:**
- `main` — старая, на 18 коммитов позади
- `predrel`, `predrel-v2`, `predrel-v3` — три устаревших ветки
- `docs/add-readme` — отдельная ветка с одним README коммитом

**После:**
- `main` — единственная ветка, актуальная
- Все остальные удалены локально и в origin

**Шаги:**
1. `git checkout main && git merge --ff-only predrel-v3` — fast-forward
   main до актуального состояния
2. `git rebase origin/main` — оказался нужен, потому что в origin/main
   был PR #1 (merge docs/add-readme через web UI), не входивший в
   predrel-v3. 17 коммитов перебазированы поверх PR #1 без конфликтов
3. `git push origin main` — push прошёл как fast-forward
4. Удалены локально: `predrel`, `predrel-v2`, `predrel-v3`, `docs/add-readme`
5. Удалены в origin: `predrel`, `docs/add-readme`

### 2. Стратегия веток

Сначала попробовал строгий workflow (feature branch + PR для каждой
задачи). Получил две ветки: `feature/test-suite` и `feature/ci-pipeline`
(stacked). Для одного разработчика это оказался оверкилл — поглощает
внимание на PR-мерджи.

После обсуждения с автором: **коммиты прямо в main** для большинства
задач. Feature-ветки только для крупных отдельных фич или
external-collaboration. История поглощена через UI PR #3 (обе задачи
одним merge).

### 3. tests/ — test-suite

**`tests/smoke.sh`** — bash + curl, ~80 проверок, два прогона за 5 секунд.
Структура:
- `pre-flight` — setup_required detection, JWT obtain
- `auth` — login, JWT, X-API-Key, rate-limit
- `static hosts` — CRUD, optional fields, tags, CSV, bulk
- `DNS aliases` — A/CNAME/PTR/TXT, A2 duplicate regression
- `config editor` — file create/delete, raw PUT, templates
- `safety` — backup, history, rollback
- `users` — CRUD, password change, cannot-delete-self
- `audit` — entries present after actions
- `metrics` — 4 auth methods
- `path traversal` — 8 векторов
- `logout` — JWT blacklist

**`tests/fixtures/arp-sample.txt`** — пример /proc/net/arp для
`-arp-file` флага в CI.

**`tests/known-bugs.txt`** — separate source-of-truth для списка
известных багов. Smoke.sh грузит его при старте, каждый `check ... A2`
ищет ID в этом списке. Workflow:
- Добавить новый баг → append в .txt + `check ... AXX` в smoke.sh
- Пофиксить баг → удалить из .txt + обновить check (pipeline подсветит
  если забыл обновить test)

**Баги в smoke.sh найденные и починенные в процессе:**
1. CONF_DIR mismatch (/tmp/intermasq-smoke-conf vs /tmp/conf) —
   маскировал 22 из 24 unexpected fails в первом прогоне
2. `PGET | tee | jval` — piped HTTP status code, not body, in
   setup_required detection. Логировал login вместо setup.
3. `dhcp-host=` (empty) принимается dnsmasq --test. Заменено на
   `port=abc`.
4. curl нормализует `..%2F` в URL → Gin не видит как `:name` → 404
   вместо 403. Добавлен `--path-as-is`, ожидание 404 (path cleaning
   от Go HTTP server).
5. `/rollback` опечатка → должен быть `/api/rollback`.
6. `File has 4 dhcp-host lines` проверял до 4-го add'а → перенесено
   после.
7. `exit 2` на FATAL убивал весь прогон → заменил на SKIP-счётчик +
   accumulation fatals до конца.
8. Missing executable bit (100644 → 100755 через `git update-index
   --chmod=+x`).

### 4. .forgejo/workflows/build.yml — новый pipeline

Заменил `release.yml` и старый `build.yml` (node:22-bookworm).
Объединил build + test + (optional) publish в один job.

**Параметры:**
- Base image: `fedora:44` (был `node:22-bookworm`)
- Trigger: `workflow_dispatch` только (как у всех pipelines автора)
- Inputs:
  - `push_to_registry` (bool, default false)
  - `version_tag` (string, default sha-<short>)
  - `run_race_tests` (bool, default true)
- Все внешние зависимости через Nora:
  - `GOPROXY=https://nora.panelka.yadr01.internal/go,direct`
  - npm registry → Nora
  - base image → через Nora Docker proxy

**Шаги:**
1. `Install git first` — minimal Fedora не имеет git
2. `Trust Local CA` — step-ca + update-ca-trust
3. `Configure Nora RPM repo`
4. `Install remaining system dependencies` — nodejs, npm, jq, dnsmasq, gcc
5. `Install Go toolchain` — tarball 1.26.0 (Fedora 44 имеет 1.24)
6. `Configure npm registry to Nora`
7. `Checkout`
8. `Build frontend` — Vue/Vite
9. `Build binary` — `CGO_ENABLED=0` для static link
10. `go vet`
11. `gofmt check` — fail если любой файл не отформатирован
12. `L1 + L2 Go tests` — `go test -race -count=1` (CGO_ENABLED=1)
13. `L3 smoke` — запускает binary в фоне, гоняет tests/smoke.sh
14. `Show binary info` — ls + sha256sum
15. `Publish to Forgejo Packages` (optional) — curl --upload-file

**Баги pipeline найденные и починенные:**
1. `git: command not found` в первом шаге — minimal Fedora, нет git.
   Добавлен отдельный шаг `Install git first`.
2. `gzip: stdin: unexpected end of file` — `SSL_CERT_FILE=step-ca.crt`
   перезаписывал system CA bundle, curl не доверял go.dev. Убран env
   var, оставлен только `update-ca-trust`.
3. `file: command not found` дважды — в шаге Install Go и в Show
   binary info. `file` не в minimal Fedora. Убран из обоих шагов.
4. `-race requires cgo` — `CGO_ENABLED=0` для binary + `-race` для
   test конфликтуют. Шаг теста теперь переопределяет `CGO_ENABLED=1`,
   в deps добавлен `gcc`.
5. `Permission denied` на `./tests/smoke.sh` — git add без executable
   bit. `git update-index --chmod=+x` (сработал только после того как
   файл был в index).
6. `upload-artifact@v4` GHES-incompatible — Forgejo не поддерживает.
   Шаг удалён (binary дистрибуция через Forgejo Packages).

### 5. gofmt cleanups

`bins.go`, `main.go`, `models.go` имели предсуществующие gofmt-дрейфы
(var block alignment). Когда pipeline начал их проверять, упало.
Зафиксировано:
```
gofmt -w bins.go main.go models.go
```

### 6. .gitignore расширения

Добавлены:
- `intermasq-linux-*` — для linux-amd64/arm64 артефактов сборки
- `intermasq-ci` — для pipeline-бинаря
- `*.sha256`
- `*.backup`

### 7. Найденные новые баги (A12, A13)

В процессе прогона smoke.sh обнаружены:
- **A12** — `aliasDomainRegex` не допускает `_` в домене → ломает
  `_dmarc.local`, `_sip._tcp`, DKIM/ACME. Тест добавлен.
- **A13** — `writeFileRaw` гоняет `dnsmasq --test` без
  `--conf-file=<path>`, поэтому dnsmasq тестирует default config, а
  не записанный файл → invalid syntax проскальзывает. Тест добавлен.

Список известных багов теперь: A1 (UI), A2, A3, A4, A5 (UI), A6, A7
(UI), A8, A10 (feature), A11, A12, A13.

### 8. Финальное состояние main

14 коммитов за сессию:
```
531b22b smoke.sh: extract known bugs into tests/known-bugs.txt
eb9bc9b Fix: drop actions/upload-artifact@v4
52bef86 Fix: drop 'file' from Show binary info
a87fe99 smoke.sh: fix 4 test bugs, document A12+A13
84454f8 smoke.sh: fix three test bugs that masked real server behaviour
e5aac3e smoke.sh: resilience — accumulate fatals
a5e065a Fix: set executable bit on tests/smoke.sh
ad9ecae Fix: enable CGO for -race tests, install gcc
fd91741 gofmt: align struct/var blocks in bins.go, main.go, models.go
877c307 Fix: drop 'file' from Go install step
98ebaed Fix: remove SSL_CERT_FILE override
fa5065d Fix: install git before any step that uses it
caa8359 Merge PR #3 (feature/ci-pipeline)
767629c Replace build.yml + drop release.yml
7e3b049 Add smoke test suite + bug report from manual QA
```

## Результат

Pipeline **зелёный**. Прогон:
- Pass: ~75 проверок
- Known-fail: ~9 (8 documented bugs, regression-covered)
- Fail: 0
- Skipped: 0

Время полного прогона: ~3-4 минуты (build + vet + gofmt + go test -race
+ smoke + cleanup).

## Что НЕ сделано (отложено)

- **L2 Go integration tests через httptest** — текущие 2685 строк unit
  тестов неплохо покрывают pure функции, но handler-уровень с
  middleware и БД-моками пока не расширен
- **L4 Playwright для UI-багов** (A1/A5/A7) — нужно поднимать
  browser automation, отдельная задача
- **L5 Real VM testing** — для init-system интеграции, требует
  persistent test server (может быть через OpenTofu nightly)
- **Вычистить "v3.0" / "v3"** из кода и доков — версия 1.0 pre-release,
  hardcode в `main.go:31` и в нескольких .md файлах
- **UPGRADE_TOKEN secret** в Forgejo repo settings для опционального
  шага Publish
