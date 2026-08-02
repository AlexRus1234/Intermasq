# Test remediation — промты перед предрелизным рефакторингом

Сессия полного аудита тестовой инфраструктуры (Go unit + bash smoke + Playwright
E2E + L5 + fuzz), проведённая 2026-08-02. Аудит делегирован 4 параллельным
explore-агентам (по слою тестов), сводка находок — в чате с оператором.

**Выявлено:** ~50 находок уровней CRITICAL/HIGH/MEDIUM, сгруппированных в 3
фазы починки. Промты — в одноимённых файлах:

| Фаза | Файл | Содержание | Трудоёмкость | Когда делать |
|------|------|------------|--------------|--------------|
| **P1** | `predrel-test-remediation-p1.md` | критичное: 6 задач — док-рассинхрон, маскирующие A14/A15-теги, ноль-ассерт тесты, argv-fake для A13, де-корреляция hosts-sort seed | ~1 день | **до старта любого рефакторинга** |
| **P2** | `predrel-test-remediation-p2.md` | перед глубоким проходом: 11 задач — body-ассерты на GET, self-seed суитов, sandbox-обход, metricsHandler/audit-тесты, fuzz-фикс | ~1.5 дня | до рефакторинга init/backup/metrics/audit/sse |
| **P3** | `predrel-test-remediation-p3.md` | полиш: 9 задач — A11/Success-fake, инвертированные комментарии, mutation-friendly селекторы, покрытие endoints | ~1 день | до v1.0 release |

## Соответствие нумерации

Оператор в чате называл фазы «P1/P2/P3». В предыдущей сводке (ответ оператору
после аудита) использовалось стандартное triage-обозначение P0/P1/P2. Решено
привести к нумерации оператора:

- **P1** (этот файл = `predrel-test-remediation-p1.md`) = бывший **P0** —
  критичные находки, маскирующие баги или дающие ложную уверенность.
- **P2** (`predrel-test-remediation-p2.md`) = бывший **P1** — обязательно перед
  глубоким рефакторингом init/backup/metrics/audit/sse.
- **P3** (`predrel-test-remediation-p3.md`) = бывший **P2** — polish до release.

## Что НЕ входит в эти промты

Промты касаются **только тестовой инфраструктуры и её согласованности с доками**.
**Продуктовые** слабые места (JWT alg-confusion в `auth.go:214`, plugin trust
boundary в `main.go:131-193`, X-Forwarded-For в `rateLimitMiddleware`,
`hash, _ :=` в `handlers.go:47`) — это отдельная задача security-аудита
продуктового кода, её делаем вне этих промтов.

## Порядок исполнения

Строго последовательно: **P1 → P2 → P3**. Внутри фазы задачи независимы, можно
делать в любом порядке или параллельно разными сессиями. Каждая задача
содержит acceptance-критерий — верификацию, что починка сработала.

## Базовые предположения (для всех промтов)

- Рабочий каталог: `B:\Repo\Intermasq\Intermasq` (Windows) или эквивалент на CI
  (Fedora 44).
- Запуск Go-тестов требует `INTERMASQ_SECRET`:
  ```powershell
  $env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXXXX"
  ```
  ```bash
  export INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXXXX"
  ```
  Иначе `main.go:107 init()` делает `os.Exit(1)`.
- Команды проверки: `go vet ./...`, `go test ./... -count=1`,
  `go test ./... -race -count=1` (CGO_ENABLED=1 обязательно для -race).
- Для smoke: `BASE=http://localhost:18081 ./tests/smoke.sh` против запущенного
  binary.
- Для Playwright: `cd tests/e2e && npx playwright test` (требует запущенного
  intermasq-ci на :18083).
- Перед стартом каждой фазы — коммит «predrel-test-remediation-PN: pre» чтобы
  иметь откатку.
