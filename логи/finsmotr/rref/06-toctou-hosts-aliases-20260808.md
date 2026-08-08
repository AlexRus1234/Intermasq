# Этап 06 — TOCTOU hosts/aliases

**Дата:** 2026-08-08.
**Волна:** A. **Зависит от:** —.
**Ветка:** не фиксировалась в рамках сессии. **Коммит:** не создан.
**CI:** не запускался.

## Что сделано

- Проверка IP/MAC-дубликатов в `addHostHandler` перемещена под
  `dnsmasq.Mu` (`handlers_hosts.go:82`; ранее проверка была на строке 83,
  lock — на 99).
- Проверки конфликтов в `bulkAddHostsHandler` выполняются под одним lock до
  записи (`handlers_hosts.go:179`; ранее проверка была на 185, lock — на 198).
- CSV hosts нормализует MAC через `validate.NormalizeMAC`, а проверки
  конфликтов выполняются под lock (`handlers_hosts.go:364`, `376`; ранее
  проверка была на 374, lock — на 386).
- `bulkEditHandler` удерживает lock на чтении, планировании, проверке IP и
  записи (`handlers_hosts.go:572`; ранее проверка была на 603, lock — на 619).
- В bulk-move и bulk-edit MAC из прочитанной записи приводится к нижнему
  регистру перед записью (`handlers_hosts.go:484`, `581`).
- Аналогичные перемещения lock выполнены для add/bulk/CSV alias-путей:
  `handlers_aliases.go:81`, `143`, `292` (ранее проверки были на 81, 144,
  292, lock — на 87, 151, 299 соответственно).

## Проверки

- `gofmt -l internal/webapi/handlers_hosts.go internal/webapi/handlers_aliases.go` — ok, вывода нет.
- `go vet ./internal/webapi/...` — ok.
- `$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXX"; go test ./internal/webapi/... -count=1` — FAIL: существующий `TestRollbackHandler_Success` в `internal/webapi/handlers_test.go:1453`, ожидался HTTP 200, получен 500 (`rollback_error`). Тест не изменялся; причина относится к rollback/dnsmasq и не затрагивает файлы этапа 06.
- `git diff --check` — ok.
- Новые тесты не добавлялись, существующие тесты не изменялись.

## Сводка изменений

- `internal/webapi/handlers_hosts.go`: закрыты TOCTOU-проверки hosts и добавлена канонизация MAC перед write-путями.
- `internal/webapi/handlers_aliases.go`: проверки конфликтов alias перенесены под `dnsmasq.Mu`.

## Координация / заметки

- Слияние дублирующейся write-логики отложено в этап 12, как предусмотрено спецификацией этапа.
- Статус этапа: частично закрыт до исправления внешнего red-теста и прохождения CI.
