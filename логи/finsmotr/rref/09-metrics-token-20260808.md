# Этап 09 — metrics token

**Дата:** 2026-08-08.
**Волна:** A. **Зависит от:** —.
**Ветка:** main. **Коммит:** будет указан после фиксации. **CI:** не запускался.

## Что сделано
- Удалён жёстко небезопасный приём `?token=` в `/metrics`.
- `/metrics` принимает JWT только через `Authorization: Bearer <JWT>` или
  секрет через `X-API-Key`.
- Для локального `jwt.Parse` добавлена проверка HMAC signing method.
- Пример Prometheus обновлён на `bearer_token`.

Выбран жёсткий режим: query-параметр больше не принимается.

## Тесты
- `gofmt -l internal/metrics/metrics.go` — ok.
- `go vet ./internal/metrics/...` — ok.
- `go test ./internal/metrics/... -count=1` — FAIL только по старым тестам
  query-аутентификации:
  - `TestCheckMetricsAuth_TokenQuerySecret` @ `internal/metrics/metrics_test.go:130`;
  - `TestCheckMetricsAuth_TokenQueryJWT` @ `internal/metrics/metrics_test.go:140`;
  - `TestMetricsHandler_TokenQuery_200` @ `internal/metrics/metrics_test.go:311`.
- Red-tests: перечисленные тесты проверяют удалённое старое поведение и будут
  обновлены на этапе 13.
- Новые тесты не добавлялись; существующие тесты не изменялись.

## Сводка изменений
- `internal/metrics/metrics.go`: заголовочная аутентификация, удаление query
  fallback, HMAC method-check.
- `README.md`: документация Prometheus без секрета в URL.

## Координация / заметки
- `/api/events` в `control/sse.go` требует такого же контроля приёмного пути
  в рамках отдельного этапа/мини-этапа; файл не изменялся.
- Сравнение `X-API-Key` в metrics осталось без изменений и требует отдельной
  унификации с auth.
