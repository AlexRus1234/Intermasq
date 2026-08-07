# Stage 7: `internal/auth`

Перенесены пользователи, JWT, blacklist, rate-limit и auth middleware из
корневого `auth.go` в `internal/auth`. Корневой `auth.go` удалён; поведение
формата users.json, JWT, кодов 401/403 и rate-limit окон сохранено.

## Exported API

- `DBPath`, `SecretKey`
- `LoadUsers`, `SaveUsers`
- `UserCount`, `UserNames`, `GetUser`, `HasUser`
- `AddUser`, `UpdateUser`, `DeleteUser`, `ErrUserExists`
- `MakeToken`, `Middleware`
- `RateLimitMiddleware`, `RateLimitReset`
- `RevokeToken`
- `SetSecretForTest` (временный шов модульного перехода)

## Проверки

- `gofmt -l .` — зелёный
- `go vet ./...` — зелёный
- `go test . -count=1` — зелёный
- `go test ./internal/auth/ -count=1` — зелёный
