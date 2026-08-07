# Этап 6 — internal/netstate + internal/audit + internal/templates (+ финализация fuzz-шага CI)

**Статус: выполнено.** CI зелёный; коммиты этапа: `8bda21a`, исправление
форматирования `dd07429`. Лог: `логи/refactor-modular-06-small-pkgs.md`.

## Контекст

Репозиторий: `B:\Repo\Intermasq\Intermasq`. Рефакторинг → `internal/*`,
этап 6. Этапы 0–5 завершены (проверь `git log --oneline -10` и `логи/`);
CI зелёный. Если нет — СТОП.

Три маленьких независимых пакета одним этапом. Плюс финальная правка
fuzz-шага CI (после этапа 4 две fuzz-цели ещё ссылаются на пакет `.`).

## Правила (обязательные)

1. Не выходить за рамки задачи. Никаких изменений поведения.
2. Без скриптов с регексами: read/edit; компилятор — навигатор; gofmt.
3. AGPL-шапки сохранять; eol=lf.
4. Тесты переезжают с кодом, white-box.
5. Локально — только проверки из конца файла. Бинарник НЕ собирать.
6. Финал: коммит + push → зелёный CI → лог в `логи/`.
7. Не сходится — остановиться и спросить. Общение — на русском.

## Задача этапа

### 1. `internal/netstate`

- Перенести `arp_leases.go` целиком: `getArpTable`, `parseArpContent`,
  `parseLeases`, `parseLeasesContent`, `getNewDevices`.
- Флаги `ArpPath` (`-arp-file`) и `LeasesPath` (`-leases`) переезжают сюда
  (exported `*string`, те же имена/дефолты).
- Импорты: `internal/dnsmasq` (`ReadAllHosts`), `internal/oui` (`LookupOUI`).
- Экспорт: `GetArpTable`, `ParseLeases`, `GetNewDevices` (+ по требованию
  компилятора).
- Fuzz: `FuzzParseArpContent`, `FuzzParseLeasesContent` переезжают сюда из
  `fuzz_test.go` (в main fuzz-файл опустевает и удаляется).

### 2. Правка fuzz-шага CI (обязательная часть этапа)

- `.forgejo/workflows/build.yml`, шаг «Fuzz parsers»: все четыре цели теперь
  живут в пакетах — маппинг: `FuzzParseDhcpHostLine`, `FuzzParseAliasLine` →
  `./internal/dnsmasq`; `FuzzParseArpContent`, `FuzzParseLeasesContent` →
  `./internal/netstate`. Запуск против пакета `.` убрать. Заодно обновить
  комментарий шага, если он ссылается на «the main package».

### 3. `internal/audit`

- Перенести `audit.go` целиком (`AuditEntry`, `writeAudit`, `auditHandler`)
  + флаг `AuditLogPath`. Экспорт: `WriteAudit`, `Handler` (gin-хендлер —
  нормально, пакет может зависеть от gin). `audit_test.go` переезжает.

### 4. `internal/templates`

- Перенести `templates.go` целиком (`templates` map, `loadTemplates`,
  `saveTemplates`, `genHostnameFromPattern`, `countHostsInFile`) + флаг
  `TemplatesPath`. Экспорт по call-site'ам (`Load`, `Save`, `Get`/`All`/
  `GenHostnameFromPattern`, `CountHostsInFile`... — минимально необходимое;
  хендлеры шаблонов пока в main, работают с map — вынести нужные операции
  в пакет, саму map не экспортировать).
- `templates_test.go` переезжает (white-box), `withTemplatesPath` — в пакет.

### В main.go

`loadTemplates()` → `templates.Load()`, остальные вызовы — по компилятору.

## Локальные проверки (обязательные, только они)

```powershell
gofmt -l .
go vet ./...
$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXX"
go test . -count=1
go test ./internal/netstate/ ./internal/audit/ ./internal/templates/ -count=1
```

SKIP-лист — сверить с baseline.

## Готово, когда

- Проверки зелёные; fuzz-шаг workflow окончательно на новых пакетах.
- Коммит `refactor(modular): stage 6 — extract netstate/audit/templates`,
  push.
- CI зелёный (при необходимости через пользователя).
- Лог: `логи/refactor-modular-06-small-pkgs.md`.
