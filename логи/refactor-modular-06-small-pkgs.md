# Этап 6 — internal/netstate + internal/audit + internal/templates

- Дата: 2026-08-07
- Предыдущий HEAD: `b7a2f9b` (stage 5 — extract dnsmasq write/history/backup)
- Коммит этапа: `8bda21a` (`refactor(modular): stage 6 — extract netstate/audit/templates`)
- Push: `origin/main` успешно обновлён.

## Что сделано

- `arp_leases.go` перенесён в `internal/netstate`; флаги `ArpPath` и
  `LeasesPath`, публичные операции и fuzz-цели перенесены вместе с кодом.
- `audit.go` и white-box-тесты перенесены в `internal/audit`; экспортированы
  `AuditLogPath`, `WriteAudit` и `Handler`.
- `templates.go` и white-box-тесты перенесены в `internal/templates`;
  операции загрузки, сохранения, доступа и генерации hostname экспортированы.
- Все четыре fuzz-цели workflow запускаются против пакетов-владельцев;
  ссылки на пакет `.` удалены.

## Локальные проверки

Успешно выполнены только проверки из задания:

```text
gofmt -l .
go vet ./...
$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXX"
go test . -count=1
go test ./internal/netstate/ ./internal/audit/ ./internal/templates/ -count=1
```

Бинарник локально не собирался. Сохранены baseline SKIP для Windows
 (отсутствующий dnsmasq и shell-script fake binaries).

## CI

Статус Forgejo CI из локальной среды не проверен: CLI `gh` отсутствует.
После push требуется подтвердить зелёный workflow в Forgejo.
