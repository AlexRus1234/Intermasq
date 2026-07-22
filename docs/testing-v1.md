# Тестирование Intermasq v1 — стратегия и гайд

Документ описывает: что тестим, как тестим, как запускать, как
интерпретировать результаты.

---

## Слои тестирования

| Слой | Где | Что ловит | Время | Текущий coverage |
|---|---|---|---|---|
| **L1** Go unit | `go test` в Go-процессе | Pure-функции: парсеры, IP-math, validation, OUI | ~18 сек | ~85% pure logic |
| **L2** Go integration | `go test -race` с httptest | Handlers целиком с mock SystemCaller и dnsmasq | +5 сек | ~30% handlers |
| **L3** Smoke e2e | `tests/smoke.sh` (bash + curl) против запущенного binary | Все API endpoints + path traversal + auth | ~30 сек | ~60% API |
| **L4** UI (Playwright) | `tests/e2e/specs/*.spec.js` против binary + chromium | Vue reactivity, modals, i18n | ~3-5 мин | **0% (не реализован)** |
| **L5** Real VM | Nightly cron на persistent test server | Init-system, real dnsmasq leases, real /proc/net/arp | ~10 мин | **0% (не реализован)** |

**Текущая суммарная coverage:** ~60-65%.

---

## Как запустить локально

### Минимум — L1

```bash
export INTERMASQ_SECRET="test-secret-32-bytes-pad-XXXXXXXXXX"
go test ./... -count=1
```

Все тесты должны быть зелёными (~18 секунд, ~2700 строк тестов в
`dnsmasq_test.go` + `new_features_test.go`).

### L1 + L2 с race detector

```bash
export INTERMASQ_SECRET="test-secret-32-bytes-pad-XXXXXXXXXX"
go test ./... -race -count=1
```

Race detector требует CGO (`gcc` в системе). ~30-40 сек из-за bcrypt
медленности.

### L3 smoke

```bash
# 1. Собери binary
CGO_ENABLED=0 go build -o /tmp/intermasq-ci .

# 2. Запусти сервер в фоне
export INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXXXX"
mkdir -p /tmp/conf /tmp/history
/tmp/intermasq-ci \
    -port 18081 \
    -conf-dir /tmp/conf \
    -db /tmp/users.json \
    -audit-log /tmp/audit.log \
    -history-dir /tmp/history \
    -templates /tmp/templates.json \
    -leases /tmp/leases \
    -arp-file tests/fixtures/arp-sample.txt \
    -init-system=none \
    -ci-mode=true &
sleep 2

# 3. Гоняй smoke
BASE=http://localhost:18081 CONF_DIR=/tmp/conf ./tests/smoke.sh
```

**Опции окружения** (можно переопределять):
- `BASE` — URL сервера (по умолчанию `http://localhost:8081`)
- `CONF_DIR` — должен **совпадать** с `-conf-dir` флагом binary
- `ADMIN_USER`, `ADMIN_PASS` — если хочешь другие креды
- `KNOWN_BUGS_FILE` — путь к known-bugs.txt (по умолчанию рядом со smoke.sh)

### L1+L2+L3 через pipeline

`workflow_dispatch` в Forgejo UI:
- `push_to_registry=false`, `run_race_tests=true` — стандартный прогон
- После зелёного — binary доступен в job artifacts (можно скачать из UI)

---

## Структура тестовых файлов

```
tests/
├── smoke.sh                  # L3 bash+curl (~80 проверок)
├── known-bugs.txt            # source-of-truth списка известных багов
├── fixtures/
│   └── arp-sample.txt        # пример /proc/net/arp для CI
├── bugreport/
│   └── bugs.md               # полный баг-репорт (A1-A13)
└── ROADMAP.md                # что нужно для 95-100% coverage

dnsmasq_test.go               # L1 unit (2247 строк)
new_features_test.go          # L1 unit для новых фич (430 строк)
```

---

## Интерпретация результатов smoke.sh

### Символы в выводе

| Символ | Значение |
|---|---|
| `✓` green | Тест прошёл |
| `✗` yellow + `KNOWN(AX)` | Тест упал на **известном** баге. Pipeline остаётся зелёным |
| `✗` red | Тест упал на **неизвестном** регрессе. Pipeline красный |
| `✗` red + `FAIL(AX)` + подсказка | Баг ID задан, но его нет в `known-bugs.txt`. Либо фикс уже сделан (обнови test), либо это новый баг (добавь ID в known-bugs.txt) |
| `-` blue | Skipped: не выполнилось из-за fail'а pre-condition (нет JWT, нет history, и т.д.) |
| `‼ FATAL` | Pre-condition failed, накоплен в FATALS список. Скрипт идёт дальше, в конце покажет все |

### Пример SUMMARY

```
=== SUMMARY ===
  Pass:        76 / 84
  Fail:        0 / 84
  Known-fail:  8 / 84  (bugs: A11 A12 A13 A2 A3 A4 A6 A8)
  Skipped:     0 / 84  (pre-condition failed)
```

**Pipeline зелёный** если `Fail=0` и `FATALS=[]`. Любой unexpected
fail краснит pipeline.

---

## Workflow с known-bugs.txt

Файл `tests/known-bugs.txt` — единственный source-of-truth. Каждая
запись:
```
A2    DNS alias duplicate allowed — findAliasesByDomain excludes self
```

### Добавить новый баг

1. Найди минимальный воспроизводимый тест в smoke.sh
2. Добавь `check "description" EXPECTED ACTUAL AXX` — где `AXX` новый ID
3. Добавь `AXX описание` в `known-bugs.txt`
4. Pipeline останется зелёным (KNOWN-fail)

### Починить баг

1. Сделай фикс в коде
2. **Удали ID из `known-bugs.txt`**
3. Обнови ожидание в `check` (например, было `400` стало `200`)
4. Запусти pipeline — должен стать чистым pass

Если забыл обновить test — pipeline **красный** с подсказкой:
```
Bug A2 not in known-bugs.txt — either fix this test (bug already resolved)
or add A2 to tests/known-bugs.txt (new bug found).
```

### Регрессия вернулась

Чинили A2, через месяц случайно сломали опять:
- ID уже удалён из `known-bugs.txt`
- Test ждёт 200 (исправленное поведение)
- Получает 400 (баг вернулся)
- Pipeline **красный** — баг снова пойман

---

## CI Pipeline (Forgejo Actions)

`.forgejo/workflows/build.yml` — единый pipeline, `workflow_dispatch`.

### Inputs

| Параметр | Default | Описание |
|---|---|---|
| `push_to_registry` | `false` | Опубликовать binary в Forgejo Packages после зелёного прогона |
| `version_tag` | `""` (→ `sha-<short>`) | Версия в registry |
| `run_race_tests` | `true` | Включить `-race` для Go test (медленнее, но ловит data races) |

### Шаги

1. `Install git first` — minimal Fedora 44 без git
2. `Trust Local CA` — step-ca + update-ca-trust
3. `Configure Nora RPM repo`
4. `Install remaining system dependencies` — nodejs, npm, jq, dnsmasq, gcc
5. `Install Go toolchain` — tarball 1.26.0 (в Fedora 44 только 1.24)
6. `Configure npm registry to Nora`
7. `Checkout`
8. `Build frontend` (Vue/Vite)
9. `Build binary` (CGO_ENABLED=0, static)
10. `go vet`
11. `gofmt check` — fail если любой файл не отформатирован
12. `L1 + L2 Go tests` (CGO_ENABLED=1 для race)
13. `L3 smoke` — запускает binary + tests/smoke.sh
14. `Show binary info`
15. `Publish to Forgejo Packages` (опционально)
16. `Cleanup` (always)

### Известные особенности

- `fedora:44` minimal образ — много команд надо ставить явно (`git`,
  `file`, etc.). Pipeline ставит только необходимое.
- `SSL_CERT_FILE` **НЕ** оверрайдить — это сломает trust к публичным
  CA. `update-ca-trust` добавляет step-ca в system bundle достаточно.
- `actions/upload-artifact@v4+` не работает на Forgejo (GHES-only).
  Binary дистрибуция через Forgejo Packages (curl --upload-file).

---

## Что тестировать при изменениях

| Меняешь | Минимум прогонов перед commit'ом |
|---|---|
| Парсер (dnsmasq.go, aliases.go) | L1 (go test) — обязательно |
| Handler (handlers_*.go) | L1 + L3 (smoke.sh соответствующей секции) |
| Auth/JWT/rate-limit (auth.go) | L1 + L3 auth секция + race |
| UI компонент (frontend/) | L4 (Playwright) — не реализован пока, ручной smoke |
| Path safety (isSafePath) | L3 path-traversal секция — обязательно |
| Системный caller (system.go) | L5 (real VM) — не автоматизировано пока, ручной тест |
| CI/pipeline (build.yml) | workflow_dispatch в Forgejo UI |

---

## Troubleshooting

### Pipeline падает на `Install git first`

`dnf install -y git curl` не сработал. Проверь что Nora RPM repo
доступен и `dnf` может резолвить пакеты. Если Nora RPM недоступен —
образ Fedora должен ходить напрямую в зеркала Fedora.

### Pipeline падает на `Build frontend`

`npm ci` требует registry. Проверь что
`npm config set registry https://nora.panelka.yadr01.internal/npm/`
сработал. Если Nora не достукается — fall back на
`registry.npmjs.org` (медленнее).

### Pipeline падает на L3 smoke

Посмотри логи GIN в выводе — каждый HTTP-запрос виден с статус-кодом и
latency. Если 500 — баг в сервере. Если 4xx — smoke.sh отправляет
что-то не то, проверь ожидания.

### Smoke.sh красный после фикса бага

Это **фича, не баг**: ты удалил ID из known-bugs.txt, но забыл обновить
`check ... EXPECTED ACTUAL AXX` на новый EXPECTED. Прочитай подсказку в
выводе pipeline и обнови test.
