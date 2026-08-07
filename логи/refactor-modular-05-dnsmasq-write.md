# Этап 5 — internal/dnsmasq, часть 2: запись, history, backup

- Дата: 2026-08-07
- Предыдущий HEAD: `04e3be3` (stage 4 — extract dnsmasq parsers)
- Локальный toolchain: `go1.26.3 windows/amd64`

## Что сделано

В `internal/dnsmasq` перенесены функции записи конфигов, history и ZIP
backup. Из main удалены `dnsmasq.go`, `aliases.go`, `history.go`, `backup.go`.

### Экспорт пакета `internal/dnsmasq`

- `IsSafePath`, `ReadFileRaw`, `WriteFileRaw`, `WriteConfigWithTest`.
- `AppendHostLine`, `RemoveHostLine`, `AppendAliasLine`, `RemoveAliasLine`,
  `EnsureAliasesFile`.
- `DefaultAliasesFileName`.
- `Mu` — единый mutex для критических секций записи конфигов.
- `HistoryDir`, `HistoryDepth`, `HistoryVersionRegex`, `EnsureHistoryDir`.
- `SaveHistory`, `RotateHistory`, `ListHistory`, `ReadHistoryVersion`,
  `RestoreHistoryVersion`, `CreateLocalBackup`, `RollbackFile`, `UnifiedDiff`;
  `HistoryEntry` и приватные хелперы имён файлов сохранены рядом с кодом.
- `CreateBackupZip`, `RestoreBackupZip`, `DeleteConfigFile`.

`WriteFileRaw` и `WriteConfigWithTest` используют `bins.Dnsmasq()` и
`stats.Counters.TestFailures.Add(1)`. Циклов импорта нет.

## Тесты

В `internal/dnsmasq` переехали white-box-тесты:

- raw-файлы: `TestIsSafePath`, `TestReadFileRaw*`, `TestWriteFileRaw*`;
- alias-запись: `TestRemoveAliasLine*`, `TestEnsureAliasesFile`;
- history: `TestSaveHistory*`, `TestRotateHistory*`,
  `TestReadHistoryVersion*`, `TestListHistory*`, `firstVersion`,
  `TestUnifiedDiff*`;
- backup: `makeTestZip`, `TestRestoreBackupZip*`, `TestDeleteConfigFile*`;
- dnsmasq-test wiring: `TestWriteConfigWithTest_*`,
  `TestRestoreHistoryVersion_*`.

Handler-level тесты остались в main. Для них сохранены handler-level seams:
`bins.SetPathForTest`, `initd.SetCurrentForTest`, а вызовы backend-функций
квалифицированы через `dnsmasq.X`. Fake-dnsmasq helpers продублированы в
`internal/dnsmasq/helpers_test.go` для перенесённых white-box-тестов; main
сохраняет свою копию для HTTP-тестов.

## Локальные проверки

```
gofmt -l .
go vet ./...
$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXX"
go test . -count=1
go test ./internal/dnsmasq/ -count=1
```

Все проверки завершились успешно. На Windows сохранены ожидаемые baseline
SKIP для shell-script fake binaries и отсутствующего dnsmasq; новых FAIL нет.
