# Этап 7 — internal/auth (пользователи, JWT, rate-limit, middleware)

**Статус: выполнено.** Коммит: `57fbe7f`; push выполнен; CI зелёный.
Лог: `логи/refactor-modular-07-auth.md`.

## Контекст

Репозиторий: `B:\Repo\Intermasq\Intermasq`. Рефакторинг → `internal/*`,
этап 7. Этапы 0–6 завершены (проверь `git log --oneline -10` и `логи/`);
CI зелёный. Если нет — СТОП.

`auth.go` содержит: хранилище пользователей (`users` map + `usersMu`,
`loadUsers`/`saveUsers` по флагу `-db`), JWT (`makeToken`, `SecretKey`),
чёрный список токенов (`revokeToken`, `isTokenRevoked`,
`cleanupBlacklistOnce`, `cleanBlacklistLoop`), rate-limit
(`rateLimitMiddleware`, `rateLimitReset`) и `authMiddleware`.

## Правила (обязательные)

1. Не выходить за рамки задачи. Никаких изменений поведения (формат
   users.json, JWT, ответы 401/403, rate-limit окна — прежние).
2. Без скриптов с регексами: read/edit; компилятор — навигатор; gofmt.
3. AGPL-шапки сохранять; eol=lf.
4. Тесты переезжают с кодом, white-box.
5. Локально — только проверки из конца файла. Бинарник НЕ собирать.
6. Финал: коммит + push → зелёный CI → лог в `логи/`.
7. Не сходится — остановиться и спросить. Общение — на русском.

## Задача этапа

- Создать `internal/auth`, перенести `auth.go` целиком.
- `SecretKey` переезжает: `var SecretKey = []byte(os.Getenv("INTERMASQ_SECRET"))`
  в пакет auth. `init()`-проверка в `main.go` (fatal при пустом секрете)
  остаётся, но читает `auth.SecretKey`.
- Флаг `DBPath` (`-db`) переезжает сюда (exported `*string`, то же
  имя/дефолт).
- Спроектировать минимальный exported API по фактическим call-site'ам в
  main (`handlers.go`, `handlers_users.go`, `main.go`, тесты):
  `LoadUsers`, `SaveUsers`, `MakeToken`, `Middleware`, `RateLimitMiddleware`,
  `RateLimitReset`, плюс операции над пользователями (`UserCount`/`HasUsers`,
  `UserHash`, `AddUser`, `DeleteUser`, ... — по факту использования).
  Саму `users` map НЕ экспортировать — хендлеры, работавшие с map напрямую,
  перевести на функции пакета.
- Для тестов, остающихся в main (login/setup/logout handler-тесты до
  этапа 11), добавить временный шов:

  ```go
  // SetSecretForTest ... Exported for cross-package tests during modularization.
  func SetSecretForTest(t *testing.T, secret []byte)
  ```

  (`setTestSecret` в main переключить на него.)

### Тесты

В `internal/auth` (white-box) переезжают: `TestLoadUsers*`, `TestSaveUsers*`,
`TestToken*`, `TestCleanBlacklist*`, `TestCleanupBlacklistOnce_*`,
`TestRateLimit*`, `TestAuthMiddleware*`, `TestRateLimitReset*`
(из `dnsmasq_test.go`, `goroutines_test.go`, `new_features_test.go`).
Хендлер-тесты login/setup/logout/users CRUD пока остаются в main с
квалификаторами. Без дублей.

## Локальные проверки (обязательные, только они)

```powershell
gofmt -l .
go vet ./...
$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXX"
go test . -count=1
go test ./internal/auth/ -count=1
```

SKIP-лист — сверить с baseline.

## Готово, когда

- Проверки зелёные; `auth.go` в корне отсутствует.
- Коммит `refactor(modular): stage 7 — extract auth`, push.
- CI зелёный (при необходимости через пользователя).
- Лог: `логи/refactor-modular-07-auth.md` — перечислить exported API.
